package httpclient

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestGetRequest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}

		cookie, err := r.Cookie("sid")
		if err != nil {
			t.Errorf("expected cookie sid: %v", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		payload := map[string]any{
			"args":      r.URL.Query(),
			"userAgent": r.UserAgent(),
			"cookie":    cookie.Value,
			"header":    r.Header.Values("X-Test"),
		}
		w.Header().Add("X-Server", "httpclient-test")
		_ = json.NewEncoder(w).Encode(payload)
	}))
	defer srv.Close()

	res, err := Get(
		srv.URL,
		AddCookie("sid", "abc"),
		WithUserAgent("HTTPClientTest/1.0"),
		AddQuery("a", "aaa"),
		AddQuery("b", "bbb c"),
		AddHeader("X-Test", "1"),
		AddHeader("X-Test", "2"),
	)
	if err != nil {
		t.Fatal(err)
	}

	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected status code 200, got %d", res.StatusCode)
	}

	var data struct {
		Args      map[string][]string `json:"args"`
		UserAgent string              `json:"userAgent"`
		Cookie    string              `json:"cookie"`
		Header    []string            `json:"header"`
	}
	if err := res.JSON(&data); err != nil {
		t.Fatal(err)
	}

	if got := data.Args["a"]; !reflect.DeepEqual(got, []string{"aaa"}) {
		t.Fatalf("unexpected arg a: %#v", got)
	}
	if got := data.Args["b"]; !reflect.DeepEqual(got, []string{"bbb c"}) {
		t.Fatalf("unexpected arg b: %#v", got)
	}
	if data.UserAgent != "HTTPClientTest/1.0" {
		t.Fatalf("unexpected user agent: %s", data.UserAgent)
	}
	if data.Cookie != "abc" {
		t.Fatalf("unexpected cookie: %s", data.Cookie)
	}
	if !reflect.DeepEqual(data.Header, []string{"1", "2"}) {
		t.Fatalf("unexpected repeated header values: %#v", data.Header)
	}
	if res.Header["X-Server"] != "httpclient-test" {
		t.Fatalf("unexpected flattened header value: %s", res.Header["X-Server"])
	}
	if res.Headers.Get("X-Server") != "httpclient-test" {
		t.Fatalf("unexpected raw header value: %s", res.Headers.Get("X-Server"))
	}
}

func TestPostJSONBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("unexpected content type: %s", got)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("failed to read request body: %v", err)
		}
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	payload := struct {
		Key    string `json:"key"`
		Number int    `json:"number"`
	}{
		Key:    "value",
		Number: 123,
	}

	res, err := Post(srv.URL, WithJSON(payload))
	if err != nil {
		t.Fatal(err)
	}

	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected status code 200, got %d", res.StatusCode)
	}
	if strings.TrimSpace(res.Text()) != `{"key":"value","number":123}` {
		t.Fatalf("unexpected response body: %s", res.Text())
	}
}

func TestPostXMLBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Content-Type"); got != "application/xml" {
			t.Errorf("unexpected content type: %s", got)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("failed to read request body: %v", err)
		}
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	res, err := Post(srv.URL, WithXML(`<data><item>123</item></data>`))
	if err != nil {
		t.Fatal(err)
	}

	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected status code 200, got %d", res.StatusCode)
	}
	if strings.TrimSpace(res.Text()) != `<data><item>123</item></data>` {
		t.Fatalf("unexpected response body: %s", res.Text())
	}
}

func TestPostForm(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Content-Type"); got != "application/x-www-form-urlencoded" {
			t.Errorf("unexpected content type: %s", got)
		}
		if err := r.ParseForm(); err != nil {
			t.Errorf("failed to parse form: %v", err)
		}
		_ = json.NewEncoder(w).Encode(r.Form)
	}))
	defer srv.Close()

	res, err := Post(
		srv.URL,
		AddFormValue("aaa", "first"),
		AddFormValue("aaa", "second"),
		AddFormValue("bbb", "value"),
	)
	if err != nil {
		t.Fatal(err)
	}

	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected status code 200, got %d", res.StatusCode)
	}

	var form map[string][]string
	if err := res.JSON(&form); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(form["aaa"], []string{"first", "second"}) {
		t.Fatalf("unexpected repeated form values: %#v", form["aaa"])
	}
	if !reflect.DeepEqual(form["bbb"], []string{"value"}) {
		t.Fatalf("unexpected form value: %#v", form["bbb"])
	}
}

