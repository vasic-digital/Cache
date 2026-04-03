package cache_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"digital.vasic.cache/pkg/cache"
	"digital.vasic.cache/pkg/memory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMemoryCache_TTLZero_UsesDefault(t *testing.T) {
	t.Parallel()
	cfg := memory.DefaultConfig()
	cfg.DefaultTTL = 50 * time.Millisecond
	cfg.CleanupInterval = 0 // disable background cleanup
	c := memory.New(cfg)
	defer func() { _ = c.Close() }()

	ctx := context.Background()

	err := c.Set(ctx, "key", []byte("value"), 0)
	require.NoError(t, err)

	// Immediately available
	val, err := c.Get(ctx, "key")
	require.NoError(t, err)
	assert.Equal(t, []byte("value"), val)

	// After default TTL expires, should be gone
	time.Sleep(100 * time.Millisecond)
	val, err = c.Get(ctx, "key")
	require.NoError(t, err)
	assert.Nil(t, val)
}

func TestMemoryCache_NegativeTTL_TreatedAsNoExpiry(t *testing.T) {
	t.Parallel()
	cfg := memory.DefaultConfig()
	cfg.DefaultTTL = 0
	cfg.CleanupInterval = 0
	c := memory.New(cfg)
	defer func() { _ = c.Close() }()

	ctx := context.Background()

	// Negative TTL is treated as non-positive, so expiresAt stays zero (no expiry)
	err := c.Set(ctx, "key", []byte("value"), -time.Hour)
	require.NoError(t, err)

	// Entry should persist since negative TTL results in no expiration
	val, err := c.Get(ctx, "key")
	require.NoError(t, err)
	assert.Equal(t, []byte("value"), val)
}

func TestMemoryCache_MaxTTL(t *testing.T) {
	t.Parallel()
	cfg := memory.DefaultConfig()
	cfg.CleanupInterval = 0
	c := memory.New(cfg)
	defer func() { _ = c.Close() }()

	ctx := context.Background()

	// Very large TTL (100 years)
	err := c.Set(ctx, "key", []byte("value"), 100*365*24*time.Hour)
	require.NoError(t, err)

	val, err := c.Get(ctx, "key")
	require.NoError(t, err)
	assert.Equal(t, []byte("value"), val)
}

func TestMemoryCache_ConcurrentEviction(t *testing.T) {
	t.Parallel()
	cfg := &memory.Config{
		MaxEntries:      10,
		DefaultTTL:      time.Minute,
		CleanupInterval: 0,
		EvictionPolicy:  cache.LRU,
	}
	c := memory.New(cfg)
	defer func() { _ = c.Close() }()

	ctx := context.Background()

	var wg sync.WaitGroup
	// Many goroutines all writing, causing evictions
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := fmt.Sprintf("key-%d", i)
			err := c.Set(ctx, key, []byte("value"), time.Minute)
			assert.NoError(t, err)
		}(i)
	}
	wg.Wait()

	// Cache should have at most MaxEntries entries
	assert.LessOrEqual(t, c.Len(), 10)
}

func TestMemoryCache_CacheStampede(t *testing.T) {
	t.Parallel()
	cfg := memory.DefaultConfig()
	cfg.CleanupInterval = 0
	c := memory.New(cfg)
	defer func() { _ = c.Close() }()

	ctx := context.Background()

	// Many goroutines all requesting the same missing key simultaneously
	var wg sync.WaitGroup
	misses := make([]bool, 50)
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			val, err := c.Get(ctx, "stampede-key")
			assert.NoError(t, err)
			misses[idx] = val == nil
		}(i)
	}
	wg.Wait()

	// All should have been misses since key was never set
	for i, miss := range misses {
		assert.True(t, miss, "goroutine %d should have got a miss", i)
	}
}

func TestMemoryCache_NilValue(t *testing.T) {
	t.Parallel()
	cfg := memory.DefaultConfig()
	cfg.CleanupInterval = 0
	c := memory.New(cfg)
	defer func() { _ = c.Close() }()

	ctx := context.Background()

	// Set nil value (empty byte slice)
	err := c.Set(ctx, "nil-key", nil, time.Minute)
	require.NoError(t, err)

	val, err := c.Get(ctx, "nil-key")
	require.NoError(t, err)
	// nil stored as empty bytes
	assert.NotNil(t, val)
	assert.Empty(t, val)
}

func TestMemoryCache_EmptyKey(t *testing.T) {
	t.Parallel()
	cfg := memory.DefaultConfig()
	cfg.CleanupInterval = 0
	c := memory.New(cfg)
	defer func() { _ = c.Close() }()

	ctx := context.Background()

	err := c.Set(ctx, "", []byte("value"), time.Minute)
	require.NoError(t, err)

	val, err := c.Get(ctx, "")
	require.NoError(t, err)
	assert.Equal(t, []byte("value"), val)
}

func TestMemoryCache_EmptyValue(t *testing.T) {
	t.Parallel()
	cfg := memory.DefaultConfig()
	cfg.CleanupInterval = 0
	c := memory.New(cfg)
	defer func() { _ = c.Close() }()

	ctx := context.Background()

	err := c.Set(ctx, "key", []byte{}, time.Minute)
	require.NoError(t, err)

	val, err := c.Get(ctx, "key")
	require.NoError(t, err)
	assert.NotNil(t, val)
	assert.Empty(t, val)
}

func TestMemoryCache_DeleteNonExistent(t *testing.T) {
	t.Parallel()
	cfg := memory.DefaultConfig()
	cfg.CleanupInterval = 0
	c := memory.New(cfg)
	defer func() { _ = c.Close() }()

	ctx := context.Background()

	// Deleting a non-existent key should not error
	err := c.Delete(ctx, "nonexistent")
	assert.NoError(t, err)
}

