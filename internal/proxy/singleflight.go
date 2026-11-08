package proxy

import (
	"sync"

	"github.com/alexandrmotologa/sqlloom/internal/cache"
)

type call struct {
	wg    sync.WaitGroup
	entry *cache.CacheEntry
	err   error
}

// SingleFlightGroup suppresses duplicate concurrent upstream queries for the same cache key.
type SingleFlightGroup struct {
	mu sync.Mutex
	m  map[string]*call
}

// NewSingleFlightGroup initializes a SingleFlightGroup.
func NewSingleFlightGroup() *SingleFlightGroup {
	return &SingleFlightGroup{
		m: make(map[string]*call),
	}
}

// Do executes and returns the results of the given function, making sure that only one execution
// is in-flight for a given key at a time. If a duplicate comes in, the duplicate caller waits
// and receives the same CacheEntry result.
func (g *SingleFlightGroup) Do(key string, fn func() (*cache.CacheEntry, error)) (*cache.CacheEntry, bool, error) {
	g.mu.Lock()
	if c, ok := g.m[key]; ok {
		g.mu.Unlock()
		c.wg.Wait()
		return c.entry, true, c.err // true = shared execution (thundering herd suppressed)
	}

	c := &call{}
	c.wg.Add(1)
	g.m[key] = c
	g.mu.Unlock()

	c.entry, c.err = fn()
	c.wg.Done()

	g.mu.Lock()
	delete(g.m, key)
	g.mu.Unlock()

	return c.entry, false, c.err // false = leader execution
}
