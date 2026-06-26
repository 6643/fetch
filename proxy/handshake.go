package proxy

import (
	"context"

	utls "github.com/refraction-networking/utls"
)

// handshakeUTLSWebsocket performs a uTLS handshake configured for WebSocket
// usage. WebSocket upgrade requires HTTP/1.1 framing; always pin ALPN to
// avoid h2 negotiation which breaks the HTTP/1.1 upgrade request.
func handshakeUTLSWebsocket(ctx context.Context, conn *utls.UConn) error {
	if err := conn.BuildHandshakeState(); err != nil {
		return err
	}
	hasALPN := false
	for _, extension := range conn.Extensions {
		alpn, ok := extension.(*utls.ALPNExtension)
		if !ok {
			continue
		}
		hasALPN = true
		alpn.AlpnProtocols = []string{"http/1.1"}
		break
	}
	if !hasALPN {
		conn.Extensions = append(conn.Extensions, &utls.ALPNExtension{AlpnProtocols: []string{"http/1.1"}})
	}
	if err := conn.BuildHandshakeState(); err != nil {
		return err
	}
	return conn.HandshakeContext(ctx)
}
