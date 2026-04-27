package security

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"digital.vasic.cache/pkg/cache"
	"digital.vasic.cache/pkg/distributed"
	"digital.vasic.cache/pkg/memory"
	"digital.vasic.cache/pkg/policy"
)

func TestSecurity_EmptyKey(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping security test in short mode")  // SKIP-OK: #short-mode
	}

	c := memory.New(memory.DefaultConfig())
	defer func() { _ = c.Close() }()

	ctx := context.Background()

	// Empty key should not panic or cause undefined behavior
	err := c.Set(ctx, "", []byte("value"), time.Minute)
	assert.NoError(t, err)

	data, err := c.Get(ctx, "")
	assert.NoError(t, err)
	assert.Equal(t, "value", string(data))

	err = c.Delete(ctx, "")
	assert.NoError(t, err)
}

func TestSecurity_NilValueHandling(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping security test in short mode")  // SKIP-OK: #short-mode
	}

	c := memory.New(memory.DefaultConfig())
	defer func() { _ = c.Close() }()

	ctx := context.Background()

	// Setting nil value should not panic
	err := c.Set(ctx, "nil-key", nil, time.Minute)
	assert.NoError(t, err)

	data, err := c.Get(ctx, "nil-key")
	assert.NoError(t, err)
	assert.Equal(t, 0, len(data))

	// Setting empty value should not panic
	err = c.Set(ctx, "empty-key", []byte{}, time.Minute)
	assert.NoError(t, err)

	data, err = c.Get(ctx, "empty-key")
	assert.NoError(t, err)
	assert.Equal(t, 0, len(data))
}

func TestSecurity_LargeKeyInjection(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping security test in short mode")  // SKIP-OK: #short-mode
	}

	c := memory.New(memory.DefaultConfig())
	defer func() { _ = c.Close() }()

	ctx := context.Background()

	// Very long key should not cause issues
	longKey := strings.Repeat("x", 100000)
	err := c.Set(ctx, longKey, []byte("value"), time.Minute)
	assert.NoError(t, err)

	data, err := c.Get(ctx, longKey)
	assert.NoError(t, err)
	assert.Equal(t, "value", string(data))
}

func TestSecurity_SpecialCharacterKeys(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping security test in short mode")  // SKIP-OK: #short-mode
	}

	c := memory.New(memory.DefaultConfig())
	defer func() { _ = c.Close() }()

	ctx := context.Background()

	specialKeys := []string{
		"key\x00with\x00nulls",
		"key\nwith\nnewlines",
		"key\twith\ttabs",
		"key with spaces",
		"key/with/slashes",
		"../../../etc/passwd",
		"key;DROP TABLE cache;--",
		"key' OR '1'='1",
		"\xff\xfe\xfd",
	}

	for _, key := range specialKeys {
		err := c.Set(ctx, key, []byte("test-value"), time.Minute)
		assert.NoError(t, err, "key %q should not cause error", key)

		data, err := c.Get(ctx, key)
		assert.NoError(t, err, "key %q should be retrievable", key)
		assert.Equal(t, "test-value", string(data))
	}
}

func TestSecurity_LargeValueHandling(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping security test in short mode")  // SKIP-OK: #short-mode
	}

	c := memory.New(&memory.Config{
		MaxEntries:      100,
		MaxMemoryBytes:  10 * 1024 * 1024, // 10MB limit
		DefaultTTL:      time.Minute,
		CleanupInterval: time.Second,
		EvictionPolicy:  cache.LRU,
	})
	defer func() { _ = c.Close() }()

	ctx := context.Background()

	// 1MB value should work
	largeValue := make([]byte, 1024*1024)
	for i := range largeValue {
		largeValue[i] = byte(i % 256)
	}

	err := c.Set(ctx, "large-key", largeValue, time.Minute)
	assert.NoError(t, err)

	data, err := c.Get(ctx, "large-key")
	assert.NoError(t, err)
	assert.Equal(t, len(largeValue), len(data))
}

func TestSecurity_ConsistentHash_EmptyRing(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping security test in short mode")  // SKIP-OK: #short-mode
	}

	ch := distributed.NewConsistentHash(100)

	// Getting node from empty ring should return empty, not panic
	node := ch.GetNode("any-key")
	assert.Empty(t, node)

	// Adding then removing all nodes should be safe
	ch.AddNode("node-1")
	ch.RemoveNode("node-1")
	node = ch.GetNode("any-key")
	assert.Empty(t, node)

	// Removing non-existent node should not panic
	ch.RemoveNode("non-existent")
	assert.Equal(t, 0, ch.NodeCount())
}

func TestSecurity_NegativeReplicaCount(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping security test in short mode")  // SKIP-OK: #short-mode
	}

	// Negative replicas should default to a positive value
	ch := distributed.NewConsistentHash(-1)
	ch.AddNode("node-1")
	node := ch.GetNode("test-key")
	assert.NotEmpty(t, node)
}

func TestSecurity_TypedCache_InvalidJSON(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping security test in short mode")  // SKIP-OK: #short-mode
	}

	type User struct {
		Name string `json:"name"`
	}

	c := memory.New(memory.DefaultConfig())
	defer func() { _ = c.Close() }()

	ctx := context.Background()

	// Store invalid JSON directly into the backing cache
	err := c.Set(ctx, "bad-json", []byte("{invalid json}"), time.Minute)
	require.NoError(t, err)

	tc := cache.NewTypedCache[User](c)

	// Typed get should return an error, not panic
	_, _, err = tc.Get(ctx, "bad-json")
	assert.Error(t, err, "invalid JSON should produce an error")
}

func TestSecurity_PolicyNilSafety(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping security test in short mode")  // SKIP-OK: #short-mode
	}

	// AdaptiveTTL with reversed min/max should auto-correct
	adaptive := policy.NewAdaptiveTTL(10*time.Second, time.Second)
	ttl := adaptive.GetTTL("key")
	assert.True(t, ttl > 0, "TTL should be positive even with reversed params")

	// CapacityEviction with invalid threshold should default
	cap := policy.NewCapacityEviction(0)
	assert.False(t, cap.ShouldEvict("key", policy.EvictionStats{
		TotalEntries: 50,
		MaxEntries:   100,
	}))

	cap2 := policy.NewCapacityEviction(2.0)
	assert.False(t, cap2.ShouldEvict("key", policy.EvictionStats{
		TotalEntries: 50,
		MaxEntries:   100,
	}))

	// CompositeEviction with no sub-policies
	composite := policy.NewCompositeEviction()
	assert.False(t, composite.ShouldEvict("key", policy.EvictionStats{}))

	// SlidingTTL: ShouldExpire for unknown key
	sliding := policy.NewSlidingTTL(time.Minute)
	assert.True(t, sliding.ShouldExpire("unknown-key"))
}
