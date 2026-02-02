package distributed

import (
	"context"
	"fmt"
	"testing"
	"time"

	"digital.vasic.cache/pkg/cache"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- stubCache for testing ---

type stubCache struct {
	data map[string][]byte
}

func newStub() *stubCache { return &stubCache{data: make(map[string][]byte)} }

func (s *stubCache) Get(_ context.Context, key string) ([]byte, error) {
	v := s.data[key]
	if v == nil {
		return nil, nil
	}
	cp := make([]byte, len(v))
	copy(cp, v)
	return cp, nil
}
func (s *stubCache) Set(_ context.Context, key string, val []byte, _ time.Duration) error {
	cp := make([]byte, len(val))
	copy(cp, val)
	s.data[key] = cp
	return nil
}
func (s *stubCache) Delete(_ context.Context, key string) error {
	delete(s.data, key)
	return nil
}
func (s *stubCache) Exists(_ context.Context, key string) (bool, error) {
	_, ok := s.data[key]
	return ok, nil
}
func (s *stubCache) Close() error { return nil }

var _ cache.Cache = (*stubCache)(nil)

// --- stubDataSource ---

type stubDataSource struct {
	data map[string][]byte
}

func newStubDS() *stubDataSource { return &stubDataSource{data: make(map[string][]byte)} }

func (s *stubDataSource) Load(_ context.Context, key string) ([]byte, error) {
	v := s.data[key]
	if v == nil {
		return nil, nil
	}
	cp := make([]byte, len(v))
	copy(cp, v)
	return cp, nil
}
func (s *stubDataSource) Store(_ context.Context, key string, val []byte) error {
	cp := make([]byte, len(val))
	copy(cp, val)
	s.data[key] = cp
	return nil
}
func (s *stubDataSource) Remove(_ context.Context, key string) error {
	delete(s.data, key)
	return nil
}

// --- ConsistentHash tests ---

func TestConsistentHash_AddAndGet(t *testing.T) {
	tests := []struct {
		name  string
		nodes []string
		key   string
	}{
		{name: "single node", nodes: []string{"node1"}, key: "mykey"},
		{name: "two nodes", nodes: []string{"node1", "node2"}, key: "test"},
		{name: "three nodes", nodes: []string{"a", "b", "c"}, key: "hello"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ch := NewConsistentHash(50)
			for _, n := range tt.nodes {
				ch.AddNode(n)
			}

			node := ch.GetNode(tt.key)
			assert.NotEmpty(t, node)
			assert.Equal(t, len(tt.nodes), ch.NodeCount())

			// Same key should always map to same node
			assert.Equal(t, node, ch.GetNode(tt.key))
		})
	}
}

func TestConsistentHash_EmptyRing(t *testing.T) {
	ch := NewConsistentHash(10)
	assert.Empty(t, ch.GetNode("key"))
	assert.Equal(t, 0, ch.NodeCount())
}

func TestConsistentHash_RemoveNode(t *testing.T) {
	ch := NewConsistentHash(50)
	ch.AddNode("node1")
	ch.AddNode("node2")
	assert.Equal(t, 2, ch.NodeCount())

	ch.RemoveNode("node1")
	assert.Equal(t, 1, ch.NodeCount())

	// All keys should now go to node2
	for i := 0; i < 20; i++ {
		assert.Equal(t, "node2", ch.GetNode(fmt.Sprintf("key%d", i)))
	}
}

func TestConsistentHash_DefaultReplicas(t *testing.T) {
	ch := NewConsistentHash(0)
	assert.Equal(t, 100, ch.replicas)
}

func TestConsistentHash_Distribution(t *testing.T) {
	ch := NewConsistentHash(100)
	ch.AddNode("node1")
	ch.AddNode("node2")
	ch.AddNode("node3")

	counts := make(map[string]int)
	for i := 0; i < 1000; i++ {
		node := ch.GetNode(fmt.Sprintf("key-%d", i))
		counts[node]++
	}

	// Each node should get at least some keys (rough check)
	for _, node := range []string{"node1", "node2", "node3"} {
		assert.Greater(t, counts[node], 100,
			"node %s got too few keys: %d", node, counts[node])
	}
}

