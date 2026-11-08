package test

import (
	"context"
	"encoding/binary"
	"net"
	"testing"
	"time"

	"github.com/alexandrmotologa/sqlloom/internal/cache"
	"github.com/alexandrmotologa/sqlloom/internal/config"
	"github.com/alexandrmotologa/sqlloom/internal/proxy"
	"github.com/alexandrmotologa/sqlloom/internal/telemetry"
	"github.com/alexandrmotologa/sqlloom/internal/wire"
)

func TestProxyCacheAndInvalidationEndToEnd(t *testing.T) {
	mock, err := StartMockPostgres()
	if err != nil {
		t.Fatalf("failed to start mock postgres: %v", err)
	}
	defer mock.Close()

	cfg := &config.Config{
		ListenAddr:       "127.0.0.1:0",
		UpstreamAddr:     mock.Addr,
		CacheMaxMemoryMB: 32,
		CacheMaxEntries:  1000,
		DefaultTTLSec:    3600,
		Verbose:          false,
	}

	cacheLRU := cache.NewShardedLRU(32*1024*1024, 1000)
	invalidator := cache.NewInvalidator(cacheLRU)
	metrics := telemetry.GlobalMetrics
	hub := telemetry.GlobalHub

	server := proxy.NewServer(cfg, cacheLRU, invalidator, metrics, hub)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to bind ephemeral port: %v", err)
	}
	cfg.ListenAddr = ln.Addr().String()
	_ = ln.Close()

	if err := server.Start(); err != nil {
		t.Fatalf("failed to start proxy server: %v", err)
	}
	defer func() {
		_ = server.Shutdown(context.Background())
	}()

	time.Sleep(20 * time.Millisecond)

	clientConn, clientReader, clientWriter := establishTestClient(t, cfg.ListenAddr)
	defer clientConn.Close()

	// Query 1: First SELECT (Expect Cache MISS, Upstream called)
	sendQuery(t, clientWriter, "SELECT * FROM users WHERE id = 1")
	readQueryResult(t, clientReader)
	if count := mock.QueryCount.Load(); count != 1 {
		t.Fatalf("expected upstream query count 1 after first query, got %d", count)
	}

	// Query 2: Second identical SELECT (Expect Cache HIT, Upstream NOT called)
	sendQuery(t, clientWriter, "SELECT * FROM users WHERE id = 1")
	readQueryResult(t, clientReader)
	if count := mock.QueryCount.Load(); count != 1 {
		t.Fatalf("expected upstream query count to REMAIN 1 on cache hit, got %d", count)
	}

	// Query 3: Write query (UPDATE users...) -> must invalidate table 'users'
	sendQuery(t, clientWriter, "UPDATE users SET name = 'Bob' WHERE id = 1")
	readQueryResult(t, clientReader)
	if count := mock.QueryCount.Load(); count != 2 {
		t.Fatalf("expected upstream query count 2 after update, got %d", count)
	}

	// Query 4: Third SELECT -> should now be Cache MISS because 'users' was invalidated!
	sendQuery(t, clientWriter, "SELECT * FROM users WHERE id = 1")
	readQueryResult(t, clientReader)
	if count := mock.QueryCount.Load(); count != 3 {
		t.Fatalf("expected upstream query count 3 after cache invalidation, got %d", count)
	}

	// Query 5: Fourth SELECT -> should be Cache HIT again!
	sendQuery(t, clientWriter, "SELECT * FROM users WHERE id = 1")
	readQueryResult(t, clientReader)
	if count := mock.QueryCount.Load(); count != 3 {
		t.Fatalf("expected upstream query count to REMAIN 3 on cache hit, got %d", count)
	}

	// Query 6: Cache bypass directive /* sqlloom:nocache */
	sendQuery(t, clientWriter, "SELECT * FROM users WHERE id = 1 /* sqlloom:nocache */")
	readQueryResult(t, clientReader)
	if count := mock.QueryCount.Load(); count != 4 {
		t.Fatalf("expected upstream query count 4 after nocache directive, got %d", count)
	}
}

