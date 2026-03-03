package e2e

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

func TestEndToEnd_FullCacheLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	cfg := &memory.Config{
		MaxEntries:      500,
		DefaultTTL:      5 * time.Minute,
		CleanupInterval: 500 * time.Millisecond,
		EvictionPolicy:  cache.LRU,
	}
	c := memory.New(cfg)
	defer func() { _ = c.Close() }()

	ctx := context.Background()

	// Phase 1: Populate cache
	for i := 0; i < 100; i++ {
		key := fmt.Sprintf("e2e-key-%d", i)
		value := []byte(fmt.Sprintf("e2e-value-%d", i))
		err := c.Set(ctx, key, value, 2*time.Minute)
		require.NoError(t, err)
	}
	assert.Equal(t, 100, c.Len())

	// Phase 2: Verify all entries readable
	for i := 0; i < 100; i++ {
		key := fmt.Sprintf("e2e-key-%d", i)
		data, err := c.Get(ctx, key)
		require.NoError(t, err)
		require.NotNil(t, data)
		assert.Equal(t, fmt.Sprintf("e2e-value-%d", i), string(data))
	}

	// Phase 3: Update entries
	for i := 0; i < 100; i++ {
		key := fmt.Sprintf("e2e-key-%d", i)
		value := []byte(fmt.Sprintf("updated-value-%d", i))
		err := c.Set(ctx, key, value, 2*time.Minute)
		require.NoError(t, err)
	}

	// Verify updates
	for i := 0; i < 100; i++ {
		key := fmt.Sprintf("e2e-key-%d", i)
		data, err := c.Get(ctx, key)
		require.NoError(t, err)
		assert.Equal(t, fmt.Sprintf("updated-value-%d", i), string(data))
	}

	// Phase 4: Stats should show hits and no misses since all keys existed
	stats := c.Stats()
	assert.True(t, stats.Hits > 0, "should have recorded hits")
	assert.Equal(t, int64(100), stats.Size)

	// Phase 5: Flush all entries
	c.Flush()
	assert.Equal(t, 0, c.Len())

	// Phase 6: Cache miss after flush
	data, err := c.Get(ctx, "e2e-key-0")
	require.NoError(t, err)
	assert.Nil(t, data)
}

func TestEndToEnd_TTLExpiration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	cfg := &memory.Config{
		MaxEntries:      100,
		DefaultTTL:      0,
		CleanupInterval: 100 * time.Millisecond,
		EvictionPolicy:  cache.LRU,
	}
	c := memory.New(cfg)
	defer func() { _ = c.Close() }()

	ctx := context.Background()

	// Set with very short TTL
	err := c.Set(ctx, "expire-key", []byte("expire-value"), 200*time.Millisecond)
	require.NoError(t, err)

	// Should exist immediately
	data, err := c.Get(ctx, "expire-key")
	require.NoError(t, err)
	assert.Equal(t, "expire-value", string(data))

	// Wait for expiration
	time.Sleep(350 * time.Millisecond)

	// Should be expired
	data, err = c.Get(ctx, "expire-key")
	require.NoError(t, err)
	assert.Nil(t, data, "entry should have expired")
}

func TestEndToEnd_EvictionPolicy(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	cfg := &memory.Config{
		MaxEntries:      10,
		DefaultTTL:      5 * time.Minute,
		CleanupInterval: time.Second,
		EvictionPolicy:  cache.FIFO,
	}
	c := memory.New(cfg)
	defer func() { _ = c.Close() }()

	ctx := context.Background()

	// Fill to capacity
	for i := 0; i < 10; i++ {
		key := fmt.Sprintf("fifo-key-%d", i)
		err := c.Set(ctx, key, []byte("value"), time.Minute)
		require.NoError(t, err)
	}
	assert.Equal(t, 10, c.Len())

	// Add one more entry -- should trigger eviction of oldest
	err := c.Set(ctx, "fifo-key-new", []byte("new-value"), time.Minute)
	require.NoError(t, err)
	assert.Equal(t, 10, c.Len())

	stats := c.Stats()
	assert.True(t, stats.Evictions > 0, "should have evicted at least one entry")
}

