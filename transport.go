package fetch

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"
)

var defaultTransport = newDefaultTransport()

func newDefaultTransport() *http.Transport {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = http.ProxyFromEnvironment
	transport.MaxIdleConns = 100
	transport.IdleConnTimeout = 90 * time.Second
	transport.ExpectContinueTimeout = time.Second
	return transport
}

func transportFor(cfg *callConfig) (*http.Transport, func(), error) {
	if !cfg.hasTransportOverrides() {
		return defaultTransport, nil, nil
	}

	transport := defaultTransport.Clone()
	if err := applyTransportOptions(transport, cfg); err != nil {
		return nil, nil, err
	}

	return transport, transport.CloseIdleConnections, nil
}

func applyTransportOptions(transport *http.Transport, cfg *callConfig) error {
	applyProxy(transport, cfg)

	if err := applyLocalAddr(transport, cfg); err != nil {
		return err
	}

	applyTLSConfig(transport, cfg)
	return nil
}

func applyProxy(transport *http.Transport, cfg *callConfig) {
	if !cfg.proxySet {
		return
	}

	if cfg.proxyURL == nil {
		transport.Proxy = nil
		return
	}

	transport.Proxy = http.ProxyURL(cfg.proxyURL)
}

func applyLocalAddr(transport *http.Transport, cfg *callConfig) error {
	if !cfg.localAddrSet {
		return nil
	}

	dialContext, err := newDialContext(cfg.localAddr)
	if err != nil {
		return err
	}

	transport.DialContext = dialContext
	return nil
}

func applyTLSConfig(transport *http.Transport, cfg *callConfig) {
	if cfg.tlsConfig != nil {
		transport.TLSClientConfig = cfg.tlsConfig.Clone()
	}
}

func newDialContext(localAddr string) (func(ctx context.Context, network, address string) (net.Conn, error), error) {
	ip := net.ParseIP(localAddr)
	if ip == nil {
		return nil, fmt.Errorf("invalid local address %q", localAddr)
	}

	dialer := &net.Dialer{
		KeepAlive: 30 * time.Second,
		LocalAddr: &net.TCPAddr{IP: ip},
	}

	return dialer.DialContext, nil
}

// WithProxy routes the current request through the provided proxy URL.
// Passing an empty string disables proxying for the current request.
func WithProxy(rawURL string) Option {
	return func(cfg *callConfig) error {
		cfg.proxySet = true
		if rawURL == "" {
			cfg.proxyURL = nil
			return nil
		}

		parsedURL, err := url.Parse(rawURL)
		if err != nil {
			return fmt.Errorf("failed to parse proxy url %q: %w", rawURL, err)
		}

		cfg.proxyURL = parsedURL
		return nil
	}
}

// WithLocalAddr binds the request to a specific local IP.
func WithLocalAddr(localAddr string) Option {
	return func(cfg *callConfig) error {
		if net.ParseIP(localAddr) == nil {
			return fmt.Errorf("invalid local address %q", localAddr)
		}

		cfg.localAddrSet = true
		cfg.localAddr = localAddr
		return nil
	}
}

// WithTLSConfig uses a custom TLS configuration for the current request.
func WithTLSConfig(tlsConfig *tls.Config) Option {
	return func(cfg *callConfig) error {
		if tlsConfig == nil {
			return fmt.Errorf("tls config cannot be nil")
		}

		cfg.tlsConfig = tlsConfig.Clone()
		return nil
	}
}
