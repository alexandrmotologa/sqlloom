package main

import (
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/alexandrmotologa/sqlloom/internal/cache"
	"github.com/alexandrmotologa/sqlloom/internal/config"
	"github.com/alexandrmotologa/sqlloom/internal/proxy"
	"github.com/alexandrmotologa/sqlloom/internal/telemetry"
	"github.com/alexandrmotologa/sqlloom/internal/ui"
	"github.com/alexandrmotologa/sqlloom/internal/wire"
	"github.com/alexandrmotologa/sqlloom/test"
)

func main() {
	log.Println("[SEED] Starting Mock PostgreSQL upstream...")
	mock, err := test.StartMockPostgres()
	if err != nil {
		log.Fatalf("failed to start mock postgres: %v", err)
	}
	defer mock.Close()
	log.Printf("[SEED] Mock PostgreSQL listening at %s", mock.Addr)

	cfg := &config.Config{
		ListenAddr:       "127.0.0.1:15433",
		UpstreamAddr:     mock.Addr,
		DashboardAddr:    "127.0.0.1:8089",
		CacheMaxMemoryMB: 32,
		CacheMaxEntries:  10000,
		DefaultTTLSec:    3600,
		GraceMode:        true,
		Verbose:          false,
	}

	maxMemoryBytes := int64(cfg.CacheMaxMemoryMB) * 1024 * 1024
	cacheLRU := cache.NewShardedLRU(maxMemoryBytes, int64(cfg.CacheMaxEntries))
	invalidator := cache.NewInvalidator(cacheLRU)
	metrics := telemetry.GlobalMetrics
	hub := telemetry.GlobalHub

	uiFS, err := ui.GetFS()
	if err != nil {
		log.Printf("[WARN] Failed to load embedded UI filesystem: %v", err)
	}

	httpServer := telemetry.NewHTTPServer(
		cfg.DashboardAddr,
		cfg.ListenAddr,
		metrics,
		hub,
		cacheLRU,
		invalidator,
		uiFS,
	)

	go func() {
		if err := httpServer.Start(); err != nil {
			log.Printf("[ERROR] HTTP Server error: %v", err)
		}
	}()

	proxyServer := proxy.NewServer(cfg, cacheLRU, invalidator, metrics, hub)
	if err := proxyServer.Start(); err != nil {
		log.Fatalf("failed to start proxy server: %v", err)
	}
	log.Printf("[SEED] sqlloom proxy listening on %s (Dashboard: http://127.0.0.1:8089)", cfg.ListenAddr)

	time.Sleep(300 * time.Millisecond)

	// Seed queries concurrently
	go runQuerySeeder("127.0.0.1:15433")

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	<-sigCh

	log.Println("[SEED] Shutting down...")
	_ = proxyServer.Shutdown(context.Background())
}

