package httpproxy

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/6643/fetch/tlsfingerprint"
	utls "github.com/refraction-networking/utls"
)

// Dialer handles a single VLESS proxy configuration and can be used to create
// connections through that proxy.
type Dialer struct {
	cfg     Config
	rootCAs *x509.CertPool

	echMu               sync.Mutex
	cachedECHConfigList []byte
	cachedECHExpiresAt  time.Time
}

// DialContext establishes a TCP connection through the VLESS WebSocket proxy.
func (d *Dialer) DialContext(ctx context.Context, network string, address string) (net.Conn, error) {
	if network != "tcp" {
		return nil, fmt.Errorf("unsupported network: %s", network)
	}

	echConfigList, err := d.echConfigList(ctx)
	if err != nil {
		return nil, fmt.Errorf("resolve ECH config: %w", err)
	}

	wsURL := "wss://" + d.cfg.ServerAddr()
	if p := d.cfg.WebSocketPath; p != "" {
		wsURL += p
	}

	wsConn, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		HTTPClient: websocketHTTPClient(d.cfg, d.rootCAs, echConfigList),
		Host:       d.cfg.WebSocketHost,
	})
	if err != nil {
		return nil, fmt.Errorf("dial VLESS websocket: %w", err)
	}

	connCtx, cancelConn := context.WithCancel(ctx)
	conn := websocket.NetConn(connCtx, wsConn, websocket.MessageBinary)

	header, err := encodeVlessTCPRequestHeader(d.cfg.UUIDBytes, address)
	if err != nil {
		cancelConn()
		conn.Close()
		return nil, err
	}
	if _, err := conn.Write(header); err != nil {
		cancelConn()
		conn.Close()
		return nil, fmt.Errorf("write VLESS request header: %w", err)
	}

	return &vlessConn{Conn: conn, cancel: cancelConn}, nil
}

func (d *Dialer) echConfigList(ctx context.Context) ([]byte, error) {
	if d.cfg.ECHSpec == nil {
		return nil, nil
	}
	return d.resolveECHConfigList(ctx)
}

func (d *Dialer) resolveECHConfigList(ctx context.Context) ([]byte, error) {
	d.echMu.Lock()
	defer d.echMu.Unlock()

	if len(d.cachedECHConfigList) > 0 && time.Now().Before(d.cachedECHExpiresAt) {
		return append([]byte(nil), d.cachedECHConfigList...), nil
	}

	resolved, err := resolveECHConfigList(ctx, *d.cfg.ECHSpec)
	if err != nil {
		return nil, err
	}
	d.cachedECHConfigList = append([]byte(nil), resolved.ConfigList...)
	if resolved.TTL > 0 {
		d.cachedECHExpiresAt = time.Now().Add(resolved.TTL)
	} else {
		d.cachedECHExpiresAt = time.Time{}
	}
	return append([]byte(nil), d.cachedECHConfigList...), nil
}

func websocketHTTPClient(cfg Config, rootCAs *x509.CertPool, echConfigList []byte) *http.Client {
	tlsConfig := &tls.Config{
		ServerName: cfg.TLSServerName,
		RootCAs:    rootCAs,
		MinVersion: tls.VersionTLS12,
		NextProtos: []string{"http/1.1"},
	}
	if len(echConfigList) > 0 {
		tlsConfig.MinVersion = tls.VersionTLS13
		tlsConfig.EncryptedClientHelloConfigList = echConfigList
	}

	transport := &http.Transport{
		Proxy:           nil,
		TLSClientConfig: tlsConfig,
	}
	if strings.EqualFold(cfg.TLSFingerprint, "chrome") {
		transport.DialTLSContext = websocketUTLSDialContext(tlsConfig)
	}

	return &http.Client{Transport: transport}
}

func websocketUTLSDialContext(tlsConfig *tls.Config) func(ctx context.Context, network string, addr string) (net.Conn, error) {
	helloID := utls.HelloChrome_Auto

	return func(ctx context.Context, network string, addr string) (net.Conn, error) {
		rawConn, err := (&net.Dialer{
			Timeout: 30 * time.Second,
		}).DialContext(ctx, network, addr)
		if err != nil {
			return nil, err
		}

		uConfig := tlsfingerprint.ToUTLSConfig(tlsConfig)
		uConfig.NextProtos = []string{"http/1.1"}

		conn := utls.UClient(rawConn, uConfig, helloID)
		if err := handshakeUTLSWebsocket(ctx, conn); err != nil {
			rawConn.Close()
			return nil, fmt.Errorf("uTLS websocket handshake: %w", err)
		}
		if !tlsConfig.InsecureSkipVerify {
			if err := conn.VerifyHostname(tlsConfig.ServerName); err != nil {
				conn.Close()
				return nil, fmt.Errorf("verify websocket server name: %w", err)
			}
		}
		return conn, nil
	}
}
