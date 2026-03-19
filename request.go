package fetch

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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

// ErrContentTypeConflict is returned when a manual Content-Type header conflicts
// with a body option that defines its own Content-Type.
var ErrContentTypeConflict = errors.New("content-type header conflicts with body option")

func setBodyType(cfg *callConfig, newType bodyType) error {
	if cfg.bodySetType != bodyTypeNone && cfg.bodySetType != newType {
		return fmt.Errorf("cannot set multiple body types. already set to %v, trying to set %v", cfg.bodySetType, newType)
	}
	cfg.bodySetType = newType
	return nil
}

func buildRequest(method, rawURL string, cfg *callConfig) (*http.Request, error) {
	if err := validateMethod(method, cfg.bodySetType); err != nil {
		return nil, err
	}

	parsedURL, err := finalizeURL(rawURL, cfg.query)
	if err != nil {
		return nil, err
	}

	if err := validateConfiguredContentTypeConflict(cfg); err != nil {
		return nil, err
	}

	reqBody, contentType, err := prepareBody(cfg)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(cfg.ctx, method, parsedURL.String(), reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	applyMetadata(req, cfg, contentType)
	return req, nil
}

func validateMethod(method string, bType bodyType) error {
	method = strings.ToUpper(method)
	if (method == http.MethodGet || method == http.MethodHead) && bType != bodyTypeNone {
		return fmt.Errorf("%s method should not have a request body", method)
	}
	return nil
}

func finalizeURL(rawURL string, query url.Values) (*url.URL, error) {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse URL %q: %w", rawURL, err)
	}

	if len(query) == 0 {
		return parsedURL, nil
	}

	currentQuery := parsedURL.Query()
	for key, values := range query {
		for _, value := range values {
			currentQuery.Add(key, value)
		}
	}
	parsedURL.RawQuery = currentQuery.Encode()
	return parsedURL, nil
}

func applyMetadata(req *http.Request, cfg *callConfig, contentType string) {
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
}

func validateConfiguredContentTypeConflict(cfg *callConfig) error {
	contentType := expectedContentType(cfg)
	if contentType == "" {
		return nil
	}
	if len(cfg.headers.Values("Content-Type")) == 0 {
		return nil
	}
	return fmt.Errorf("%w; use body options to set Content-Type", ErrContentTypeConflict)
}

func expectedContentType(cfg *callConfig) string {
	switch cfg.bodySetType {
	case bodyTypeFormURLEncoded:
		return "application/x-www-form-urlencoded"
	case bodyTypeMultipart:
		return "multipart/form-data"
	case bodyTypeRaw:
		return cfg.contentType
	default:
		return ""
	}
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
		err := writeMultipartContent(mw, cfg)
		if closeErr := mw.Close(); err == nil && closeErr != nil {
			err = fmt.Errorf("failed to close multipart writer: %w", closeErr)
		}
		pw.CloseWithError(err)
	}()

	return pr, mw.FormDataContentType(), nil
}

func writeMultipartContent(mw *multipart.Writer, cfg *callConfig) error {
	if err := writeMultipartFields(mw, cfg.multipartFields); err != nil {
		if closeErr := closeMultipartFiles(cfg.multipartFiles); closeErr != nil {
			return closeErr
		}
		return err
	}
	return writeMultipartFiles(mw, cfg.multipartFiles)
}

func writeMultipartFields(mw *multipart.Writer, fields url.Values) error {
	for key, values := range fields {
		for _, value := range values {
			if err := mw.WriteField(key, value); err != nil {
				return fmt.Errorf("failed to write multipart field %q: %w", key, err)
			}
		}
	}
	return nil
}

func writeMultipartFiles(mw *multipart.Writer, files []multipartFile) error {
	for i, file := range files {
		if err := writeOneFile(mw, file); err != nil {
			if closeErr := closeMultipartFiles(files[i+1:]); closeErr != nil {
				return closeErr
			}
			return err
		}
	}
	return nil
}

func closeMultipartFiles(files []multipartFile) error {
	for _, file := range files {
		if err := closeMultipartFile(file.content, file.filename); err != nil {
			return err
		}
	}
	return nil
}

func writeOneFile(mw *multipart.Writer, file multipartFile) error {
	if file.content == nil {
		return fmt.Errorf("multipart file %q content cannot be nil", file.filename)
	}

	writer, err := mw.CreateFormFile(file.fieldname, file.filename)
	if err != nil {
		if closeErr := closeMultipartFile(file.content, file.filename); closeErr != nil {
			return closeErr
		}
		return fmt.Errorf("failed to create multipart file %q: %w", file.filename, err)
	}

	if _, err := io.Copy(writer, file.content); err != nil {
		if closeErr := closeMultipartFile(file.content, file.filename); closeErr != nil {
			return closeErr
		}
		return fmt.Errorf("failed to copy multipart file %q: %w", file.filename, err)
	}

	return closeMultipartFile(file.content, file.filename)
}

func closeMultipartFile(content io.Reader, filename string) error {
	closer, ok := content.(io.Closer)
	if !ok {
		return nil
	}

	if err := closer.Close(); err != nil {
		return fmt.Errorf("failed to close multipart file %q: %w", filename, err)
	}
	return nil
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
		if content == nil {
			return fmt.Errorf("multipart file %q content cannot be nil", filename)
		}
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