func runQuerySeeder(proxyAddr string) {
	conn, reader, writer, err := dialTestClient(proxyAddr)
	if err != nil {
		log.Printf("[SEED ERROR] Failed to connect test client: %v", err)
		return
	}
	defer conn.Close()

	queries := []struct {
		sql    string
		repeat int
		delay  time.Duration
	}{
		// Users table queries
		{"SELECT id, name, email, plan FROM users WHERE id = 1", 1, 30 * time.Millisecond},
		{"SELECT id, name, email, plan FROM users WHERE id = 1", 4, 20 * time.Millisecond},
		{"SELECT id, name, email, plan FROM users WHERE id = 2", 3, 20 * time.Millisecond},
		{"SELECT count(*) FROM users WHERE active = true", 5, 25 * time.Millisecond},

		// Products table
		{"SELECT id, title, price, stock FROM products WHERE category = 'electronics'", 1, 30 * time.Millisecond},
		{"SELECT id, title, price, stock FROM products WHERE category = 'electronics'", 6, 15 * time.Millisecond},
		{"SELECT * FROM products WHERE in_stock = true ORDER BY price DESC LIMIT 10", 4, 20 * time.Millisecond},

		// Orders table
		{"SELECT id, user_id, amount, status FROM orders WHERE status = 'shipped'", 1, 35 * time.Millisecond},
		{"SELECT id, user_id, amount, status FROM orders WHERE status = 'shipped'", 5, 15 * time.Millisecond},
		{"SELECT sum(amount) FROM orders WHERE created_at >= '2026-01-01'", 3, 20 * time.Millisecond},

		// Directives & Bypass
		{"/* sqlloom:nocache */ SELECT token, expires_at FROM session_tokens WHERE user_id = 1", 2, 30 * time.Millisecond},

		// Invalidation on write
		{"UPDATE users SET name = 'Alexander Motologa' WHERE id = 1", 1, 40 * time.Millisecond},
		{"SELECT id, name, email, plan FROM users WHERE id = 1", 1, 30 * time.Millisecond},
		{"SELECT id, name, email, plan FROM users WHERE id = 1", 5, 15 * time.Millisecond},

		// Insert order & invalidate orders table
		{"INSERT INTO orders (id, user_id, amount, status) VALUES (992, 1, 240.50, 'paid')", 1, 40 * time.Millisecond},
		{"SELECT id, user_id, amount, status FROM orders WHERE status = 'shipped'", 1, 30 * time.Millisecond},
		{"SELECT id, user_id, amount, status FROM orders WHERE status = 'shipped'", 4, 15 * time.Millisecond},
	}

	// Initial burst
	for _, q := range queries {
		for i := 0; i < q.repeat; i++ {
			msg := wire.BuildQueryMessage(q.sql)
			_ = writer.WriteRaw(msg)
			_ = writer.Flush()
			drainResult(reader)
			time.Sleep(q.delay)
		}
	}
	log.Println("[SEED] Initial seed query burst completed successfully.")

	// Continuous background traffic
	ticker := time.NewTicker(400 * time.Millisecond)
	defer ticker.Stop()

	seq := 0
	for range ticker.C {
		seq++
		var sql string
		switch seq % 7 {
		case 0:
			sql = fmt.Sprintf("SELECT * FROM users WHERE id = %d", (seq%3)+1)
		case 1:
			sql = "SELECT id, title, price FROM products WHERE category = 'electronics'"
		case 2:
			sql = "SELECT count(*) FROM orders WHERE status = 'shipped'"
		case 3:
			sql = fmt.Sprintf("SELECT * FROM tenants WHERE org_id = %d", (seq%2)+1)
		case 4:
			if seq%28 == 0 {
				sql = "UPDATE products SET stock = stock - 1 WHERE id = 10"
			} else {
				sql = "SELECT * FROM products WHERE in_stock = true ORDER BY price DESC LIMIT 10"
			}
		case 5:
			sql = "/* sqlloom:nocache */ SELECT now(), version()"
		case 6:
			sql = fmt.Sprintf("SELECT id, name, email FROM users WHERE id = %d", (seq%4)+1)
		}

		msg := wire.BuildQueryMessage(sql)
		_ = writer.WriteRaw(msg)
		_ = writer.Flush()
		drainResult(reader)
	}
}

func dialTestClient(proxyAddr string) (net.Conn, *wire.Reader, *wire.Writer, error) {
	conn, err := net.DialTimeout("tcp", proxyAddr, 2*time.Second)
	if err != nil {
		return nil, nil, nil, err
	}

	reader := wire.NewReader(conn)
	writer := wire.NewWriter(conn)

	var sslBuf [8]byte
	binary.BigEndian.PutUint32(sslBuf[0:4], 8)
	binary.BigEndian.PutUint32(sslBuf[4:8], uint32(wire.SSLRequestCode))
	if _, err := conn.Write(sslBuf[:]); err != nil {
		conn.Close()
		return nil, nil, nil, err
	}

	var sslReply [1]byte
	if _, err := conn.Read(sslReply[:]); err != nil || sslReply[0] != 'N' {
		conn.Close()
		return nil, nil, nil, fmt.Errorf("unexpected ssl reply: %v", err)
	}

	var startupPayload []byte
	var proto [4]byte
	binary.BigEndian.PutUint32(proto[:], uint32(wire.ProtocolVersion30))
	startupPayload = append(startupPayload, proto[:]...)
	startupPayload = append(startupPayload, []byte("user\x00postgres\x00database\x00demo\x00\x00")...)

	var startupLen [4]byte
	binary.BigEndian.PutUint32(startupLen[:], uint32(len(startupPayload)+4))
	_, _ = conn.Write(startupLen[:])
	_, _ = conn.Write(startupPayload)

	for {
		msg, err := reader.ReadMessage()
		if err != nil {
			conn.Close()
			return nil, nil, nil, err
		}
		if msg.Type == wire.MsgReadyForQuery {
			break
		}
	}

	return conn, reader, writer, nil
}

func drainResult(r *wire.Reader) {
	for {
		msg, err := r.ReadMessage()
		if err != nil {
			return
		}
		if msg.Type == wire.MsgReadyForQuery {
			return
		}
	}
}
