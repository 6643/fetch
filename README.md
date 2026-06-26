# fetch — HTTP 客户端库

基于 Go 标准库 `net/http` 的轻量级 HTTP 客户端，支持代理、TLS 指纹注入和自定义 TLS 配置。

一个无状态、打平函数式参数的 HTTP 工具包。

## 安装

```bash
go get github.com/6643/fetch
```

## 快速开始

### 一次性请求

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

### 复用 Client

```go
package main

import (
    "fmt"
    "time"

    "github.com/6643/fetch"
)

func main() {
    client, err := fetch.NewClient(
        fetch.WithFingerprint("chrome"),
        fetch.WithTimeout(30*time.Second),
    )
    if err != nil {
        panic(err)
    }
    defer client.Close()

    // 复用同一 transport，多次请求
    res1, _ := client.Get("https://a.com", fetch.AddQuery("k", "1"))
    res2, _ := client.Get("https://b.com", fetch.WithJSON(payload))

    fmt.Println(res1.Text())
    fmt.Println(res2.StatusCode)
}
```

## 项目结构

本项目采用 monorepo 结构，包含三个 Go 子模块：

```
fetch/
├── tlsfingerprint/    # TLS 指纹 (uTLS) 库
├── httpproxy/         # VLESS 代理协议 + 本地 HTTP 代理
├── go.work            # Go workspace
└── README.md
```

### 子模块

| 模块 | 文档 | 用途 |
|------|------|------|
| `fetch` | 本 README | HTTP 客户端库 |
| `tlsfingerprint` | [tlsfingerprint/README.md](tlsfingerprint/README.md) | TLS 指纹 / uTLS |
| `httpproxy` | [httpproxy/README.md](httpproxy/README.md) | VLESS 代理协议 / 本地 HTTP 代理 |

## 特性

- 默认无状态，不保存 Cookie 会话。
- 不自动跟随重定向。
- 默认总超时为 `5s`。
- 默认响应体大小上限为 `10 MiB`。
- `Get`、`Post`、`Do` 等方法直接接收当次请求的全部参数。
- 标准 HTTP 方法会按规范大写发送；空方法会按 `GET` 处理。
- `GET`、`HEAD` 请求不允许携带请求体，包括通过 `Do` 传入时也是如此。
- 默认复用内部 `Transport` 以获得更好的连接复用性能。
- 当使用 `WithProxy`、`WithLocalAddr`、`WithTLSConfig`、`WithFingerprint` 时，会按连接参数选择内部 `Transport`。
- 相同的 `WithProxy`、`WithLocalAddr`、`WithFingerprint` 参数会复用内部 `Transport`, 以保留 keep-alive 连接复用收益。
- `WithTLSConfig` 为避免复用过期 TLS 配置, 仍会为该次请求使用独立的 `Transport`。
- 当响应体超出 `WithResponseBodyLimit` 时, 会返回错误并继续丢弃剩余数据, 以尽量保留连接复用能力。
- override `Transport` 缓存使用固定上限; 达到上限后, 新的 override 组合会退回当次临时 `Transport`, 以避免缓存持续增长。
- proxy override `Transport` 的缓存键不会保留明文代理凭据。

## Client

`Client` 封装了一个可复用的 transport 和默认请求参数，适合高频请求场景：

```go
client, err := fetch.NewClient(
    // 传输层选项：锁定 transport 配置，不可按请求覆盖
    fetch.WithFingerprint("chrome"),
    fetch.WithProxy("http://127.0.0.1:8080"),

    // 默认请求参数：可按请求覆盖
    fetch.WithTimeout(30*time.Second),
    fetch.WithResponseBodyLimit(10 << 20),
    fetch.WithUserAgent("my-agent/1.0"),
)
if err != nil {
    panic(err)
}
defer client.Close()
```

说明:

- 传输层选项 (`WithProxy`、`WithLocalAddr`、`WithTLSConfig`、`WithFingerprint`) 在 `NewClient` 时锁定，按请求传入会返回错误。
- 非传输层选项 (`WithTimeout`、`WithResponseBodyLimit`、`WithUserAgent`) 设为 client 级默认值，按请求传入时会覆盖默认值。
- `Close()` 释放底层 transport 资源（包括空闲连接），可安全多次调用。

## 一次性请求

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
// 传统代理
fetch.WithProxy("http://127.0.0.1:8080")
fetch.WithProxy("socks5://127.0.0.1:1080")

// VLESS 隧道
fetch.WithProxy("vless://uuid@host:443?security=tls&type=ws&path=%2Fws")

