package proxy

import (
	"bytes"
	"encoding/binary"
	"net"
	"sync"

	"github.com/alexandrmotologa/sqlloom/internal/cache"
	"github.com/alexandrmotologa/sqlloom/internal/config"
	"github.com/alexandrmotologa/sqlloom/internal/telemetry"
	"github.com/alexandrmotologa/sqlloom/internal/wire"
)

// PreparedStatement stores metadata about a parsed SQL statement.
type PreparedStatement struct {
	Name      string
	SQL       string
	ParamOIDs []uint32
}

// Portal stores metadata about a bound statement execution context.
type Portal struct {
	Name          string
	StatementName string
	Parameters    []string
}

// Session represents a single client connection lifecycle.
type Session struct {
	cfg                *config.Config
	clientConn         net.Conn
	clientReader       *wire.Reader
	clientWriter       *wire.Writer
	upstream           *UpstreamConn
	database           string
	user               string
	txStatus           byte
	modifiedTablesInTx []string
	preparedStmts      map[string]*PreparedStatement
	portals            map[string]*Portal
	mu                 sync.Mutex
	cache              *cache.ShardedLRU
	invalidator        *cache.Invalidator
	metrics            *telemetry.Metrics
	hub                *telemetry.EventHub
}

// NewSession initializes a client session handler.
func NewSession(
	conn net.Conn,
	cfg *config.Config,
	cache *cache.ShardedLRU,
	invalidator *cache.Invalidator,
	metrics *telemetry.Metrics,
	hub *telemetry.EventHub,
) *Session {
	if tcp, ok := conn.(*net.TCPConn); ok {
		_ = tcp.SetNoDelay(true)
	}

	return &Session{
		cfg:           cfg,
		clientConn:    conn,
		clientReader:  wire.NewReader(conn),
		clientWriter:  wire.NewWriter(conn),
		txStatus:      wire.TxStatusIdle,
		preparedStmts: make(map[string]*PreparedStatement),
		portals:       make(map[string]*Portal),
		cache:         cache,
		invalidator:   invalidator,
		metrics:       metrics,
		hub:           hub,
	}
}

// Handle executes the handshake and starts query routing.
func (s *Session) Handle() error {
	defer func() {
		_ = s.clientConn.Close()
		if s.upstream != nil {
			_ = s.upstream.Close()
		}
	}()

	// 1. Perform client handshake (SSL negotiation and StartupMessage)
	startup, err := wire.PerformClientHandshake(s.clientConn, s.clientReader)
	if err != nil {
		return err
	}

	// 2. Connect to upstream PostgreSQL
	upstream, err := DialUpstream(s.cfg.UpstreamAddr)
	if err != nil {
		_ = s.clientWriter.WriteError("FATAL", "08006", "sqlloom: failed to connect to upstream database")
		return err
	}
	s.upstream = upstream

	// 3. Relay handshake and complete authentication
	handshakeRes, err := wire.ForwardHandshakeToUpstream(
		startup,
		s.clientReader,
		s.clientWriter,
		s.upstream.Conn,
		s.upstream.Reader,
		s.upstream.Writer,
	)
	if err != nil {
		return err
	}

	s.database = handshakeRes.Database
	s.user = handshakeRes.User
	s.txStatus = handshakeRes.TxStatus

	// 4. Main message dispatch loop
	return s.runDispatchLoop()
}

func (s *Session) runDispatchLoop() error {
	router := NewRouter(s)

	for {
		msg, err := s.clientReader.ReadMessage()
		if err != nil {
			return nil // Client disconnected normally
		}

		if err := router.Dispatch(msg); err != nil {
			return err
		}
	}
}

func parseBindParameters(payload []byte) (portalName, stmtName string, params []string) {
	// Format: portal (null-terminated), stmt (null-terminated), param formats count, ...
	idx := bytes.IndexByte(payload, 0)
	if idx == -1 {
		return "", "", nil
	}
	portalName = string(payload[:idx])
	rest := payload[idx+1:]

	idx2 := bytes.IndexByte(rest, 0)
	if idx2 == -1 {
		return portalName, "", nil
	}
	stmtName = string(rest[:idx2])
	rest = rest[idx2+1:]

	if len(rest) < 2 {
		return portalName, stmtName, nil
	}

	// Parameter format codes
	numFormats := int(binary.BigEndian.Uint16(rest[:2]))
	offset := 2 + (numFormats * 2)
	if len(rest) < offset+2 {
		return portalName, stmtName, nil
	}

	// Parameter values count
	numParams := int(binary.BigEndian.Uint16(rest[offset : offset+2]))
	offset += 2

	for i := 0; i < numParams && offset < len(rest); i++ {
		if len(rest)-offset < 4 {
			break
		}
		paramLen := int(int32(binary.BigEndian.Uint32(rest[offset : offset+4])))
		offset += 4

		if paramLen == -1 {
			params = append(params, "NULL")
		} else if offset+paramLen <= len(rest) {
			params = append(params, string(rest[offset:offset+paramLen]))
			offset += paramLen
		}
	}

	return portalName, stmtName, params
}

func parseParseMessage(payload []byte) (stmtName, sql string, oids []uint32) {
	idx := bytes.IndexByte(payload, 0)
	if idx == -1 {
		return "", "", nil
	}
	stmtName = string(payload[:idx])
	rest := payload[idx+1:]

	idx2 := bytes.IndexByte(rest, 0)
	if idx2 == -1 {
		return stmtName, string(rest), nil
	}
	sql = string(rest[:idx2])
	rest = rest[idx2+1:]

	if len(rest) >= 2 {
		numOIDs := int(binary.BigEndian.Uint16(rest[:2]))
		offset := 2
		for i := 0; i < numOIDs && offset+4 <= len(rest); i++ {
			oid := binary.BigEndian.Uint32(rest[offset : offset+4])
			oids = append(oids, oid)
			offset += 4
		}
	}

	return stmtName, sql, oids
}