// --- TwoLevel tests ---

func TestTwoLevel_GetFromLocal(t *testing.T) {
	local := newStub()
	remote := newStub()
	tl := NewTwoLevel(local, remote, time.Minute)
	defer tl.Close()
	ctx := context.Background()

	local.data["key"] = []byte("from-local")

	got, err := tl.Get(ctx, "key")
	require.NoError(t, err)
	assert.Equal(t, []byte("from-local"), got)
}

func TestTwoLevel_GetFromRemotePromotes(t *testing.T) {
	local := newStub()
	remote := newStub()
	tl := NewTwoLevel(local, remote, time.Minute)
	defer tl.Close()
	ctx := context.Background()

	remote.data["key"] = []byte("from-remote")

	got, err := tl.Get(ctx, "key")
	require.NoError(t, err)
	assert.Equal(t, []byte("from-remote"), got)

	// Should now be in local
	assert.Equal(t, []byte("from-remote"), local.data["key"])
}

func TestTwoLevel_GetMiss(t *testing.T) {
	tl := NewTwoLevel(newStub(), newStub(), time.Minute)
	defer tl.Close()

	got, err := tl.Get(context.Background(), "missing")
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestTwoLevel_Set(t *testing.T) {
	local := newStub()
	remote := newStub()
	tl := NewTwoLevel(local, remote, time.Minute)
	defer tl.Close()
	ctx := context.Background()

	err := tl.Set(ctx, "key", []byte("val"), 5*time.Minute)
	require.NoError(t, err)

	assert.Equal(t, []byte("val"), local.data["key"])
	assert.Equal(t, []byte("val"), remote.data["key"])
}

func TestTwoLevel_Delete(t *testing.T) {
	local := newStub()
	remote := newStub()
	tl := NewTwoLevel(local, remote, time.Minute)
	ctx := context.Background()

	tl.Set(ctx, "key", []byte("val"), time.Minute)
	err := tl.Delete(ctx, "key")
	require.NoError(t, err)

	assert.Nil(t, local.data["key"])
	assert.Nil(t, remote.data["key"])
}

func TestTwoLevel_Exists(t *testing.T) {
	tests := []struct {
		name      string
		localKey  bool
		remoteKey bool
		expected  bool
	}{
		{"in local", true, false, true},
		{"in remote", false, true, true},
		{"in both", true, true, true},
		{"in neither", false, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			local := newStub()
			remote := newStub()
			if tt.localKey {
				local.data["key"] = []byte("v")
			}
			if tt.remoteKey {
				remote.data["key"] = []byte("v")
			}
			tl := NewTwoLevel(local, remote, time.Minute)

			ok, err := tl.Exists(context.Background(), "key")
			require.NoError(t, err)
			assert.Equal(t, tt.expected, ok)
		})
	}
}

func TestTwoLevel_DefaultL1TTL(t *testing.T) {
	tl := NewTwoLevel(newStub(), newStub(), 0)
	assert.Equal(t, 5*time.Minute, tl.l1TTL)
}

// --- WriteThrough tests ---

func TestWriteThrough_SetAndGet(t *testing.T) {
	c := newStub()
	src := newStubDS()
	wt := NewWriteThrough(c, src)
	ctx := context.Background()

	assert.Equal(t, "write-through", wt.Name())

	err := wt.Set(ctx, "key", []byte("val"), time.Minute)
	require.NoError(t, err)

	// Should be in both cache and source
	assert.Equal(t, []byte("val"), c.data["key"])
	assert.Equal(t, []byte("val"), src.data["key"])

	got, err := wt.Get(ctx, "key")
	require.NoError(t, err)
	assert.Equal(t, []byte("val"), got)
}

func TestWriteThrough_GetFromSourceOnMiss(t *testing.T) {
	c := newStub()
	src := newStubDS()
	src.data["key"] = []byte("from-source")
	wt := NewWriteThrough(c, src)
	ctx := context.Background()

	got, err := wt.Get(ctx, "key")
	require.NoError(t, err)
	assert.Equal(t, []byte("from-source"), got)

	// Should now be in cache
	assert.Equal(t, []byte("from-source"), c.data["key"])
}