// 其他传输层选项（可与传统代理组合，不可与 VLESS 组合）
fetch.WithLocalAddr("192.168.1.10")
fetch.WithTLSConfig(tlsConfig)
fetch.WithFingerprint("chrome")
```

说明:

- `fetch.WithProxy` 根据 URL scheme 自动识别代理类型: `http://`/`https://`/`socks5://`/`socks5h://` 为传统代理。
- VLESS 代理推荐通过 `httpproxy` 子模块启动本地 HTTP 代理后，再通过 `WithProxy` 使用标准 HTTP 代理访问。见 [VLESS 示例](#vless-示例)。
- 传统代理可与 `WithFingerprint`、`WithTLSConfig`、`WithLocalAddr` 组合使用。
- 使用 `Client` 时，这些选项必须在 `NewClient` 时传入，不可按请求覆盖。

## 示例

### JSON 示例

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

### 文件上传示例

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

### TLS 示例

```go
res, err := fetch.Get(
    "https://example.com/secure",
    fetch.WithTLSConfig(tlsConfig),
)
if err != nil {
    panic(err)
}
```

### TLS 指纹示例

使用 `WithFingerprint` 模拟浏览器 TLS ClientHello 指纹:

```go
// 一次性请求
res, err := fetch.Get(
    "https://example.com",
    fetch.WithFingerprint("chrome"),
)

// 或通过 Client 复用
client, _ := fetch.NewClient(
    fetch.WithFingerprint("chrome"),
)
defer client.Close()
res, err := client.Get("https://example.com")
```

支持的值: `"chrome"`、`"firefox"`、`"safari"`、`"edge"`、`"ios"`、`"android"`、`"random"`、`"randomized"`、`"golang"`、`"custom"`。

可与 `WithTLSConfig` 组合使用，此时 `WithFingerprint` 控制 ClientHello 指纹形态，`WithTLSConfig` 提供根证书等参数。

### VLESS 示例

**方式一：通过 httpproxy 本地 HTTP 代理（推荐）**

启动本地 HTTP 代理服务器，对外暴露标准 HTTP 代理端口，fetch 无需感知 VLESS：

```go
import (
    "github.com/6643/fetch"
    "github.com/6643/fetch/httpproxy"
)

// 1. 启动本地 HTTP 代理（自动绑定随机端口）
proxy, err := httpproxy.NewProxy(
    "vless://uuid@server:443?security=tls&type=ws&path=%2Fws&fp=chrome",
)
if err != nil {
    panic(err)
}
ctx, cancel := context.WithCancel(context.Background())
defer cancel()
if err := proxy.Start(ctx, ""); err != nil {
    panic(err)
}
defer proxy.Close()

// proxy.Addr() 返回实际监听地址，如 127.0.0.1:54321
proxyAddr := proxy.Addr().String()

// 2. fetch 通过标准 HTTP 代理发送请求，完全不知道后端是 VLESS
resp, err := fetch.Get(
    "https://example.com",
    fetch.WithProxy("http://"+proxyAddr),
)
```

**方式二：通过 httpproxy.DialContext 集成到 Transport**

```go
import "github.com/6643/fetch/httpproxy"

dialFn, err := httpproxy.DialContext(
    "vless://uuid@server:443?security=tls&type=ws&path=%2Fws&fp=chrome",
)
if err != nil {
    panic(err)
}

transport := &http.Transport{DialContext: dialFn}
client := &http.Client{Transport: transport}
resp, err := client.Get("https://example.com")
```

**方式三：通过 fetch.WithProxy 直接使用（仅供一次性请求）**

```go
// 一次性请求
resp, err := fetch.Get(
    "https://example.com",
    fetch.WithProxy("vless://uuid@server:443?security=tls&type=ws&path=%2Fws&fp=chrome"),
)

// 或通过 Client 复用
client, _ := fetch.NewClient(
    fetch.WithProxy("vless://uuid@server:443?security=tls&type=ws&path=%2Fws&fp=chrome"),
)
defer client.Close()
resp, err := client.Get("https://example.com")
```

说明:

- URI 必须是 `security=tls&type=ws` 的 VLESS 分享链接
- VLESS 代理不可与传统代理、`WithLocalAddr`、`WithTLSConfig`、`WithFingerprint` 等传输层选项混用；当前实现会返回错误，且请求不会被发送
- 使用 `Client` 时，传输层选项在 `NewClient` 时锁定；非传输层选项 (`WithTimeout`、`AddHeader`、`WithJSON` 等) 仍正常工作
- 传输配置 (`sni`/`host`/`path`/`fp`/`ech`) 在 VLESS URI 的参数中设置
- `ech` 支持 `<public-name>+<https-doh-endpoint>` 形式，会按需通过 DoH 解析并按 TTL 缓存 ECH ConfigList
- 推荐通过 `Client` 使用 VLESS，以复用底层 WebSocket 连接和 ECH 缓存

## 响应

```go
type Response struct {
    StatusCode  int
    Status      string
    Location    string
    CookiesList []*http.Cookie
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

- 当响应体为空时，`res.JSON(&dst)` 会返回 `fetch.ErrEmptyBody`。
- `Response.Headers` 包含完整的原始响应头。
- `Response.CookiesList` 包含完整的解析后 Cookie 列表。
- 响应体会先完整读入 `Response.Body`；默认单次读取上限为 `10 MiB`。

## 实现说明

### Request 构建流程

- 请求构建按 `normalizeMethod -> validateMethod -> finalizeURL -> validateConfiguredContentTypeConflict -> prepareBody -> applyMetadata` 顺序执行。
- 请求方法与 body 冲突会在真正发包前失败，不会把非法请求发送到服务端。
- multipart body 通过 `io.Pipe` 流式写入，避免先把整份 multipart 数据落到内存。

### Response 构建流程

- 响应保留原始 `Headers` (`http.Header`) 和 `CookiesList` (`[]*http.Cookie`)，提供完整的 HTTP 语义。
- `Location` 会分别提取，便于处理重定向。
- 响应体会完整读入 `Response.Body`，并受 `WithResponseBodyLimit` 控制。

### Transport 策略

- 默认请求复用共享 `defaultTransport`。
- `WithProxy`、`WithLocalAddr` 组合会命中内部 override `Transport` 缓存。
- `WithTLSConfig` 会为每次请求创建独立 `Transport`，避免复用过期 TLS 配置；与 `WithFingerprint` 组合时同样不会进入缓存。
- override 缓存键会对代理 URL 做摘要，不直接保存明文凭据。
- `Client` 在创建时锁定 transport，所有请求复用同一连接池。

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
