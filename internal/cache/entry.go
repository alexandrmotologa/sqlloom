package cache

import (
	"time"

	"github.com/alexandrmotologa/sqlloom/internal/wire"
)

// CacheEntry stores a cached PostgreSQL result set and metadata.
type CacheEntry struct {
	Key                    string
	Query                  string
	Tables                 []string
	RowDescription         wire.RawMessage
	DataRows               []wire.RawMessage
	CommandComplete        wire.RawMessage
	SizeBytes              int64
	UpstreamDurationMicros int64
	CreatedAt              time.Time
	ExpiresAt              time.Time
	HitCount               int64
	LastAccessed           time.Time
}

// NewCacheEntry constructs an entry and computes its memory footprint.
func NewCacheEntry(
	key string,
	query string,
	tables []string,
	rowDesc wire.RawMessage,
	rows []wire.RawMessage,
	complete wire.RawMessage,
	ttl time.Duration,
) *CacheEntry {
	now := time.Now()
	var expiresAt time.Time
	if ttl > 0 {
		expiresAt = now.Add(ttl)
	}

	entry := &CacheEntry{
		Key:             key,
		Query:           query,
		Tables:          tables,
		RowDescription:  rowDesc,
		DataRows:        rows,
		CommandComplete: complete,
		CreatedAt:       now,
		ExpiresAt:       expiresAt,
		LastAccessed:    now,
	}

	entry.SizeBytes = entry.computeSize()
	return entry
}

// IsExpired checks if the entry has passed its TTL.
func (e *CacheEntry) IsExpired() bool {
	if e.ExpiresAt.IsZero() {
		return false
	}
	return time.Now().After(e.ExpiresAt)
}

func (e *CacheEntry) computeSize() int64 {
	// Base struct overhead
	size := int64(128)
	size += int64(len(e.Key))
	size += int64(len(e.Query))

	for _, t := range e.Tables {
		size += int64(len(t) + 16)
	}

	size += int64(len(e.RowDescription.Payload) + 16)
	size += int64(len(e.CommandComplete.Payload) + 16)

	for _, r := range e.DataRows {
		size += int64(len(r.Payload) + 16)
	}

	return size
}
