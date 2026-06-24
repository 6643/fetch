package fetch

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	utls "github.com/refraction-networking/utls"
)

const testVlessUUID = "11111111-2222-3333-4444-555555555555"

func TestVlessConnConsumesResponseHeaderOnlyOnce(t *testing.T) {
	conn := &vlessConn{Conn: &chunkedConn{chunks: [][]byte{
		{0x00, 0x00},
		[]byte("HTTP/1.1 "),
		[]byte("200 OK\r\n"),
	}}}

	buf := make([]byte, len("HTTP/1.1 "))
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(buf[:n]); got != "HTTP/1.1 " {
		t.Fatalf("unexpected first payload read: %q", got)
	}

	buf = make([]byte, len("200 OK\r\n"))
	n, err = conn.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(buf[:n]); got != "200 OK\r\n" {
		t.Fatalf("unexpected second payload read: %q", got)
	}
}

func TestHTTPGetThroughLoopbackVlessWebSocket(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/resource" {
			t.Errorf("target path = %q", r.URL.Path)
		}
		w.Header().Set("Connection", "close")
		w.Header().Set("X-Target", "ok")
		_, _ = w.Write([]byte("proxied"))
	}))
	defer target.Close()

	targetURL, err := url.Parse(target.URL)
	if err != nil {
		t.Fatal(err)
	}

	seenTarget := make(chan string, 1)
	vlessServer := newLoopbackVlessServer(t, seenTarget)
	defer vlessServer.Close()

	serverURL, err := url.Parse(vlessServer.URL)
	if err != nil {
		t.Fatal(err)
	}
	cfg := vlessConfig{
		UUIDBytes:     mustUUIDBytes(t, testVlessUUID),
		ServerHost:    serverURL.Hostname(),
		ServerPort:    serverURL.Port(),
		TLSServerName: serverURL.Hostname(),
		WebSocketHost: serverURL.Host,
		WebSocketPath: "/ws",
	}

	client := &http.Client{
		Transport: newVlessTransportForTest(cfg, certPoolForServer(t, vlessServer)),
		Timeout:   5 * time.Second,
	}
	resp, err := client.Get(target.URL + "/resource")
	if err != nil {
		t.Fatalf("client.Get returned error: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "proxied" {
		t.Fatalf("body = %q", body)
	}
	if resp.Header.Get("X-Target") != "ok" {
		t.Fatalf("X-Target = %q", resp.Header.Get("X-Target"))
	}

	select {
	case got := <-seenTarget:
		if got != targetURL.Host {
			t.Fatalf("VLESS target = %q, want %q", got, targetURL.Host)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for VLESS target")
	}
}

func TestHTTPGetThroughVlessWhenServerDelaysResponseHeader(t *testing.T) {
	seenTarget := make(chan string, 1)
	vlessServer := newDelayedHeaderVlessServer(t, seenTarget)
	defer vlessServer.Close()

	client := clientForLoopbackServer(t, vlessServer)

	resp, err := client.Get("http://delayed.example:80/")
	if err != nil {
		t.Fatalf("client.Get returned error: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "ok" {
		t.Fatalf("body = %q", body)
	}
	if got := <-seenTarget; got != "delayed.example:80" {
		t.Fatalf("seen target = %q", got)
	}
}

func TestHandshakeUTLSWebsocketForcesHTTP1ALPN(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	tlsConn := utls.UClient(clientConn, &utls.Config{
		ServerName: "example.com",
		NextProtos: []string{"http/1.1"},
	}, utls.HelloChrome_Auto)
	errCh := make(chan error, 1)
	go func() {
		errCh <- handshakeUTLSWebsocket(context.Background(), tlsConn, false)
	}()

	hello := readTLSRecord(t, serverConn)
	serverConn.Close()
	if err := <-errCh; err == nil {
		t.Fatal("expected handshake error after closing server side")
	}

	if got := clientHelloALPNProtocols(t, hello); !reflect.DeepEqual(got, []string{"http/1.1"}) {
		t.Fatalf("ALPN protocols = %#v, want only http/1.1", got)
	}
}

func TestVlessProxyRejectsOtherTransportOptions(t *testing.T) {
	_, err := newCallConfig(
		WithProxy("vless://"+testVlessUUID+"@example.com:443?security=tls&type=ws"),
		WithFingerprint("chrome"),
	)
	if err == nil {
		t.Fatal("expected VLESS and fingerprint options to be mutually exclusive")
	}

	_, err = newCallConfig(
		WithProxy("vless://"+testVlessUUID+"@example.com:443?security=tls&type=ws"),
		WithLocalAddr("127.0.0.1"),
	)
	if err == nil {
		t.Fatal("expected VLESS and local addr options to be mutually exclusive")
	}

	_, err = newCallConfig(
		WithProxy("vless://"+testVlessUUID+"@example.com:443?security=tls&type=ws"),
		WithTLSConfig(&tls.Config{}),
	)
	if err == nil {
		t.Fatal("expected VLESS and TLS config options to be mutually exclusive")
	}
}

func TestVlessProxyAcceptsValidURI(t *testing.T) {
	_, err := newCallConfig(WithProxy("vless://" + testVlessUUID + "@example.com:443?security=tls&type=ws"))
	if err != nil {
		t.Fatalf("expected valid VLESS URI, got error: %v", err)
	}
}

func TestVlessProxyRejectsEmpty(t *testing.T) {
	_, err := newCallConfig(WithProxy(""))
	if err != nil {
		t.Fatalf("unexpected error for empty proxy URI: %v", err)
	}
}

func TestWithProxyRejectsUnsupportedScheme(t *testing.T) {
	_, err := newCallConfig(WithProxy("ftp://example.com"))
	if err == nil {
		t.Fatal("expected error for unsupported proxy scheme")
	}
}

func TestVlessProxySetsTransportOverride(t *testing.T) {
	cfg, err := newCallConfig(WithProxy("vless://" + testVlessUUID + "@example.com:443?security=tls&type=ws"))
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.hasTransportOverrides() {
		t.Fatal("expected hasTransportOverrides to be true with VLESS set")
	}
}

func TestVlessProxyReplacesTransport(t *testing.T) {
	cfg, err := newCallConfig(WithProxy("vless://" + testVlessUUID + "@example.com:443?security=tls&type=ws"))
	if err != nil {
		t.Fatal(err)
	}
	rt, cleanup, err := transportFor(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if cleanup != nil {
		defer cleanup()
	}
	if rt == nil {
		t.Fatal("expected non-nil round tripper for VLESS")
	}
}

func TestWithProxyLastCallWins(t *testing.T) {
	// VLESS overrides HTTP proxy
	cfg, err := newCallConfig(
		WithProxy("http://127.0.0.1:8080"),
		WithProxy("vless://"+testVlessUUID+"@example.com:443?security=tls&type=ws"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.proxyURI == "" {
		t.Fatal("expected proxyURI to be set after VLESS override")
	}
	if cfg.proxySet {
		t.Fatal("expected proxySet to be false after VLESS override")
	}

	// HTTP proxy overrides VLESS
	cfg2, err := newCallConfig(
		WithProxy("vless://"+testVlessUUID+"@example.com:443?security=tls&type=ws"),
		WithProxy("http://127.0.0.1:8080"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if cfg2.proxyURI != "" {
		t.Fatal("expected proxyURI to be empty after HTTP override")
	}
	if !cfg2.proxySet {
		t.Fatal("expected proxySet to be true after HTTP override")
	}
}

func TestParseVlessURIStoresECHQuerySpec(t *testing.T) {
	cfg, err := parseVlessURI("vless://" + testVlessUUID + "@example.com:443?security=tls&type=ws&ech=cloudflare-ech.com%2Bhttps%3A%2F%2Fdns.google%2Fdns-query")
	if err != nil {
		t.Fatalf("parseVlessURI returned error: %v", err)
	}
	if cfg.ECHSpec == nil {
		t.Fatal("ECHSpec is nil")
	}
	if cfg.ECHSpec.PublicName != "cloudflare-ech.com" {
		t.Fatalf("ECHSpec.PublicName = %q", cfg.ECHSpec.PublicName)
	}
	if cfg.ECHSpec.DoHURL != "https://dns.google/dns-query" {
		t.Fatalf("ECHSpec.DoHURL = %q", cfg.ECHSpec.DoHURL)
	}
}

func TestParseVlessURIStoresECHQuerySpecWithRawPlusSeparator(t *testing.T) {
	cfg, err := parseVlessURI("vless://" + testVlessUUID + "@example.com:443?security=tls&type=ws&ech=cloudflare-ech.com+https://dns.google/dns-query")
	if err != nil {
		t.Fatalf("parseVlessURI returned error: %v", err)
	}
	if cfg.ECHSpec == nil {
		t.Fatal("ECHSpec is nil")
	}
	if cfg.ECHSpec.PublicName != "cloudflare-ech.com" {
		t.Fatalf("ECHSpec.PublicName = %q", cfg.ECHSpec.PublicName)
	}
	if cfg.ECHSpec.DoHURL != "https://dns.google/dns-query" {
		t.Fatalf("ECHSpec.DoHURL = %q", cfg.ECHSpec.DoHURL)
	}
}

type chunkedConn struct {
	chunks [][]byte
}

func (c *chunkedConn) Read(p []byte) (int, error) {
	if len(c.chunks) == 0 {
		return 0, io.EOF
	}
	n := copy(p, c.chunks[0])
	c.chunks[0] = c.chunks[0][n:]
	if len(c.chunks[0]) == 0 {
		c.chunks = c.chunks[1:]
	}
	return n, nil
}

func (c *chunkedConn) Write(p []byte) (int, error)      { return len(p), nil }
func (c *chunkedConn) Close() error                     { return nil }
func (c *chunkedConn) LocalAddr() net.Addr              { return mockAddr("local") }
func (c *chunkedConn) RemoteAddr() net.Addr             { return mockAddr("remote") }
func (c *chunkedConn) SetDeadline(time.Time) error      { return nil }
func (c *chunkedConn) SetReadDeadline(time.Time) error  { return nil }
func (c *chunkedConn) SetWriteDeadline(time.Time) error { return nil }

type mockAddr string

func (a mockAddr) Network() string { return string(a) }
func (a mockAddr) String() string  { return string(a) }

func newVlessTransportForTest(cfg vlessConfig, roots *x509.CertPool) http.RoundTripper {
	dialer := &vlessDialer{cfg: cfg, rootCAs: roots}
	return &http.Transport{
		Proxy:                 nil,
		DialContext:           dialer.DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: time.Second,
	}
}

func clientForLoopbackServer(t *testing.T, server *httptest.Server) *http.Client {
	t.Helper()

	serverURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	cfg := vlessConfig{
		UUIDBytes:     mustUUIDBytes(t, testVlessUUID),
		ServerHost:    serverURL.Hostname(),
		ServerPort:    serverURL.Port(),
		TLSServerName: serverURL.Hostname(),
		WebSocketHost: serverURL.Host,
		WebSocketPath: "/ws",
	}
	return &http.Client{
		Transport: newVlessTransportForTest(cfg, certPoolForServer(t, server)),
		Timeout:   5 * time.Second,
	}
}

func newLoopbackVlessServer(t *testing.T, seenTarget chan<- string) *httptest.Server {
	t.Helper()

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ws" {
			http.NotFound(w, r)
			return
		}
		wsConn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
		if err != nil {
			t.Errorf("websocket accept: %v", err)
			return
		}
		defer wsConn.CloseNow()

		conn := websocket.NetConn(context.Background(), wsConn, websocket.MessageBinary)
		defer conn.Close()

		target, err := readVlessRequestTarget(conn)
		if err != nil {
			t.Errorf("read VLESS request: %v", err)
			return
		}
		seenTarget <- target

		if _, err := conn.Write([]byte{0x00, 0x00}); err != nil {
			t.Errorf("write VLESS response header: %v", err)
			return
		}

		if strings.HasSuffix(target, ".example:80") {
			writeStaticHTTPResponse(t, conn)
			return
		}
		proxyTCP(t, conn, target)
	}))
	return server
}

func newDelayedHeaderVlessServer(t *testing.T, seenTarget chan<- string) *httptest.Server {
	t.Helper()

	return httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ws" {
			http.NotFound(w, r)
			return
		}
		wsConn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
		if err != nil {
			t.Errorf("websocket accept: %v", err)
			return
		}
		defer wsConn.CloseNow()

		conn := websocket.NetConn(context.Background(), wsConn, websocket.MessageBinary)
		defer conn.Close()

		target, err := readVlessRequestTarget(conn)
		if err != nil {
			t.Errorf("read VLESS request: %v", err)
			return
		}
		seenTarget <- target

		reader := bufio.NewReader(conn)
		if _, err := http.ReadRequest(reader); err != nil {
			t.Errorf("read delayed HTTP request: %v", err)
			return
		}

		if _, err := conn.Write([]byte{0x00, 0x00}); err != nil {
			t.Errorf("write delayed VLESS response header: %v", err)
			return
		}
		_, _ = conn.Write([]byte("HTTP/1.1 200 OK\r\nContent-Length: 2\r\nConnection: close\r\n\r\nok"))
	}))
}

