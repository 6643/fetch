# fetch 三库分拆实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将 `github.com/6643/fetch` 拆分为三个子库：`fetch`（HTTP 客户端）、`tls-fingerprint`（TLS 指纹/ECH 能力）、`http-proxy`（VLESS 代理协议封装）。

**Architecture:** monorepo 子目录方案，三个子库通过 `go.work` 统一管理。`github.com/6643/fetch/{tlsfingerprint,httpproxy}` 作为独立 module 路径。http-proxy 库启动一个随机端口本地 HTTP 代理服务器，通过 VLESS 隧道转发请求。

**Tech Stack:** Go 1.22+, `github.com/refraction-networking/utls`, `github.com/coder/websocket`, `golang.org/x/net/http2`, `go.work`

## Global Constraints

- Monorepo 子目录结构：`fetch/`、`tlsfingerprint/`、`httpproxy/`
- module 路径：`github.com/6643/fetch`、`github.com/6643/fetch/tlsfingerprint`、`github.com/6643/fetch/httpproxy`
- 所有现有 test 必须持续通过（前置失败除外）
- 步骤要求：测试先行，提交频繁

---

## File Structure

拆分后目录结构：

```
/home/_/._/fetch/
├── go.work                     # 管理三个 module 的 workspace
├── go.mod                      # fetch 主 module
├── client.go                   # 保持不变
├── do.go                       # 保持不变
├── option.go                   # 保持不变
├── request.go                  # 保持不变
├── response.go                 # 保持不变
├── transport.go                # 移除 VLESS 和 fingerprint 相关逻辑
├── *_test.go                   # 对应的测试文件
│
├── tlsfingerprint/
│   ├── go.mod                  # module github.com/6643/fetch/tlsfingerprint
│   ├── fingerprint.go          # 从 fetch/ 迁移：resolveFingerprint, toUTLSConfig, 常量
│   ├── transport.go            # 从 fetch/transport.go 迁移：fingerprintTransport 结构体及方法
│   └── fingerprint_test.go     # 从 fetch/ 迁移：测试
│
└── httpproxy/
    ├── go.mod                  # module github.com/6643/fetch/httpproxy
    ├── config.go               # 从 fetch/vless_config.go 迁移：vlessConfig, parseVlessURI
    ├── dialer.go               # 从 fetch/vless.go 迁移：vlessDialer.DialContext
    ├── conn.go                 # 从 fetch/vless.go 迁移：vlessConn, consumeVlessResponseHeader
    ├── header.go               # 从 fetch/vless.go 迁移：encodeVlessTCPRequestHeader, 常量
    ├── ech.go                  # 从 fetch/ech.go 迁移：ECH DNS 解析
    ├── proxy.go                # 新增：本地 HTTP 代理服务器
    ├── export.go               # 从 fetch/vless_export.go 迁移：VlessDialContext
    ├── httpproxy.go            # 新增：HttpProxy 结构体及 NewHttpProxy
    └── *_test.go               # 迁移：vless_test.go 等测试
```

---

## 依赖关系

```
fetch (github.com/6643/fetch)
  ├── transport.go → 移除 fingerprintTransport, 移除 vless 路由
  ├── 新增 dep: github.com/6643/fetch/tlsfingerprint (fingerprintTransport)
  └── 新增 dep: github.com/6643/fetch/httpproxy (WithProxy vless:// 路由)

tlsfingerprint (github.com/6643/fetch/tlsfingerprint)
  └── utls, http2

httpproxy (github.com/6643/fetch/httpproxy)
  ├── 依赖 utls (ECH + fp=chrome 的 WebSocket uTLS)
  ├── 依赖 websocket
  └── 依赖 tlsfingerprint (toUTLSConfig)
```

---

### Task 1: 创建 go.work 和子库骨架

**Files:**
- Create: `/home/_/._/fetch/go.work`
- Create: `/home/_/._/fetch/tlsfingerprint/go.mod`
- Create: `/home/_/._/fetch/httpproxy/go.mod`