func TestEndToEnd_TwoLevelWithExpiry(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	l1 := memory.New(&memory.Config{
		MaxEntries:      10,
		DefaultTTL:      200 * time.Millisecond,
		CleanupInterval: 100 * time.Millisecond,
		EvictionPolicy:  cache.LRU,
	})
	l2 := memory.New(&memory.Config{
		MaxEntries:      100,
		DefaultTTL:      5 * time.Minute,
		CleanupInterval: time.Second,
		EvictionPolicy:  cache.LRU,
	})

	tl := distributed.NewTwoLevel(l1, l2, 200*time.Millisecond)
	defer func() { _ = tl.Close() }()

	ctx := context.Background()

	// Set via two-level
	err := tl.Set(ctx, "tl-key", []byte("tl-value"), 5*time.Minute)
	require.NoError(t, err)

	// Both levels should have it
	data, err := l1.Get(ctx, "tl-key")
	require.NoError(t, err)
	assert.NotNil(t, data)

	data, err = l2.Get(ctx, "tl-key")
	require.NoError(t, err)
	assert.NotNil(t, data)

	// Wait for L1 to expire
	time.Sleep(400 * time.Millisecond)

	// L1 should be expired, but L2 still has it
	data, err = l1.Get(ctx, "tl-key")
	require.NoError(t, err)
	assert.Nil(t, data, "L1 entry should have expired")

	// Two-level get should re-promote from L2
	data, err = tl.Get(ctx, "tl-key")
	require.NoError(t, err)
	assert.Equal(t, "tl-value", string(data))
}

func TestEndToEnd_ConsistentHashDistribution(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	ch := distributed.NewConsistentHash(150)
	nodes := []string{"node-a", "node-b", "node-c", "node-d"}
	for _, n := range nodes {
		ch.AddNode(n)
	}
	assert.Equal(t, 4, ch.NodeCount())

	// Distribute 1000 keys and check distribution
	distribution := make(map[string]int)
	for i := 0; i < 1000; i++ {
		key := fmt.Sprintf("dist-key-%d", i)
		node := ch.GetNode(key)
		assert.NotEmpty(t, node)
		distribution[node]++
	}

	// Each node should have gotten some keys
	for _, node := range nodes {
		count := distribution[node]
		assert.True(t, count > 50,
			"node %s should have at least 50 keys, got %d", node, count)
	}

	// Remove a node and verify redistribution
	ch.RemoveNode("node-a")
	assert.Equal(t, 3, ch.NodeCount())

	for i := 0; i < 100; i++ {
		key := fmt.Sprintf("dist-key-%d", i)
		node := ch.GetNode(key)
		assert.NotEqual(t, "node-a", node, "removed node should not receive keys")
	}
}

func TestEndToEnd_PolicyComposition(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	age := policy.NewAgeEviction(time.Hour)
	freq := policy.NewFrequencyEviction(5)
	cap := policy.NewCapacityEviction(0.8)
	composite := policy.NewCompositeEviction(age, freq, cap)

	stats := policy.EvictionStats{
		TotalEntries:     90,
		MaxEntries:       100,
		EntryAge:         30 * time.Minute,
		EntryAccessCount: 3,
	}

	// Frequency eviction triggers (accessCount < 5)
	assert.True(t, composite.ShouldEvict("key-1", stats))

	// All conditions passing -- no eviction
	statsOK := policy.EvictionStats{
		TotalEntries:     50,
		MaxEntries:       100,
		EntryAge:         10 * time.Minute,
		EntryAccessCount: 10,
	}
	assert.False(t, composite.ShouldEvict("key-2", statsOK))
}

func TestEndToEnd_WriteBackFlush(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	c := memory.New(memory.DefaultConfig())
	defer func() { _ = c.Close() }()

	src := &syncDataSource{data: make(map[string][]byte)}
	wb := distributed.NewWriteBack(c, src)
	ctx := context.Background()

	// Write entries -- only cache should be populated
	for i := 0; i < 20; i++ {
		key := fmt.Sprintf("wb-key-%d", i)
		err := wb.Set(ctx, key, []byte(fmt.Sprintf("wb-val-%d", i)), time.Minute)
		require.NoError(t, err)
	}
	assert.Equal(t, 20, wb.DirtyCount())

	// Source should have nothing yet
	srcData, err := src.Load(ctx, "wb-key-0")
	require.NoError(t, err)
	assert.Nil(t, srcData)

	// Flush writes all dirty entries to source
	err = wb.Flush(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, wb.DirtyCount())

	// Source should now have all entries
	for i := 0; i < 20; i++ {
		key := fmt.Sprintf("wb-key-%d", i)
		d, err := src.Load(ctx, key)
		require.NoError(t, err)
		assert.Equal(t, fmt.Sprintf("wb-val-%d", i), string(d))
	}
}

// syncDataSource implements distributed.DataSource with mutex protection.
type syncDataSource struct {
	mu   sync.Mutex
	data map[string][]byte
}

func (s *syncDataSource) Load(_ context.Context, key string) ([]byte, error) {
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

func (s *syncDataSource) Store(_ context.Context, key string, value []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make([]byte, len(value))
	copy(cp, value)
	s.data[key] = cp
	return nil
}

func (s *syncDataSource) Remove(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, key)
	return nil
}
