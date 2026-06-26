package proxy

import (
	"context"
	"testing"
)

func TestProxyAddr(t *testing.T) {
	p, err := NewVlessProxy("vless://00000000-0000-0000-0000-000000000000@example.com:443?security=tls&type=ws&encryption=none")
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	if err := p.Start(ctx, ""); err != nil {
		t.Fatal(err)
	}
	defer p.Close()

	addr := p.Addr()
	if addr == nil {
		t.Fatal("Addr() returned nil")
	}
	if addr.String() == "" {
		t.Fatal("Addr() returned empty string")
	}

	t.Logf("Proxy listening on %s", addr.String())
}

func TestProxyStartStop(t *testing.T) {
	p, err := NewVlessProxy("vless://00000000-0000-0000-0000-000000000000@example.com:443?security=tls&type=ws&encryption=none")
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	if err := p.Start(ctx, "127.0.0.1:0"); err != nil {
		t.Fatal(err)
	}

	addr := p.Addr()
	if addr == nil {
		t.Fatal("Addr() returned nil after Start")
	}
	t.Logf("Proxy listening on %s", addr.String())

	if err := p.Close(); err != nil {
		t.Fatal(err)
	}
}
