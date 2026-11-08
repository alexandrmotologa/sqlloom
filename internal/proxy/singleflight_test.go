package proxy

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alexandrmotologa/sqlloom/internal/cache"
	"github.com/alexandrmotologa/sqlloom/internal/wire"
)

func TestSingleFlightDeduplication(t *testing.T) {
	sf := NewSingleFlightGroup()

	var upstreamCalls atomic.Int64
	var wg sync.WaitGroup

	const numCallers = 25
	results := make([]*cache.CacheEntry, numCallers)
	sharedFlags := make([]bool, numCallers)

	// Simulate 25 concurrent callers asking for the same cold cache key
	for i := 0; i < numCallers; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			entry, shared, err := sf.Do("cold-key-1", func() (*cache.CacheEntry, error) {
				upstreamCalls.Add(1)
				// Simulate 30ms database latency
				time.Sleep(30 * time.Millisecond)
				return cache.NewCacheEntry(
					"cold-key-1",
					"SELECT * FROM settings",
					[]string{"settings"},
					wire.RawMessage{},
					nil,
					wire.RawMessage{},
					time.Hour,
				), nil
			})
			if err != nil {
				t.Errorf("caller %d error: %v", idx, err)
			}
			results[idx] = entry
			sharedFlags[idx] = shared
		}(i)
	}

	wg.Wait()

	// Upstream query MUST have executed only ONCE!
	if calls := upstreamCalls.Load(); calls != 1 {
		t.Fatalf("expected exactly 1 upstream call, got %d", calls)
	}

	// At least some callers must have shared = true
	sharedCount := 0
	for _, s := range sharedFlags {
		if s {
			sharedCount++
		}
	}

	if sharedCount == 0 {
		t.Fatalf("expected concurrent callers to receive shared execution, got 0")
	}

	// Verify all received the same entry
	for i, res := range results {
		if res == nil || res.Key != "cold-key-1" {
			t.Fatalf("caller %d received invalid entry: %v", i, res)
		}
	}
}