func TestPostFile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1024 * 1024); err != nil {
			t.Errorf("failed to parse multipart form: %v", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		firstFile, _, err := r.FormFile("file1")
		if err != nil {
			t.Errorf("missing file1: %v", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		defer firstFile.Close()
		firstFileBody, err := io.ReadAll(firstFile)
		if err != nil {
			t.Errorf("failed to read file1: %v", err)
		}

		secondFile, _, err := r.FormFile("file2")
		if err != nil {
			t.Errorf("missing file2: %v", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		defer secondFile.Close()
		secondFileBody, err := io.ReadAll(secondFile)
		if err != nil {
			t.Errorf("failed to read file2: %v", err)
		}

		_ = json.NewEncoder(w).Encode(map[string]any{
			"description": r.FormValue("description"),
			"file1":       string(firstFileBody),
			"file2":       string(secondFileBody),
		})
	}))
	defer srv.Close()

	res, err := Post(
		srv.URL,
		AddMultipartFile("file1", "first.txt", strings.NewReader("first file")),
		AddMultipartFile("file2", "second.txt", strings.NewReader("second file")),
		AddMultipartField("description", "A test upload"),
	)
	if err != nil {
		t.Fatal(err)
	}

	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected status code 200, got %d", res.StatusCode)
	}

	var data struct {
		Description string `json:"description"`
		File1       string `json:"file1"`
		File2       string `json:"file2"`
	}
	if err := res.JSON(&data); err != nil {
		t.Fatal(err)
	}
	if data.Description != "A test upload" {
		t.Fatalf("unexpected description: %s", data.Description)
	}
	if data.File1 != "first file" || data.File2 != "second file" {
		t.Fatalf("unexpected file contents: %#v", data)
	}
}

func TestAddFileData(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1024 * 1024); err != nil {
			t.Errorf("failed to parse multipart form: %v", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		file, header, err := r.FormFile("file")
		if err != nil {
			t.Errorf("missing file: %v", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		defer file.Close()

		body, err := io.ReadAll(file)
		if err != nil {
			t.Errorf("failed to read file: %v", err)
		}

		_ = json.NewEncoder(w).Encode(map[string]any{
			"filename": header.Filename,
			"body":     string(body),
		})
	}))
	defer srv.Close()

	res, err := Post(
		srv.URL,
		AddFileData("file", "data.txt", []byte("from bytes")),
	)
	if err != nil {
		t.Fatal(err)
	}

	var data struct {
		Filename string `json:"filename"`
		Body     string `json:"body"`
	}
	if err := res.JSON(&data); err != nil {
		t.Fatal(err)
	}
	if data.Filename != "data.txt" {
		t.Fatalf("unexpected filename: %s", data.Filename)
	}
	if data.Body != "from bytes" {
		t.Fatalf("unexpected file body: %s", data.Body)
	}
}

func TestDefaultTimeout(t *testing.T) {
	original := defaultRequestTimeout
	defaultRequestTimeout = 20 * time.Millisecond
	defer func() {
		defaultRequestTimeout = original
	}()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	_, err := Get(srv.URL)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}
}

func TestWithTimeoutOverride(t *testing.T) {
	original := defaultRequestTimeout
	defaultRequestTimeout = 20 * time.Millisecond
	defer func() {
		defaultRequestTimeout = original
	}()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	res, err := Get(srv.URL, WithTimeout(200*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("expected status code 204, got %d", res.StatusCode)
	}
}

func TestResponseRedirectAndCookies(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/target" {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "session", Value: "abc"})
		w.Header().Set("Location", "/target")
		w.WriteHeader(http.StatusFound)
	}))
	defer srv.Close()

	res, err := Get(srv.URL + "/redirect")
	if err != nil {
		t.Fatal(err)
	}

	if res.StatusCode != http.StatusFound {
		t.Fatalf("expected status code 302, got %d", res.StatusCode)
	}
	if res.Location != srv.URL+"/target" {
		t.Fatalf("unexpected location: %s", res.Location)
	}
	if res.Cookie["session"] != "abc" {
		t.Fatalf("unexpected cookie map: %#v", res.Cookie)
	}
	if res.Cookies != "session=abc" {
		t.Fatalf("unexpected cookies string: %s", res.Cookies)
	}
	if len(res.CookiesList) != 1 || res.CookiesList[0].Name != "session" {
		t.Fatalf("unexpected cookies list: %#v", res.CookiesList)
	}
}

func TestWithTLSConfig(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("secure"))
	}))
	defer srv.Close()

	_, err := Get(srv.URL, WithTimeout(200*time.Millisecond))
	if err == nil {
		t.Fatal("expected tls verification error")
	}

	baseTransport, ok := srv.Client().Transport.(*http.Transport)
	if !ok {
		t.Fatal("expected *http.Transport from test tls client")
	}

	res, err := Get(srv.URL, WithTLSConfig(baseTransport.TLSClientConfig.Clone()))
	if err != nil {
		t.Fatal(err)
	}
	if res.Text() != "secure" {
		t.Fatalf("unexpected response body: %s", res.Text())
	}
}

func TestWithProxy(t *testing.T) {
	var directHits int

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		directHits++
		w.WriteHeader(http.StatusTeapot)
	}))
	defer target.Close()

	targetURL := target.URL + "/resource?x=1"
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.String(); got != targetURL {
			t.Errorf("unexpected proxied url: %s", got)
		}
		w.Header().Set("X-Proxy", "yes")
		_, _ = w.Write([]byte("proxied"))
	}))
	defer proxy.Close()

	res, err := Get(targetURL, WithProxy(proxy.URL))
	if err != nil {
		t.Fatal(err)
	}

	if directHits != 0 {
		t.Fatalf("expected proxy to handle request, got %d direct hits", directHits)
	}
	if res.Text() != "proxied" {
		t.Fatalf("unexpected response body: %s", res.Text())
	}
	if res.Headers.Get("X-Proxy") != "yes" {
		t.Fatalf("unexpected proxy header: %s", res.Headers.Get("X-Proxy"))
	}
}

func TestWithLocalAddrRejectsInvalidIP(t *testing.T) {
	_, err := Get("http://example.com", WithLocalAddr("not-an-ip"))
	if err == nil {
		t.Fatal("expected invalid local address error")
	}
}

func TestWithContext(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	_, err := Get(srv.URL, WithContext(ctx), WithTimeout(200*time.Millisecond))
	if err == nil {
		t.Fatal("expected context deadline exceeded error")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}
}

func TestSetJSONBodyCompatibility(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("unexpected content type: %s", got)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("failed to read request body: %v", err)
		}
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	res, err := Post(srv.URL, SetJSONBody(`{"legacy":true}`))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(res.Text()) != `{"legacy":true}` {
		t.Fatalf("unexpected response body: %s", res.Text())
	}
}

func TestWithTLSConfigRejectsNil(t *testing.T) {
	_, err := Get("https://example.com", WithTLSConfig((*tls.Config)(nil)))
	if err == nil {
		t.Fatal("expected nil tls config error")
	}
}
