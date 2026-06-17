package fetch

import (
	"encoding/binary"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestParseECHQuerySpec(t *testing.T) {
	spec, ok, err := parseECHQuerySpec("cloudflare-ech.com+https://dns.google/dns-query")
	if err != nil {
		t.Fatalf("parseECHQuerySpec returned error: %v", err)
	}
	if !ok {
		t.Fatal("expected ECH spec")
	}
	if spec.PublicName != "cloudflare-ech.com" {
		t.Fatalf("PublicName = %q", spec.PublicName)
	}
	if spec.DoHURL != "https://dns.google/dns-query" {
		t.Fatalf("DoHURL = %q", spec.DoHURL)
	}
}

func TestParseECHQuerySpecRejectsInvalidValues(t *testing.T) {
	tests := []string{
		"cloudflare-ech.com",
		"+https://dns.google/dns-query",
		"cloudflare-ech.com+http://dns.google/dns-query",
		"cloudflare-ech.com+https://",
	}

	for _, value := range tests {
		t.Run(value, func(t *testing.T) {
			_, _, err := parseECHQuerySpec(value)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), "invalid ECH query") {
				t.Fatalf("error = %q", err.Error())
			}
		})
	}
}

func TestExtractECHConfigListFromDNSMessage(t *testing.T) {
	want := []byte{0xfe, 0x0d, 0x00, 0x04, 0xaa, 0xbb, 0xcc, 0xdd}
	msg := buildHTTPSResponseWithECH(t, "cloudflare-ech.com", want)

	got, err := extractECHConfigListFromDNSMessage(msg)
	if err != nil {
		t.Fatalf("extractECHConfigListFromDNSMessage returned error: %v", err)
	}
	if string(got.ConfigList) != string(want) {
		t.Fatalf("ECH config = %x, want %x", got.ConfigList, want)
	}
	if got.TTL != 60*time.Second {
		t.Fatalf("ECH TTL = %s, want %s", got.TTL, 60*time.Second)
	}
}

func TestExtractECHConfigListFromDNSMessageRequiresECHParam(t *testing.T) {
	msg := buildHTTPSResponseWithECH(t, "cloudflare-ech.com", nil)

	_, err := extractECHConfigListFromDNSMessage(msg)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "missing ECH config") {
		t.Fatalf("error = %q", err.Error())
	}
}

func TestVlessDialerCachesECHConfigPerClient(t *testing.T) {
	var queries atomic.Int32
	echConfigList, err := hex.DecodeString("0041fe0d003d0100200020204bed0a11fc0dde595a9b78d966b0011128eb83f65d3c91c1cc5ac786cd246f000400010001ff0e6578616d706c652e676f6c616e670000")
	if err != nil {
		t.Fatal(err)
	}
	doh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		queries.Add(1)
		w.Header().Set("Content-Type", "application/dns-message")
		_, _ = w.Write(buildHTTPSResponseWithECH(t, "cloudflare-ech.com", echConfigList))
	}))
	defer doh.Close()

	dialer := &vlessDialer{
		cfg: vlessConfig{
			UUIDBytes:     mustUUIDBytes(t, testVlessUUID),
			ServerHost:    "127.0.0.1",
			ServerPort:    "1",
			TLSServerName: "example.golang",
			WebSocketHost: "example.golang",
			WebSocketPath: "/ws",
			ECHSpec: &echQuerySpec{
				PublicName: "cloudflare-ech.com",
				DoHURL:     doh.URL,
			},
		},
	}

	for range 2 {
		conn, err := dialer.DialContext(t.Context(), "tcp", "cache.example:80")
		if err == nil {
			conn.Close()
			t.Fatal("expected DialContext error")
		}
	}

	if got := queries.Load(); got != 1 {
		t.Fatalf("ECH queries = %d, want 1", got)
	}
}

func TestResolveECHConfigListRefreshesAfterTTLExpiry(t *testing.T) {
	var queries atomic.Int32
	firstECH := []byte{0xaa}
	secondECH := []byte{0xbb}
	doh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch queries.Add(1) {
		case 1:
			w.Header().Set("Content-Type", "application/dns-message")
			_, _ = w.Write(buildHTTPSResponseWithECHTTL(t, "cloudflare-ech.com", 0, firstECH))
		default:
			w.Header().Set("Content-Type", "application/dns-message")
			_, _ = w.Write(buildHTTPSResponseWithECHTTL(t, "cloudflare-ech.com", 60, secondECH))
		}
	}))
	defer doh.Close()

	dialer := &vlessDialer{
		cfg: vlessConfig{
			ECHSpec: &echQuerySpec{
				PublicName: "cloudflare-ech.com",
				DoHURL:     doh.URL,
			},
		},
	}

	first, err := dialer.resolveECHConfigList(t.Context())
	if err != nil {
		t.Fatalf("first resolveECHConfigList returned error: %v", err)
	}
	second, err := dialer.resolveECHConfigList(t.Context())
	if err != nil {
		t.Fatalf("second resolveECHConfigList returned error: %v", err)
	}

	if string(first) != string(firstECH) {
		t.Fatalf("first ECH config = %x, want %x", first, firstECH)
	}
	if string(second) != string(secondECH) {
		t.Fatalf("second ECH config = %x, want %x", second, secondECH)
	}
	if got := queries.Load(); got != 2 {
		t.Fatalf("ECH queries = %d, want 2", got)
	}
}

func buildHTTPSResponseWithECH(t *testing.T, name string, ech []byte) []byte {
	return buildHTTPSResponseWithECHTTL(t, name, 60, ech)
}

func buildHTTPSResponseWithECHTTL(t *testing.T, name string, ttl uint32, ech []byte) []byte {
	t.Helper()

	msg := make([]byte, 12)
	binary.BigEndian.PutUint16(msg[0:2], 0x1234)
	binary.BigEndian.PutUint16(msg[2:4], 0x8180)
	binary.BigEndian.PutUint16(msg[4:6], 1)
	binary.BigEndian.PutUint16(msg[6:8], 1)

	var err error
	msg, err = appendDNSName(msg, name)
	if err != nil {
		t.Fatal(err)
	}
	msg = binary.BigEndian.AppendUint16(msg, dnsTypeHTTPS)
	msg = binary.BigEndian.AppendUint16(msg, dnsClassIN)

	msg = append(msg, 0xc0, 0x0c)
	msg = binary.BigEndian.AppendUint16(msg, dnsTypeHTTPS)
	msg = binary.BigEndian.AppendUint16(msg, dnsClassIN)
	msg = binary.BigEndian.AppendUint32(msg, ttl)
	rdLengthAt := len(msg)
	msg = append(msg, 0x00, 0x00)
	rdStart := len(msg)

	msg = binary.BigEndian.AppendUint16(msg, 1)
	msg = append(msg, 0x00)
	if len(ech) > 0 {
		msg = binary.BigEndian.AppendUint16(msg, dnsSVCParamECH)
		msg = binary.BigEndian.AppendUint16(msg, uint16(len(ech)))
		msg = append(msg, ech...)
	}

	binary.BigEndian.PutUint16(msg[rdLengthAt:rdLengthAt+2], uint16(len(msg)-rdStart))
	return msg
}
