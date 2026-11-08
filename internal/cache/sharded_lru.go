package cache

import (
	"hash/fnv"
	"sync"
	"sync/atomic"
	"time"
)

const numShards = 16

type lruNode struct {
	key   string
	entry *CacheEntry
	prev  *lruNode
	next  *lruNode
}

type cacheShard struct {
	mu    sync.RWMutex
	items map[string]*lruNode
	head  *lruNode // Most recently used
	tail  *lruNode // Least recently used
}

func newCacheShard() *cacheShard {
	s := &cacheShard{
		items: make(map[string]*lruNode),
	}
	s.head = &lruNode{}
	s.tail = &lruNode{}
	s.head.next = s.tail
	s.tail.prev = s.head
	return s
}

func (s *cacheShard) get(key string) (*CacheEntry, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	node, found := s.items[key]
	if !found {
		return nil, false
	}

	if node.entry.IsExpired() {
		s.removeNode(node)
		delete(s.items, key)
		return nil, false
	}

	s.moveToHead(node)
	node.entry.HitCount++
	node.entry.LastAccessed = time.Now()
	return node.entry, true
}

func (s *cacheShard) put(entry *CacheEntry) (oldEntry *CacheEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if node, found := s.items[entry.Key]; found {
		oldEntry = node.entry
		node.entry = entry
		s.moveToHead(node)
		return oldEntry
	}

	node := &lruNode{
		key:   entry.Key,
		entry: entry,
	}
	s.items[entry.Key] = node
	s.addToHead(node)
	return nil
}

func (s *cacheShard) delete(key string) *CacheEntry {
	s.mu.Lock()
	defer s.mu.Unlock()

	node, found := s.items[key]
	if !found {
		return nil
	}

	s.removeNode(node)
	delete(s.items, key)
	return node.entry
}

func (s *cacheShard) evictOldest() *CacheEntry {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.items) == 0 {
		return nil
	}

	oldest := s.tail.prev
	if oldest == s.head {
		return nil
	}

	s.removeNode(oldest)
	delete(s.items, oldest.key)
	return oldest.entry
}

func (s *cacheShard) purge() []*CacheEntry {
	s.mu.Lock()
	defer s.mu.Unlock()

	var evicted []*CacheEntry
	for _, node := range s.items {
		evicted = append(evicted, node.entry)
	}

	s.items = make(map[string]*lruNode)
	s.head.next = s.tail
	s.tail.prev = s.head
	return evicted
}

func (s *cacheShard) addToHead(node *lruNode) {
	node.prev = s.head
	node.next = s.head.next
	s.head.next.prev = node
	s.head.next = node
}

func (s *cacheShard) removeNode(node *lruNode) {
	node.prev.next = node.next
	node.next.prev = node.prev
}

func (s *cacheShard) moveToHead(node *lruNode) {
	s.removeNode(node)
	s.addToHead(node)
}

// ShardedLRU implements a concurrent striped LRU cache with memory budget limits.
type ShardedLRU struct {
	shards       [numShards]*cacheShard
	maxBytes     int64
	maxEntries   int64
	totalBytes   atomic.Int64
	totalEntries atomic.Int64
}

// NewShardedLRU initializes a 16-shard LRU cache bounded by memory and entry count.
func NewShardedLRU(maxBytes int64, maxEntries int64) *ShardedLRU {
	c := &ShardedLRU{
		maxBytes:   maxBytes,
		maxEntries: maxEntries,
	}
	for i := 0; i < numShards; i++ {
		c.shards[i] = newCacheShard()
	}
	return c
}

// Get retrieves a cache entry by key.
func (c *ShardedLRU) Get(key string) (*CacheEntry, bool) {
	shard := c.getShard(key)
	return shard.get(key)
}

// Set inserts or updates an entry, evicting the least recently used entries if memory bounds are exceeded.
func (c *ShardedLRU) Set(entry *CacheEntry) (evicted []*CacheEntry) {
	shard := c.getShard(entry.Key)
	oldEntry := shard.put(entry)

	if oldEntry != nil {
		c.totalBytes.Add(entry.SizeBytes - oldEntry.SizeBytes)
	} else {
		c.totalBytes.Add(entry.SizeBytes)
		c.totalEntries.Add(1)
	}

	// Enforce memory and entry limits
	for (c.maxBytes > 0 && c.totalBytes.Load() > c.maxBytes) || (c.maxEntries > 0 && c.totalEntries.Load() > c.maxEntries) {
		// Round-robin eviction across shards
		var evictedOne *CacheEntry
		for i := 0; i < numShards; i++ {
			evictedOne = c.shards[i].evictOldest()
			if evictedOne != nil {
				c.totalBytes.Add(-evictedOne.SizeBytes)
				c.totalEntries.Add(-1)
				evicted = append(evicted, evictedOne)
				break
			}
		}
		if evictedOne == nil {
			break
		}
	}

	return evicted
}

// Delete removes an entry by key.
func (c *ShardedLRU) Delete(key string) *CacheEntry {
	shard := c.getShard(key)
	entry := shard.delete(key)
	if entry != nil {
		c.totalBytes.Add(-entry.SizeBytes)
		c.totalEntries.Add(-1)
	}
	return entry
}

// Purge removes all cached entries and resets memory counters.
func (c *ShardedLRU) Purge() []*CacheEntry {
	var allEvicted []*CacheEntry
	for i := 0; i < numShards; i++ {
		evicted := c.shards[i].purge()
		allEvicted = append(allEvicted, evicted...)
	}
	c.totalBytes.Store(0)
	c.totalEntries.Store(0)
	return allEvicted
}

// MemoryUsage returns the current allocated memory in bytes.
func (c *ShardedLRU) MemoryUsage() int64 {
	return c.totalBytes.Load()
}

// EntryCount returns the current count of cached query items.
func (c *ShardedLRU) EntryCount() int64 {
	return c.totalEntries.Load()
}

func (c *ShardedLRU) getShard(key string) *cacheShard {
	h := fnv.New32a()
	_, _ = h.Write([]byte(key))
	idx := h.Sum32() % numShards
	return c.shards[idx]
}
