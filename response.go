package httpclient

import (
	"encoding/json"
	"net/http"
	"strings"
)

// Response represents an HTTP response.
type Response struct {
	StatusCode int
	Status     string
	Location   string
	// Cookie contains a map of cookie names to values from the response.
	Cookie map[string]string
	// Cookies contains the raw Cookie string from the response, separated by semicolons.
	Cookies string
	// CookiesList contains the parsed cookies from the response.
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

	for name, values := range httpResponse.Header {
		if !strings.EqualFold(name, "Location") && !strings.EqualFold(name, "Set-Cookie") {
			res.Header[name] = strings.Join(values, "; ")
		}
	}

	if u, err := httpResponse.Location(); err == nil {
		res.Location = u.String()
	} else if location := httpResponse.Header.Get("Location"); location != "" {
		res.Location = location
	}

	cookies := httpResponse.Cookies()
	if len(cookies) > 0 {
		res.CookiesList = append([]*http.Cookie(nil), cookies...)
	}

	var cookiePairs []string
	for _, cookie := range cookies {
		res.Cookie[cookie.Name] = cookie.Value
		cookiePairs = append(cookiePairs, cookie.Name+"="+cookie.Value)
	}
	res.Cookies = strings.Join(cookiePairs, "; ")

	return res
}

// JSON unmarshals the response body into the given interface.
func (r *Response) JSON(v interface{}) error {
	return json.Unmarshal(r.Body, v)
}

// Text returns the response body as a string.
func (r *Response) Text() string {
	return string(r.Body)
}