func readVlessRequestTarget(r io.Reader) (string, error) {
	var fixed [20]byte
	if _, err := io.ReadFull(r, fixed[:]); err != nil {
		return "", err
	}
	if fixed[0] != vlessVersion || fixed[17] != 0x00 || fixed[18] != vlessCommandTCP {
		return "", fmt.Errorf("unexpected VLESS request prefix")
	}
	port := int(fixed[19]) << 8
	var portLow [1]byte
	if _, err := io.ReadFull(r, portLow[:]); err != nil {
		return "", err
	}
	port += int(portLow[0])

	var addressType [1]byte
	if _, err := io.ReadFull(r, addressType[:]); err != nil {
		return "", err
	}

	var host string
	switch addressType[0] {
	case vlessAddressTypeIPv4:
		var b [4]byte
		if _, err := io.ReadFull(r, b[:]); err != nil {
			return "", err
		}
		host = net.IP(b[:]).String()
	case vlessAddressTypeIPv6:
		var b [16]byte
		if _, err := io.ReadFull(r, b[:]); err != nil {
			return "", err
		}
		host = net.IP(b[:]).String()
	case vlessAddressTypeDomain:
		var length [1]byte
		if _, err := io.ReadFull(r, length[:]); err != nil {
			return "", err
		}
		b := make([]byte, int(length[0]))
		if _, err := io.ReadFull(r, b); err != nil {
			return "", err
		}
		host = string(b)
	default:
		return "", fmt.Errorf("unknown address type %d", addressType[0])
	}

	return net.JoinHostPort(host, fmt.Sprintf("%d", port)), nil
}

