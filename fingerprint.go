package fetch

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"strings"
	"time"

	utls "github.com/refraction-networking/utls"
)

const (
	FingerprintChrome     = "chrome"
	FingerprintFirefox    = "firefox"
	FingerprintSafari     = "safari"
	FingerprintEdge       = "edge"
	FingerprintIOS        = "ios"
	FingerprintAndroid    = "android"
	FingerprintRandom     = "random"
	FingerprintRandomized = "randomized"
	FingerprintGolang     = "golang"
	FingerprintCustom     = "custom"
)

func resolveFingerprint(name string) (utls.ClientHelloID, error) {
	switch strings.ToLower(name) {
	case FingerprintChrome:
		return utls.HelloChrome_Auto, nil
	case FingerprintFirefox:
		return utls.HelloFirefox_Auto, nil
	case FingerprintSafari:
		return utls.HelloSafari_Auto, nil
	case FingerprintEdge:
		return utls.HelloEdge_Auto, nil
	case FingerprintIOS:
		return utls.HelloIOS_Auto, nil
	case FingerprintAndroid:
		return utls.HelloAndroid_11_OkHttp, nil
	case FingerprintRandom, FingerprintRandomized:
		return utls.HelloRandomized, nil
	case FingerprintGolang:
		return utls.HelloGolang, nil
	case FingerprintCustom:
		return utls.HelloCustom, nil
	default:
		return utls.ClientHelloID{}, fmt.Errorf("unknown TLS fingerprint %q", name)
	}
}

func newUTLSDialContext(fingerprint string, tlsCfg *tls.Config, localAddr string) (func(ctx context.Context, network, addr string) (net.Conn, error), error) {
	helloID, err := resolveFingerprint(fingerprint)
	if err != nil {
		return nil, err
	}

	tcpDialer := &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
	}
	if localAddr != "" {
		ip := net.ParseIP(localAddr)
		if ip == nil {
			return nil, fmt.Errorf("invalid local address %q", localAddr)
		}
		tcpDialer.LocalAddr = &net.TCPAddr{IP: ip}
	}

	uConfig := toUTLSConfig(tlsCfg)

	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, _, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}

		tcpConn, err := tcpDialer.DialContext(ctx, network, addr)
		if err != nil {
			return nil, err
		}

		connCfg := uConfig.Clone()
		if connCfg.ServerName == "" {
			connCfg.ServerName = host
		}

		tlsConn := utls.UClient(tcpConn, connCfg, helloID)
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			tcpConn.Close()
			return nil, err
		}

		return tlsConn, nil
	}, nil
}

func toUTLSConfig(cfg *tls.Config) *utls.Config {
	if cfg == nil {
		return &utls.Config{}
	}

	var certs []utls.Certificate
	if cfg.Certificates != nil {
		certs = make([]utls.Certificate, len(cfg.Certificates))
		for i, c := range cfg.Certificates {
			sigSchemes := make([]utls.SignatureScheme, len(c.SupportedSignatureAlgorithms))
			for j, s := range c.SupportedSignatureAlgorithms {
				sigSchemes[j] = utls.SignatureScheme(s)
			}
			certs[i] = utls.Certificate{
				Certificate:                  c.Certificate,
				PrivateKey:                   c.PrivateKey,
				SupportedSignatureAlgorithms: sigSchemes,
				OCSPStaple:                   c.OCSPStaple,
				SignedCertificateTimestamps:  c.SignedCertificateTimestamps,
				Leaf:                         c.Leaf,
			}
		}
	}

	var curves []utls.CurveID
	if cfg.CurvePreferences != nil {
		curves = make([]utls.CurveID, len(cfg.CurvePreferences))
		for i, c := range cfg.CurvePreferences {
			curves[i] = utls.CurveID(c)
		}
	}

	return &utls.Config{
		InsecureSkipVerify:             cfg.InsecureSkipVerify,
		RootCAs:                        cfg.RootCAs,
		ClientCAs:                      cfg.ClientCAs,
		Certificates:                   certs,
		Time:                           cfg.Time,
		ServerName:                     cfg.ServerName,
		ClientAuth:                     utls.ClientAuthType(cfg.ClientAuth),
		MinVersion:                     cfg.MinVersion,
		MaxVersion:                     cfg.MaxVersion,
		CipherSuites:                   cfg.CipherSuites,
		CurvePreferences:               curves,
		DynamicRecordSizingDisabled:    cfg.DynamicRecordSizingDisabled,
		KeyLogWriter:                   cfg.KeyLogWriter,
		SessionTicketsDisabled:         cfg.SessionTicketsDisabled,
		NextProtos:                     append([]string(nil), cfg.NextProtos...),
		VerifyPeerCertificate:          cfg.VerifyPeerCertificate,
		EncryptedClientHelloConfigList: append([]byte(nil), cfg.EncryptedClientHelloConfigList...),
	}
}
