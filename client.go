package fetch

import (
	"fmt"
	"net/http"
	"time"
)

// Client is a reusable HTTP client with a pre-configured transport and default
// request settings. Transport-level options (WithProxy, WithLocalAddr,
// WithTLSConfig, WithFingerprint) are locked at creation time and cannot be
// overridden per-request.
//
// Non-transport options passed to NewClient become per-request defaults:
// WithTimeout, WithResponseBodyLimit, and WithUserAgent. Each can be overridden
// on individual requests.
//
// Close must be called when the client is no longer needed to release
// underlying transport resources.
type Client struct {
	transport         http.RoundTripper
	cleanup           func()
	timeout           time.Duration
	responseBodyLimit int64
	userAgent         string
}

// NewClient creates a reusable HTTP client. Transport options configure the
// underlying connection pool once; non-transport options set request defaults.
//
// Example:
//
//	client := fetch.NewClient(
//	    fetch.WithFingerprint("chrome"),
//	    fetch.WithTimeout(30*time.Second),
//	)
//	defer client.Close()
//	res, _ := client.Get("https://example.com")
func NewClient(opts ...Option) (*Client, error) {
	cfg, err := newCallConfig(opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}

	transport, cleanup, err := transportFor(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}

	return &Client{
		transport:         transport,
		cleanup:           cleanup,
		timeout:           cfg.timeout,
		responseBodyLimit: cfg.responseBodyLimit,
		userAgent:         cfg.userAgent,
	}, nil
}

// Close releases resources held by the client, including idle connections in
// the underlying transport. It is safe to call multiple times.
func (c *Client) Close() {
	if c.cleanup != nil {
		c.cleanup()
	}
}

// Do sends a request with the provided method using the client's transport.
// Per-request options can override timeout, response body limit, user agent,
// headers, body, query parameters, and cookies. Transport-level options are
// rejected — configure them at NewClient instead.
func (c *Client) Do(method, url string, opts ...Option) (*Response, error) {
	cfg, err := newCallConfig(opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to apply options: %w", err)
	}

	if cfg.hasTransportOverrides() {
		return nil, fmt.Errorf(
			"transport options (WithProxy, WithLocalAddr, WithTLSConfig, WithFingerprint) " +
				"cannot be used with Client; configure at NewClient",
		)
	}

	// Apply client-level defaults for fields not explicitly overridden.
	if cfg.timeout == defaultRequestTimeout {
		cfg.timeout = c.timeout
	}
	if cfg.responseBodyLimit == defaultResponseBodyLimit {
		cfg.responseBodyLimit = c.responseBodyLimit
	}
	if cfg.userAgent == "" {
		cfg.userAgent = c.userAgent
	}

	ctx, cancel := cfg.contextWithTimeout()
	if cancel != nil {
		defer cancel()
	}
	cfg.ctx = ctx

	req, err := buildRequest(method, url, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpResponse, err := c.transport.RoundTrip(req)
	if err != nil {
		return nil, fmt.Errorf("failed to perform request: %w", err)
	}
	defer httpResponse.Body.Close()

	bodyBytes, err := readResponseBody(httpResponse.Body, cfg.responseBodyLimit)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	return buildResponse(httpResponse, bodyBytes), nil
}

// Get sends a GET request using the client's transport.
func (c *Client) Get(url string, opts ...Option) (*Response, error) {
	return c.Do(http.MethodGet, url, opts...)
}

// Post sends a POST request using the client's transport.
func (c *Client) Post(url string, opts ...Option) (*Response, error) {
	return c.Do(http.MethodPost, url, opts...)
}

// Put sends a PUT request using the client's transport.
func (c *Client) Put(url string, opts ...Option) (*Response, error) {
	return c.Do(http.MethodPut, url, opts...)
}

// Delete sends a DELETE request using the client's transport.
func (c *Client) Delete(url string, opts ...Option) (*Response, error) {
	return c.Do(http.MethodDelete, url, opts...)
}

// Patch sends a PATCH request using the client's transport.
func (c *Client) Patch(url string, opts ...Option) (*Response, error) {
	return c.Do(http.MethodPatch, url, opts...)
}

// Head sends a HEAD request using the client's transport.
func (c *Client) Head(url string, opts ...Option) (*Response, error) {
	return c.Do(http.MethodHead, url, opts...)
}
