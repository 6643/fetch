# tlsfingerprint — TLS 指纹库

使用 [uTLS](https://github.com/refraction-networking/utls) 实现浏览器 TLS 握手模仿，支持 JA3/JA4 指纹。

## 安装

```bash
go get github.com/6643/fetch/tlsfingerprint
```

注意：本模块作为 monorepo 的一部分，开发时通过 `go.work` 管理。

## 使用

```go
import "github.com/6643/fetch/tlsfingerprint"

// 1. 获取指纹 ID
helloID, err := tlsfingerprint.ResolveFingerprint("chrome")

// 2. 创建 Fingerprint Transport
transport, err := tlsfingerprint.NewTransport("chrome", tlsConfig, localAddr)

// 3. 用于 HTTP 请求
client := &http.Client{Transport: transport}
resp, err := client.Get("https://example.com")
```

## 支持的指纹

- `chrome`, `firefox`, `safari`, `edge`
- `ios`, `android`
- `random`, `randomized`
- `golang`, `custom`

## API

### ResolveFingerprint

```go
func ResolveFingerprint(name string) (utls.ClientHelloID, error)
```

根据名称映射 uTLS ClientHelloID。支持的值见上方列表。

### ToUTLSConfig

```go
func ToUTLSConfig(cfg *tls.Config) *utls.Config
```

将标准 `crypto/tls.Config` 转换为 uTLS 配置，保留证书、椭圆曲线、ALPN、ECH 等字段。

### NewTransport

```go
func NewTransport(fingerprint string, tlsConfig *tls.Config, localAddr string) (*Transport, error)
```

创建带浏览器指纹的 HTTP/2 Transport。`localAddr` 为空字符串表示不绑定本地地址。

### Transport

```go
type Transport struct { ... }
func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error)
func (t *Transport) CloseIdleConnections()
func (t *Transport) Fingerprint() string
```

实现 `http.RoundTripper`，可替代标准 `http.Transport` 使用。
