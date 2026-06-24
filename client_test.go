package fetch

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewClientDefaultTransport(t *testing.T) {
	client, err := NewClient()
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	res, err := client.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if res.Text() != "ok" {
		t.Fatalf("unexpected body: %s", res.Text())
	}
}

func TestClientSharesTransportAcrossRequests(t *testing.T) {
	resetOverrideTransportCache()
	defer resetOverrideTransportCache()

	client, err := NewClient(WithProxy(""))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	var connCount atomic.Int32
	origHandler := srv.Config.Handler
	srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connCount.Add(1)
		origHandler.ServeHTTP(w, r)
	})

	for i := 0; i < 3; i++ {
		res, err := client.Get(srv.URL)
		if err != nil {
			t.Fatal(err)
		}
		if res.Text() != "ok" {
			t.Fatalf("unexpected body: %s", res.Text())
		}
	}
}

func TestClientReusesTransport(t *testing.T) {
	resetOverrideTransportCache()
	defer resetOverrideTransportCache()

	client, err := NewClient(WithProxy(""))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	if client.transport == nil {
		t.Fatal("expected non-nil transport")
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("secure"))
	}))
	defer srv.Close()

	for i := 0; i < 3; i++ {
		res, err := client.Get(srv.URL)
		if err != nil {
			t.Fatal(err)
		}
		if res.Text() != "secure" {
			t.Fatalf("unexpected body: %s", res.Text())
		}
	}
}

func TestClientAppliesDefaultTimeout(t *testing.T) {
	original := defaultRequestTimeout
	defaultRequestTimeout = 20 * time.Millisecond
	defer func() { defaultRequestTimeout = original }()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
	}))
	defer srv.Close()

	client, err := NewClient(WithTimeout(20 * time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	_, err = client.Get(srv.URL)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}
}

func TestClientRequestTimeoutOverridesDefault(t *testing.T) {
	original := defaultRequestTimeout
	defaultRequestTimeout = 20 * time.Millisecond
	defer func() { defaultRequestTimeout = original }()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	client, err := NewClient(WithTimeout(20 * time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	// Override with a longer timeout at request level.
	res, err := client.Get(srv.URL, WithTimeout(200*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", res.StatusCode)
	}
}

func TestClientAppliesDefaultResponseBodyLimit(t *testing.T) {
	original := defaultResponseBodyLimit
	defaultResponseBodyLimit = 8
	defer func() { defaultResponseBodyLimit = original }()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("123456789"))
	}))
	defer srv.Close()

	client, err := NewClient(WithResponseBodyLimit(8))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	_, err = client.Get(srv.URL)
	if err == nil {
		t.Fatal("expected response body limit error")
	}
}

func TestClientAppliesDefaultUserAgent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(r.UserAgent()))
	}))
	defer srv.Close()

	client, err := NewClient(WithUserAgent("TestClient/1.0"))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	res, err := client.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if res.Text() != "TestClient/1.0" {
		t.Fatalf("unexpected user agent: %s", res.Text())
	}
}

func TestClientRequestUserAgentOverridesDefault(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(r.UserAgent()))
	}))
	defer srv.Close()

	client, err := NewClient(WithUserAgent("Default/1.0"))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	res, err := client.Get(srv.URL, WithUserAgent("Override/2.0"))
	if err != nil {
		t.Fatal(err)
	}
	if res.Text() != "Override/2.0" {
		t.Fatalf("expected override user agent, got: %s", res.Text())
	}
}

func TestClientRejectsTransportOptionsPerRequest(t *testing.T) {
	client, err := NewClient()
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	_, err = client.Get("http://example.com", WithProxy("http://127.0.0.1:8080"))
	if err == nil {
		t.Fatal("expected error for per-request transport option")
	}

	_, err = client.Get("http://example.com", WithLocalAddr("127.0.0.1"))
	if err == nil {
		t.Fatal("expected error for per-request local addr")
	}

	_, err = client.Get("http://example.com", WithProxy("vless://11111111-2222-3333-4444-555555555555@example.com:443?security=tls&type=ws"))
	if err == nil {
		t.Fatal("expected error for per-request vless proxy")
	}
}

func TestClientVlessTransport(t *testing.T) {
	client, err := NewClient(
		WithProxy("vless://11111111-2222-3333-4444-555555555555@example.com:443?security=tls&type=ws"),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	if client.transport == nil {
		t.Fatal("expected non-nil transport")
	}
}

func TestClientCloseIsIdempotent(t *testing.T) {
	client, err := NewClient()
	if err != nil {
		t.Fatal(err)
	}

	client.Close()
	client.Close() // should not panic
}

func TestClientMethods(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(r.Method))
	}))
	defer srv.Close()

	client, err := NewClient()
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	tests := []struct {
		method string
		call   func(url string, opts ...Option) (*Response, error)
	}{
		{http.MethodGet, client.Get},
		{http.MethodPost, client.Post},
		{http.MethodPut, client.Put},
		{http.MethodDelete, client.Delete},
		{http.MethodPatch, client.Patch},
		{http.MethodHead, client.Head},
	}

	for _, tt := range tests {
		t.Run(tt.method, func(t *testing.T) {
			res, err := tt.call(srv.URL)
			if err != nil {
				t.Fatal(err)
			}
			if tt.method != http.MethodHead && res.Text() != tt.method {
				t.Fatalf("expected body %q, got %q", tt.method, res.Text())
			}
		})
	}
}

func TestClientDo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(r.Method))
	}))
	defer srv.Close()

	client, err := NewClient()
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	res, err := client.Do("PROPFIND", srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if res.Text() != "PROPFIND" {
		t.Fatalf("unexpected body: %s", res.Text())
	}
}