func TestTransactionInvalidationOnCommit(t *testing.T) {
	mock, err := StartMockPostgres()
	if err != nil {
		t.Fatalf("failed to start mock postgres: %v", err)
	}
	defer mock.Close()

	cfg := &config.Config{
		ListenAddr:       "127.0.0.1:0",
		UpstreamAddr:     mock.Addr,
		CacheMaxMemoryMB: 32,
		CacheMaxEntries:  1000,
		DefaultTTLSec:    3600,
		Verbose:          false,
	}

	cacheLRU := cache.NewShardedLRU(32*1024*1024, 1000)
	invalidator := cache.NewInvalidator(cacheLRU)
	metrics := telemetry.GlobalMetrics
	hub := telemetry.GlobalHub

	server := proxy.NewServer(cfg, cacheLRU, invalidator, metrics, hub)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to bind ephemeral port: %v", err)
	}
	cfg.ListenAddr = ln.Addr().String()
	_ = ln.Close()

	if err := server.Start(); err != nil {
		t.Fatalf("failed to start proxy server: %v", err)
	}
	defer func() {
		_ = server.Shutdown(context.Background())
	}()

	time.Sleep(20 * time.Millisecond)

	clientConn, clientReader, clientWriter := establishTestClient(t, cfg.ListenAddr)
	defer clientConn.Close()

	// 1. Populate cache with a query for table 'orders'
	sendQuery(t, clientWriter, "SELECT * FROM orders WHERE id = 100")
	readQueryResult(t, clientReader)

	// Verify it's cached
	sendQuery(t, clientWriter, "SELECT * FROM orders WHERE id = 100")
	readQueryResult(t, clientReader)
	if count := mock.QueryCount.Load(); count != 1 {
		t.Fatalf("expected query to be served from cache, count=%d", count)
	}

	// 2. Start transaction: BEGIN
	sendQuery(t, clientWriter, "BEGIN")
	readQueryResult(t, clientReader)

	// 3. Write inside transaction: UPDATE orders SET total = 50
	sendQuery(t, clientWriter, "UPDATE orders SET total = 50 WHERE id = 100")
	readQueryResult(t, clientReader)

	// 4. Query inside transaction should bypass cache (ACID consistency)
	sendQuery(t, clientWriter, "SELECT * FROM orders WHERE id = 100")
	readQueryResult(t, clientReader)
	if count := mock.QueryCount.Load(); count != 4 {
		t.Fatalf("expected query in transaction to bypass cache, count=%d", count)
	}

	// 5. Commit transaction: COMMIT
	sendQuery(t, clientWriter, "COMMIT")
	readQueryResult(t, clientReader)

	// 6. Query after commit should be Cache MISS because 'orders' was invalidated on COMMIT!
	sendQuery(t, clientWriter, "SELECT * FROM orders WHERE id = 100")
	readQueryResult(t, clientReader)
	if count := mock.QueryCount.Load(); count != 6 {
		t.Fatalf("expected query to miss cache after COMMIT invalidation, count=%d", count)
	}
}

func establishTestClient(t *testing.T, addr string) (net.Conn, *wire.Reader, *wire.Writer) {
	t.Helper()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("failed to connect to sqlloom: %v", err)
	}

	reader := wire.NewReader(conn)
	writer := wire.NewWriter(conn)

	// SSLRequest
	var sslBuf [8]byte
	binary.BigEndian.PutUint32(sslBuf[0:4], 8)
	binary.BigEndian.PutUint32(sslBuf[4:8], uint32(wire.SSLRequestCode))
	if _, err := conn.Write(sslBuf[:]); err != nil {
		t.Fatalf("failed to send SSLRequest: %v", err)
	}

	var sslReply [1]byte
	if _, err := conn.Read(sslReply[:]); err != nil || sslReply[0] != 'N' {
		t.Fatalf("expected SSL 'N' reply, got '%c' (err: %v)", sslReply[0], err)
	}

	// StartupMessage
	var startupPayload []byte
	var proto [4]byte
	binary.BigEndian.PutUint32(proto[:], uint32(wire.ProtocolVersion30))
	startupPayload = append(startupPayload, proto[:]...)
	startupPayload = append(startupPayload, []byte("user\x00postgres\x00database\x00testdb\x00\x00")...)

	var startupLen [4]byte
	binary.BigEndian.PutUint32(startupLen[:], uint32(len(startupPayload)+4))
	_, _ = conn.Write(startupLen[:])
	_, _ = conn.Write(startupPayload)

	for {
		msg, err := reader.ReadMessage()
		if err != nil {
			t.Fatalf("failed reading handshake message: %v", err)
		}
		if msg.Type == wire.MsgReadyForQuery {
			break
		}
	}

	return conn, reader, writer
}

func sendQuery(t *testing.T, w *wire.Writer, sql string) {
	t.Helper()
	msg := wire.BuildQueryMessage(sql)
	if err := w.WriteRaw(msg); err != nil {
		t.Fatalf("failed to write query '%s': %v", sql, err)
	}
	if err := w.Flush(); err != nil {
		t.Fatalf("failed to flush query '%s': %v", sql, err)
	}
}

func readQueryResult(t *testing.T, r *wire.Reader) {
	t.Helper()
	for {
		msg, err := r.ReadMessage()
		if err != nil {
			t.Fatalf("error reading query result message: %v", err)
		}
		if msg.Type == wire.MsgReadyForQuery {
			return
		}
	}
}
