package httpproxy

import (
	"fmt"
	"io"
	"net"
	"sync"
)

// vlessConn wraps a net.Conn and lazily consumes the VLESS response header on
// the first Read call.
type vlessConn struct {
	net.Conn

	cancel             func()
	responseHeaderMu   sync.Mutex
	responseHeaderRead bool
	responseHeaderErr  error
}

func (c *vlessConn) Close() error {
	if c.cancel != nil {
		c.cancel()
	}
	return c.Conn.Close()
}

func (c *vlessConn) Read(p []byte) (int, error) {
	if err := c.consumeResponseHeader(); err != nil {
		return 0, err
	}
	return c.Conn.Read(p)
}

func (c *vlessConn) consumeResponseHeader() error {
	c.responseHeaderMu.Lock()
	defer c.responseHeaderMu.Unlock()

	if c.responseHeaderRead || c.responseHeaderErr != nil {
		return c.responseHeaderErr
	}

	c.responseHeaderErr = consumeVlessResponseHeader(c.Conn)
	if c.responseHeaderErr == nil {
		c.responseHeaderRead = true
	}
	return c.responseHeaderErr
}

// consumeVlessResponseHeader reads and validates the VLESS response header
// from the server.
func consumeVlessResponseHeader(reader io.Reader) error {
	var fixed [2]byte
	if _, err := io.ReadFull(reader, fixed[:]); err != nil {
		return fmt.Errorf("read VLESS response header: %w", err)
	}
	if fixed[0] != vlessVersion {
		return fmt.Errorf("unexpected VLESS response version: %d", fixed[0])
	}
	if fixed[1] == 0 {
		return nil
	}

	addons := make([]byte, int(fixed[1]))
	if _, err := io.ReadFull(reader, addons); err != nil {
		return fmt.Errorf("read VLESS response addons: %w", err)
	}
	return nil
}
