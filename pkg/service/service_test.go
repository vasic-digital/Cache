package service

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- mockCache for testing ---

type mockCache struct {
	mu   sync.RWMutex
	data map[string]interface{}

	// Optional error injection.
	getErr    error
	setErr    error
	deleteErr error
}

func newMockCache() *mockCache {
	return &mockCache{data: make(map[string]interface{})}
}

func (m *mockCache) Get(_ context.Context, key string) (interface{}, bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.getErr != nil {
		return nil, false, m.getErr
	}
	v, ok := m.data[key]
	return v, ok, nil
}

func (m *mockCache) Set(_ context.Context, key string, value interface{}, _ time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.setErr != nil {
		return m.setErr
	}
	m.data[key] = value
	return nil
}

func (m *mockCache) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.deleteErr != nil {
		return m.deleteErr
	}
	delete(m.data, key)
	return nil
}

var _ Cache = (*mockCache)(nil)

// --- Tests ---

func TestNew(t *testing.T) {
	mc := newMockCache()
	cfg := DefaultConfig()

	w := New(mc, cfg)

	require.NotNil(t, w)
	assert.Equal(t, 5*time.Minute, w.config.DefaultTTL)
	assert.Equal(t, "", w.config.KeyPrefix)
}

func TestGetOrLoad_CacheHit(t *testing.T) {
	mc := newMockCache()
	mc.data["user:42"] = "Alice"

	w := New(mc, Config{DefaultTTL: time.Minute, KeyPrefix: ""})

	loadCalled := false
	val, err := w.GetOrLoad(context.Background(), "user:42", func(_ context.Context, _ string) (interface{}, error) {
		loadCalled = true
		return "should-not-reach", nil
	})

	require.NoError(t, err)
	assert.Equal(t, "Alice", val)
	assert.False(t, loadCalled, "load function should not be called on cache hit")

	stats := w.GetStats()
	assert.Equal(t, int64(1), stats.Hits)
	assert.Equal(t, int64(0), stats.Misses)
}

func TestGetOrLoad_CacheMiss(t *testing.T) {
	mc := newMockCache()

	w := New(mc, Config{DefaultTTL: time.Minute, KeyPrefix: ""})

	val, err := w.GetOrLoad(context.Background(), "user:99", func(_ context.Context, key string) (interface{}, error) {
		assert.Equal(t, "user:99", key)
		return "Bob", nil
	})

	require.NoError(t, err)
	assert.Equal(t, "Bob", val)

	// Verify the value was cached.
	cached, ok := mc.data["user:99"]
	assert.True(t, ok)
	assert.Equal(t, "Bob", cached)

	stats := w.GetStats()
	assert.Equal(t, int64(0), stats.Hits)
	assert.Equal(t, int64(1), stats.Misses)
}

func TestGetOrLoad_LoadError(t *testing.T) {
	mc := newMockCache()

	w := New(mc, Config{DefaultTTL: time.Minute})

	val, err := w.GetOrLoad(context.Background(), "bad-key", func(_ context.Context, _ string) (interface{}, error) {
		return nil, fmt.Errorf("database connection refused")
	})

	require.Error(t, err)
	assert.Nil(t, val)
	assert.Contains(t, err.Error(), "service cache load")
	assert.Contains(t, err.Error(), "database connection refused")
}

func TestGetOrLoad_CacheError_FallsBack(t *testing.T) {
	mc := newMockCache()
	mc.getErr = fmt.Errorf("redis timeout")

	w := New(mc, Config{DefaultTTL: time.Minute})

	val, err := w.GetOrLoad(context.Background(), "key", func(_ context.Context, _ string) (interface{}, error) {
		return "fallback-value", nil
	})

	require.NoError(t, err)
	assert.Equal(t, "fallback-value", val)

	stats := w.GetStats()
	assert.Equal(t, int64(1), stats.Errors, "cache get error should be counted")
	assert.Equal(t, int64(1), stats.Misses, "should count as a miss after cache error")
}

