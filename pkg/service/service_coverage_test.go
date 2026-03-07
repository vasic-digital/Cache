package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- InvalidatePrefix: non-"not found" error from cache.Delete ---

func TestInvalidatePrefix_NonNotFoundError(t *testing.T) {
	mc := newMockCache()
	mc.deleteErr = fmt.Errorf("connection refused")

	w := New(mc, Config{DefaultTTL: time.Minute, KeyPrefix: "svc:"})

	err := w.InvalidatePrefix(context.Background(), "users")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "service cache invalidate prefix")
	assert.Contains(t, err.Error(), "connection refused")
}

// --- InvalidatePrefix: "not found" error is suppressed ---

func TestInvalidatePrefix_NotFoundError_Suppressed(t *testing.T) {
	mc := newMockCache()
	mc.deleteErr = fmt.Errorf("key not found in store")

	w := New(mc, Config{DefaultTTL: time.Minute, KeyPrefix: "svc:"})

	err := w.InvalidatePrefix(context.Background(), "nonexistent")
	assert.NoError(t, err, "not found errors should be suppressed")
}

// --- InvalidatePrefix: success with no prefix ---

func TestInvalidatePrefix_NoPrefix_Success(t *testing.T) {
	mc := newMockCache()
	mc.data["users:all"] = "data"

	w := New(mc, Config{DefaultTTL: time.Minute, KeyPrefix: ""})

	err := w.InvalidatePrefix(context.Background(), "users:all")
	require.NoError(t, err)

	_, ok := mc.data["users:all"]
	assert.False(t, ok, "key should be deleted")
}

// --- GetOrLoadWithTTL: cache get error + set error combined ---

func TestGetOrLoadWithTTL_GetError_SetError(t *testing.T) {
	mc := newMockCache()
	mc.getErr = fmt.Errorf("redis timeout")
	mc.setErr = fmt.Errorf("redis write timeout")

	w := New(mc, Config{DefaultTTL: time.Minute})

	val, err := w.GetOrLoad(context.Background(), "key", func(_ context.Context, _ string) (interface{}, error) {
		return "value", nil
	})

	require.NoError(t, err)
	assert.Equal(t, "value", val)

	stats := w.GetStats()
	assert.Equal(t, int64(2), stats.Errors, "both get and set errors should be counted")
	assert.Equal(t, int64(1), stats.Misses)
	assert.Equal(t, int64(0), stats.Hits)
}

// --- Stats after concurrent operations ---

func TestGetStats_AfterMultipleOperations(t *testing.T) {
	mc := newMockCache()
	w := New(mc, Config{DefaultTTL: time.Minute})

	loader := func(_ context.Context, key string) (interface{}, error) {
		return "val-" + key, nil
	}

	// Miss 1
	_, _ = w.GetOrLoad(context.Background(), "a", loader)
	// Hit (now cached)
	_, _ = w.GetOrLoad(context.Background(), "a", loader)
	// Miss 2
	_, _ = w.GetOrLoad(context.Background(), "b", loader)
	// Hit
	_, _ = w.GetOrLoad(context.Background(), "b", loader)
	// Miss 3
	_, _ = w.GetOrLoad(context.Background(), "c", loader)

	stats := w.GetStats()
	assert.Equal(t, int64(2), stats.Hits)
	assert.Equal(t, int64(3), stats.Misses)
	assert.Equal(t, int64(0), stats.Errors)
}
