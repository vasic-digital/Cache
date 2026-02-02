// Package cache provides the core interfaces and types for the Cache module.
//
// This package defines the Cache interface that all cache implementations
// must satisfy, along with typed wrappers, configuration, statistics,
// and eviction policy enumerations.
package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// Cache is the core interface that all cache backends must implement.
// It operates on raw byte slices to remain serialization-agnostic.
type Cache interface {
	// Get retrieves a value by key. Returns nil, nil when the key
	// does not exist (cache miss).
	Get(ctx context.Context, key string) ([]byte, error)

	// Set stores a value with an optional TTL. A zero TTL means
	// the entry does not expire (implementation-defined behaviour).
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error

	// Delete removes a single key. Deleting a non-existent key is
	// not an error.
	Delete(ctx context.Context, key string) error

	// Exists reports whether the key is present in the cache.
	Exists(ctx context.Context, key string) (bool, error)

	// Close releases any resources held by the cache.
	Close() error
}

// EvictionPolicy determines how entries are evicted when the cache
// reaches its capacity limit.
type EvictionPolicy int

const (
	// LRU evicts the least recently used entry.
	LRU EvictionPolicy = iota
	// LFU evicts the least frequently used entry.
	LFU
	// FIFO evicts the oldest entry (first-in, first-out).
	FIFO
)

// String returns the human-readable name of the eviction policy.
func (p EvictionPolicy) String() string {
	switch p {
	case LRU:
		return "LRU"
	case LFU:
		return "LFU"
	case FIFO:
		return "FIFO"
	default:
		return fmt.Sprintf("EvictionPolicy(%d)", int(p))
	}
}

// Config holds general cache configuration.
type Config struct {
	// DefaultTTL applied when a Set call specifies zero TTL.
	DefaultTTL time.Duration
	// MaxSize is the maximum number of entries (0 = unlimited).
	MaxSize int
	// EvictionPolicy to use when MaxSize is reached.
	EvictionPolicy EvictionPolicy
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		DefaultTTL:     30 * time.Minute,
		MaxSize:        10000,
		EvictionPolicy: LRU,
	}
}

// Stats captures runtime cache statistics.
type Stats struct {
	Hits      int64 `json:"hits"`
	Misses    int64 `json:"misses"`
	Evictions int64 `json:"evictions"`
	Size      int64 `json:"size"`
}

// HitRate returns the hit ratio as a percentage (0-100).
// Returns 0 when there have been no requests.
func (s *Stats) HitRate() float64 {
	total := s.Hits + s.Misses
	if total == 0 {
		return 0
	}
	return float64(s.Hits) / float64(total) * 100
}

// TypedCache is a generic wrapper around Cache that handles
// JSON serialization and deserialization for a concrete type T.
type TypedCache[T any] struct {
	inner Cache
}

// NewTypedCache wraps an existing Cache with typed Get/Set methods.
func NewTypedCache[T any](c Cache) *TypedCache[T] {
	return &TypedCache[T]{inner: c}
}

// Get retrieves and deserializes a value of type T.
// Returns the zero value of T and nil error on a cache miss.
func (tc *TypedCache[T]) Get(ctx context.Context, key string) (T, bool, error) {
	var zero T
	data, err := tc.inner.Get(ctx, key)
	if err != nil {
		return zero, false, fmt.Errorf("typed cache get: %w", err)
	}
	if data == nil {
		return zero, false, nil
	}
	var val T
	if err := json.Unmarshal(data, &val); err != nil {
		return zero, false, fmt.Errorf("typed cache unmarshal: %w", err)
	}
	return val, true, nil
}

// Set serializes a value of type T and stores it.
func (tc *TypedCache[T]) Set(ctx context.Context, key string, value T, ttl time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("typed cache marshal: %w", err)
	}
	return tc.inner.Set(ctx, key, data, ttl)
}

// Delete removes a key from the underlying cache.
func (tc *TypedCache[T]) Delete(ctx context.Context, key string) error {
	return tc.inner.Delete(ctx, key)
}

// Exists checks whether a key exists in the underlying cache.
func (tc *TypedCache[T]) Exists(ctx context.Context, key string) (bool, error) {
	return tc.inner.Exists(ctx, key)
}

// Close closes the underlying cache.
func (tc *TypedCache[T]) Close() error {
	return tc.inner.Close()
}
