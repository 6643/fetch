# httpclient

一个无状态、打平函数式参数的 HTTP 工具包。

## 特性

- 默认无状态，不保存 cookie 会话。
- 不自动跟随重定向。
- 默认总超时为 `5s`。
- `Get`、`Post`、`Do` 等方法直接接收当次请求的全部参数。
- 默认复用内部 `Transport` 以获得更好的连接复用性能。
- 当使用 `WithProxy`、`WithLocalAddr`、`WithTLSConfig` 时，会为该次请求克隆一个临时 `Transport`。

## 安装

```bash
go get github.com/6643/httpclient
```

## 快速开始

```go
package main

import (
	"fmt"
	"time"

	"github.com/6643/httpclient"
)

func main() {
	res, err := httpclient.Get(
		"https://example.com/api",
		httpclient.WithTimeout(5*time.Second),
		httpclient.AddQuery("q", "golang"),
		httpclient.AddHeader("X-Trace-ID", "req-1"),
		httpclient.AddCookie("sid", "abc"),
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
httpclient.Do(method, url, opts...)
httpclient.Get(url, opts...)
httpclient.Post(url, opts...)
httpclient.Put(url, opts...)
httpclient.Delete(url, opts...)
httpclient.Patch(url, opts...)
httpclient.Head(url, opts...)
```

## 可用参数

### 通用参数

```go
httpclient.WithContext(ctx)
httpclient.WithTimeout(5 * time.Second)
httpclient.WithUserAgent("my-agent/1.0")
```

### 请求头、Cookie、Query

```go
httpclient.AddHeader("X-Trace-ID", "req-1")
httpclient.AddCookie("sid", "abc")
httpclient.AddQuery("q", "golang")
```

### Body

```go
httpclient.WithBody("application/json", reader)
httpclient.WithJSON(v)
httpclient.WithXML("<root />")
```

### 表单与文件上传

```go
httpclient.AddFormValue("name", "alice")
httpclient.AddMultipartField("note", "hello")
httpclient.AddMultipartFile("file", "a.txt", reader)
httpclient.AddFileData("file", "a.txt", []byte("hello"))
```

### 当次连接参数

```go
httpclient.WithProxy("http://127.0.0.1:8080")
httpclient.WithLocalAddr("192.168.1.10")
httpclient.WithTLSConfig(tlsConfig)
```

## JSON 示例

```go
payload := struct {
	Name string `json:"name"`
}{
	Name: "alice",
}

res, err := httpclient.Post(
	"https://example.com/users",
	httpclient.WithJSON(payload),
)
if err != nil {
	panic(err)
}
```

## 文件上传示例

```go
res, err := httpclient.Post(
	"https://example.com/upload",
	httpclient.AddMultipartField("description", "sample upload"),
	httpclient.AddFileData("file", "hello.txt", []byte("hello")),
)
if err != nil {
	panic(err)
}
```

## TLS 示例

```go
res, err := httpclient.Get(
	"https://example.com/secure",
	httpclient.WithTLSConfig(tlsConfig),
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
# fetch
