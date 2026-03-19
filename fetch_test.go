package fetch

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

type trackingReadCloser struct {
	readErr error
	closed  bool
}

func (r *trackingReadCloser) Read(_ []byte) (int, error) {
	if r.readErr != nil {
		return 0, r.readErr
	}
	return 0, io.EOF
}

func (r *trackingReadCloser) Close() error {
	r.closed = true
	return nil
}

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
		w.Header().Add("X-Server", "fetch-test")
		_ = json.NewEncoder(w).Encode(payload)
	}))
	defer srv.Close()

	res, err := Get(
		srv.URL,
		AddCookie("sid", "abc"),
		WithUserAgent("FetchTest/1.0"),
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
	if data.UserAgent != "FetchTest/1.0" {
		t.Fatalf("unexpected user agent: %s", data.UserAgent)
	}
	if data.Cookie != "abc" {
		t.Fatalf("unexpected cookie: %s", data.Cookie)
	}
	if !reflect.DeepEqual(data.Header, []string{"1", "2"}) {
		t.Fatalf("unexpected repeated header values: %#v", data.Header)
	}
	if res.Header["X-Server"] != "fetch-test" {
		t.Fatalf("unexpected flattened header value: %s", res.Header["X-Server"])
	}
	if res.Headers.Get("X-Server") != "fetch-test" {
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

func TestWithJSONRejectsExplicitContentTypeHeader(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	_, err := Post(
		srv.URL,
		AddHeader("Content-Type", "application/problem+json"),
		WithJSON(map[string]string{"key": "value"}),
	)
	if !errors.Is(err, ErrContentTypeConflict) {
		t.Fatalf("expected ErrContentTypeConflict, got %v", err)
	}
	if hits != 0 {
		t.Fatalf("expected request to fail before sending, got %d hits", hits)
	}
}

func TestSetJSONBodyRejectsExplicitContentTypeHeader(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	_, err := Post(
		srv.URL,
		AddHeader("Content-Type", "application/problem+json"),
		SetJSONBody(`{"key":"value"}`),
	)
	if !errors.Is(err, ErrContentTypeConflict) {
		t.Fatalf("expected ErrContentTypeConflict, got %v", err)
	}
	if hits != 0 {
		t.Fatalf("expected request to fail before sending, got %d hits", hits)
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

func TestFormBodyRejectsExplicitContentTypeHeader(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	_, err := Post(
		srv.URL,
		AddHeader("Content-Type", "application/json"),
		AddFormValue("aaa", "first"),
	)
	if !errors.Is(err, ErrContentTypeConflict) {
		t.Fatalf("expected ErrContentTypeConflict, got %v", err)
	}
	if hits != 0 {
		t.Fatalf("expected request to fail before sending, got %d hits", hits)
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

func TestMultipartRejectsExplicitContentTypeHeader(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	_, err := Post(
		srv.URL,
		AddHeader("Content-Type", "multipart/form-data"),
		AddFileData("file", "data.txt", []byte("from bytes")),
	)
	if !errors.Is(err, ErrContentTypeConflict) {
		t.Fatalf("expected ErrContentTypeConflict, got %v", err)
	}
	if hits != 0 {
		t.Fatalf("expected request to fail before sending, got %d hits", hits)
	}
}

func TestWithBodyWithoutContentTypeAllowsExplicitHeader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Content-Type"); got != "application/custom" {
			t.Fatalf("unexpected content type: %s", got)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	_, err := Post(
		srv.URL,
		AddHeader("Content-Type", "application/custom"),
		WithBody("", strings.NewReader("payload")),
	)
	if err != nil {
		t.Fatal(err)
	}
}

func TestWithBodyWithoutContentTypeAllowsExplicitHeaderRegardlessOfOptionOrder(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Content-Type"); got != "application/custom" {
			t.Fatalf("unexpected content type: %s", got)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	_, err := Post(
		srv.URL,
		WithBody("", strings.NewReader("payload")),
		AddHeader("Content-Type", "application/custom"),
	)
	if err != nil {
		t.Fatal(err)
	}
}

func TestAddMultipartFileRejectsNilContent(t *testing.T) {
	_, err := Post("http://example.com", AddMultipartFile("file", "a.txt", nil))
	if err == nil {
		t.Fatal("expected nil multipart content error")
	}
}

func TestWriteMultipartFilesClosesRemainingReadersOnFailure(t *testing.T) {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)

	first := &trackingReadCloser{readErr: errors.New("read failed")}
	second := &trackingReadCloser{}

	err := writeMultipartFiles(mw, []multipartFile{
		{fieldname: "file1", filename: "first.txt", content: first},
		{fieldname: "file2", filename: "second.txt", content: second},
	})
	if err == nil {
		t.Fatal("expected multipart write error")
	}
	if !first.closed {
		t.Fatal("expected failed reader to be closed")
	}
	if !second.closed {
		t.Fatal("expected remaining reader to be closed")
	}
}

func TestRequestRejectsBodyForLowercaseGet(t *testing.T) {
	_, err := Request("get", "http://example.com", WithJSON(map[string]string{"a": "b"}))
	if err == nil {
		t.Fatal("expected body validation error")
	}
}

func TestRequestRejectsBodyForMixedCaseHead(t *testing.T) {
	_, err := Request("HeAd", "http://example.com", WithBody("text/plain", strings.NewReader("x")))
	if err == nil {
		t.Fatal("expected body validation error")
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

func TestWithTimeoutZeroDisablesDefaultTimeout(t *testing.T) {
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

	res, err := Get(srv.URL, WithTimeout(0))
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("expected status code 204, got %d", res.StatusCode)
	}
}

func TestResponseJSONReturnsErrEmptyBodyForNoContent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	res, err := Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}

	var dst map[string]any
	err = res.JSON(&dst)
	if !errors.Is(err, ErrEmptyBody) {
		t.Fatalf("expected ErrEmptyBody, got %v", err)
	}
}

func TestDefaultResponseBodyLimit(t *testing.T) {
	original := defaultResponseBodyLimit
	defaultResponseBodyLimit = 8
	defer func() {
		defaultResponseBodyLimit = original
	}()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("123456789"))
	}))
	defer srv.Close()

	_, err := Get(srv.URL)
	if err == nil {
		t.Fatal("expected response body limit error")
	}
}

