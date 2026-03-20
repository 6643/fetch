package fetch

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"time"
)

var defaultTransport = newDefaultTransport()
var overrideTransportCache sync.Map
var overrideTransportCacheEntries atomic.Int64

var maxOverrideTransportCacheEntries int64 = 64

type transportCacheKey struct {
	proxySet     bool
	proxyURLSum  [32]byte
	localAddrSet bool
	localAddr    string
}

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
	if cfg.tlsConfig != nil {
		transport := defaultTransport.Clone()
		if err := applyTransportOptions(transport, cfg); err != nil {
			return nil, nil, err
		}
		return transport, transport.CloseIdleConnections, nil
	}

	cacheKey := newTransportCacheKey(cfg)
	if cachedTransport, ok := loadCachedTransport(cacheKey); ok {
		return cachedTransport, nil, nil
	}

	transport := defaultTransport.Clone()
	if err := applyTransportOptions(transport, cfg); err != nil {
		return nil, nil, err
	}

	cachedTransport, cached := storeCachedTransport(cacheKey, transport)
	if !cached {
		return cachedTransport, cachedTransport.CloseIdleConnections, nil
	}
	return cachedTransport, nil, nil
}

func newTransportCacheKey(cfg *callConfig) transportCacheKey {
	key := transportCacheKey{
		proxySet:     cfg.proxySet,
		localAddrSet: cfg.localAddrSet,
		localAddr:    cfg.localAddr,
	}
	if cfg.proxyURL != nil {
		key.proxyURLSum = sha256.Sum256([]byte(cfg.proxyURL.String()))
	}
	return key
}

func loadCachedTransport(key transportCacheKey) (*http.Transport, bool) {
	transport, ok := overrideTransportCache.Load(key)
	if !ok {
		return nil, false
	}
	return transport.(*http.Transport), true
}

func storeCachedTransport(key transportCacheKey, transport *http.Transport) (*http.Transport, bool) {
	if overrideTransportCacheEntries.Load() >= maxOverrideTransportCacheEntries {
		return transport, false
	}

	actualTransport, loaded := overrideTransportCache.LoadOrStore(key, transport)
	if loaded {
		transport.CloseIdleConnections()
		return actualTransport.(*http.Transport), true
	}
	overrideTransportCacheEntries.Add(1)
	if overrideTransportCacheEntries.Load() > maxOverrideTransportCacheEntries {
		overrideTransportCache.Delete(key)
		overrideTransportCacheEntries.Add(-1)
		return transport, false
	}
	return transport, true
}

func resetOverrideTransportCache() {
	overrideTransportCache = sync.Map{}
	overrideTransportCacheEntries.Store(0)
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
	dialer, err := newLocalDialer(localAddr)
	if err != nil {
		return nil, err
	}

	return dialer.DialContext, nil
}

func newLocalDialer(localAddr string) (*net.Dialer, error) {
	ip := net.ParseIP(localAddr)
	if ip == nil {
		return nil, fmt.Errorf("invalid local address %q", localAddr)
	}

	return &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
		LocalAddr: &net.TCPAddr{IP: ip},
	}, nil
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
		if !parsedURL.IsAbs() || parsedURL.Scheme == "" || parsedURL.Host == "" {
			return fmt.Errorf("proxy url %q must be an absolute URL", rawURL)
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