func proxyTCP(t *testing.T, left net.Conn, target string) {
	t.Helper()

	right, err := net.DialTimeout("tcp", target, 3*time.Second)
	if err != nil {
		t.Errorf("dial target: %v", err)
		return
	}
	defer right.Close()

	responseDone := make(chan struct{})
	go func() {
		_, _ = io.Copy(right, left)
		closeWrite(right)
	}()
	go func() {
		defer close(responseDone)
		_, _ = io.Copy(left, right)
	}()

	select {
	case <-responseDone:
	case <-time.After(3 * time.Second):
		t.Errorf("timed out proxying target response")
	}
}

func closeWrite(conn net.Conn) {
	type closeWriter interface {
		CloseWrite() error
	}
	if cw, ok := conn.(closeWriter); ok {
		_ = cw.CloseWrite()
	}
}

func writeStaticHTTPResponse(t *testing.T, conn net.Conn) {
	t.Helper()
	reader := bufio.NewReader(conn)
	if _, err := http.ReadRequest(reader); err != nil {
		t.Errorf("read static HTTP request: %v", err)
		return
	}
	_, _ = conn.Write([]byte("HTTP/1.1 200 OK\r\nContent-Length: 2\r\nConnection: close\r\n\r\nok"))
}

func certPoolForServer(t *testing.T, server *httptest.Server) *x509.CertPool {
	t.Helper()
	pool := x509.NewCertPool()
	pool.AddCert(server.Certificate())
	return pool
}

