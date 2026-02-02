// Package distributed provides distributed cache patterns including
// consistent hashing for node selection, two-level caching (local +
// remote), and cache write strategies (write-through, write-back,
// cache-aside).
package distributed

import (
	"context"
	"crypto/md5"
	"encoding/binary"
	"fmt"
	"sort"
	"sync"
	"time"

	"digital.vasic.cache/pkg/cache"
)

// --- Consistent Hashing ---

// ConsistentHash provides consistent hashing for distributing cache
// keys across a set of nodes. It uses virtual nodes (replicas) to
// improve distribution uniformity.
type ConsistentHash struct {
	mu       sync.RWMutex
	ring     []uint32         // sorted hash values
	nodes    map[uint32]string // hash -> node name
	replicas int
}

// NewConsistentHash creates a consistent hash ring. The replicas
// parameter controls how many virtual nodes each physical node gets
// (higher values give more uniform distribution).
func NewConsistentHash(replicas int) *ConsistentHash {
	if replicas <= 0 {
		replicas = 100
	}
	return &ConsistentHash{
		nodes:    make(map[uint32]string),
		replicas: replicas,
	}
}

// AddNode adds a node to the hash ring.
func (ch *ConsistentHash) AddNode(node string) {
	ch.mu.Lock()
	defer ch.mu.Unlock()

	for i := 0; i < ch.replicas; i++ {
		h := ch.hash(fmt.Sprintf("%s:%d", node, i))
		ch.ring = append(ch.ring, h)
		ch.nodes[h] = node
	}
	sort.Slice(ch.ring, func(i, j int) bool { return ch.ring[i] < ch.ring[j] })
}

// RemoveNode removes a node from the hash ring.
func (ch *ConsistentHash) RemoveNode(node string) {
	ch.mu.Lock()
	defer ch.mu.Unlock()

	newRing := make([]uint32, 0, len(ch.ring))
	for _, h := range ch.ring {
		if ch.nodes[h] == node {
			delete(ch.nodes, h)
		} else {
			newRing = append(newRing, h)
		}
	}
	ch.ring = newRing
}

// GetNode returns the node responsible for the given key. Returns
// an empty string if there are no nodes.
func (ch *ConsistentHash) GetNode(key string) string {
	ch.mu.RLock()
	defer ch.mu.RUnlock()

	if len(ch.ring) == 0 {
		return ""
	}

	h := ch.hash(key)
	idx := sort.Search(len(ch.ring), func(i int) bool {
		return ch.ring[i] >= h
	})
	if idx >= len(ch.ring) {
		idx = 0
	}
	return ch.nodes[ch.ring[idx]]
}

// NodeCount returns the number of unique physical nodes.
func (ch *ConsistentHash) NodeCount() int {
	ch.mu.RLock()
	defer ch.mu.RUnlock()

	unique := make(map[string]struct{})
	for _, node := range ch.nodes {
		unique[node] = struct{}{}
	}
	return len(unique)
}

// hash produces a uint32 hash of the key using MD5 (not for
// security; just for distribution).
func (ch *ConsistentHash) hash(key string) uint32 {
	// #nosec G401 -- MD5 used for hashing, not security
	h := md5.Sum([]byte(key))
	return binary.BigEndian.Uint32(h[:4])
}

// --- Two-Level Cache ---

// TwoLevel combines a local (L1) cache with a remote (L2) cache.
// Reads check L1 first and promote L2 hits to L1. Writes go to
// both levels.
type TwoLevel struct {
	local  cache.Cache
	remote cache.Cache
	l1TTL  time.Duration
}

// NewTwoLevel creates a two-level cache. l1TTL controls how long
// entries promoted from L2 remain in L1.
func NewTwoLevel(local, remote cache.Cache, l1TTL time.Duration) *TwoLevel {
	if l1TTL <= 0 {
		l1TTL = 5 * time.Minute
	}
	return &TwoLevel{
		local:  local,
		remote: remote,
		l1TTL:  l1TTL,
	}
}