func TestWithResponseBodyLimitZeroDisablesLimit(t *testing.T) {
	original := defaultResponseBodyLimit
	defaultResponseBodyLimit = 8
	defer func() {
		defaultResponseBodyLimit = original
	}()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("123456789"))
	}))
	defer srv.Close()

	res, err := Get(srv.URL, WithResponseBodyLimit(0))
	if err != nil {
		t.Fatal(err)
	}
	if res.Text() != "123456789" {
		t.Fatalf("unexpected response body: %s", res.Text())
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

func TestResponseHeaderCompatibilityViewIsLossy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Link", "<https://example.com/a>; rel=preload")
		w.Header().Add("Link", "<https://example.com/b>; rel=next")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	res, err := Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}

	if got := res.Headers.Values("Link"); !reflect.DeepEqual(got, []string{"<https://example.com/a>; rel=preload", "<https://example.com/b>; rel=next"}) {
		t.Fatalf("unexpected raw link headers: %#v", got)
	}
	if got := res.Header["Link"]; got != "<https://example.com/a>; rel=preload; <https://example.com/b>; rel=next" {
		t.Fatalf("unexpected flattened link header: %s", got)
	}
}

func TestResponseCookieCompatibilityViewIsLossy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "session", Value: "root", Path: "/"})
		http.SetCookie(w, &http.Cookie{Name: "session", Value: "admin", Path: "/admin"})
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	res, err := Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}

	if len(res.CookiesList) != 2 {
		t.Fatalf("expected 2 cookies, got %#v", res.CookiesList)
	}
	if got := res.Cookie["session"]; got != "admin" {
		t.Fatalf("unexpected compatibility cookie value: %s", got)
	}
	if got := res.Cookies; got != "session=root; session=admin" {
		t.Fatalf("unexpected cookies summary: %s", got)
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

func TestTransportForReusesProxyOnlyOverrides(t *testing.T) {
	resetOverrideTransportCache()
	defer resetOverrideTransportCache()

	cfgA, err := newCallConfig(WithProxy(""))
	if err != nil {
		t.Fatal(err)
	}
	cfgB, err := newCallConfig(WithProxy(""))
	if err != nil {
		t.Fatal(err)
	}

	transportA, _, err := transportFor(cfgA)
	if err != nil {
		t.Fatal(err)
	}
	transportB, _, err := transportFor(cfgB)
	if err != nil {
		t.Fatal(err)
	}

	if transportA != transportB {
		t.Fatal("expected proxy-only overrides to reuse cached transport")
	}
}

func TestTransportForDoesNotCacheNewKeyWhenCacheIsFull(t *testing.T) {
	resetOverrideTransportCache()
	defer resetOverrideTransportCache()

	originalLimit := maxOverrideTransportCacheEntries
	maxOverrideTransportCacheEntries = 1
	defer func() {
		maxOverrideTransportCacheEntries = originalLimit
	}()

	firstCfg, err := newCallConfig(WithProxy(""))
	if err != nil {
		t.Fatal(err)
	}
	firstTransport, cleanup, err := transportFor(firstCfg)
	if err != nil {
		t.Fatal(err)
	}
	if cleanup != nil {
		defer cleanup()
	}

	secondCfg, err := newCallConfig(WithLocalAddr("127.0.0.1"))
	if err != nil {
		t.Fatal(err)
	}
	secondTransport, secondCleanup, err := transportFor(secondCfg)
	if err != nil {
		t.Fatal(err)
	}
	if secondCleanup == nil {
		t.Fatal("expected uncached transport cleanup when cache is full")
	}
	defer secondCleanup()

	secondTransportRepeat, secondCleanupRepeat, err := transportFor(secondCfg)
	if err != nil {
		t.Fatal(err)
	}
	if secondCleanupRepeat == nil {
		t.Fatal("expected uncached transport cleanup on repeated overflow key")
	}
	defer secondCleanupRepeat()

	if firstTransport == secondTransport {
		t.Fatal("expected different transport for different override key")
	}
	if secondTransport == secondTransportRepeat {
		t.Fatal("expected overflow key to bypass cache")
	}
	if got := overrideTransportCacheEntries.Load(); got != 1 {
		t.Fatalf("expected cache entry count to stay at 1, got %d", got)
	}
}

func TestWithTLSConfigUsesUpdatedConfigOnRepeatedCalls(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("secure"))
	}))
	defer srv.Close()

	baseTransport, ok := srv.Client().Transport.(*http.Transport)
	if !ok {
		t.Fatal("expected *http.Transport from test tls client")
	}
	tlsConfig := baseTransport.TLSClientConfig.Clone()

	res, err := Get(srv.URL, WithTLSConfig(tlsConfig), WithTimeout(200*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	if res.Text() != "secure" {
		t.Fatalf("unexpected response body: %s", res.Text())
	}

	tlsConfig.RootCAs = nil

	_, err = Get(srv.URL, WithTLSConfig(tlsConfig), WithTimeout(200*time.Millisecond))
	if err == nil {
		t.Fatal("expected tls verification error after config mutation")
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

func TestWithProxyRejectsSchemalessURL(t *testing.T) {
	_, err := newCallConfig(WithProxy("localhost:8080"))
	if err == nil {
		t.Fatal("expected invalid proxy url error")
	}
}

func TestWithProxyRejectsRelativeURL(t *testing.T) {
	_, err := newCallConfig(WithProxy("/relative"))
	if err == nil {
		t.Fatal("expected invalid proxy url error")
	}
}

func TestWithLocalAddrRejectsInvalidIP(t *testing.T) {
	_, err := Get("http://example.com", WithLocalAddr("not-an-ip"))
	if err == nil {
		t.Fatal("expected invalid local address error")
	}
}

func TestNewLocalDialerMatchesDefaultDialerTimeouts(t *testing.T) {
	dialer, err := newLocalDialer("127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if dialer.Timeout != 30*time.Second {
		t.Fatalf("expected 30s timeout, got %v", dialer.Timeout)
	}
	if dialer.KeepAlive != 30*time.Second {
		t.Fatalf("expected 30s keepalive, got %v", dialer.KeepAlive)
	}
	if dialer.LocalAddr == nil {
		t.Fatal("expected local addr to be set")
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