- [ ] **Step 1: 创建 go.work**

```bash
cd /home/_/._/fetch
go work init
go work use . ./tlsfingerprint ./httpproxy
```

- [ ] **Step 2: 创建 tlsfingerprint 子库 go.mod**

```bash
cd /home/_/._/fetch/tlsfingerprint
go mod init github.com/6643/fetch/tlsfingerprint
go get github.com/refraction-networking/utls@v1.8.3-0.20260301010127-aa6edf4b11af
go get golang.org/x/net@v0.56.0
```

- [ ] **Step 3: 创建 httpproxy 子库 go.mod**

```bash
cd /home/_/._/fetch/httpproxy
go mod init github.com/6643/fetch/httpproxy
go get github.com/refraction-networking/utls@v1.8.3-0.20260301010127-aa6edf4b11af
go get github.com/coder/websocket@v1.8.14
```

- [ ] **Step 4: 验证 go.work 识别所有 module**

```bash
cd /home/_/._/fetch
go work sync
go list -m all | head -5
# 应看到所有三个 module
```

- Commit:

```bash
git add go.work tlsfingerprint/go.mod httpproxy/go.mod
git commit -m "chore: add go.work and submodule skeletons"
```

---

### Task 2: 提取 tls-fingerprint 库

**Files:**
- Create: `/home/_/._/fetch/tlsfingerprint/fingerprint.go`
- Create: `/home/_/._/fetch/tlsfingerprint/transport.go`
- Create: `/home/_/._/fetch/tlsfingerprint/fingerprint_test.go`
- Remove from fetch: `fingerprint.go` 中 `resolveFingerprint`、`toUTLSConfig`、常量（transport.go 中的 `fingerprintTransport` 暂保留在最后一步移除）
- Modify: `/home/_/._/fetch/transport.go` — 改为 import `tlsfingerprint`

- [ ] **Step 1: 创建 tlsfingerprint/fingerprint.go**

内容：将 fetch/fingerprint.go 完整迁移，导出所有常量和函数。

```go
package tlsfingerprint

import (
	"crypto/tls"
	"fmt"
	"strings"

	utls "github.com/refraction-networking/utls"
)

const (
	FingerprintChrome     = "chrome"
	FingerprintFirefox    = "firefox"
	FingerprintSafari     = "safari"
	FingerprintEdge       = "edge"
	FingerprintIOS        = "ios"
	FingerprintAndroid    = "android"
	FingerprintRandom     = "random"
	FingerprintRandomized = "randomized"
	FingerprintGolang     = "golang"
	FingerprintCustom     = "custom"
)

func ResolveFingerprint(name string) (utls.ClientHelloID, error) {
	switch strings.ToLower(name) {
	case FingerprintChrome:
		return utls.HelloChrome_Auto, nil
	// ... 完整映射
	case FingerprintCustom:
		return utls.HelloCustom, nil
	default:
		return utls.ClientHelloID{}, fmt.Errorf("unknown TLS fingerprint %q", name)
	}
}

func ToUTLSConfig(cfg *tls.Config) *utls.Config {
	if cfg == nil {
		return &utls.Config{}
	}
	// ... 完整转换逻辑（同现有 fingerprint.go）
}
```

- [ ] **Step 2: 创建 tlsfingerprint/transport.go**

内容：从 fetch/transport.go 迁移 `fingerprintTransport` 结构体和 `NewFingerprintTransport`（将 newFingerprintTransport 改为导出）。

```go
package tlsfingerprint

import (
	"context"
	"crypto/tls"
	"net"
	"time"

	utls "github.com/refraction-networking/utls"
	"golang.org/x/net/http2"
)

// Transport 是用 uTLS 指纹的 HTTP/2 transport。
type Transport struct {
	h2Transport *http2.Transport
	dialer      *net.Dialer
	fingerprint string
	tlsConfig   *tls.Config
}

func NewTransport(fingerprint string, opts ...TransportOption) (*Transport, error) {
	// ...
}

func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	return t.h2Transport.RoundTrip(req)
}

func (t *Transport) CloseIdleConnections() {
	t.h2Transport.CloseIdleConnections()
}
```