// Get checks local first, then remote. Remote hits are promoted to
// local.
func (t *TwoLevel) Get(ctx context.Context, key string) ([]byte, error) {
	data, err := t.local.Get(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("two-level local get: %w", err)
	}
	if data != nil {
		return data, nil
	}

	data, err = t.remote.Get(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("two-level remote get: %w", err)
	}
	if data != nil {
		// Promote to local
		_ = t.local.Set(ctx, key, data, t.l1TTL)
	}
	return data, nil
}

// Set writes to both local and remote caches.
func (t *TwoLevel) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	l1TTL := t.l1TTL
	if ttl > 0 && ttl < l1TTL {
		l1TTL = ttl
	}

	if err := t.local.Set(ctx, key, value, l1TTL); err != nil {
		return fmt.Errorf("two-level local set: %w", err)
	}
	if err := t.remote.Set(ctx, key, value, ttl); err != nil {
		return fmt.Errorf("two-level remote set: %w", err)
	}
	return nil
}

// Delete removes a key from both caches.
func (t *TwoLevel) Delete(ctx context.Context, key string) error {
	if err := t.local.Delete(ctx, key); err != nil {
		return fmt.Errorf("two-level local delete: %w", err)
	}
	if err := t.remote.Delete(ctx, key); err != nil {
		return fmt.Errorf("two-level remote delete: %w", err)
	}
	return nil
}

// Exists checks both caches.
func (t *TwoLevel) Exists(ctx context.Context, key string) (bool, error) {
	ok, err := t.local.Exists(ctx, key)
	if err != nil {
		return false, fmt.Errorf("two-level local exists: %w", err)
	}
	if ok {
		return true, nil
	}
	ok, err = t.remote.Exists(ctx, key)
	if err != nil {
		return false, fmt.Errorf("two-level remote exists: %w", err)
	}
	return ok, nil
}

// Close closes both caches.
func (t *TwoLevel) Close() error {
	errL := t.local.Close()
	errR := t.remote.Close()
	if errL != nil {
		return errL
	}
	return errR
}

// Compile-time check
var _ cache.Cache = (*TwoLevel)(nil)

// --- Write Strategies ---

// Strategy defines the interface for cache write strategies.
type Strategy interface {
	// Name returns the strategy name.
	Name() string
	// Get retrieves a value, using the strategy's read path.
	Get(ctx context.Context, key string) ([]byte, error)
	// Set writes a value using the strategy's write path.
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
	// Delete removes a value using the strategy's delete path.
	Delete(ctx context.Context, key string) error
}

// DataSource is an abstraction for the backing data store used by
// cache strategies. Implementations fetch/store canonical data.
type DataSource interface {
	// Load retrieves data from the source.
	Load(ctx context.Context, key string) ([]byte, error)
	// Store persists data to the source.
	Store(ctx context.Context, key string, value []byte) error
	// Remove deletes data from the source.
	Remove(ctx context.Context, key string) error
}

// --- Write-Through ---

// WriteThrough writes to the cache and the backing store
// synchronously on every Set.
type WriteThrough struct {
	c   cache.Cache
	src DataSource
}

// NewWriteThrough creates a write-through strategy.
func NewWriteThrough(c cache.Cache, src DataSource) *WriteThrough {
	return &WriteThrough{c: c, src: src}
}

func (w *WriteThrough) Name() string { return "write-through" }

func (w *WriteThrough) Get(ctx context.Context, key string) ([]byte, error) {
	data, err := w.c.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	if data != nil {
		return data, nil
	}
	// Cache miss -- load from source
	data, err = w.src.Load(ctx, key)
	if err != nil {
		return nil, err
	}
	if data != nil {
		_ = w.c.Set(ctx, key, data, 0)
	}
	return data, nil
}

func (w *WriteThrough) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	// Write to source first
	if err := w.src.Store(ctx, key, value); err != nil {
		return fmt.Errorf("write-through source store: %w", err)
	}
	// Then write to cache
	if err := w.c.Set(ctx, key, value, ttl); err != nil {
		return fmt.Errorf("write-through cache set: %w", err)
	}
	return nil
}

