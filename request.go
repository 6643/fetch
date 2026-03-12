package httpclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type bodyType int

const (
	bodyTypeNone bodyType = iota
	bodyTypeRaw
	bodyTypeFormURLEncoded
	bodyTypeMultipart
)

type multipartFile struct {
	fieldname string
	filename  string
	content   io.Reader
}

func setBodyType(cfg *callConfig, newType bodyType) error {
	if cfg.bodySetType != bodyTypeNone && cfg.bodySetType != newType {
		return fmt.Errorf("cannot set multiple body types. already set to %v, trying to set %v", cfg.bodySetType, newType)
	}
	cfg.bodySetType = newType
	return nil
}

func buildRequest(method, rawURL string, cfg *callConfig) (*http.Request, error) {
	if (method == http.MethodGet || method == http.MethodHead) && cfg.bodySetType != bodyTypeNone {
		return nil, fmt.Errorf("%s method should not have a request body", method)
	}

	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse URL %q: %w", rawURL, err)
	}
	if len(cfg.query) > 0 {
		currentQuery := parsedURL.Query()
		for key, values := range cfg.query {
			for _, value := range values {
				currentQuery.Add(key, value)
			}
		}
		parsedURL.RawQuery = currentQuery.Encode()
	}

	reqBody, contentType, err := prepareBody(cfg)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(cfg.ctx, method, parsedURL.String(), reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	for key, values := range cfg.headers {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	for _, cookie := range cfg.cookies {
		req.AddCookie(cookie)
	}
	if cfg.userAgent != "" {
		req.Header.Set("User-Agent", cfg.userAgent)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	return req, nil
}

func prepareBody(cfg *callConfig) (io.Reader, string, error) {
	switch cfg.bodySetType {
	case bodyTypeFormURLEncoded:
		return strings.NewReader(cfg.formValues.Encode()), "application/x-www-form-urlencoded", nil
	case bodyTypeMultipart:
		return prepareMultipartBody(cfg)
	case bodyTypeRaw:
		return cfg.body, cfg.contentType, nil
	case bodyTypeNone:
		return nil, "", nil
	default:
		return nil, "", fmt.Errorf("unsupported body type: %d", cfg.bodySetType)
	}
}

func prepareMultipartBody(cfg *callConfig) (io.Reader, string, error) {
	pr, pw := io.Pipe()
	mw := multipart.NewWriter(pw)

	go func() {
		var err error
		defer func() {
			if closeErr := mw.Close(); err == nil && closeErr != nil {
				err = fmt.Errorf("failed to close multipart writer: %w", closeErr)
			}
			pw.CloseWithError(err)
		}()

		for key, values := range cfg.multipartFields {
			for _, value := range values {
				if err = mw.WriteField(key, value); err != nil {
					err = fmt.Errorf("failed to write multipart field %q: %w", key, err)
					return
				}
			}
		}

		for _, file := range cfg.multipartFiles {
			writer, createErr := mw.CreateFormFile(file.fieldname, file.filename)
			if createErr != nil {
				err = fmt.Errorf("failed to create multipart file %q: %w", file.filename, createErr)
				return
			}
			if _, copyErr := io.Copy(writer, file.content); copyErr != nil {
				err = fmt.Errorf("failed to copy multipart file %q: %w", file.filename, copyErr)
				return
			}
			if closer, ok := file.content.(io.Closer); ok {
				if closeErr := closer.Close(); err == nil && closeErr != nil {
					err = fmt.Errorf("failed to close multipart file %q: %w", file.filename, closeErr)
					return
				}
			}
		}
	}()

	return pr, mw.FormDataContentType(), nil
}

// WithContext sets the request context.
func WithContext(ctx context.Context) Option {
	return func(cfg *callConfig) error {
		if ctx == nil {
			return fmt.Errorf("context cannot be nil")
		}
		cfg.ctx = ctx
		return nil
	}
}

// WithTimeout sets the total timeout for the current request.
func WithTimeout(timeout time.Duration) Option {
	return func(cfg *callConfig) error {
		if timeout < 0 {
			return fmt.Errorf("timeout cannot be negative")
		}
		cfg.timeout = timeout
		return nil
	}
}

// WithUserAgent sets the User-Agent request header.
func WithUserAgent(ua string) Option {
	return func(cfg *callConfig) error {
		cfg.userAgent = ua
		return nil
	}
}

// WithBody sets a raw request body with the provided content type.
func WithBody(contentType string, body io.Reader) Option {
	return func(cfg *callConfig) error {
		if err := setBodyType(cfg, bodyTypeRaw); err != nil {
			return err
		}
		cfg.contentType = contentType
		cfg.body = body
		return nil
	}
}

// WithJSON marshals the provided value and uses it as the JSON request body.
func WithJSON(v any) Option {
	return func(cfg *callConfig) error {
		body, err := json.Marshal(v)
		if err != nil {
			return fmt.Errorf("failed to marshal json body: %w", err)
		}
		return WithBody("application/json", bytes.NewReader(body))(cfg)
	}
}

// WithXML sets the request body to raw XML data.
func WithXML(data string) Option {
	return WithBody("application/xml", strings.NewReader(data))
}

// AddHeader adds a request header.
func AddHeader(key, value string) Option {
	return func(cfg *callConfig) error {
		if cfg.headers == nil {
			cfg.headers = make(http.Header)
		}
		cfg.headers.Add(key, value)
		return nil
	}
}

// AddCookie adds a cookie.
func AddCookie(name, value string) Option {
	return func(cfg *callConfig) error {
		cfg.cookies = append(cfg.cookies, &http.Cookie{Name: name, Value: value})
		return nil
	}
}

// AddQuery adds a URL query parameter.
func AddQuery(key, value string) Option {
	return func(cfg *callConfig) error {
		if cfg.query == nil {
			cfg.query = make(url.Values)
		}
		cfg.query.Add(key, value)
		return nil
	}
}

// AddFormValue adds an application/x-www-form-urlencoded field.
func AddFormValue(key, value string) Option {
	return func(cfg *callConfig) error {
		if err := setBodyType(cfg, bodyTypeFormURLEncoded); err != nil {
			return err
		}
		if cfg.formValues == nil {
			cfg.formValues = make(url.Values)
		}
		cfg.formValues.Add(key, value)
		return nil
	}
}

// AddMultipartField adds a multipart field.
func AddMultipartField(key, value string) Option {
	return func(cfg *callConfig) error {
		if err := setBodyType(cfg, bodyTypeMultipart); err != nil {
			return err
		}
		if cfg.multipartFields == nil {
			cfg.multipartFields = make(url.Values)
		}
		cfg.multipartFields.Add(key, value)
		return nil
	}
}

// AddMultipartFile adds a multipart file.
func AddMultipartFile(fieldname, filename string, content io.Reader) Option {
	return func(cfg *callConfig) error {
		if err := setBodyType(cfg, bodyTypeMultipart); err != nil {
			return err
		}
		cfg.multipartFiles = append(cfg.multipartFiles, multipartFile{
			fieldname: fieldname,
			filename:  filename,
			content:   content,
		})
		return nil
	}
}

// AddFileData adds a multipart file from an in-memory byte slice.
func AddFileData(fieldname, filename string, data []byte) Option {
	return AddMultipartFile(fieldname, filename, bytes.NewReader(data))
}

// SetUserAgent is kept for compatibility with older call sites.
func SetUserAgent(ua string) Option {
	return WithUserAgent(ua)
}

// SetBody is kept for compatibility with older call sites.
func SetBody(contentType string, body io.Reader) Option {
	return WithBody(contentType, body)
}

// SetJSONBody is kept for compatibility with older call sites.
func SetJSONBody(data string) Option {
	return WithBody("application/json", strings.NewReader(data))
}

// SetXMLBody is kept for compatibility with older call sites.
func SetXMLBody(data string) Option {
	return WithXML(data)
}

// AddData is kept for compatibility with older call sites.
func AddData(key, value string) Option {
	return AddFormValue(key, value)
}

// AddField is kept for compatibility with older call sites.
func AddField(key, value string) Option {
	return AddMultipartField(key, value)
}

// AddFile is kept for compatibility with older call sites.
func AddFile(fieldname, filename string, content io.Reader) Option {
	return AddMultipartFile(fieldname, filename, content)
}

// AddUrlArg is kept for compatibility with older call sites.
func AddUrlArg(key, value string) Option {
	return AddQuery(key, value)
}