- [ ] **Step 3: 创建 tlsfingerprint/fingerprint_test.go**

迁移 fetch/fingerprint_test.go 中所有测试，导入路径改为 `tlsfingerprint`。

```go
package tlsfingerprint

import (
	"testing"
)

func TestResolveFingerprintAcceptsValidNames(t *testing.T) {
	// ...
}
```

- [ ] **Step 4: 验证 tlsfingerprint 库可独立编译测试**

```bash
cd /home/_/._/fetch/tlsfingerprint
go test ./... -count=1 -timeout=30s
```

- Commit:

```bash
git add tlsfingerprint/
git commit -m "feat: extract tls-fingerprint submodule"
```

---

### Task 3: 提取 http-proxy 库（VLESS 核心协议）

**Files:**
- Create: `/home/_/._/fetch/httpproxy/config.go`
- Create: `/home/_/._/fetch/httpproxy/dialer.go`
- Create: `/home/_/._/fetch/httpproxy/conn.go`
- Create: `/home/_/._/fetch/httpproxy/header.go`
- Create: `/home/_/._/fetch/httpproxy/ech.go`
- Create: `/home/_/._/fetch/httpproxy/handshake.go`
- Create: `/home/_/._/fetch/httpproxy/export.go`

- [ ] **Step 1: 创建 httpproxy/config.go**

迁移 fetch/vless_config.go 内容：`vlessConfig` 结构体、`ParseVlessURI`（导出）、`parseUUID`、`configMessage`、`rawQueryValue`。

```go
package httpproxy

import (
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/6643/fetch/tlsfingerprint"
)

var errInvalidConfig = errors.New("invalid vless config")

type Config struct {
	UUIDBytes      [16]byte
	ServerHost     string
	ServerPort     string
	TLSServerName  string
	TLSFingerprint string
	WebSocketHost  string
	WebSocketPath  string
	ECHSpec        *ECHQuerySpec
}

func (c Config) ServerAddr() string {
	return net.JoinHostPort(c.ServerHost, c.ServerPort)
}

type ECHQuerySpec struct {
	PublicName string
	DoHURL     string
}

func ParseVlessURI(raw string) (Config, error) {
	// ... 完整迁移 parseVlessURI 逻辑
}
```

注意：`echQuerySpec` 改为 `ECHQuerySpec`（导出），`parseECHQuerySpec` 迁移到 `httpproxy` 包内。

- [ ] **Step 2: 创建 httpproxy/header.go**

迁移 `encodeVlessTCPRequestHeader` 和常量（`vlessVersion`、`vlessCommandTCP`、`vlessAddressTypeIPv4` 等）。

```go
package httpproxy

import (
	"encoding/binary"
	"fmt"
	"net"
	"strconv"
)

const (
	vlessVersion           = byte(0x00)
	vlessCommandTCP        = byte(0x01)
	vlessAddressTypeIPv4   = byte(0x01)
	vlessAddressTypeDomain = byte(0x02)
	vlessAddressTypeIPv6   = byte(0x03)
)

func encodeVlessTCPRequestHeader(uuidBytes [16]byte, targetAddr string) ([]byte, error) {
	// ...
}
```

- [ ] **Step 3: 创建 httpproxy/conn.go**

迁移 `vlessConn`、`consumeVlessResponseHeader`。

```go
package httpproxy

import (
	"io"
	"net"
	"sync"
)

type vlessConn struct {
	net.Conn
	cancel             func()
	responseHeaderMu   sync.Mutex
	responseHeaderRead bool
	responseHeaderErr  error
}

func (c *vlessConn) Close() error {
	// ...
}

func (c *vlessConn) Read(p []byte) (int, error) {
	// ...
}

func consumeVlessResponseHeader(reader io.Reader) error {
	// ...
}
```

