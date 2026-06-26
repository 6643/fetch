package tlsfingerprint

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"reflect"
	"testing"
	"time"
)

func TestResolveFingerprintAcceptsValidNames(t *testing.T) {
	names := []string{
		"chrome", "firefox", "safari", "edge",
		"ios", "android", "random", "randomized",
		"golang", "custom",
	}
	for _, name := range names {
		_, err := ResolveFingerprint(name)
		if err != nil {
			t.Fatalf("expected valid fingerprint %q, got error: %v", name, err)
		}
	}
}

func TestResolveFingerprintRejectsInvalidName(t *testing.T) {
	_, err := ResolveFingerprint("nonexistent-browser")
	if err == nil {
		t.Fatal("expected error for invalid fingerprint name")
	}
}

func TestResolveFingerprintIsCaseInsensitive(t *testing.T) {
	id1, err := ResolveFingerprint("Chrome")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	id2, err := ResolveFingerprint("chrome")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id1 != id2 {
		t.Fatal("expected case-insensitive fingerprint resolution")
	}
}

func TestToUTLSConfigPreservesSecurityCallbacksAndProtocolFields(t *testing.T) {
	roots := x509.NewCertPool()
	verifyPeer := func(_ [][]byte, _ [][]*x509.Certificate) error { return nil }

	cfg := ToUTLSConfig(&tls.Config{
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

func TestToUTLSConfigReturnsEmptyConfigForNil(t *testing.T) {
	cfg := ToUTLSConfig(nil)
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	if len(cfg.Certificates) != 0 {
		t.Fatal("expected empty certificates")
	}
}

func TestToUTLSConfigConvertsCertificates(t *testing.T) {
	cert, _ := testCertificateForName(t, "test.example")
	tlsCfg := &tls.Config{
		Certificates: []tls.Certificate{cert},
	}

	utlsCfg := ToUTLSConfig(tlsCfg)
	if len(utlsCfg.Certificates) != 1 {
		t.Fatalf("expected 1 certificate, got %d", len(utlsCfg.Certificates))
	}
	if !reflect.DeepEqual(utlsCfg.Certificates[0].Certificate, cert.Certificate) {
		t.Fatal("expected certificate data to be preserved")
	}
}

func TestNewTransportWithValidFingerprint(t *testing.T) {
	tr, err := NewTransport("chrome", nil, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tr == nil {
		t.Fatal("expected non-nil transport")
	}
}

func TestNewTransportWithInvalidLocalAddr(t *testing.T) {
	_, err := NewTransport("chrome", nil, "not-an-ip")
	if err == nil {
		t.Fatal("expected error for invalid local address")
	}
}

func TestNewTransportWithLocalAddr(t *testing.T) {
	tr, err := NewTransport("chrome", nil, "127.0.0.1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tr == nil {
		t.Fatal("expected non-nil transport")
	}
}

func TestTransportCloseIdleConnections(t *testing.T) {
	tr, err := NewTransport("chrome", nil, "")
	if err != nil {
		t.Fatal(err)
	}

	// CloseIdleConnections should not panic or error on a fresh transport.
	tr.CloseIdleConnections()
}

func TestTransportWithTLSConfig(t *testing.T) {
	tr, err := NewTransport("firefox", &tls.Config{ServerName: "example.com"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if tr.fingerprint != "firefox" {
		t.Fatalf("expected fingerprint firefox, got %q", tr.fingerprint)
	}
	if tr.tlsConfig == nil || tr.tlsConfig.ServerName != "example.com" {
		t.Fatal("expected tlsConfig to be preserved")
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