func (w *WriteThrough) Delete(ctx context.Context, key string) error {
	if err := w.src.Remove(ctx, key); err != nil {
		return fmt.Errorf("write-through source remove: %w", err)
	}
	return w.c.Delete(ctx, key)
}

// --- Write-Back ---

// WriteBack writes to the cache immediately and asynchronously
// flushes to the backing store. Dirty entries are tracked and
// flushed on Flush() or Close().
type WriteBack struct {
	c     cache.Cache
	src   DataSource
	dirty map[string][]byte
	mu    sync.Mutex
}

// NewWriteBack creates a write-back strategy.
func NewWriteBack(c cache.Cache, src DataSource) *WriteBack {
	return &WriteBack{
		c:     c,
		src:   src,
		dirty: make(map[string][]byte),
	}
}

func (w *WriteBack) Name() string { return "write-back" }

func (w *WriteBack) Get(ctx context.Context, key string) ([]byte, error) {
	data, err := w.c.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	if data != nil {
		return data, nil
	}
	data, err = w.src.Load(ctx, key)
	if err != nil {
		return nil, err
	}
	if data != nil {
		_ = w.c.Set(ctx, key, data, 0)
	}
	return data, nil
}

func (w *WriteBack) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	if err := w.c.Set(ctx, key, value, ttl); err != nil {
		return fmt.Errorf("write-back cache set: %w", err)
	}
	// Mark dirty
	w.mu.Lock()
	cp := make([]byte, len(value))
	copy(cp, value)
	w.dirty[key] = cp
	w.mu.Unlock()
	return nil
}

func (w *WriteBack) Delete(ctx context.Context, key string) error {
	w.mu.Lock()
	delete(w.dirty, key)
	w.mu.Unlock()
	if err := w.src.Remove(ctx, key); err != nil {
		return fmt.Errorf("write-back source remove: %w", err)
	}
	return w.c.Delete(ctx, key)
}

// Flush writes all dirty entries to the backing store.
func (w *WriteBack) Flush(ctx context.Context) error {
	w.mu.Lock()
	dirty := w.dirty
	w.dirty = make(map[string][]byte)
	w.mu.Unlock()

	for key, value := range dirty {
		if err := w.src.Store(ctx, key, value); err != nil {
			return fmt.Errorf("write-back flush %q: %w", key, err)
		}
	}
	return nil
}

// DirtyCount returns the number of entries pending flush.
func (w *WriteBack) DirtyCount() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.dirty)
}

// --- Cache-Aside ---

// CacheAside (also known as lazy-loading) loads data into the cache
// only when it is requested. Writes go directly to the source and
// invalidate the cache.
type CacheAside struct {
	c   cache.Cache
	src DataSource
}

// NewCacheAside creates a cache-aside strategy.
func NewCacheAside(c cache.Cache, src DataSource) *CacheAside {
	return &CacheAside{c: c, src: src}
}

func (ca *CacheAside) Name() string { return "cache-aside" }

func (ca *CacheAside) Get(ctx context.Context, key string) ([]byte, error) {
	data, err := ca.c.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	if data != nil {
		return data, nil
	}
	// Load from source and populate cache
	data, err = ca.src.Load(ctx, key)
	if err != nil {
		return nil, err
	}
	if data != nil {
		_ = ca.c.Set(ctx, key, data, 0)
	}
	return data, nil
}

func (ca *CacheAside) Set(ctx context.Context, key string, value []byte, _ time.Duration) error {
	// Write to source first
	if err := ca.src.Store(ctx, key, value); err != nil {
		return fmt.Errorf("cache-aside source store: %w", err)
	}
	// Invalidate cache
	return ca.c.Delete(ctx, key)
}

func (ca *CacheAside) Delete(ctx context.Context, key string) error {
	if err := ca.src.Remove(ctx, key); err != nil {
		return fmt.Errorf("cache-aside source remove: %w", err)
	}
	return ca.c.Delete(ctx, key)
}

// Compile-time checks
var (
	_ Strategy = (*WriteThrough)(nil)
	_ Strategy = (*WriteBack)(nil)
	_ Strategy = (*CacheAside)(nil)
)
