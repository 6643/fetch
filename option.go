package fetch

import (
	"context"
	"crypto/tls"
	"io"
	"net/http"
	"net/url"
	"time"
)

var defaultRequestTimeout = 5 * time.Second

// Option configures a single request execution.
type Option func(cfg *callConfig) error

// RequestOption is kept as an alias for compatibility with older call sites.
type RequestOption = Option

type callConfig struct {
	ctx     context.Context
	timeout time.Duration

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
}

func newCallConfig(opts ...Option) (*callConfig, error) {
	cfg := &callConfig{
		ctx:     context.Background(),
		timeout: defaultRequestTimeout,
		headers: make(http.Header),
		cookies: []*http.Cookie{},
	}

	for _, opt := range opts {
		if err := opt(cfg); err != nil {
			return nil, err
		}
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
	return cfg.proxySet || cfg.localAddrSet || cfg.tlsConfig != nil
}
