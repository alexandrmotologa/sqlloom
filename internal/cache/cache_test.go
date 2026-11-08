package cache

import (
	"fmt"
	"testing"
	"time"

	"github.com/alexandrmotologa/sqlloom/internal/wire"
)

func TestShardedLRUBasic(t *testing.T) {
	cache := NewShardedLRU(1024*1024, 100)

	entry := NewCacheEntry(
		"key-1",
		"SELECT * FROM users",
		[]string{"users"},
		wire.RawMessage{Type: wire.MsgRowDescription, Payload: []byte{0, 1}},
		[]wire.RawMessage{{Type: wire.MsgDataRow, Payload: []byte("row-data")}},
		wire.RawMessage{Type: wire.MsgCommandComplete, Payload: []byte("SELECT 1")},
		0,
	)

	cache.Set(entry)

	got, found := cache.Get("key-1")
	if !found {
		t.Fatalf("expected to find key-1")
	}
	if got.Query != entry.Query {
		t.Fatalf("expected query %s, got %s", entry.Query, got.Query)
	}
	if got.HitCount != 1 {
		t.Fatalf("expected hit count 1, got %d", got.HitCount)
	}

	// Delete
	del := cache.Delete("key-1")
	if del == nil {
		t.Fatalf("expected delete to return entry")
	}

	_, found = cache.Get("key-1")
	if found {
		t.Fatalf("expected key-1 to be deleted")
	}
}

func TestShardedLRUEviction(t *testing.T) {
	// Set very small max entries: 3
	cache := NewShardedLRU(10*1024*1024, 3)

	for i := 0; i < 10; i++ {
		key := fmt.Sprintf("k-%d", i)
		entry := NewCacheEntry(
			key,
			"SELECT 1",
			[]string{"t"},
			wire.RawMessage{},
			nil,
			wire.RawMessage{},
			0,
		)
		cache.Set(entry)
	}

	count := cache.EntryCount()
	if count > 3 {
		t.Fatalf("expected count <= 3 after eviction, got %d", count)
	}
}

func TestTableInvalidation(t *testing.T) {
	cache := NewShardedLRU(1024*1024, 100)
	inv := NewInvalidator(cache)

	// Register 2 queries touching users, 1 query touching products
	e1 := NewCacheEntry("q-users-1", "SELECT id FROM users", []string{"users"}, wire.RawMessage{}, nil, wire.RawMessage{}, 0)
	e2 := NewCacheEntry("q-users-2", "SELECT name FROM users WHERE id=1", []string{"users"}, wire.RawMessage{}, nil, wire.RawMessage{}, 0)
	e3 := NewCacheEntry("q-prod-1", "SELECT * FROM products", []string{"products"}, wire.RawMessage{}, nil, wire.RawMessage{}, 0)

	inv.Register(e1)
	inv.Register(e2)
	inv.Register(e3)

	stats := inv.GetTableStats()
	if stats["users"] != 2 {
		t.Fatalf("expected 2 queries for users, got %d", stats["users"])
	}
	if stats["products"] != 1 {
		t.Fatalf("expected 1 query for products, got %d", stats["products"])
	}

	// Write to users table triggers invalidation
	evicted := inv.InvalidateTables([]string{"users"})
	if len(evicted) != 2 {
		t.Fatalf("expected 2 evicted keys, got %d", len(evicted))
	}

	// Verify users queries purged from cache
	if _, found := cache.Get("q-users-1"); found {
		t.Fatalf("expected q-users-1 to be purged")
	}
	if _, found := cache.Get("q-users-2"); found {
		t.Fatalf("expected q-users-2 to be purged")
	}

	// Products query should still be intact
	if _, found := cache.Get("q-prod-1"); !found {
		t.Fatalf("expected q-prod-1 to still exist")
	}

	// Stats should reflect change
	statsAfter := inv.GetTableStats()
	if _, exists := statsAfter["users"]; exists {
		t.Fatalf("expected users table to have 0 entries in stats")
	}
	if statsAfter["products"] != 1 {
		t.Fatalf("expected products table to still have 1 entry")
	}
}

func TestCacheExpirationTTL(t *testing.T) {
	cache := NewShardedLRU(1024*1024, 100)

	// Short TTL of 50ms
	entry := NewCacheEntry(
		"ttl-key",
		"SELECT 1",
		[]string{"tbl"},
		wire.RawMessage{},
		nil,
		wire.RawMessage{},
		50*time.Millisecond,
	)

	cache.Set(entry)

	// Immediate get
	if _, found := cache.Get("ttl-key"); !found {
		t.Fatalf("expected entry to be found immediately")
	}

	// Wait for TTL expiration
	time.Sleep(70 * time.Millisecond)

	if _, found := cache.Get("ttl-key"); found {
		t.Fatalf("expected entry to be expired")
	}
}
