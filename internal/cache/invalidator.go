package cache

import (
	"strings"
	"sync"
)

// Invalidator manages table-to-query reverse dependencies and coordinates invalidation.
type Invalidator struct {
	mu          sync.RWMutex
	cache       *ShardedLRU
	tableToKeys map[string]map[string]struct{}
	keyToTables map[string][]string
}

// NewInvalidator initializes the invalidation coordinator over a ShardedLRU cache.
func NewInvalidator(cache *ShardedLRU) *Invalidator {
	return &Invalidator{
		cache:       cache,
		tableToKeys: make(map[string]map[string]struct{}),
		keyToTables: make(map[string][]string),
	}
}

// Register associates a cache entry with all its dependent tables.
func (inv *Invalidator) Register(entry *CacheEntry) {
	if entry == nil || len(entry.Tables) == 0 {
		return
	}

	evicted := inv.cache.Set(entry)

	inv.mu.Lock()
	defer inv.mu.Unlock()

	// Clean up any LRU evicted items from index
	for _, ev := range evicted {
		inv.unindexLocked(ev.Key)
	}

	// Index new entry
	inv.keyToTables[entry.Key] = entry.Tables
	for _, rawTbl := range entry.Tables {
		tbl := strings.ToLower(rawTbl)
		if _, ok := inv.tableToKeys[tbl]; !ok {
			inv.tableToKeys[tbl] = make(map[string]struct{})
		}
		inv.tableToKeys[tbl][entry.Key] = struct{}{}
	}
}

// InvalidateTables purges all cached queries dependent on any of the given tables.
func (inv *Invalidator) InvalidateTables(tables []string) []string {
	if len(tables) == 0 {
		return nil
	}

	inv.mu.Lock()
	var keysToEvict []string
	seen := make(map[string]struct{})

	for _, rawTbl := range tables {
		tbl := strings.ToLower(rawTbl)
		if keySet, ok := inv.tableToKeys[tbl]; ok {
			for k := range keySet {
				if _, already := seen[k]; !already {
					seen[k] = struct{}{}
					keysToEvict = append(keysToEvict, k)
				}
			}
			delete(inv.tableToKeys, tbl)
		}
	}

	// Also purge inverse mappings
	for _, k := range keysToEvict {
		inv.unindexLocked(k)
	}
	inv.mu.Unlock()

	// Remove from LRU cache
	for _, k := range keysToEvict {
		inv.cache.Delete(k)
	}

	return keysToEvict
}

// PurgeTable manually evicts all queries referencing a specific table.
func (inv *Invalidator) PurgeTable(table string) int {
	keys := inv.InvalidateTables([]string{table})
	return len(keys)
}

// PurgeAll clears all cached entries and the dependency graph.
func (inv *Invalidator) PurgeAll() int {
	inv.mu.Lock()
	inv.tableToKeys = make(map[string]map[string]struct{})
	inv.keyToTables = make(map[string][]string)
	inv.mu.Unlock()

	evicted := inv.cache.Purge()
	return len(evicted)
}

// GetTableStats returns the count of cached query templates per table.
func (inv *Invalidator) GetTableStats() map[string]int {
	inv.mu.RLock()
	defer inv.mu.RUnlock()

	stats := make(map[string]int, len(inv.tableToKeys))
	for tbl, keys := range inv.tableToKeys {
		stats[tbl] = len(keys)
	}
	return stats
}

func (inv *Invalidator) unindexLocked(key string) {
	if tables, ok := inv.keyToTables[key]; ok {
		for _, rawTbl := range tables {
			tbl := strings.ToLower(rawTbl)
			if keySet, exists := inv.tableToKeys[tbl]; exists {
				delete(keySet, key)
				if len(keySet) == 0 {
					delete(inv.tableToKeys, tbl)
				}
			}
		}
		delete(inv.keyToTables, key)
	}
}