- [ ] **Step 4: 创建 httpproxy/handshake.go**

迁移 `handshakeUTLSWebsocket` 函数。这里需要 `tlsfingerprint` 依赖（`toUTLSConfig` → `tlsfingerprint.ToUTLSConfig`）。

```go
package httpproxy

import (
	"context"
	utls "github.com/refraction-networking/utls"
)

func handshakeUTLSWebsocket(ctx context.Context, conn *utls.UConn) error {
	// ...
}
```

- [ ] **Step 5: 创建 httpproxy/dialer.go**

迁移 `vlessDialer` 及其方法（`DialContext`、`echConfigList`、`resolveECHConfigList`、`websocketHTTPClient`、`websocketUTLSDialContext`）。

```go
package httpproxy

type Dialer struct {
	cfg     Config
	rootCAs *x509.CertPool
	echMu   sync.Mutex
	cachedECHConfigList []byte
	cachedECHExpiresAt  time.Time
}

func (d *Dialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	// ...
}
```

- [ ] **Step 6: 创建 httpproxy/ech.go**

迁移 ech.go 全部内容：`ECHConfigListResult`、`resolveECHConfigList`、`buildDNSHTTPSQuery`、`extractECHConfigListFromDNSMessage` 等。

- [ ] **Step 7: 创建 httpproxy/export.go**

迁移 `VlessDialContext`。

```go
package httpproxy

import (
	"context"
	"net"
)

func DialContext(vlessURI string) (func(ctx context.Context, network, address string) (net.Conn, error), error) {
	cfg, err := ParseVlessURI(vlessURI)
	if err != nil {
		return nil, err
	}
	d := &Dialer{cfg: cfg}
	return d.DialContext, nil
}
```

- [ ] **Step 8: 验证 httpproxy 库可编译**

```bash
cd /home/_/._/fetch/httpproxy
go build ./...
```

- Commit:

```bash
git add httpproxy/
git commit -m "feat: extract http-proxy submodule with VLESS protocol"
```

---

### Task 4: 创建本地 HTTP 代理服务器

**Files:**
- Create: `/home/_/._/fetch/httpproxy/proxy.go`
- Create: `/home/_/._/fetch/httpproxy/proxy_test.go`

- [ ] **Step 1: 设计 Proxy 类型**

```go
package httpproxy

import (
	"net"
	"net/http"
)

// Proxy 包装了 VLESS dialer 以提供本地 HTTP 代理。
type Proxy struct {
	dialer *Dialer
	server *http.Server
	listener net.Listener
}

// NewProxy 创建一个新的本地 HTTP 代理服务器，绑定到随机端口。
func NewProxy(vlessURI string) (*Proxy, error) {
	cfg, err := ParseVlessURI(vlessURI)
	if err != nil {
		return nil, err
	}
	dialer := &Dialer{cfg: cfg}
	return &Proxy{dialer: dialer}, nil
}

// Start 启动本地 HTTP 代理服务器。
// addr 为监听地址，为空则绑定到随机端口。
func (p *Proxy) Start(ctx context.Context, addr string) error {
	if addr == "" {
		addr = "127.0.0.1:0"
	}
	// 启动 net/http server
	// 处理 CONNECT 方法和普通 HTTP 方法
}

// Addr 返回监听地址。
func (p *Proxy) Addr() net.Addr {
	if p.listener != nil {
		return p.listener.Addr()
	}
	return nil
}

// Close 关闭代理服务器。
func (p *Proxy) Close() error {
	return p.server.Close()
}
```

- [ ] **Step 2: 实现 HTTP 代理处理逻辑**

处理 CONNECT 方法：建立 VLESS 隧道后使用 `Hijack` 连接
处理 GET/POST 等：通过 VLESS 隧道以 `Forward` 方式代理

```go
func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodConnect {
		p.serveConnect(w, r)
		return
	}
	p.serveHTTP(w, r)
}
```

