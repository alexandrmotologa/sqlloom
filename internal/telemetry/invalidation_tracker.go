package telemetry

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// InvalidationRecord captures details of a table write event and its eviction impact.
type InvalidationRecord struct {
	ID           string    `json:"id"`
	Table        string    `json:"table"`
	Operation    string    `json:"operation"`
	EvictedCount int       `json:"evicted_count"`
	DurationMs   float64   `json:"duration_ms"`
	Timestamp    time.Time `json:"timestamp"`
}

// ChurnRecommendation suggests actions for tables suffering from high cache invalidation churn.
type ChurnRecommendation struct {
	Table       string `json:"table"`
	WritesPerMin float64 `json:"writes_per_min"`
	Severity    string `json:"severity"` // "HIGH", "MEDIUM"
	Message     string `json:"message"`
}

// InvalidationTracker monitors write frequency and table churn.
type InvalidationTracker struct {
	mu           sync.RWMutex
	records      []InvalidationRecord
	tableTimestamps map[string][]time.Time
	seq          int64
}

// GlobalInvalidationTracker is the singleton instance.
var GlobalInvalidationTracker = NewInvalidationTracker()

// NewInvalidationTracker initializes the invalidation history tracker.
func NewInvalidationTracker() *InvalidationTracker {
	return &InvalidationTracker{
		records:         make([]InvalidationRecord, 0, 200),
		tableTimestamps: make(map[string][]time.Time),
	}
}

// RecordInvalidation stores an invalidation event and tracks table write rate.
func (t *InvalidationTracker) RecordInvalidation(table, op string, evicted int, durationMs float64) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.seq++
	now := time.Now()
	cleanTbl := strings.ToLower(strings.TrimSpace(table))

	rec := InvalidationRecord{
		ID:           fmt.Sprintf("inv-%d", t.seq),
		Table:        cleanTbl,
		Operation:    op,
		EvictedCount: evicted,
		DurationMs:   durationMs,
		Timestamp:    now,
	}

	if len(t.records) >= 200 {
		t.records = t.records[1:]
	}
	t.records = append(t.records, rec)

	// Append timestamp for churn calculation
	t.tableTimestamps[cleanTbl] = append(t.tableTimestamps[cleanTbl], now)
}

// GetHistory returns recent invalidation records in reverse chronological order.
func (t *InvalidationTracker) GetHistory() []InvalidationRecord {
	t.mu.RLock()
	defer t.mu.RUnlock()

	n := len(t.records)
	res := make([]InvalidationRecord, n)
	for i, r := range t.records {
		res[n-1-i] = r
	}
	return res
}

// GetRecommendations analyzes table write frequency over the last 2 minutes and returns recommendations.
func (t *InvalidationTracker) GetRecommendations() []ChurnRecommendation {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-2 * time.Minute)

	recs := make([]ChurnRecommendation, 0)

	for tbl, timestamps := range t.tableTimestamps {
		// Prune timestamps older than 2 minutes
		validIdx := 0
		for _, ts := range timestamps {
			if ts.After(cutoff) {
				timestamps[validIdx] = ts
				validIdx++
			}
		}
		t.tableTimestamps[tbl] = timestamps[:validIdx]

		count := len(t.tableTimestamps[tbl])
		if count >= 6 { // More than 3 writes/minute
			rate := float64(count) / 2.0
			severity := "MEDIUM"
			if rate >= 15.0 {
				severity = "HIGH"
			}

			recs = append(recs, ChurnRecommendation{
				Table:        tbl,
				WritesPerMin: rate,
				Severity:     severity,
				Message: fmt.Sprintf(
					"Table '%s' receives ~%.1f writes/min, triggering frequent cache evictions. Consider adding '%s' to cache exclusions to eliminate churn.",
					tbl, rate, tbl,
				),
			})
		}
	}

	return recs
}
