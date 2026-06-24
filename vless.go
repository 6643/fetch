package fetch

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	utls "github.com/refraction-networking/utls"
)

func setupVless(cfg *callConfig, vlessURI string) error {
	if !strings.HasPrefix(vlessURI, "vless://") {
		return fmt.Errorf("VLESS URI must start with vless://")
	}
	cfg.proxyURI = vlessURI
	cfg.proxySet = false
	return nil
}

func newVlessRoundTripper(vlessURI string) (http.RoundTripper, error) {
	cfg, err := parseVlessURI(vlessURI)
	if err != nil {
		return nil, err
	}

	dialer := &vlessDialer{cfg: cfg}
	return &http.Transport{
		Proxy:                 nil,
		DialContext:           dialer.DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}, nil
}

type vlessDialer struct {
	cfg     vlessConfig
	rootCAs *x509.CertPool

	echMu               sync.Mutex
	cachedECHConfigList []byte
	cachedECHExpiresAt  time.Time
}

func (d *vlessDialer) DialContext(ctx context.Context, network string, address string) (net.Conn, error) {
	if network != "tcp" {
		return nil, fmt.Errorf("unsupported network: %s", network)
	}

	echConfigList, err := d.echConfigList(ctx)
	if err != nil {
		return nil, fmt.Errorf("resolve ECH config: %w", err)
	}

	wsURL := "wss://" + d.cfg.serverAddr()
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

	connCtx, cancelConn := context.WithCancel(context.Background())
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

type vlessConn struct {
	net.Conn

	cancel             context.CancelFunc
	responseHeaderMu   sync.Mutex
	responseHeaderRead bool
	responseHeaderErr  error
}

func (c *vlessConn) Close() error {
	if c.cancel != nil {
		c.cancel()
	}
	return c.Conn.Close()
}

func (c *vlessConn) Read(p []byte) (int, error) {
	if err := c.consumeResponseHeader(); err != nil {
		return 0, err
	}
	return c.Conn.Read(p)
}

func (c *vlessConn) consumeResponseHeader() error {
	c.responseHeaderMu.Lock()
	defer c.responseHeaderMu.Unlock()

	if c.responseHeaderRead || c.responseHeaderErr != nil {
		return c.responseHeaderErr
	}

	c.responseHeaderErr = consumeVlessResponseHeader(c.Conn)
	if c.responseHeaderErr == nil {
		c.responseHeaderRead = true
	}
	return c.responseHeaderErr
}

func (d *vlessDialer) echConfigList(ctx context.Context) ([]byte, error) {
	if d.cfg.ECHSpec == nil {
		return nil, nil
	}
	return d.resolveECHConfigList(ctx)
}

func (d *vlessDialer) resolveECHConfigList(ctx context.Context) ([]byte, error) {
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

func websocketHTTPClient(cfg vlessConfig, rootCAs *x509.CertPool, echConfigList []byte) *http.Client {
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

		uConfig := toUTLSConfig(tlsConfig)
		uConfig.NextProtos = []string{"http/1.1"}

		conn := utls.UClient(rawConn, uConfig, helloID)
		if err := handshakeUTLSWebsocket(ctx, conn, len(tlsConfig.EncryptedClientHelloConfigList) > 0); err != nil {
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

func handshakeUTLSWebsocket(ctx context.Context, conn *utls.UConn, hasECH bool) error {
	if err := conn.BuildHandshakeState(); err != nil {
		return err
	}
	if !hasECH {
		hasALPN := false
		for _, extension := range conn.Extensions {
			alpn, ok := extension.(*utls.ALPNExtension)
			if !ok {
				continue
			}
			hasALPN = true
			alpn.AlpnProtocols = []string{"http/1.1"}
			break
		}
		if !hasALPN {
			conn.Extensions = append(conn.Extensions, &utls.ALPNExtension{AlpnProtocols: []string{"http/1.1"}})
		}
		if err := conn.BuildHandshakeState(); err != nil {
			return err
		}
	}
	return conn.HandshakeContext(ctx)
}

const (
	vlessVersion           = byte(0x00)
	vlessCommandTCP        = byte(0x01)
	vlessAddressTypeIPv4   = byte(0x01)
	vlessAddressTypeDomain = byte(0x02)
	vlessAddressTypeIPv6   = byte(0x03)
)

func encodeVlessTCPRequestHeader(uuidBytes [16]byte, targetAddr string) ([]byte, error) {
	host, portText, err := net.SplitHostPort(targetAddr)
	if err != nil {
		return nil, fmt.Errorf("split target address: %w", err)
	}

	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return nil, fmt.Errorf("invalid target port")
	}

	header := make([]byte, 0, 64+len(host))
	header = append(header, vlessVersion)
	header = append(header, uuidBytes[:]...)
	header = append(header, 0x00)
	header = append(header, vlessCommandTCP)
	header = binary.BigEndian.AppendUint16(header, uint16(port))

	ip := net.ParseIP(host)
	switch {
	case ip.To4() != nil:
		header = append(header, vlessAddressTypeIPv4)
		header = append(header, ip.To4()...)
	case ip.To16() != nil:
		header = append(header, vlessAddressTypeIPv6)
		header = append(header, ip.To16()...)
	default:
		if len(host) == 0 || len(host) > 255 {
			return nil, fmt.Errorf("invalid target domain length")
		}
		header = append(header, vlessAddressTypeDomain, byte(len(host)))
		header = append(header, host...)
	}

	return header, nil
}

func consumeVlessResponseHeader(reader io.Reader) error {
	var fixed [2]byte
	if _, err := io.ReadFull(reader, fixed[:]); err != nil {
		return fmt.Errorf("read VLESS response header: %w", err)
	}
	if fixed[0] != vlessVersion {
		return fmt.Errorf("unexpected VLESS response version: %d", fixed[0])
	}
	if fixed[1] == 0 {
		return nil
	}

	addons := make([]byte, int(fixed[1]))
	if _, err := io.ReadFull(reader, addons); err != nil {
		return fmt.Errorf("read VLESS response addons: %w", err)
	}
	return nil
}
