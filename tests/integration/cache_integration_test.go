package integration

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"digital.vasic.cache/pkg/cache"
	"digital.vasic.cache/pkg/distributed"
	"digital.vasic.cache/pkg/memory"
	"digital.vasic.cache/pkg/policy"
)

func TestMemoryCache_SetGetDelete_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")  // SKIP-OK: #short-mode
	}

	cfg := &memory.Config{
		MaxEntries:      1000,
		DefaultTTL:      5 * time.Minute,
		CleanupInterval: time.Second,
		EvictionPolicy:  cache.LRU,
	}
	c := memory.New(cfg)
	defer func() { _ = c.Close() }()

	ctx := context.Background()

	// Set multiple entries
	for i := 0; i < 50; i++ {
		key := fmt.Sprintf("integration-key-%d", i)
		value := []byte(fmt.Sprintf("integration-value-%d", i))
		err := c.Set(ctx, key, value, time.Minute)
		require.NoError(t, err)
	}

	// Verify all entries exist
	for i := 0; i < 50; i++ {
		key := fmt.Sprintf("integration-key-%d", i)
		data, err := c.Get(ctx, key)
		require.NoError(t, err)
		assert.Equal(t, fmt.Sprintf("integration-value-%d", i), string(data))
	}

	// Delete half
	for i := 0; i < 25; i++ {
		key := fmt.Sprintf("integration-key-%d", i)
		err := c.Delete(ctx, key)
		require.NoError(t, err)
	}

	// Verify deleted entries are gone
	for i := 0; i < 25; i++ {
		key := fmt.Sprintf("integration-key-%d", i)
		exists, err := c.Exists(ctx, key)
		require.NoError(t, err)
		assert.False(t, exists)
	}

	// Verify remaining entries still exist
	for i := 25; i < 50; i++ {
		key := fmt.Sprintf("integration-key-%d", i)
		exists, err := c.Exists(ctx, key)
		require.NoError(t, err)
		assert.True(t, exists)
	}

	assert.Equal(t, 25, c.Len())
}

func TestTwoLevel_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")  // SKIP-OK: #short-mode
	}

	l1Cfg := &memory.Config{
		MaxEntries:      100,
		DefaultTTL:      time.Minute,
		CleanupInterval: time.Second,
		EvictionPolicy:  cache.LRU,
	}
	l2Cfg := &memory.Config{
		MaxEntries:      1000,
		DefaultTTL:      10 * time.Minute,
		CleanupInterval: time.Second,
		EvictionPolicy:  cache.LRU,
	}

	l1 := memory.New(l1Cfg)
	l2 := memory.New(l2Cfg)
	twoLevel := distributed.NewTwoLevel(l1, l2, time.Minute)
	defer func() { _ = twoLevel.Close() }()

	ctx := context.Background()

	// Write through both levels
	err := twoLevel.Set(ctx, "shared-key", []byte("shared-value"), 5*time.Minute)
	require.NoError(t, err)

	// Read should come from L1
	data, err := twoLevel.Get(ctx, "shared-key")
	require.NoError(t, err)
	assert.Equal(t, "shared-value", string(data))

	// Write directly to L2 (simulating L1 miss)
	err = l2.Set(ctx, "l2-only-key", []byte("l2-only-value"), 5*time.Minute)
	require.NoError(t, err)

	// Read should promote from L2 to L1
	data, err = twoLevel.Get(ctx, "l2-only-key")
	require.NoError(t, err)
	assert.Equal(t, "l2-only-value", string(data))

	// Verify L1 now has the promoted key
	data, err = l1.Get(ctx, "l2-only-key")
	require.NoError(t, err)
	assert.Equal(t, "l2-only-value", string(data))

	// Delete removes from both levels
	err = twoLevel.Delete(ctx, "shared-key")
	require.NoError(t, err)

	exists, err := l1.Exists(ctx, "shared-key")
	require.NoError(t, err)
	assert.False(t, exists)

	exists, err = l2.Exists(ctx, "shared-key")
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestWriteThrough_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")  // SKIP-OK: #short-mode
	}

	c := memory.New(memory.DefaultConfig())
	defer func() { _ = c.Close() }()

	src := &inMemoryDataSource{data: make(map[string][]byte)}
	wt := distributed.NewWriteThrough(c, src)
	ctx := context.Background()

	// Write-through writes to both cache and source
	err := wt.Set(ctx, "wt-key", []byte("wt-value"), time.Minute)
	require.NoError(t, err)

	// Verify cache has the value
	data, err := c.Get(ctx, "wt-key")
	require.NoError(t, err)
	assert.Equal(t, "wt-value", string(data))

	// Verify source has the value
	srcData, err := src.Load(ctx, "wt-key")
	require.NoError(t, err)
	assert.Equal(t, "wt-value", string(srcData))

	// Delete through strategy
	err = wt.Delete(ctx, "wt-key")
	require.NoError(t, err)

	data, err = c.Get(ctx, "wt-key")
	require.NoError(t, err)
	assert.Nil(t, data)
}

func TestTypedCache_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")  // SKIP-OK: #short-mode
	}

	type User struct {
		Name  string `json:"name"`
		Email string `json:"email"`
		Age   int    `json:"age"`
	}

	c := memory.New(memory.DefaultConfig())
	defer func() { _ = c.Close() }()

	tc := cache.NewTypedCache[User](c)
	ctx := context.Background()

	user := User{Name: "Alice", Email: "alice@example.com", Age: 30}
	err := tc.Set(ctx, "user:1", user, time.Minute)
	require.NoError(t, err)

	got, found, err := tc.Get(ctx, "user:1")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, user, got)

	exists, err := tc.Exists(ctx, "user:1")
	require.NoError(t, err)
	assert.True(t, exists)

	err = tc.Delete(ctx, "user:1")
	require.NoError(t, err)

	_, found, err = tc.Get(ctx, "user:1")
	require.NoError(t, err)
	assert.False(t, found)
}

func TestAdaptiveTTL_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")  // SKIP-OK: #short-mode
	}

	adaptive := policy.NewAdaptiveTTL(time.Second, 10*time.Second)

	// Initially get MinTTL for unknown key
	ttl := adaptive.GetTTL("cold-key")
	assert.Equal(t, time.Second, ttl)

	// Access 50 times to get a mid-range TTL
	for i := 0; i < 50; i++ {
		adaptive.RecordAccess("hot-key")
	}

	ttl = adaptive.GetTTL("hot-key")
	assert.True(t, ttl > time.Second, "TTL should increase with accesses")
	assert.True(t, ttl <= 10*time.Second, "TTL should not exceed MaxTTL")

	// Access 100+ times to cap at MaxTTL
	for i := 0; i < 60; i++ {
		adaptive.RecordAccess("very-hot-key")
	}
	for i := 0; i < 60; i++ {
		adaptive.RecordAccess("very-hot-key")
	}

	ttl = adaptive.GetTTL("very-hot-key")
	assert.Equal(t, 10*time.Second, ttl)
}

// inMemoryDataSource implements distributed.DataSource for testing.
type inMemoryDataSource struct {
	mu   sync.Mutex
	data map[string][]byte
}

func (s *inMemoryDataSource) Load(_ context.Context, key string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.data[key]
	if !ok {
		return nil, nil
	}
	cp := make([]byte, len(v))
	copy(cp, v)
	return cp, nil
}

func (s *inMemoryDataSource) Store(_ context.Context, key string, value []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make([]byte, len(value))
	copy(cp, value)
	s.data[key] = cp
	return nil
}

func (s *inMemoryDataSource) Remove(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, key)
	return nil
}
