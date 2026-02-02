// Package memory provides a thread-safe, in-memory cache implementation
// with configurable eviction policies (LRU, LFU, FIFO), maximum entry
// limits, maximum memory limits, and background expiration cleanup.
package memory

import (
	"container/list"
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"digital.vasic.cache/pkg/cache"
)

// Config holds configuration for the in-memory cache.
type Config struct {
	// MaxEntries is the maximum number of entries. 0 means unlimited.
	MaxEntries int
	// MaxMemoryBytes is the maximum memory budget in bytes. 0 means unlimited.
	MaxMemoryBytes int64
	// DefaultTTL applied when Set is called with zero TTL.
	DefaultTTL time.Duration
	// CleanupInterval is how often expired entries are removed.
	CleanupInterval time.Duration
	// EvictionPolicy determines which entries to evict on capacity.
	EvictionPolicy cache.EvictionPolicy
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		MaxEntries:      10000,
		MaxMemoryBytes:  0, // unlimited
		DefaultTTL:      30 * time.Minute,
		CleanupInterval: time.Minute,
		EvictionPolicy:  cache.LRU,
	}
}

// entry represents a single cached item.
type entry struct {
	key       string
	value     []byte
	expiresAt time.Time
	createdAt time.Time
	accessCnt int64 // for LFU
	element   *list.Element
}

// Cache is a thread-safe in-memory cache that implements the
// cache.Cache interface. It supports LRU, LFU, and FIFO eviction.
type Cache struct {
	mu sync.RWMutex

	entries map[string]*entry
	// evictList is used for LRU and FIFO ordering.
	evictList *list.List

	config *Config

	// stats
	hits      int64
	misses    int64
	evictions int64
	memUsed   int64

	// background cleanup
	ctx    context.Context
	cancel context.CancelFunc
}

// New creates a new in-memory Cache and starts the background
// cleanup goroutine.
func New(cfg *Config) *Cache {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	c := &Cache{
		entries:   make(map[string]*entry),
		evictList: list.New(),
		config:    cfg,
		ctx:       ctx,
		cancel:    cancel,
	}

	if cfg.CleanupInterval > 0 {
		go c.cleanupLoop()
	}

	return c
}

// Get retrieves a value by key. Returns nil, nil on cache miss.
func (c *Cache) Get(_ context.Context, key string) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	e, ok := c.entries[key]
	if !ok {
		atomic.AddInt64(&c.misses, 1)
		return nil, nil
	}

	// Check expiration
	if !e.expiresAt.IsZero() && time.Now().After(e.expiresAt) {
		c.removeEntry(e)
		atomic.AddInt64(&c.misses, 1)
		return nil, nil
	}

	// Update access for eviction policy
	atomic.AddInt64(&e.accessCnt, 1)
	if c.config.EvictionPolicy == cache.LRU {
		c.evictList.MoveToFront(e.element)
	}

	atomic.AddInt64(&c.hits, 1)

	// Return a copy to avoid data races on the returned slice
	result := make([]byte, len(e.value))
	copy(result, e.value)
	return result, nil
}

// Set stores a value with the given TTL.
func (c *Cache) Set(_ context.Context, key string, value []byte, ttl time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if ttl == 0 {
		ttl = c.config.DefaultTTL
	}

	var expiresAt time.Time
	if ttl > 0 {
		expiresAt = time.Now().Add(ttl)
	}

	// Store a copy
	valCopy := make([]byte, len(value))
	copy(valCopy, value)

	// If key already exists, update it
	if existing, ok := c.entries[key]; ok {
		atomic.AddInt64(&c.memUsed, int64(len(valCopy)-len(existing.value)))
		existing.value = valCopy
		existing.expiresAt = expiresAt
		if c.config.EvictionPolicy == cache.LRU {
			c.evictList.MoveToFront(existing.element)
		}
		return nil
	}

	// Check capacity and evict if necessary
	if c.config.MaxEntries > 0 && len(c.entries) >= c.config.MaxEntries {
		c.evict()
	}

	// Check memory and evict if necessary
	if c.config.MaxMemoryBytes > 0 {
		for atomic.LoadInt64(&c.memUsed)+int64(len(valCopy)) > c.config.MaxMemoryBytes &&
			len(c.entries) > 0 {
			c.evict()
		}
	}

	e := &entry{
		key:       key,
		value:     valCopy,
		expiresAt: expiresAt,
		createdAt: time.Now(),
	}

	// For LRU: new entries go to front. For FIFO: new entries go to back.
	switch c.config.EvictionPolicy {
	case cache.FIFO:
		e.element = c.evictList.PushBack(e)
	default: // LRU, LFU
		e.element = c.evictList.PushFront(e)
	}

	c.entries[key] = e
	atomic.AddInt64(&c.memUsed, int64(len(valCopy)))

	return nil
}

