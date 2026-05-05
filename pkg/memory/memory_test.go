package memory

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"digital.vasic.cache/pkg/cache"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew_NilConfig(t *testing.T) {
	c := New(nil)
	require.NotNil(t, c)
	defer c.Close()

	assert.Equal(t, 10000, c.config.MaxEntries)
}

func TestCache_SetAndGet(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		value []byte
		ttl   time.Duration
	}{
		{name: "basic", key: "k1", value: []byte("v1"), ttl: time.Minute},
		{name: "empty value", key: "k2", value: []byte{}, ttl: time.Minute},
		{name: "binary data", key: "k3", value: []byte{0x00, 0xFF, 0x80}, ttl: time.Minute},
		{name: "zero ttl uses default", key: "k4", value: []byte("v4"), ttl: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := New(&Config{
				MaxEntries:      100,
				DefaultTTL:      5 * time.Minute,
				CleanupInterval: 0, // disable cleanup for tests
			})
			defer c.Close()
			ctx := context.Background()

			err := c.Set(ctx, tt.key, tt.value, tt.ttl)
			require.NoError(t, err)

			got, err := c.Get(ctx, tt.key)
			require.NoError(t, err)
			assert.Equal(t, tt.value, got)
		})
	}
}

func TestCache_GetMiss(t *testing.T) {
	c := New(&Config{CleanupInterval: 0})
	defer c.Close()
	ctx := context.Background()

	got, err := c.Get(ctx, "nonexistent")
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestCache_GetExpired(t *testing.T) {
	c := New(&Config{CleanupInterval: 0})
	defer c.Close()
	ctx := context.Background()

	err := c.Set(ctx, "exp", []byte("val"), time.Millisecond)
	require.NoError(t, err)

	time.Sleep(5 * time.Millisecond)

	got, err := c.Get(ctx, "exp")
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestCache_SetOverwrite(t *testing.T) {
	c := New(&Config{MaxEntries: 100, CleanupInterval: 0})
	defer c.Close()
	ctx := context.Background()

	err := c.Set(ctx, "key", []byte("v1"), time.Minute)
	require.NoError(t, err)

	err = c.Set(ctx, "key", []byte("v2"), time.Minute)
	require.NoError(t, err)

	got, err := c.Get(ctx, "key")
	require.NoError(t, err)
	assert.Equal(t, []byte("v2"), got)
	assert.Equal(t, 1, c.Len())
}

func TestCache_Delete(t *testing.T) {
	c := New(&Config{CleanupInterval: 0})
	defer c.Close()
	ctx := context.Background()

	err := c.Set(ctx, "key", []byte("val"), time.Minute)
	require.NoError(t, err)

	err = c.Delete(ctx, "key")
	require.NoError(t, err)

	got, err := c.Get(ctx, "key")
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestCache_DeleteNonExistent(t *testing.T) {
	c := New(&Config{CleanupInterval: 0})
	defer c.Close()

	err := c.Delete(context.Background(), "ghost")
	assert.NoError(t, err)
}

func TestCache_Exists(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(c *Cache)
		key      string
		expected bool
	}{
		{
			name:     "key exists",
			setup:    func(c *Cache) { c.Set(context.Background(), "k", []byte("v"), time.Minute) },
			key:      "k",
			expected: true,
		},
		{
			name:     "key does not exist",
			setup:    func(c *Cache) {},
			key:      "nope",
			expected: false,
		},
		{
			name: "key expired",
			setup: func(c *Cache) {
				c.Set(context.Background(), "exp", []byte("v"), time.Millisecond)
				time.Sleep(5 * time.Millisecond)
			},
			key:      "exp",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := New(&Config{CleanupInterval: 0})
			defer c.Close()
			tt.setup(c)

			exists, err := c.Exists(context.Background(), tt.key)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, exists)
		})
	}
}

