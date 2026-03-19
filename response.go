package fetch

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

// ErrEmptyBody is returned by Response.JSON when the response body is empty.
var ErrEmptyBody = errors.New("response body is empty")

// Response represents an HTTP response.
type Response struct {
	StatusCode int
	Status     string
	Location   string
	// Cookie contains a lossy compatibility view of response cookies keyed by name.
	// If multiple cookies share the same name, the last parsed value wins.
	Cookie map[string]string
	// Cookies contains a lossy compatibility summary string built from parsed
	// response cookies as "name=value" pairs joined by semicolons. It is not the
	// raw Set-Cookie header value and should not be treated as a lossless replay format.
	Cookies string
	// CookiesList contains the parsed response cookies and is the preferred field
	// for new code that needs complete cookie semantics.
	CookiesList []*http.Cookie
	// Header contains a flattened view of response headers for compatibility.
	Header map[string]string
	// Headers contains the raw response headers.
	Headers http.Header
	// Body contains the raw bytes of the response body.
	Body []byte
}

// response is kept as an alias for internal backward compatibility.
type response = Response

func buildResponse(httpResponse *http.Response, body []byte) *Response {
	res := &Response{
		StatusCode: httpResponse.StatusCode,
		Status:     httpResponse.Status,
		Body:       body,
		Header:     make(map[string]string),
		Headers:    httpResponse.Header.Clone(),
		Cookie:     make(map[string]string),
	}

	extractHeaders(res, httpResponse.Header)
	extractLocation(res, httpResponse)
	extractCookies(res, httpResponse.Cookies())

	return res
}

func extractHeaders(res *Response, httpHeader http.Header) {
	for name, values := range httpHeader {
		if strings.EqualFold(name, "Location") || strings.EqualFold(name, "Set-Cookie") {
			continue
		}
		res.Header[name] = strings.Join(values, "; ")
	}
}

func extractLocation(res *Response, httpResponse *http.Response) {
	if u, err := httpResponse.Location(); err == nil {
		res.Location = u.String()
		return
	}
	res.Location = httpResponse.Header.Get("Location")
}

func extractCookies(res *Response, cookies []*http.Cookie) {
	if len(cookies) == 0 {
		return
	}

	res.CookiesList = append([]*http.Cookie(nil), cookies...)
	var cookiePairs []string
	for _, cookie := range cookies {
		res.Cookie[cookie.Name] = cookie.Value
		cookiePairs = append(cookiePairs, cookie.Name+"="+cookie.Value)
	}
	res.Cookies = strings.Join(cookiePairs, "; ")
}

// JSON unmarshals the response body into the given interface.
func (r *Response) JSON(v interface{}) error {
	if len(r.Body) == 0 {
		return ErrEmptyBody
	}
	return json.Unmarshal(r.Body, v)
}

// Text returns the response body as a string.
func (r *Response) Text() string {
	return string(r.Body)
}
