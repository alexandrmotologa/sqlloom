package telemetry

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"strings"
	"time"

	"github.com/alexandrmotologa/sqlloom/internal/cache"
	"github.com/alexandrmotologa/sqlloom/internal/parser"
)

// HTTPServer exposes metrics, health endpoints, and the embedded Cockpit UI.
type HTTPServer struct {
	addr        string
	proxyAddr   string
	metrics     *Metrics
	hub         *EventHub
	cache       *cache.ShardedLRU
	invalidator *cache.Invalidator
	tracker     *InvalidationTracker
	embeddedUI  fs.FS
	startTime   time.Time
}

// NewHTTPServer initializes the web telemetry and cockpit server.
func NewHTTPServer(
	addr string,
	proxyAddr string,
	metrics *Metrics,
	hub *EventHub,
	cache *cache.ShardedLRU,
	invalidator *cache.Invalidator,
	embeddedUI fs.FS,
) *HTTPServer {
	return &HTTPServer{
		addr:        addr,
		proxyAddr:   proxyAddr,
		metrics:     metrics,
		hub:         hub,
		cache:       cache,
		invalidator: invalidator,
		tracker:     GlobalInvalidationTracker,
		embeddedUI:  embeddedUI,
		startTime:   time.Now(),
	}
}

// Start launches the HTTP server in a background goroutine.
func (s *HTTPServer) Start() error {
	mux := http.NewServeMux()

	// Operational endpoints
	mux.HandleFunc("/healthz", s.handleHealthz)
	mux.HandleFunc("/metrics", s.handleMetrics)

	// Cockpit API
	mux.HandleFunc("/api/stats", s.handleStats)
	mux.HandleFunc("/api/queries/recent", s.handleRecentQueries)
	mux.HandleFunc("/api/tables", s.handleTables)
	mux.HandleFunc("/api/cache/purge", s.handlePurge)
	mux.HandleFunc("/api/events", s.handleEvents)

	// Declarative Rules API
	mux.HandleFunc("/api/rules", s.handleGetRules)
	mux.HandleFunc("/api/rules/exclude", s.handleRuleExclude)
	mux.HandleFunc("/api/rules/ttl", s.handleRuleTTL)
	mux.HandleFunc("/api/rules/pattern", s.handleRulePattern)

	// Invalidation History & Churn API
	mux.HandleFunc("/api/invalidation/history", s.handleInvalidationHistory)
	mux.HandleFunc("/api/churn/recommendations", s.handleChurnRecommendations)

	// Benchmark Load Generator API
	mux.HandleFunc("/api/benchmark/run", s.handleBenchmarkRun)

	// Embedded Static Cockpit Web UI
	if s.embeddedUI != nil {
		fileServer := http.FileServer(http.FS(s.embeddedUI))
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			path := strings.TrimPrefix(r.URL.Path, "/")
			if path == "" {
				fileServer.ServeHTTP(w, r)
				return
			}
			f, err := s.embeddedUI.Open(path)
			if err != nil {
				// Fallback to index.html for client-side SPA routing
				r.URL.Path = "/"
				fileServer.ServeHTTP(w, r)
				return
			}
			_ = f.Close()
			fileServer.ServeHTTP(w, r)
		})
	} else {
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = fmt.Fprint(w, `<!DOCTYPE html><html><head><title>sqlloom Cockpit</title></head><body style="background:#090d16;color:#e2e8f0;font-family:sans-serif;padding:2rem;"><h1>sqlloom Query Proxy</h1><p>Running. UI static build assets pending.</p></body></html>`)
		})
	}

	server := &http.Server{
		Addr:         s.addr,
		Handler:      corsMiddleware(mux),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 0,
	}

	go func() {
		_ = server.ListenAndServe()
	}()

	return nil
}

func (s *HTTPServer) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":      "healthy",
		"uptime_sec":  int(time.Since(s.startTime).Seconds()),
		"memory_mb":   float64(s.cache.MemoryUsage()) / (1024 * 1024),
		"entries":     s.cache.EntryCount(),
		"connections": s.metrics.ActiveConnections.Load(),
	})
}

func (s *HTTPServer) handleMetrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	_, _ = fmt.Fprint(w, s.metrics.PrometheusFormat(s.cache.MemoryUsage(), s.cache.EntryCount()))
}