func TestCache_LRUEviction(t *testing.T) {
	c := New(&Config{
		MaxEntries:      3,
		EvictionPolicy:  cache.LRU,
		CleanupInterval: 0,
	})
	defer c.Close()
	ctx := context.Background()

	// Fill the cache
	c.Set(ctx, "a", []byte("1"), time.Minute)
	c.Set(ctx, "b", []byte("2"), time.Minute)
	c.Set(ctx, "c", []byte("3"), time.Minute)

	// Access "a" to make it recently used
	c.Get(ctx, "a")

	// Add "d" -- should evict "b" (least recently used)
	c.Set(ctx, "d", []byte("4"), time.Minute)

	assert.Equal(t, 3, c.Len())

	// "b" should be evicted
	got, _ := c.Get(ctx, "b")
	assert.Nil(t, got, "b should have been evicted")

	// "a" should still be there
	got, _ = c.Get(ctx, "a")
	assert.NotNil(t, got, "a should still exist")
}

func TestCache_LFUEviction(t *testing.T) {
	c := New(&Config{
		MaxEntries:      3,
		EvictionPolicy:  cache.LFU,
		CleanupInterval: 0,
	})
	defer c.Close()
	ctx := context.Background()

	c.Set(ctx, "a", []byte("1"), time.Minute)
	c.Set(ctx, "b", []byte("2"), time.Minute)
	c.Set(ctx, "c", []byte("3"), time.Minute)

	// Access "a" and "c" multiple times
	for i := 0; i < 5; i++ {
		c.Get(ctx, "a")
		c.Get(ctx, "c")
	}
	// "b" accessed 0 times (only set, no get)

	// Add "d" -- should evict "b" (least frequently used)
	c.Set(ctx, "d", []byte("4"), time.Minute)

	assert.Equal(t, 3, c.Len())
	got, _ := c.Get(ctx, "b")
	assert.Nil(t, got, "b should have been evicted (LFU)")
}

func TestCache_FIFOEviction(t *testing.T) {
	c := New(&Config{
		MaxEntries:      3,
		EvictionPolicy:  cache.FIFO,
		CleanupInterval: 0,
	})
	defer c.Close()
	ctx := context.Background()

	c.Set(ctx, "a", []byte("1"), time.Minute)
	c.Set(ctx, "b", []byte("2"), time.Minute)
	c.Set(ctx, "c", []byte("3"), time.Minute)

	// Add "d" -- should evict "a" (first in)
	c.Set(ctx, "d", []byte("4"), time.Minute)

	assert.Equal(t, 3, c.Len())
	got, _ := c.Get(ctx, "a")
	assert.Nil(t, got, "a should have been evicted (FIFO)")

	got, _ = c.Get(ctx, "b")
	assert.NotNil(t, got, "b should still exist")
}

func TestCache_MaxMemoryEviction(t *testing.T) {
	c := New(&Config{
		MaxEntries:      1000, // high entry limit
		MaxMemoryBytes:  20,   // very low memory limit
		EvictionPolicy:  cache.LRU,
		CleanupInterval: 0,
	})
	defer c.Close()
	ctx := context.Background()

	// Each value is 10 bytes, so only 2 can fit
	c.Set(ctx, "a", make([]byte, 10), time.Minute)
	c.Set(ctx, "b", make([]byte, 10), time.Minute)
	c.Set(ctx, "c", make([]byte, 10), time.Minute)

	assert.LessOrEqual(t, c.MemoryUsed(), int64(20))
}

func TestCache_Stats(t *testing.T) {
	c := New(&Config{MaxEntries: 100, CleanupInterval: 0})
	defer c.Close()
	ctx := context.Background()

	c.Set(ctx, "k", []byte("v"), time.Minute)
	c.Get(ctx, "k")       // hit
	c.Get(ctx, "missing") // miss

	stats := c.Stats()
	assert.Equal(t, int64(1), stats.Hits)
	assert.Equal(t, int64(1), stats.Misses)
	assert.Equal(t, int64(1), stats.Size)
	assert.InDelta(t, 50.0, stats.HitRate(), 0.01)
}