- [ ] **Step 3: 实现 CONNECT 处理**

```go
func (p *Proxy) serveConnect(w http.ResponseWriter, r *http.Request) {
	dialCtx, cancel := context.WithCancel(r.Context())
	defer cancel()

	target := r.Host
	if target == "" {
		http.Error(w, "missing target", http.StatusBadRequest)
		return
	}

	conn, err := p.dialer.DialContext(dialCtx, "tcp", target)
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer conn.Close()

	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "hijacking not supported", http.StatusInternalServerError)
		return
	}
	clientConn, _, err := hijacker.Hijack()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer clientConn.Close()

	clientConn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))

	// bidirection copy
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { io.Copy(conn, clientConn); wg.Done() }()
	go func() { io.Copy(clientConn, conn); wg.Done() }()
	wg.Wait()
}
```

- [ ] **Step 4: 实现 HTTP Forward 处理**

```go
func (p *Proxy) serveHTTP(w http.ResponseWriter, r *http.Request) {
	dialCtx, cancel := context.WithCancel(r.Context())
	defer cancel()

	target := r.Host
	if target == "" {
		http.Error(w, "missing target", http.StatusBadRequest)
		return
	}

	conn, err := p.dialer.DialContext(dialCtx, "tcp", target)
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer conn.Close()

	// 通过 VLESS 隧道发送原始 HTTP 请求
	r.Write(conn)
	resp, err := http.ReadResponse(bufio.NewReader(conn), r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// 将响应写回客户端
	for k, v := range resp.Header {
		w.Header()[k] = v
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}
```

- [ ] **Step 5: 创建 httpproxy/proxy_test.go**

```go
package httpproxy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestProxyConnect(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("proxied"))
	}))
	defer target.Close()

	// 启动 VLESS 代理服务器（回环测试）
	// ...
}
```

- [ ] **Step 6: 验证编译和测试**

```bash
cd /home/_/._/fetch/httpproxy
go build ./...
go test ./... -count=1 -timeout=60s -run TestProxy
```

- Commit:

```bash
git add httpproxy/proxy.go httpproxy/proxy_test.go
git commit -m "feat: add local HTTP proxy server for VLESS"
```

---

### Task 5: 改造 fetch 核心库引用新子库

**Files:**
- Modify: `/home/_/._/fetch/go.mod` — 添加 replace 指令指向 tlsfingerprint 和 httpproxy
- Modify: `/home/_/._/fetch/transport.go` — 移除 VLESS 和 fingerprint 实现，改为 import 子库
- Modify: `/home/_/._/fetch/vless.go` — 删除（已迁移到 httpproxy）
- Modify: `/home/_/._/fetch/vless_config.go` — 删除
- Modify: `/home/_/._/fetch/vless_export.go` — 删除
- Modify: `/home/_/._/fetch/ech.go` — 删除（已迁移到 httpproxy）
- Modify: `/home/_/._/fetch/fingerprint.go` — 删除（已迁移到 tlsfingerprint）
- Modify: `/home/_/._/fetch/vless_test.go` — 删除/改为 httpproxy 测试
- Modify: `/home/_/._/fetch/fingerprint_test.go` — 删除/改为 tlsfingerprint 测试

- [ ] **Step 1: 更新 fetch/go.mod**

```bash
cd /home/_/._/fetch
go get github.com/6643/fetch/tlsfingerprint@v0.0.0
go get github.com/6643/fetch/httpproxy@v0.0.0
```

由于是本地开发，需要在 go.mod 中添加 replace 指令：

```
replace github.com/6643/fetch/tlsfingerprint => ./tlsfingerprint
replace github.com/6643/fetch/httpproxy => ./httpproxy
```

- [ ] **Step 2: 改造 transport.go**

在 `transport.go` 中：
- 删除 `fingerprintTransport` 结构体和方法（替换为 `tlsfingerprint.Transport`）
- 删除 `newFingerprintTransport`（替换为 `tlsfingerprint.NewTransport`）
- 修改 `transportFor` 中的 fingerprint 分支：调用 `tlsfingerprint.NewTransport`
- 修改 `transportFor` 中的 VLESS 分支：调用 `httpproxy.NewProxy` 的 dialer