func TestWriteThrough_Delete(t *testing.T) {
	c := newStub()
	src := newStubDS()
	wt := NewWriteThrough(c, src)
	ctx := context.Background()

	wt.Set(ctx, "key", []byte("val"), time.Minute)
	err := wt.Delete(ctx, "key")
	require.NoError(t, err)

	assert.Nil(t, c.data["key"])
	assert.Nil(t, src.data["key"])
}

// --- WriteBack tests ---

func TestWriteBack_SetAndFlush(t *testing.T) {
	c := newStub()
	src := newStubDS()
	wb := NewWriteBack(c, src)
	ctx := context.Background()

	assert.Equal(t, "write-back", wb.Name())

	err := wb.Set(ctx, "key", []byte("val"), time.Minute)
	require.NoError(t, err)

	// In cache but not yet in source
	assert.Equal(t, []byte("val"), c.data["key"])
	assert.Nil(t, src.data["key"])
	assert.Equal(t, 1, wb.DirtyCount())

	// Flush
	err = wb.Flush(ctx)
	require.NoError(t, err)
	assert.Equal(t, []byte("val"), src.data["key"])
	assert.Equal(t, 0, wb.DirtyCount())
}

func TestWriteBack_GetFromSourceOnMiss(t *testing.T) {
	c := newStub()
	src := newStubDS()
	src.data["key"] = []byte("from-source")
	wb := NewWriteBack(c, src)
	ctx := context.Background()

	got, err := wb.Get(ctx, "key")
	require.NoError(t, err)
	assert.Equal(t, []byte("from-source"), got)
}

func TestWriteBack_Delete(t *testing.T) {
	c := newStub()
	src := newStubDS()
	wb := NewWriteBack(c, src)
	ctx := context.Background()

	wb.Set(ctx, "key", []byte("val"), time.Minute)
	err := wb.Delete(ctx, "key")
	require.NoError(t, err)

	assert.Nil(t, c.data["key"])
	assert.Equal(t, 0, wb.DirtyCount())
}

// --- CacheAside tests ---

func TestCacheAside_GetPopulatesCache(t *testing.T) {
	c := newStub()
	src := newStubDS()
	src.data["key"] = []byte("from-source")
	ca := NewCacheAside(c, src)
	ctx := context.Background()

	assert.Equal(t, "cache-aside", ca.Name())

	got, err := ca.Get(ctx, "key")
	require.NoError(t, err)
	assert.Equal(t, []byte("from-source"), got)

	// Now in cache
	assert.Equal(t, []byte("from-source"), c.data["key"])
}

func TestCacheAside_GetFromCache(t *testing.T) {
	c := newStub()
	src := newStubDS()
	c.data["key"] = []byte("cached")
	ca := NewCacheAside(c, src)
	ctx := context.Background()

	got, err := ca.Get(ctx, "key")
	require.NoError(t, err)
	assert.Equal(t, []byte("cached"), got)
}

func TestCacheAside_SetInvalidatesCache(t *testing.T) {
	c := newStub()
	src := newStubDS()
	c.data["key"] = []byte("old")
	ca := NewCacheAside(c, src)
	ctx := context.Background()

	err := ca.Set(ctx, "key", []byte("new"), time.Minute)
	require.NoError(t, err)

	// Cache should be invalidated
	assert.Nil(t, c.data["key"])
	// Source should have new value
	assert.Equal(t, []byte("new"), src.data["key"])
}

func TestCacheAside_Delete(t *testing.T) {
	c := newStub()
	src := newStubDS()
	ca := NewCacheAside(c, src)
	ctx := context.Background()

	c.data["key"] = []byte("cached")
	src.data["key"] = []byte("stored")

	err := ca.Delete(ctx, "key")
	require.NoError(t, err)

	assert.Nil(t, c.data["key"])
	assert.Nil(t, src.data["key"])
}

func TestCacheAside_GetMiss(t *testing.T) {
	ca := NewCacheAside(newStub(), newStubDS())
	got, err := ca.Get(context.Background(), "nope")
	require.NoError(t, err)
	assert.Nil(t, got)
}