func TestCache_Flush(t *testing.T) {
	c := New(&Config{CleanupInterval: 0})
	defer c.Close()
	ctx := context.Background()

	for i := 0; i < 10; i++ {
		c.Set(ctx, fmt.Sprintf("key%d", i), []byte("val"), time.Minute)
	}
	assert.Equal(t, 10, c.Len())

	c.Flush()
	assert.Equal(t, 0, c.Len())
	assert.Equal(t, int64(0), c.MemoryUsed())
}

func TestCache_ConcurrentAccess(t *testing.T) {
	// bluff-scan: no-assert-ok (concurrency test — go test -race catches data races; absence of panic == correctness)
	c := New(&Config{
		MaxEntries:      100,
		CleanupInterval: 0,
		EvictionPolicy:  cache.LRU,
	})
	defer c.Close()
	ctx := context.Background()

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			key := fmt.Sprintf("key%d", n%10)
			val := []byte(fmt.Sprintf("val%d", n))

			_ = c.Set(ctx, key, val, time.Minute)
			_, _ = c.Get(ctx, key)
			_, _ = c.Exists(ctx, key)
			if n%3 == 0 {
				_ = c.Delete(ctx, key)
			}
		}(i)
	}
	wg.Wait()
}

func TestCache_BackgroundCleanup(t *testing.T) {
	c := New(&Config{
		MaxEntries:      100,
		CleanupInterval: 10 * time.Millisecond,
	})
	defer c.Close()
	ctx := context.Background()

	c.Set(ctx, "short", []byte("val"), 5*time.Millisecond)
	c.Set(ctx, "long", []byte("val"), 10*time.Second)

	time.Sleep(50 * time.Millisecond)

	got, _ := c.Get(ctx, "short")
	assert.Nil(t, got, "short-lived entry should be cleaned up")

	got, _ = c.Get(ctx, "long")
	assert.NotNil(t, got, "long-lived entry should still exist")
}

func TestCache_Close(t *testing.T) {
	c := New(&Config{CleanupInterval: 10 * time.Millisecond})
	err := c.Close()
	assert.NoError(t, err)
}

func TestCache_GetReturnsCopy(t *testing.T) {
	c := New(&Config{CleanupInterval: 0})
	defer c.Close()
	ctx := context.Background()

	original := []byte("hello")
	c.Set(ctx, "key", original, time.Minute)

	got, _ := c.Get(ctx, "key")
	got[0] = 'X'

	got2, _ := c.Get(ctx, "key")
	assert.Equal(t, byte('h'), got2[0], "modifying returned slice should not affect cache")
}

func TestFormatSize(t *testing.T) {
	tests := []struct {
		name     string
		bytes    int64
		expected string
	}{
		{name: "bytes", bytes: 512, expected: "512 B"},
		{name: "kilobytes", bytes: 2048, expected: "2.00 KB"},
		{name: "megabytes", bytes: 1048576, expected: "1.00 MB"},
		{name: "gigabytes", bytes: 1073741824, expected: "1.00 GB"},
		{name: "zero", bytes: 0, expected: "0 B"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, FormatSize(tt.bytes))
		})
	}
}

// Interface compliance
var _ cache.Cache = (*Cache)(nil)

func TestCache_EvictLRU_EmptyList(t *testing.T) {
	// Create cache and directly call evict without any entries
	c := New(&Config{
		MaxEntries:      1,
		EvictionPolicy:  cache.LRU,
		CleanupInterval: 0,
	})
	defer c.Close()

	// Directly trigger evict on empty cache to cover the nil check
	c.mu.Lock()
	c.evictLRU()
	c.mu.Unlock()

	// Should not panic and evictions should remain 0
	assert.Equal(t, int64(0), c.Stats().Evictions)
}

func TestCache_EvictFIFO_EmptyList(t *testing.T) {
	// Create cache and directly call evict without any entries
	c := New(&Config{
		MaxEntries:      1,
		EvictionPolicy:  cache.FIFO,
		CleanupInterval: 0,
	})
	defer c.Close()

	// Directly trigger evict on empty cache to cover the nil check
	c.mu.Lock()
	c.evictFIFO()
	c.mu.Unlock()

	// Should not panic and evictions should remain 0
	assert.Equal(t, int64(0), c.Stats().Evictions)
}