func TestMemoryCache_ExistsExpired(t *testing.T) {
	t.Parallel()
	cfg := memory.DefaultConfig()
	cfg.DefaultTTL = 0
	cfg.CleanupInterval = 0
	c := memory.New(cfg)
	defer func() { _ = c.Close() }()

	ctx := context.Background()

	err := c.Set(ctx, "key", []byte("value"), time.Millisecond)
	require.NoError(t, err)

	time.Sleep(10 * time.Millisecond)

	exists, err := c.Exists(ctx, "key")
	require.NoError(t, err)
	assert.False(t, exists, "expired key should not exist")
}

func TestMemoryCache_OverwriteExistingKey(t *testing.T) {
	t.Parallel()
	cfg := memory.DefaultConfig()
	cfg.CleanupInterval = 0
	c := memory.New(cfg)
	defer func() { _ = c.Close() }()

	ctx := context.Background()

	err := c.Set(ctx, "key", []byte("original"), time.Minute)
	require.NoError(t, err)

	err = c.Set(ctx, "key", []byte("updated"), time.Minute)
	require.NoError(t, err)

	val, err := c.Get(ctx, "key")
	require.NoError(t, err)
	assert.Equal(t, []byte("updated"), val)

	// Only one entry should exist
	assert.Equal(t, 1, c.Len())
}

func TestMemoryCache_LargeValue(t *testing.T) {
	t.Parallel()
	cfg := memory.DefaultConfig()
	cfg.CleanupInterval = 0
	c := memory.New(cfg)
	defer func() { _ = c.Close() }()

	ctx := context.Background()

	// Store a 1MB value
	largeVal := make([]byte, 1024*1024)
	for i := range largeVal {
		largeVal[i] = byte(i % 256)
	}

	err := c.Set(ctx, "large", largeVal, time.Minute)
	require.NoError(t, err)

	val, err := c.Get(ctx, "large")
	require.NoError(t, err)
	assert.Equal(t, largeVal, val)
}

func TestMemoryCache_FlushAll(t *testing.T) {
	t.Parallel()
	cfg := memory.DefaultConfig()
	cfg.CleanupInterval = 0
	c := memory.New(cfg)
	defer func() { _ = c.Close() }()

	ctx := context.Background()

	for i := 0; i < 100; i++ {
		err := c.Set(ctx, fmt.Sprintf("key-%d", i), []byte("v"), time.Minute)
		require.NoError(t, err)
	}
	assert.Equal(t, 100, c.Len())

	c.Flush()
	assert.Equal(t, 0, c.Len())
}

func TestMemoryCache_MemoryBudgetEviction(t *testing.T) {
	t.Parallel()
	cfg := &memory.Config{
		MaxEntries:      0, // unlimited entries
		MaxMemoryBytes:  100,
		DefaultTTL:      time.Minute,
		CleanupInterval: 0,
		EvictionPolicy:  cache.LRU,
	}
	c := memory.New(cfg)
	defer func() { _ = c.Close() }()

	ctx := context.Background()

	// Insert 50 bytes
	err := c.Set(ctx, "key1", make([]byte, 50), time.Minute)
	require.NoError(t, err)

	// Insert 60 bytes -- must evict key1 to fit
	err = c.Set(ctx, "key2", make([]byte, 60), time.Minute)
	require.NoError(t, err)

	val, err := c.Get(ctx, "key1")
	require.NoError(t, err)
	assert.Nil(t, val, "key1 should have been evicted to make room")

	val, err = c.Get(ctx, "key2")
	require.NoError(t, err)
	assert.NotNil(t, val)
}

func TestStats_HitRate_EdgeCases(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		stats    cache.Stats
		expected float64
	}{
		{"zero requests", cache.Stats{Hits: 0, Misses: 0}, 0},
		{"single hit", cache.Stats{Hits: 1, Misses: 0}, 100},
		{"single miss", cache.Stats{Hits: 0, Misses: 1}, 0},
		{"large numbers", cache.Stats{Hits: 1000000, Misses: 1000000}, 50},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.InDelta(t, tt.expected, tt.stats.HitRate(), 0.01)
		})
	}
}

func TestTypedCache_NilInnerCacheValue(t *testing.T) {
	t.Parallel()
	// Test TypedCache with a value that unmarshal would handle oddly
	type item struct {
		Name *string `json:"name"`
	}

	stub := &stubCacheForEdge{data: make(map[string][]byte)}
	tc := cache.NewTypedCache[item](stub)
	ctx := context.Background()

	// Store an item with nil pointer field
	err := tc.Set(ctx, "null-field", item{Name: nil}, time.Minute)
	require.NoError(t, err)

	got, found, err := tc.Get(ctx, "null-field")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Nil(t, got.Name)
}

// stubCacheForEdge is a minimal Cache implementation for edge tests.
type stubCacheForEdge struct {
	data map[string][]byte
}

func (s *stubCacheForEdge) Get(_ context.Context, key string) ([]byte, error) {
	v, ok := s.data[key]
	if !ok {
		return nil, nil
	}
	return v, nil
}

func (s *stubCacheForEdge) Set(_ context.Context, key string, value []byte, _ time.Duration) error {
	s.data[key] = value
	return nil
}

func (s *stubCacheForEdge) Delete(_ context.Context, key string) error {
	delete(s.data, key)
	return nil
}

func (s *stubCacheForEdge) Exists(_ context.Context, key string) (bool, error) {
	_, ok := s.data[key]
	return ok, nil
}

func (s *stubCacheForEdge) Close() error { return nil }
