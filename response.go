package fetch

import (
	"encoding/json"
	"errors"
	"net/http"
)

// ErrEmptyBody is returned by Response.JSON when the response body is empty.
var ErrEmptyBody = errors.New("response body is empty")

// Response represents an HTTP response.
type Response struct {
	StatusCode int
	Status     string
	Location   string
	// CookiesList contains the parsed response cookies.
	CookiesList []*http.Cookie
	// Headers contains the raw response headers.
	Headers http.Header
	// Body contains the raw bytes of the response body.
	Body []byte
}

func buildResponse(httpResponse *http.Response, body []byte) *Response {
	res := &Response{
		StatusCode: httpResponse.StatusCode,
		Status:     httpResponse.Status,
		Body:       body,
		Headers:    httpResponse.Header.Clone(),
	}

	extractLocation(res, httpResponse)
	extractCookies(res, httpResponse.Cookies())

	return res
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