// Delete removes a key from the cache.
func (c *Cache) Delete(_ context.Context, key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if e, ok := c.entries[key]; ok {
		c.removeEntry(e)
	}
	return nil
}

// Exists reports whether a key is present and not expired.
func (c *Cache) Exists(_ context.Context, key string) (bool, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	e, ok := c.entries[key]
	if !ok {
		return false, nil
	}
	if !e.expiresAt.IsZero() && time.Now().After(e.expiresAt) {
		return false, nil
	}
	return true, nil
}

// Close stops the background cleanup goroutine and releases resources.
func (c *Cache) Close() error {
	c.cancel()
	return nil
}

// Stats returns current cache statistics.
func (c *Cache) Stats() *cache.Stats {
	c.mu.RLock()
	size := int64(len(c.entries))
	c.mu.RUnlock()

	return &cache.Stats{
		Hits:      atomic.LoadInt64(&c.hits),
		Misses:    atomic.LoadInt64(&c.misses),
		Evictions: atomic.LoadInt64(&c.evictions),
		Size:      size,
	}
}

// Len returns the current number of entries (including expired ones
// not yet cleaned up).
func (c *Cache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}

// MemoryUsed returns an approximation of the memory used by cached
// values in bytes. This does not include map/list overhead.
func (c *Cache) MemoryUsed() int64 {
	return atomic.LoadInt64(&c.memUsed)
}

// evict removes one entry according to the eviction policy.
// Caller must hold c.mu write lock.
func (c *Cache) evict() {
	switch c.config.EvictionPolicy {
	case cache.LFU:
		c.evictLFU()
	case cache.FIFO:
		c.evictFIFO()
	default: // LRU
		c.evictLRU()
	}
}

// evictLRU removes the least recently used entry (back of the list).
func (c *Cache) evictLRU() {
	elem := c.evictList.Back()
	if elem == nil {
		return
	}
	e := elem.Value.(*entry)
	c.removeEntry(e)
	atomic.AddInt64(&c.evictions, 1)
}

// evictLFU removes the least frequently used entry.
func (c *Cache) evictLFU() {
	var victim *entry
	var minAccess int64 = -1

	for _, e := range c.entries {
		cnt := atomic.LoadInt64(&e.accessCnt)
		if minAccess < 0 || cnt < minAccess {
			minAccess = cnt
			victim = e
		}
	}

	if victim != nil {
		c.removeEntry(victim)
		atomic.AddInt64(&c.evictions, 1)
	}
}

// evictFIFO removes the oldest entry (front of the list since we
// PushBack for FIFO).
func (c *Cache) evictFIFO() {
	elem := c.evictList.Front()
	if elem == nil {
		return
	}
	e := elem.Value.(*entry)
	c.removeEntry(e)
	atomic.AddInt64(&c.evictions, 1)
}

// removeEntry removes an entry from both the map and the eviction
// list. Caller must hold c.mu write lock.
func (c *Cache) removeEntry(e *entry) {
	delete(c.entries, e.key)
	if e.element != nil {
		c.evictList.Remove(e.element)
	}
	atomic.AddInt64(&c.memUsed, -int64(len(e.value)))
}

// cleanupLoop runs periodically to remove expired entries.
func (c *Cache) cleanupLoop() {
	ticker := time.NewTicker(c.config.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			c.cleanup()
		}
	}
}

// cleanup removes all expired entries.
func (c *Cache) cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	for _, e := range c.entries {
		if !e.expiresAt.IsZero() && now.After(e.expiresAt) {
			c.removeEntry(e)
		}
	}
}

// Compile-time interface check.
var _ cache.Cache = (*Cache)(nil)

// Flush removes all entries from the cache.
func (c *Cache) Flush() {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, e := range c.entries {
		c.removeEntry(e)
	}
}

// FormatSize returns a human-readable string for a byte count.
func FormatSize(bytes int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)
	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.2f GB", float64(bytes)/float64(GB))
	case bytes >= MB:
		return fmt.Sprintf("%.2f MB", float64(bytes)/float64(MB))
	case bytes >= KB:
		return fmt.Sprintf("%.2f KB", float64(bytes)/float64(KB))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}