func mustUUIDBytes(t *testing.T, value string) [16]byte {
	t.Helper()
	uuidBytes, err := parseUUID(value)
	if err != nil {
		t.Fatal(err)
	}
	return uuidBytes
}

func readTLSRecord(t *testing.T, conn net.Conn) []byte {
	t.Helper()
	header := make([]byte, 5)
	if _, err := io.ReadFull(conn, header); err != nil {
		t.Fatal(err)
	}
	length := int(binary.BigEndian.Uint16(header[3:5]))
	body := make([]byte, length)
	if _, err := io.ReadFull(conn, body); err != nil {
		t.Fatal(err)
	}
	return append(header, body...)
}

func clientHelloALPNProtocols(t *testing.T, record []byte) []string {
	t.Helper()
	if len(record) < 9 || record[0] != 0x16 || record[5] != 0x01 {
		t.Fatalf("not a TLS ClientHello record")
	}
	offset := 9
	offset += 2 + 32
	if offset >= len(record) {
		t.Fatalf("short ClientHello")
	}
	sessionIDLength := int(record[offset])
	offset += 1 + sessionIDLength
	if offset+2 > len(record) {
		t.Fatalf("short cipher suites")
	}
	cipherSuitesLength := int(binary.BigEndian.Uint16(record[offset : offset+2]))
	offset += 2 + cipherSuitesLength
	if offset >= len(record) {
		t.Fatalf("short compression methods")
	}
	compressionMethodsLength := int(record[offset])
	offset += 1 + compressionMethodsLength
	if offset+2 > len(record) {
		return nil
	}
	extensionsLength := int(binary.BigEndian.Uint16(record[offset : offset+2]))
	offset += 2
	end := offset + extensionsLength
	if end > len(record) {
		t.Fatalf("invalid extensions length")
	}
	for offset+4 <= end {
		extensionType := binary.BigEndian.Uint16(record[offset : offset+2])
		extensionLength := int(binary.BigEndian.Uint16(record[offset+2 : offset+4]))
		offset += 4
		if offset+extensionLength > end {
			t.Fatalf("invalid extension length")
		}
		if extensionType == 16 {
			return parseALPNExtension(t, record[offset:offset+extensionLength])
		}
		offset += extensionLength
	}
	return nil
}

func parseALPNExtension(t *testing.T, data []byte) []string {
	t.Helper()
	if len(data) < 2 {
		t.Fatalf("short ALPN extension")
	}
	listLength := int(binary.BigEndian.Uint16(data[:2]))
	if listLength != len(data)-2 {
		t.Fatalf("invalid ALPN list length")
	}
	var protocols []string
	for offset := 2; offset < len(data); {
		length := int(data[offset])
		offset++
		if offset+length > len(data) {
			t.Fatalf("invalid ALPN protocol length")
		}
		protocols = append(protocols, string(data[offset:offset+length]))
		offset += length
	}
	return protocols
}
