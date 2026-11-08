package proxy

import (
	"fmt"
	"net"
	"time"

	"github.com/alexandrmotologa/sqlloom/internal/wire"
)

// UpstreamConn wraps an active connection to upstream PostgreSQL.
type UpstreamConn struct {
	Conn   net.Conn
	Reader *wire.Reader
	Writer *wire.Writer
}

// DialUpstream connects to the target PostgreSQL server.
func DialUpstream(addr string) (*UpstreamConn, error) {
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to upstream PostgreSQL at %s: %w", addr, err)
	}

	// Disable Nagle's algorithm for lowest latency
	if tcp, ok := conn.(*net.TCPConn); ok {
		_ = tcp.SetNoDelay(true)
	}

	return &UpstreamConn{
		Conn:   conn,
		Reader: wire.NewReader(conn),
		Writer: wire.NewWriter(conn),
	}, nil
}

// Close terminates the upstream connection.
func (u *UpstreamConn) Close() error {
	if u.Conn != nil {
		return u.Conn.Close()
	}
	return nil
}
