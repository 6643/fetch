package httpproxy

import (
	"context"
	"net"
)

// DialContext creates a dial function that tunnels TCP connections
// through a VLESS WebSocket proxy. Use this with SOCKS5 or HTTP proxy servers.
func DialContext(vlessURI string) (func(ctx context.Context, network, address string) (net.Conn, error), error) {
	cfg, err := ParseVlessURI(vlessURI)
	if err != nil {
		return nil, err
	}
	d := &Dialer{cfg: cfg}
	return d.DialContext, nil
}
