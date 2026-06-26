package fingerprint

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"time"

	utls "github.com/refraction-networking/utls"
	"golang.org/x/net/http2"
)

// Transport is an HTTP/2 transport that uses uTLS for TLS fingerprinting.
type Transport struct {
	h2Transport *http2.Transport
	dialer      *net.Dialer
	fingerprint string
	tlsConfig   *tls.Config
}

// NewTransport creates a new Transport with the given fingerprint name,
// TLS configuration, and optional local bind address. Pass nil for tlsConfig
// to use defaults. Pass an empty string for localAddr to skip local binding.
func NewTransport(fingerprint string, tlsConfig *tls.Config, localAddr string) (*Transport, error) {
	var dialer *net.Dialer
	if localAddr != "" {
		ip := net.ParseIP(localAddr)
		if ip == nil {
			return nil, &net.AddrError{Err: "invalid local address", Addr: localAddr}
		}
		dialer = &net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
			LocalAddr: &net.TCPAddr{IP: ip},
		}
	} else {
		dialer = &net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}
	}

	t := &Transport{
		dialer:      dialer,
		fingerprint: fingerprint,
		tlsConfig:   tlsConfig,
	}

	h2Transport := &http2.Transport{
		DialTLSContext: func(ctx context.Context, network, addr string, _ *tls.Config) (net.Conn, error) {
			return t.dialWithFingerprint(ctx, network, addr)
		},
		AllowHTTP:       true,
		ReadIdleTimeout: 30 * time.Second,
		PingTimeout:     10 * time.Second,
	}
	t.h2Transport = h2Transport

	return t, nil
}

func (t *Transport) dialWithFingerprint(ctx context.Context, network, addr string) (net.Conn, error) {
	helloID, err := ResolveFingerprint(t.fingerprint)
	if err != nil {
		return nil, err
	}

	tcpConn, err := t.dialer.DialContext(ctx, network, addr)
	if err != nil {
		return nil, err
	}

	host, _, _ := net.SplitHostPort(addr)
	tlsCfg := t.tlsConfig
	if tlsCfg == nil {
		tlsCfg = &tls.Config{}
	}
	cloned := tlsCfg.Clone()
	if cloned.ServerName == "" {
		cloned.ServerName = host
	}

	uConfig := ToUTLSConfig(cloned)
	uConfig.NextProtos = []string{"h2", "http/1.1"}

	tlsConn := utls.UClient(tcpConn, uConfig, helloID)
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		tcpConn.Close()
		return nil, err
	}

	return tlsConn, nil
}

// Fingerprint returns the fingerprint name used by this transport.
func (t *Transport) Fingerprint() string {
	return t.fingerprint
}

// RoundTrip executes a single HTTP transaction.
func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	return t.h2Transport.RoundTrip(req)
}

// CloseIdleConnections closes any idle connections in the underlying HTTP/2 transport.
func (t *Transport) CloseIdleConnections() {
	t.h2Transport.CloseIdleConnections()
}
