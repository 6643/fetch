# fetch

一个无状态、打平函数式参数的 HTTP 工具包。

## 特性

- 默认无状态，不保存 Cookie 会话。
- 不自动跟随重定向。
- 默认总超时为 `5s`。
- `Get`、`Post`、`Do` 等方法直接接收当次请求的全部参数。
- 默认复用内部 `Transport` 以获得更好的连接复用性能。
- 当使用 `WithProxy`、`WithLocalAddr`、`WithTLSConfig` 时，会按连接参数选择内部 `Transport`。
- 相同的 `WithProxy`、`WithLocalAddr` 参数会复用内部 `Transport`, 以保留 keep-alive 连接复用收益。
- `WithTLSConfig` 为避免复用过期 TLS 配置, 仍会为该次请求使用独立的 `Transport`。
- override `Transport` 缓存使用固定上限; 达到上限后, 新的 override 组合会退回当次临时 `Transport`, 以避免缓存持续增长。

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

## 可用参数

### 通用参数

```go
fetch.WithContext(ctx)
fetch.WithTimeout(5 * time.Second)
fetch.WithUserAgent("my-agent/1.0")
```

说明:

- 默认总超时为 `5s`。
- `fetch.WithTimeout(0)` 会禁用默认超时。

### 请求头、Cookie、Query

```go
fetch.AddHeader("X-Trace-ID", "req-1")
fetch.AddCookie("sid", "abc")
fetch.AddQuery("q", "golang")
```

### Body

```go
fetch.WithBody("application/json", reader)
fetch.WithJSON(v)
fetch.WithXML("<root />")
```

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

兼容字段说明:

- `Response.Header` 是响应头的便捷打平视图, 多值头会被合并, 不适合作为完整 HTTP 语义来源。
- `Response.Cookie` 和 `Response.Cookies` 也是便捷视图, 同名 Cookie 可能被覆盖, 不保留完整属性。
- 需要完整响应头和 Cookie 语义时, 请使用 `Response.Headers` 和 `Response.CookiesList`。

## 兼容别名

为了兼容旧调用，下面这些名字仍然可用，并会转发到新接口：

- `Request`
- `SetUserAgent`
- `SetBody`
- `SetJSONBody`
- `SetXMLBody`
- `AddData`
- `AddField`
- `AddFile`
- `AddUrlArg`
