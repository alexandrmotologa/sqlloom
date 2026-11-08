package telemetry

import (
	"fmt"
	"sync/atomic"
)

// Metrics maintains operational counters and gauges for sqlloom.
type Metrics struct {
	TotalQueries      atomic.Int64
	CacheHits         atomic.Int64
	CacheMisses       atomic.Int64
	CacheBypasses     atomic.Int64
	Invalidations     atomic.Int64
	EvictedEntries    atomic.Int64
	SavedLatencyMicros atomic.Int64
	ActiveConnections atomic.Int64
}

// GlobalMetrics is the singleton metrics registry.
var GlobalMetrics = &Metrics{}

// HitRatio computes current cache hit percentage [0.0 - 100.0].
func (m *Metrics) HitRatio() float64 {
	hits := m.CacheHits.Load()
	misses := m.CacheMisses.Load()
	total := hits + misses
	if total == 0 {
		return 0.0
	}
	return (float64(hits) / float64(total)) * 100.0
}

// PrometheusFormat serializes metrics in standard Prometheus exposition format.
func (m *Metrics) PrometheusFormat(cacheMemoryBytes, cacheEntries int64) string {
	return fmt.Sprintf(`# HELP sqlloom_query_total Total number of SQL statements processed.
# TYPE sqlloom_query_total counter
sqlloom_query_total %d

# HELP sqlloom_cache_hits_total Number of queries served from in-memory cache.
# TYPE sqlloom_cache_hits_total counter
sqlloom_cache_hits_total %d

# HELP sqlloom_cache_misses_total Number of queries forwarded to upstream database.
# TYPE sqlloom_cache_misses_total counter
sqlloom_cache_misses_total %d

# HELP sqlloom_cache_bypass_total Number of queries that bypassed cache (transactions, volatile, etc).
# TYPE sqlloom_cache_bypass_total counter
sqlloom_cache_bypass_total %d

# HELP sqlloom_invalidation_total Number of table invalidation triggers executed.
# TYPE sqlloom_invalidation_total counter
sqlloom_invalidation_total %d

# HELP sqlloom_saved_latency_seconds_total Total database query execution latency saved by cache.
# TYPE sqlloom_saved_latency_seconds_total counter
sqlloom_saved_latency_seconds_total %.6f

# HELP sqlloom_cache_memory_bytes Current memory consumed by cached query results.
# TYPE sqlloom_cache_memory_bytes gauge
sqlloom_cache_memory_bytes %d

# HELP sqlloom_cache_entries Total number of unique queries in cache.
# TYPE sqlloom_cache_entries gauge
sqlloom_cache_entries %d

# HELP sqlloom_active_connections Number of client connections currently open.
# TYPE sqlloom_active_connections gauge
sqlloom_active_connections %d
`,
		m.TotalQueries.Load(),
		m.CacheHits.Load(),
		m.CacheMisses.Load(),
		m.CacheBypasses.Load(),
		m.Invalidations.Load(),
		float64(m.SavedLatencyMicros.Load())/1000000.0,
		cacheMemoryBytes,
		cacheEntries,
		m.ActiveConnections.Load(),
	)
}