func TestGetOrLoadWithTTL(t *testing.T) {
	mc := newMockCache()

	w := New(mc, Config{DefaultTTL: time.Hour, KeyPrefix: "svc:"})

	val, err := w.GetOrLoadWithTTL(context.Background(), "item", 30*time.Second,
		func(_ context.Context, key string) (interface{}, error) {
			assert.Equal(t, "item", key)
			return 42, nil
		},
	)

	require.NoError(t, err)
	assert.Equal(t, 42, val)

	// Verify the prefixed key was used.
	cached, ok := mc.data["svc:item"]
	assert.True(t, ok, "value should be stored with prefix")
	assert.Equal(t, 42, cached)
}

func TestInvalidate(t *testing.T) {
	mc := newMockCache()
	mc.data["svc:user:1"] = "Alice"

	w := New(mc, Config{DefaultTTL: time.Minute, KeyPrefix: "svc:"})

	err := w.Invalidate(context.Background(), "user:1")
	require.NoError(t, err)

	_, ok := mc.data["svc:user:1"]
	assert.False(t, ok, "key should be deleted after invalidation")
}

func TestInvalidate_Error(t *testing.T) {
	mc := newMockCache()
	mc.deleteErr = fmt.Errorf("delete failed")

	w := New(mc, Config{DefaultTTL: time.Minute})

	err := w.Invalidate(context.Background(), "key")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "service cache invalidate")
	assert.Contains(t, err.Error(), "delete failed")
}

func TestInvalidatePrefix(t *testing.T) {
	mc := newMockCache()
	mc.data["svc:users"] = "data"

	w := New(mc, Config{DefaultTTL: time.Minute, KeyPrefix: "svc:"})

	err := w.InvalidatePrefix(context.Background(), "users")
	require.NoError(t, err)

	_, ok := mc.data["svc:users"]
	assert.False(t, ok, "prefixed key should be deleted")
}

func TestGetStats(t *testing.T) {
	mc := newMockCache()

	w := New(mc, Config{DefaultTTL: time.Minute})

	// Generate some hits and misses.
	mc.data["exist"] = "value"

	_, _ = w.GetOrLoad(context.Background(), "exist", func(_ context.Context, _ string) (interface{}, error) {
		return "x", nil
	})
	_, _ = w.GetOrLoad(context.Background(), "missing", func(_ context.Context, _ string) (interface{}, error) {
		return "loaded", nil
	})

	stats := w.GetStats()
	assert.Equal(t, int64(1), stats.Hits)
	assert.Equal(t, int64(1), stats.Misses)
	assert.Equal(t, int64(0), stats.Errors)
}

func TestPrefixKey(t *testing.T) {
	tests := []struct {
		name     string
		prefix   string
		key      string
		expected string
	}{
		{name: "empty prefix", prefix: "", key: "mykey", expected: "mykey"},
		{name: "with prefix", prefix: "svc:", key: "mykey", expected: "svc:mykey"},
		{name: "nested prefix", prefix: "app:cache:", key: "user:1", expected: "app:cache:user:1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := New(newMockCache(), Config{KeyPrefix: tt.prefix})
			assert.Equal(t, tt.expected, w.prefixKey(tt.key))
		})
	}
}

func TestGetOrLoad_CacheSetError_StillReturnsValue(t *testing.T) {
	mc := newMockCache()
	mc.setErr = fmt.Errorf("cache write failed")

	w := New(mc, Config{DefaultTTL: time.Minute})

	val, err := w.GetOrLoad(context.Background(), "key", func(_ context.Context, _ string) (interface{}, error) {
		return "loaded-value", nil
	})

	// The value should still be returned even though cache set failed.
	require.NoError(t, err)
	assert.Equal(t, "loaded-value", val)

	stats := w.GetStats()
	assert.Equal(t, int64(1), stats.Errors, "cache set error should be counted")
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	assert.Equal(t, 5*time.Minute, cfg.DefaultTTL)
	assert.Equal(t, "", cfg.KeyPrefix)
}
