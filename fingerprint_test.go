package fetch

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/6643/fetch/fingerprint"
)

func TestWithFingerprintAcceptsValidNames(t *testing.T) {
	names := []string{
		"chrome", "firefox", "safari", "edge",
		"ios", "android", "random", "randomized",
		"golang", "custom",
	}
	for _, name := range names {
		_, err := newCallConfig(WithFingerprint(name))
		if err != nil {
			t.Fatalf("expected valid fingerprint %q, got error: %v", name, err)
		}
	}
}

func TestWithFingerprintRejectsInvalidName(t *testing.T) {
	_, err := newCallConfig(WithFingerprint("nonexistent-browser"))
	if err == nil {
		t.Fatal("expected error for invalid fingerprint name")
	}
}

func TestWithFingerprintEmptyDisablesOverride(t *testing.T) {
	cfg, err := newCallConfig(
		WithFingerprint("chrome"),
		WithFingerprint(""),
	)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.fingerprint != "" {
		t.Fatalf("expected empty fingerprint after reset, got %q", cfg.fingerprint)
	}
}

func TestWithFingerprintSetsTransportOverride(t *testing.T) {
	cfg, err := newCallConfig(WithFingerprint("chrome"))
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.hasTransportOverrides() {
		t.Fatal("expected hasTransportOverrides to be true with fingerprint set")
	}
}

func TestWithFingerprintCreatesTransport(t *testing.T) {
	cfg, err := newCallConfig(WithFingerprint("chrome"))
	if err != nil {
		t.Fatal(err)
	}
	rt, cleanup, err := transportFor(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if cleanup != nil {
		defer cleanup()
	}
	if _, ok := rt.(*fingerprint.Transport); !ok {
		t.Fatalf("expected *fingerprint.Transport from transportFor, got %T", rt)
	}
}

func TestFingerprintTransportCache(t *testing.T) {
	resetOverrideTransportCache()
	defer resetOverrideTransportCache()

	cfgA, err := newCallConfig(WithFingerprint("chrome"))
	if err != nil {
		t.Fatal(err)
	}
	cfgB, err := newCallConfig(WithFingerprint("chrome"))
	if err != nil {
		t.Fatal(err)
	}
	cfgC, err := newCallConfig(WithFingerprint("firefox"))
	if err != nil {
		t.Fatal(err)
	}

	transportA, cleanupA, err := transportFor(cfgA)
	if err != nil {
		t.Fatal(err)
	}
	if cleanupA != nil {
		defer cleanupA()
	}
	transportB, cleanupB, err := transportFor(cfgB)
	if err != nil {
		t.Fatal(err)
	}
	if cleanupB != nil {
		defer cleanupB()
	}
	transportC, cleanupC, err := transportFor(cfgC)
	if err != nil {
		t.Fatal(err)
	}
	if cleanupC != nil {
		defer cleanupC()
	}

	fpA, ok := transportA.(*fingerprint.Transport)
	_, okB := transportB.(*fingerprint.Transport)
	_, okC := transportC.(*fingerprint.Transport)
	if !ok || !okB || !okC {
		t.Fatal("expected *fingerprint.Transport")
	}
	if fpA.Fingerprint() != "chrome" {
		t.Fatal("expected chrome fingerprint")
	}
	if fpA == transportB {
		t.Fatal("expected new transport instance per call")
	}
}

func TestWithFingerprintRejectsInvalidNameAtOptionTime(t *testing.T) {
	_, err := newCallConfig(WithFingerprint(""))
	if err != nil {
		t.Fatal("empty fingerprint should not error at option time")
	}
	_, err = newCallConfig(WithFingerprint("invalid!"))
	if err == nil {
		t.Fatal("expected error for invalid fingerprint name at option time")
	}
}

func TestUTLSConfigPreservesSecurityCallbacksAndProtocolFields(t *testing.T) {
	roots := x509.NewCertPool()
	verifyPeer := func(_ [][]byte, _ [][]*x509.Certificate) error { return nil }

	cfg := fingerprint.ToUTLSConfig(&tls.Config{
		RootCAs:               roots,
		ServerName:            "front.example",
		NextProtos:            []string{"h2", "http/1.1"},
		VerifyPeerCertificate: verifyPeer,
	})

	if cfg.RootCAs != roots {
		t.Fatal("expected RootCAs to be preserved")
	}
	if cfg.ServerName != "front.example" {
		t.Fatalf("expected ServerName to be preserved, got %q", cfg.ServerName)
	}
	if !reflect.DeepEqual(cfg.NextProtos, []string{"h2", "http/1.1"}) {
		t.Fatalf("unexpected NextProtos: %#v", cfg.NextProtos)
	}
	if cfg.VerifyPeerCertificate == nil {
		t.Fatal("expected VerifyPeerCertificate to be preserved")
	}
}

func TestFingerprintPreservesExplicitTLSServerName(t *testing.T) {
	resetOverrideTransportCache()
	defer resetOverrideTransportCache()

	cert, roots := testCertificateForName(t, "front.example")
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(r.TLS.ServerName))
	}))
	srv.TLS = &tls.Config{Certificates: []tls.Certificate{cert}}
	srv.StartTLS()
	defer srv.Close()

	res, err := Get(
		srv.URL,
		WithFingerprint("chrome"),
		WithTLSConfig(&tls.Config{RootCAs: roots, ServerName: "front.example"}),
		WithTimeout(10*time.Second),
	)
	if err != nil {
		t.Fatal(err)
	}
	if res.Text() != "front.example" {
		t.Fatalf("expected explicit ServerName to be sent, got %q", res.Text())
	}
}

