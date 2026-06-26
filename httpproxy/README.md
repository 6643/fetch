# httpproxy — VLESS 代理协议库

实现 VLESS 代理协议，支持 WebSocket 传输、TLS/uTLS、ECH（Encrypted Client Hello）和本地 HTTP 代理。

## 安装

```bash
go get github.com/6643/fetch/httpproxy
```

## 快速开始

### 直接使用 DialContext（集成到 http.Transport）

```go
import "github.com/6643/fetch/httpproxy"

dialFn, err := httpproxy.DialContext("vless://uuid@host:port?security=tls&type=ws")
if err != nil {
    panic(err)
}

transport := &http.Transport{DialContext: dialFn}
client := &http.Client{Transport: transport}
resp, err := client.Get("https://httpbin.org/get")
```

### 启动本地 HTTP 代理服务器

```go
import "github.com/6643/fetch/httpproxy"

proxy, err := httpproxy.NewProxy("vless://uuid@host:port?security=tls&type=ws")
if err != nil {
    panic(err)
}
defer proxy.Close()

proxy.Start(ctx, "")  // 随机端口
fmt.Printf("HTTP proxy listening on %s\n", proxy.Addr())

// 现在可以通过 curl -x http://<addr> 使用了
```

## URI 格式

```
vless://<UUID>@<host>:<port>?security=tls&type=ws[&fp=chrome][&host=example.com][&path=/ws][&sni=sni.example][&ech=public.name+https://doh.example/dns-query]
```

| 参数 | 说明 |
|------|------|
| `security=tls` | 必须为 tls |
| `type=ws` | 必须为 ws |
| `fp=chrome` | TLS 指纹（可选，当前仅支持 chrome） |
| `host` | WebSocket Host 头，默认同 SNI |
| `path` | WebSocket 路径，默认 `/` |
| `sni` | TLS SNI，默认同服务端地址 |
| `ech` | ECH 配置查询，格式 `<public-name>+<doh-endpoint>` |

## API

### 核心类型

- `Config` — VLESS 配置（UUID、服务器、WebSocket、ECH）
- `Dialer` — VLESS 拨号器（支持 ECH 缓存）
- `Proxy` — 本地 HTTP 代理服务器
- `ECHQuerySpec` — ECH 查询参数
- `ECHConfigListResult` — ECH 解析结果

### 主要函数

| 函数 | 说明 |
|------|------|
| `ParseVlessURI(raw) (Config, error)` | 解析 VLESS URI |
| `DialContext(vlessURI) (DialContext, error)` | 创建 VLESS 拨号函数 |
| `NewProxy(vlessURI) (*Proxy, error)` | 创建本地 HTTP 代理 |
| `(*Proxy).Start(ctx, addr) error` | 启动本地 HTTP 代理 |
| `(*Proxy).Addr() net.Addr` | 返回监听地址 |
| `(*Proxy).Close() error` | 关闭代理 |
