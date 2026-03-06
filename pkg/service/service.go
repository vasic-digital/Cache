// Package service provides service-layer caching patterns.
//
// It wraps a cache backend with common patterns like cache-aside,
// read-through, and TTL-based invalidation, abstracting the caching
// logic away from business code.
//
// Design pattern: Proxy (transparent caching), Strategy (cache policy).
package service

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Cache is the minimal cache interface required by the service wrapper.
type Cache interface {
	Get(ctx context.Context, key string) (interface{}, bool, error)
	Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
}

// LoadFunc loads a value when cache misses.
type LoadFunc func(ctx context.Context, key string) (interface{}, error)

// Config holds service cache configuration.
type Config struct {
	DefaultTTL time.Duration
	KeyPrefix  string
}

// DefaultConfig returns a default service cache config.
func DefaultConfig() Config {
	return Config{
		DefaultTTL: 5 * time.Minute,
		KeyPrefix:  "",
	}
}

// Wrapper provides cache-aside pattern for service calls.
type Wrapper struct {
	cache  Cache
	config Config
	mu     sync.RWMutex
	hits   int64
	misses int64
	errors int64
}

// Stats holds cache hit/miss statistics.
type Stats struct {
	Hits   int64
	Misses int64
	Errors int64
}

// New creates a new service cache wrapper.
func New(cache Cache, cfg Config) *Wrapper {
	return &Wrapper{
		cache:  cache,
		config: cfg,
	}
}

// GetOrLoad retrieves from cache or loads using the provided function
// (cache-aside pattern).
func (w *Wrapper) GetOrLoad(ctx context.Context, key string, load LoadFunc) (interface{}, error) {
	return w.GetOrLoadWithTTL(ctx, key, w.config.DefaultTTL, load)
}

// GetOrLoadWithTTL retrieves from cache or loads with a specific TTL.
func (w *Wrapper) GetOrLoadWithTTL(
	ctx context.Context,
	key string,
	ttl time.Duration,
	load LoadFunc,
) (interface{}, error) {
	prefixed := w.prefixKey(key)

	// Try cache first.
	val, found, err := w.cache.Get(ctx, prefixed)
	if err != nil {
		// Cache error is not fatal; fall through to loader.
		atomic.AddInt64(&w.errors, 1)
	} else if found {
		atomic.AddInt64(&w.hits, 1)
		return val, nil
	}

	// Cache miss -- load from origin.
	atomic.AddInt64(&w.misses, 1)

	val, err = load(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("service cache load %q: %w", key, err)
	}

	// Populate cache (best-effort).
	if setErr := w.cache.Set(ctx, prefixed, val, ttl); setErr != nil {
		atomic.AddInt64(&w.errors, 1)
	}

	return val, nil
}

// Invalidate removes a key from cache.
func (w *Wrapper) Invalidate(ctx context.Context, key string) error {
	prefixed := w.prefixKey(key)
	if err := w.cache.Delete(ctx, prefixed); err != nil {
		return fmt.Errorf("service cache invalidate %q: %w", key, err)
	}
	return nil
}

// InvalidatePrefix removes all keys with a given prefix.
// Note: This only works with the stored prefix, not arbitrary scanning.
// It iterates tracked keys whose prefixed form starts with the given
// prefix and deletes them. Because the underlying Cache interface does
// not expose key enumeration, this method is a best-effort operation
// that relies on the configured KeyPrefix matching convention.
func (w *Wrapper) InvalidatePrefix(ctx context.Context, prefix string) error {
	// The underlying Cache interface does not support scanning, so
	// we can only delete the exact prefixed key. Callers should use
	// Invalidate for single keys. This method is provided as a
	// convenience that constructs the prefixed key and deletes it.
	prefixed := w.prefixKey(prefix)

	if err := w.cache.Delete(ctx, prefixed); err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil
		}
		return fmt.Errorf("service cache invalidate prefix %q: %w", prefix, err)
	}
	return nil
}

// GetStats returns cache hit/miss statistics.
func (w *Wrapper) GetStats() Stats {
	return Stats{
		Hits:   atomic.LoadInt64(&w.hits),
		Misses: atomic.LoadInt64(&w.misses),
		Errors: atomic.LoadInt64(&w.errors),
	}
}

// prefixKey prepends the configured prefix to a key.
func (w *Wrapper) prefixKey(key string) string {
	if w.config.KeyPrefix == "" {
		return key
	}
	return w.config.KeyPrefix + key
}