func (s *HTTPServer) handleStats(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"total_queries": s.metrics.TotalQueries.Load(),
		"cache_hits":    s.metrics.CacheHits.Load(),
		"cache_misses":  s.metrics.CacheMisses.Load(),
		"bypasses":      s.metrics.CacheBypasses.Load(),
		"hit_ratio_pct": s.metrics.HitRatio(),
		"saved_time_ms": float64(s.metrics.SavedLatencyMicros.Load()) / 1000.0,
		"invalidations": s.metrics.Invalidations.Load(),
		"memory_bytes":  s.cache.MemoryUsage(),
		"entries":       s.cache.EntryCount(),
		"active_conns":  s.metrics.ActiveConnections.Load(),
	})
}

func (s *HTTPServer) handleRecentQueries(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(s.hub.GetRecent())
}

func (s *HTTPServer) handleTables(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	stats := s.invalidator.GetTableStats()

	type tableInfo struct {
		Name         string `json:"name"`
		CachedCounts int    `json:"cached_queries"`
	}

	var list []tableInfo
	for name, count := range stats {
		list = append(list, tableInfo{
			Name:         name,
			CachedCounts: count,
		})
	}

	_ = json.NewEncoder(w).Encode(list)
}

func (s *HTTPServer) handlePurge(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Table string `json:"table"`
		All   bool   `json:"all"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	evictedCount := 0
	if req.All {
		evictedCount = s.invalidator.PurgeAll()
		s.tracker.RecordInvalidation("*", "MANUAL_FLUSH", evictedCount, 0.1)
	} else if req.Table != "" {
		evictedCount = s.invalidator.PurgeTable(req.Table)
		s.tracker.RecordInvalidation(req.Table, "MANUAL_PURGE", evictedCount, 0.1)
	}

	s.metrics.Invalidations.Add(1)
	s.metrics.EvictedEntries.Add(int64(evictedCount))

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success":       true,
		"evicted_count": evictedCount,
	})
}

func (s *HTTPServer) handleGetRules(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(parser.GlobalRules.GetSummary())
}

func (s *HTTPServer) handleRuleExclude(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodDelete {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Table   string `json:"table"`
		Exclude bool   `json:"exclude"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	if req.Exclude || r.Method == http.MethodPost {
		parser.GlobalRules.ExcludeTable(req.Table)
		// Purge any existing cached items for this excluded table
		_ = s.invalidator.PurgeTable(req.Table)
	} else {
		parser.GlobalRules.IncludeTable(req.Table)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"success": true})
}

func (s *HTTPServer) handleRuleTTL(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Table  string `json:"table"`
		TTLSec int    `json:"ttl_sec"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	parser.GlobalRules.SetTableTTL(req.Table, req.TTLSec)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"success": true})
}

func (s *HTTPServer) handleRulePattern(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodDelete {
		var req struct {
			ID string `json:"id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}
		parser.GlobalRules.RemovePatternRule(req.ID)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true})
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		ID      string `json:"id"`
		Pattern string `json:"pattern"`
		Action  string `json:"action"`
		TTLSec  int    `json:"ttl_sec"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	if req.ID == "" {
		req.ID = fmt.Sprintf("pat-%d", time.Now().UnixNano())
	}

	if err := parser.GlobalRules.AddPatternRule(req.ID, req.Pattern, req.Action, req.TTLSec); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "id": req.ID})
}

func (s *HTTPServer) handleInvalidationHistory(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(s.tracker.GetHistory())
}

func (s *HTTPServer) handleChurnRecommendations(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(s.tracker.GetRecommendations())
}

func (s *HTTPServer) handleBenchmarkRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	var params BenchmarkParams
	_ = json.NewDecoder(r.Body).Decode(&params)

	targetAddr := s.proxyAddr
	if strings.HasPrefix(targetAddr, ":") {
		targetAddr = "127.0.0.1" + targetAddr
	}

	result, err := RunSmokeBenchmark(targetAddr, params)
	if err != nil {
		http.Error(w, fmt.Sprintf("benchmark failed: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

func (s *HTTPServer) handleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch := s.hub.Subscribe()
	defer s.hub.Unsubscribe(ch)

	_, _ = fmt.Fprint(w, ": connected\n\n")
	flusher.Flush()

	notify := r.Context().Done()
	for {
		select {
		case <-notify:
			return
		case event, ok := <-ch:
			if !ok {
				return
			}
			data, err := json.Marshal(event)
			if err == nil {
				_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
				flusher.Flush()
			}
		}
	}
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}