func TestFingerprintWithTLSConfigUsesDistinctTransportPerConfig(t *testing.T) {
	resetOverrideTransportCache()
	defer resetOverrideTransportCache()

	cfgA, err := newCallConfig(WithFingerprint("chrome"), WithTLSConfig(&tls.Config{ServerName: "a.example"}))
	if err != nil {
		t.Fatal(err)
	}
	cfgB, err := newCallConfig(WithFingerprint("chrome"), WithTLSConfig(&tls.Config{ServerName: "b.example"}))
	if err != nil {
		t.Fatal(err)
	}

	transportA, cleanupA, err := transportFor(cfgA)
	if err != nil {
		t.Fatal(err)
	}
	if cleanupA != nil {
		defer cleanupA()
	}
	transportB, cleanupB, err := transportFor(cfgB)
	if err != nil {
		t.Fatal(err)
	}
	if cleanupB != nil {
		defer cleanupB()
	}
	if transportA == transportB {
		t.Fatal("expected different transports for different TLS configs with fingerprint")
	}
}

func TestFingerprintWithTLSServer(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("fingerprinted"))
	}))
	defer srv.Close()

	tlsCfg := srv.Client().Transport.(*http.Transport).TLSClientConfig.Clone()

	res, err := Get(
		srv.URL,
		WithFingerprint("chrome"),
		WithTLSConfig(tlsCfg),
		WithTimeout(10*time.Second),
	)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.StatusCode)
	}
	if res.Text() != "fingerprinted" {
		t.Fatalf("unexpected body: %s", res.Text())
	}
}

func TestFingerprintInCacheKeyPreventsCredentialLeak(t *testing.T) {
	cfg, err := newCallConfig(WithFingerprint("chrome"))
	if err != nil {
		t.Fatal(err)
	}
	key := newTransportCacheKey(cfg)
	if key.fingerprint != "chrome" {
		t.Fatalf("expected fingerprint in cache key, got %q", key.fingerprint)
	}
}

func testCertificateForName(t *testing.T, dnsName string) (tls.Certificate, *x509.CertPool) {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: dnsName},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		DNSNames:              []string{dnsName},
	}
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER := x509.MarshalPKCS1PrivateKey(key)
	cert, err := tls.X509KeyPair(
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER}),
		pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: keyDER}),
	)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})) {
		t.Fatal("failed to append test certificate")
	}
	return cert, roots
}
