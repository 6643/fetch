# fetch

一个无状态、打平函数式参数的 HTTP 工具包。

## 特性

- 默认无状态，不保存 Cookie 会话。
- 不自动跟随重定向。
- 默认总超时为 `5s`。
- 默认响应体大小上限为 `10 MiB`。
- `Get`、`Post`、`Do` 等方法直接接收当次请求的全部参数。
- 标准 HTTP 方法会按规范大写发送；空方法会按 `GET` 处理。
- `GET`、`HEAD` 请求不允许携带请求体，包括通过 `Do` 传入时也是如此。
- 默认复用内部 `Transport` 以获得更好的连接复用性能。
- 当使用 `WithProxy`、`WithLocalAddr`、`WithTLSConfig` 时，会按连接参数选择内部 `Transport`。
- 相同的 `WithProxy`、`WithLocalAddr` 参数会复用内部 `Transport`, 以保留 keep-alive 连接复用收益。
- `WithTLSConfig` 为避免复用过期 TLS 配置, 仍会为该次请求使用独立的 `Transport`。
- 当响应体超出 `WithResponseBodyLimit` 时, 会返回错误并继续丢弃剩余数据, 以尽量保留连接复用能力。
- override `Transport` 缓存使用固定上限; 达到上限后, 新的 override 组合会退回当次临时 `Transport`, 以避免缓存持续增长。
- proxy override `Transport` 的缓存键不会保留明文代理凭据。

## 安装

```bash
go get github.com/6643/fetch
```

## 快速开始

```go
package main

import (
	"fmt"
	"time"

	"github.com/6643/fetch"
)

func main() {
	res, err := fetch.Get(
		"https://example.com/api",
		fetch.WithTimeout(5*time.Second),
		fetch.AddQuery("q", "golang"),
		fetch.AddHeader("X-Trace-ID", "req-1"),
		fetch.AddCookie("sid", "abc"),
	)
	if err != nil {
		panic(err)
	}

	fmt.Println(res.StatusCode)
	fmt.Println(res.Text())
}
```

## 方法

```go
fetch.Do(method, url, opts...)
fetch.Get(url, opts...)
fetch.Post(url, opts...)
fetch.Put(url, opts...)
fetch.Delete(url, opts...)
fetch.Patch(url, opts...)
fetch.Head(url, opts...)
```

说明:

- `fetch.Do("", url, opts...)` 会按 `GET` 发送。
- 标准方法名会被规范化为大写后再发出；非标准扩展方法保持原样。

## 可用参数

### 通用参数

```go
fetch.WithContext(ctx)
fetch.WithTimeout(5 * time.Second)
fetch.WithResponseBodyLimit(10 << 20)
fetch.WithUserAgent("my-agent/1.0")
```

说明:

- 默认总超时为 `5s`。
- `fetch.WithTimeout(0)` 会禁用默认超时。
- 默认响应体大小上限为 `10 MiB`。
- `fetch.WithResponseBodyLimit(0)` 会禁用响应体大小限制。
- 超限时会返回错误，并在关闭响应前继续读取并丢弃剩余 body，以尽量复用底层连接。

### 请求头、Cookie、Query

```go
fetch.AddHeader("X-Trace-ID", "req-1")
fetch.AddCookie("sid", "abc")
fetch.AddQuery("q", "golang")
```

### Body

```go
fetch.WithJSON(v)
fetch.WithXML("<root />")
```

说明:

- `Content-Type` 应优先通过 `WithJSON`、`WithXML`、表单和 multipart API 设置。
- 不要把 `fetch.AddHeader("Content-Type", ...)` 与这些 body 选项混用; 当前实现会返回 `fetch.ErrContentTypeConflict`, 且请求不会被发送。

### 表单与文件上传

```go
fetch.AddFormValue("name", "alice")
fetch.AddMultipartField("note", "hello")
fetch.AddMultipartFile("file", "a.txt", reader)
fetch.AddFileData("file", "a.txt", []byte("hello"))
```

### 当次连接参数

```go
fetch.WithProxy("http://127.0.0.1:8080")
fetch.WithLocalAddr("192.168.1.10")
fetch.WithTLSConfig(tlsConfig)
```

说明:

- `fetch.WithProxy` 只接受带 `scheme` 和 `host` 的绝对 URL。

## JSON 示例

```go
payload := struct {
	Name string `json:"name"`
}{
	Name: "alice",
}

res, err := fetch.Post(
	"https://example.com/users",
	fetch.WithJSON(payload),
)
if err != nil {
	panic(err)
}
```

## 文件上传示例

```go
res, err := fetch.Post(
	"https://example.com/upload",
	fetch.AddMultipartField("description", "sample upload"),
	fetch.AddFileData("file", "hello.txt", []byte("hello")),
)
if err != nil {
	panic(err)
}
```

## TLS 示例

```go
res, err := fetch.Get(
	"https://example.com/secure",
	fetch.WithTLSConfig(tlsConfig),
)
if err != nil {
	panic(err)
}
```

## 响应

```go
type Response struct {
	StatusCode  int
	Status      string
	Location    string
	Cookie      map[string]string
	Cookies     string
	CookiesList []*http.Cookie
	Header      map[string]string
	Headers     http.Header
	Body        []byte
}
```

辅助方法：

```go
res.Text()
res.JSON(&dst)
```

说明:

- 当响应体为空时, `res.JSON(&dst)` 会返回 `fetch.ErrEmptyBody`。

兼容字段说明:

- `Response.Header` 是响应头的便捷打平视图, 多值头会被合并, 不适合作为完整 HTTP 语义来源。
- `Response.Cookie` 是按名称索引的兼容视图; 同名 Cookie 会以后出现者覆盖先出现者。
- `Response.Cookies` 是由解析后的响应 Cookie 生成的 `name=value` 摘要串, 不是原始 `Set-Cookie` 头, 也不适合作为无损回放格式。
- 新代码需要完整响应头和 Cookie 语义时, 请优先使用 `Response.Headers` 和 `Response.CookiesList`。
- 响应体会先完整读入 `Response.Body`; 默认单次读取上限为 `10 MiB`。

## 实现说明

### Request 构建流程

- 请求构建按 `normalizeMethod -> validateMethod -> finalizeURL -> validateConfiguredContentTypeConflict -> prepareBody -> applyMetadata` 顺序执行。
- 请求方法与 body 冲突会在真正发包前失败，不会把非法请求发送到服务端。
- multipart body 通过 `io.Pipe` 流式写入，避免先把整份 multipart 数据落到内存。

### Response 构建流程

- 响应会先保留原始 `Headers`，再生成兼容用的打平 `Header`。
- `Location`、Cookie 兼容视图和完整 `CookiesList` 会分别提取，避免相互混淆。
- 响应体会完整读入 `Response.Body`，并受 `WithResponseBodyLimit` 控制。

### Transport 策略

- 默认请求复用共享 `defaultTransport`。
- `WithProxy`、`WithLocalAddr` 组合会命中内部 override `Transport` 缓存。
- `WithTLSConfig` 会为每次请求创建独立 `Transport`，避免复用过期 TLS 配置。
- override 缓存键会对代理 URL 做摘要，不直接保存明文凭据。

### 代码维护约束

- 主干逻辑优先使用守卫式写法。
- 核心逻辑嵌套深度控制在 2 层以内。
- 复杂流程拆分为单一职责的私有函数，便于审查和测试。

## 开发验证

```bash
go test ./...
go test -race ./...
go vet ./...
```
