package proxy

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/alexandrmotologa/sqlloom/internal/cache"
	"github.com/alexandrmotologa/sqlloom/internal/config"
	"github.com/alexandrmotologa/sqlloom/internal/telemetry"
)

// Server coordinates the TCP ingress listener and client sessions.
type Server struct {
	cfg          *config.Config
	listener     net.Listener
	cache        *cache.ShardedLRU
	invalidator  *cache.Invalidator
	metrics      *telemetry.Metrics
	hub          *telemetry.EventHub
	tracker      *telemetry.InvalidationTracker
	singleflight *SingleFlightGroup
	wg           sync.WaitGroup
	closed       chan struct{}
}

// NewServer initializes the proxy server.
func NewServer(
	cfg *config.Config,
	cache *cache.ShardedLRU,
	invalidator *cache.Invalidator,
	metrics *telemetry.Metrics,
	hub *telemetry.EventHub,
) *Server {
	return &Server{
		cfg:          cfg,
		cache:        cache,
		invalidator:  invalidator,
		metrics:      metrics,
		hub:          hub,
		tracker:      telemetry.GlobalInvalidationTracker,
		singleflight: NewSingleFlightGroup(),
		closed:       make(chan struct{}),
	}
}

// Start begins listening on the configured TCP address.
func (s *Server) Start() error {
	ln, err := net.Listen("tcp", s.cfg.ListenAddr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", s.cfg.ListenAddr, err)
	}
	s.listener = ln

	go s.acceptLoop()
	return nil
}

func (s *Server) acceptLoop() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-s.closed:
				return
			default:
				time.Sleep(10 * time.Millisecond)
				continue
			}
		}

		s.metrics.ActiveConnections.Add(1)
		s.wg.Add(1)

		go func(c net.Conn) {
			defer func() {
				s.metrics.ActiveConnections.Add(-1)
				s.wg.Done()
			}()

			session := NewSession(c, s.cfg, s.cache, s.invalidator, s.metrics, s.hub, s.singleflight, s.tracker)
			_ = session.Handle()
		}(conn)
	}
}

// Shutdown gracefully stops accepting connections and waits for active connections.
func (s *Server) Shutdown(ctx context.Context) error {
	close(s.closed)
	if s.listener != nil {
		_ = s.listener.Close()
	}

	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
