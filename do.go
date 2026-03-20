package fetch

import (
	"fmt"
	"io"
	"net/http"
)

// Do sends a request with the provided method.
func Do(method, url string, opts ...Option) (*Response, error) {
	cfg, err := newCallConfig(opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to apply options: %w", err)
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

	transport, cleanup, err := transportFor(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to configure transport: %w", err)
	}
	if cleanup != nil {
		defer cleanup()
	}

	httpResponse, err := transport.RoundTrip(req)
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

func readResponseBody(body io.Reader, limit int64) ([]byte, error) {
	if limit <= 0 {
		return io.ReadAll(body)
	}

	bodyBytes, err := io.ReadAll(io.LimitReader(body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(bodyBytes)) > limit {
		if err := discardResponseBody(body); err != nil {
			return nil, fmt.Errorf("response body exceeds limit of %d bytes and failed to discard the remainder: %w", limit, err)
		}
		return nil, fmt.Errorf("response body exceeds limit of %d bytes", limit)
	}
	return bodyBytes, nil
}

func discardResponseBody(body io.Reader) error {
	_, err := io.Copy(io.Discard, body)
	return err
}

// Get sends a GET request.
func Get(url string, opts ...Option) (*Response, error) {
	return Do(http.MethodGet, url, opts...)
}

// Post sends a POST request.
func Post(url string, opts ...Option) (*Response, error) {
	return Do(http.MethodPost, url, opts...)
}

// Put sends a PUT request.
func Put(url string, opts ...Option) (*Response, error) {
	return Do(http.MethodPut, url, opts...)
}

// Delete sends a DELETE request.
func Delete(url string, opts ...Option) (*Response, error) {
	return Do(http.MethodDelete, url, opts...)
}

// Patch sends a PATCH request.
func Patch(url string, opts ...Option) (*Response, error) {
	return Do(http.MethodPatch, url, opts...)
}

// Head sends a HEAD request.
func Head(url string, opts ...Option) (*Response, error) {
	return Do(http.MethodHead, url, opts...)
}