```go
import (
	tlsfp "github.com/6643/fetch/tlsfingerprint"
	httppxy "github.com/6643/fetch/httpproxy"
)

func transportFor(cfg *callConfig) (http.RoundTripper, func(), error) {
	// VLESS 分支
	if cfg.proxyURI != "" {
		dialFn, err := httppxy.DialContext(cfg.proxyURI)
		if err != nil {
			return nil, nil, err
		}
		transport := defaultTransport.Clone()
		transport.DialContext = dialFn
		return transport, transport.CloseIdleConnections, nil
	}
	
	// fingerprint 分支
	if cfg.fingerprint != "" {
		rt, err := tlsfp.NewTransport(cfg.fingerprint, tlsfp.WithTLSConfig(cfg.tlsConfig), tlsfp.WithLocalAddr(cfg.localAddr))
		if err != nil {
			return nil, nil, err
		}
		return rt, rt.CloseIdleConnections, nil
	}
	
	// 其余路径保持不变
	// ...
}
```

- [ ] **Step 3: 删除已迁移的 fetch 源文件**

```bash
cd /home/_/._/fetch
rm vless.go vless_config.go vless_export.go ech.go fingerprint.go
```

- [ ] **Step 4: 更新 fetch 测试文件**

- 删除 `vless_test.go`（完整迁移到 httpproxy）
- 删除 `fingerprint_test.go`（完整迁移到 tlsfingerprint）
- 删除 `ech_test.go`（完整迁移到 httpproxy）
- 删除 `vless_nodes_test.go`、`vless_sub_test.go`（灰度测试，已迁移）
- 更新 `client_test.go`，移除 VLESS 相关测试
- 保留 `fetch_test.go` 和 `client_test.go` 中不依赖 VLESS/fingerprint 的测试

- [ ] **Step 5: 重构 WithProxy 路由**

`transport.go` 中 `WithProxy` 函数：
- 移除 `vless://` scheme 路由
- 只保留 `http://`、`https://`、`socks5://`、`socks5h://`

- [ ] **Step 6: 验证所有测试通过**

```bash
cd /home/_/._/fetch
go build ./...
go test ./... -count=1 -timeout=120s -run 'TestHTTP|TestDo|TestClient|TestGet|TestPost|TestPut|TestDelete|TestWithProxy|TestWithLocalAddr|TestWithTLSConfig|TestResponseBodyLimit|TestNewLocalDialer|TestTransportFor'
```

- Commit:

```bash
git add go.mod go.sum transport.go client.go
git rm vless.go vless_config.go vless_export.go ech.go fingerprint.go vless_test.go fingerprint_test.go ech_test.go vless_nodes_test.go vless_sub_test.go client_test.go
git commit -m "refactor: switch fetch to use tlsfingerprint and httpproxy submodules"
```

---

### Task 6: 清理和最终验证

- [ ] **Step 1: 验证 tlsfingerprint 独立测试**

```bash
cd /home/_/._/fetch/tlsfingerprint
go test ./... -count=1 -timeout=60s
```

- [ ] **Step 2: 验证 httpproxy 独立测试**

```bash
cd /home/_/._/fetch/httpproxy
go test ./... -count=1 -timeout=60s
```

- [ ] **Step 3: 验证 fetch 完整测试**

```bash
cd /home/_/._/fetch
go test ./... -count=1 -timeout=120s
```

- [ ] **Step 4: 验证 go.work 构建**

```bash
cd /home/_/._/fetch
go work sync
go build ./... github.com/6643/fetch/tlsfingerprint/... github.com/6643/fetch/httpproxy/...
```

- [ ] **Step 5: 检查 go.sum 一致性**

```bash
cd /home/_/._/fetch
git diff --stat
# 确认没有多余文件
```
