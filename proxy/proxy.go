package proxy

import (
	"bufio"
	"context"
	"io"
	"net"
	"net/http"
	"sync"
)

// Proxy wraps a VLESS dialer to provide a local HTTP proxy server.
type Proxy struct {
	dialer   *Dialer
	server   *http.Server
	listener net.Listener
}

// NewProxy creates a new local HTTP proxy server for the given VLESS URI.
func NewProxy(vlessURI string) (*Proxy, error) {
	cfg, err := ParseVlessURI(vlessURI)
	if err != nil {
		return nil, err
	}
	dialer := &Dialer{cfg: cfg}
	return &Proxy{dialer: dialer}, nil
}

// Start starts the local HTTP proxy server.
// If addr is empty, it binds to "127.0.0.1:0" (random port).
func (p *Proxy) Start(ctx context.Context, addr string) error {
	if addr == "" {
		addr = "127.0.0.1:0"
	}

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	p.listener = listener

	p.server = &http.Server{
		Handler: p,
	}

	go p.server.Serve(listener) //nolint:errcheck

	return nil
}

// Addr returns the listener address, or nil if not started.
func (p *Proxy) Addr() net.Addr {
	if p.listener != nil {
		return p.listener.Addr()
	}
	return nil
}

// Close closes the proxy server.
func (p *Proxy) Close() error {
	if p.server != nil {
		return p.server.Close()
	}
	return nil
}

// ServeHTTP implements http.Handler. It routes CONNECT and plain HTTP requests.
func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodConnect {
		p.serveConnect(w, r)
		return
	}
	p.serveHTTP(w, r)
}

// serveConnect handles CONNECT requests by establishing a VLESS tunnel
// and performing bidirectional copy between client and target.
func (p *Proxy) serveConnect(w http.ResponseWriter, r *http.Request) {
	dialCtx, cancel := context.WithCancel(r.Context())
	defer cancel()

	target := r.Host
	if target == "" {
		http.Error(w, "missing target", http.StatusBadRequest)
		return
	}

	conn, err := p.dialer.DialContext(dialCtx, "tcp", target)
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer conn.Close()

	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "hijacking not supported", http.StatusInternalServerError)
		return
	}

	clientConn, _, err := hijacker.Hijack()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer clientConn.Close()

	// Write 200 Connection Established before starting copy.
	if _, err := clientConn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n")); err != nil {
		return
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		io.Copy(conn, clientConn) //nolint:errcheck
		wg.Done()
	}()
	go func() {
		io.Copy(clientConn, conn) //nolint:errcheck
		wg.Done()
	}()
	wg.Wait()
}

// serveHTTP handles plain HTTP requests by forwarding them through the VLESS tunnel.
func (p *Proxy) serveHTTP(w http.ResponseWriter, r *http.Request) {
	dialCtx, cancel := context.WithCancel(r.Context())
	defer cancel()

	target := r.Host
	if target == "" {
		http.Error(w, "missing target", http.StatusBadRequest)
		return
	}

	conn, err := p.dialer.DialContext(dialCtx, "tcp", target)
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer conn.Close()

	// Write the original request through the VLESS tunnel.
	if err := r.Write(conn); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	// Read the response from the tunnel.
	resp, err := http.ReadResponse(bufio.NewReader(conn), r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Copy headers and body back to the client.
	for k, v := range resp.Header {
		w.Header()[k] = v
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body) //nolint:errcheck
}
