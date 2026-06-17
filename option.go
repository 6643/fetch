package fetch

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

var defaultRequestTimeout = 5 * time.Second
var defaultResponseBodyLimit int64 = 10 << 20

// Option configures a single request execution.
type Option func(cfg *callConfig) error

type callConfig struct {
	ctx               context.Context
	timeout           time.Duration
	responseBodyLimit int64

	userAgent   string
	contentType string
	body        io.Reader
	headers     http.Header
	cookies     []*http.Cookie
	query       url.Values

	formValues      url.Values
	multipartFields url.Values
	multipartFiles  []multipartFile
	bodySetType     bodyType

	proxyURL     *url.URL
	proxySet     bool
	localAddr    string
	localAddrSet bool
	tlsConfig    *tls.Config
	fingerprint  string
	vlessURI     string
}

func newCallConfig(opts ...Option) (*callConfig, error) {
	cfg := &callConfig{
		ctx:               context.Background(),
		timeout:           defaultRequestTimeout,
		responseBodyLimit: defaultResponseBodyLimit,
		headers:           make(http.Header),
		cookies:           []*http.Cookie{},
	}

	for _, opt := range opts {
		if err := opt(cfg); err != nil {
			return nil, err
		}
	}
	if err := cfg.validateTransportOptions(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (cfg *callConfig) contextWithTimeout() (context.Context, context.CancelFunc) {
	if cfg.timeout <= 0 {
		return cfg.ctx, nil
	}
	return context.WithTimeout(cfg.ctx, cfg.timeout)
}

func (cfg *callConfig) hasTransportOverrides() bool {
	return cfg.proxySet || cfg.localAddrSet || cfg.tlsConfig != nil || cfg.fingerprint != "" || cfg.vlessURI != ""
}

func (cfg *callConfig) validateTransportOptions() error {
	if cfg.vlessURI == "" {
		return nil
	}
	if cfg.proxySet || cfg.localAddrSet || cfg.tlsConfig != nil || cfg.fingerprint != "" {
		return fmt.Errorf("WithVless cannot be combined with other transport options")
	}
	return nil
}

// WithResponseBodyLimit sets the maximum number of bytes read from the response body.
// Passing 0 disables the limit for the current request.
func WithResponseBodyLimit(limit int64) Option {
	return func(cfg *callConfig) error {
		if limit < 0 {
			return fmt.Errorf("response body limit cannot be negative")
		}
		cfg.responseBodyLimit = limit
		return nil
	}
}
