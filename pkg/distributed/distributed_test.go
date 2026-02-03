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

	// Verify the data was promoted to cache
	assert.Equal(t, []byte("from-source"), c.data["key"])
}

func TestWriteBack_GetFromCache(t *testing.T) {
	// Test the cache hit case (data != nil after cache Get)
	c := newStub()
	src := newStubDS()
	c.data["key"] = []byte("cached-value")
	wb := NewWriteBack(c, src)
	ctx := context.Background()

	got, err := wb.Get(ctx, "key")
	require.NoError(t, err)
	assert.Equal(t, []byte("cached-value"), got)

	// Source should not be consulted, so it should still be empty
	assert.Nil(t, src.data["key"])
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

// --- Error-returning stubs for coverage ---

// errorCache is a cache stub that returns errors for all operations.
type errorCache struct {
	getErr    error
	setErr    error
	deleteErr error
	existsErr error
	closeErr  error
	data      map[string][]byte
}

func newErrorCache() *errorCache {
	return &errorCache{data: make(map[string][]byte)}
}

func (e *errorCache) Get(_ context.Context, key string) ([]byte, error) {
	if e.getErr != nil {
		return nil, e.getErr
	}
	v := e.data[key]
	if v == nil {
		return nil, nil
	}
	cp := make([]byte, len(v))
	copy(cp, v)
	return cp, nil
}

func (e *errorCache) Set(_ context.Context, key string, val []byte, _ time.Duration) error {
	if e.setErr != nil {
		return e.setErr
	}
	cp := make([]byte, len(val))
	copy(cp, val)
	e.data[key] = cp
	return nil
}

func (e *errorCache) Delete(_ context.Context, key string) error {
	if e.deleteErr != nil {
		return e.deleteErr
	}
	delete(e.data, key)
	return nil
}

func (e *errorCache) Exists(_ context.Context, key string) (bool, error) {
	if e.existsErr != nil {
		return false, e.existsErr
	}
	_, ok := e.data[key]
	return ok, nil
}

func (e *errorCache) Close() error {
	return e.closeErr
}

var _ cache.Cache = (*errorCache)(nil)

// errorDataSource is a data source stub that returns errors.
type errorDataSource struct {
	loadErr   error
	storeErr  error
	removeErr error
	data      map[string][]byte
}

func newErrorDS() *errorDataSource {
	return &errorDataSource{data: make(map[string][]byte)}
}

func (e *errorDataSource) Load(_ context.Context, key string) ([]byte, error) {
	if e.loadErr != nil {
		return nil, e.loadErr
	}
	v := e.data[key]
	if v == nil {
		return nil, nil
	}
	cp := make([]byte, len(v))
	copy(cp, v)
	return cp, nil
}

func (e *errorDataSource) Store(_ context.Context, key string, val []byte) error {
	if e.storeErr != nil {
		return e.storeErr
	}
	cp := make([]byte, len(val))
	copy(cp, val)
	e.data[key] = cp
	return nil
}

func (e *errorDataSource) Remove(_ context.Context, key string) error {
	if e.removeErr != nil {
		return e.removeErr
	}
	delete(e.data, key)
	return nil
}

// --- TwoLevel error tests ---

func TestTwoLevel_GetLocalError(t *testing.T) {
	local := newErrorCache()
	local.getErr = fmt.Errorf("local get failure")
	remote := newStub()
	tl := NewTwoLevel(local, remote, time.Minute)
	ctx := context.Background()

	_, err := tl.Get(ctx, "key")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "two-level local get")
	assert.Contains(t, err.Error(), "local get failure")
}

func TestTwoLevel_GetRemoteError(t *testing.T) {
	local := newStub()
	remote := newErrorCache()
	remote.getErr = fmt.Errorf("remote get failure")
	tl := NewTwoLevel(local, remote, time.Minute)
	ctx := context.Background()

	_, err := tl.Get(ctx, "key")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "two-level remote get")
	assert.Contains(t, err.Error(), "remote get failure")
}

func TestTwoLevel_SetLocalError(t *testing.T) {
	local := newErrorCache()
	local.setErr = fmt.Errorf("local set failure")
	remote := newStub()
	tl := NewTwoLevel(local, remote, time.Minute)
	ctx := context.Background()

	err := tl.Set(ctx, "key", []byte("val"), time.Minute)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "two-level local set")
	assert.Contains(t, err.Error(), "local set failure")
}

func TestTwoLevel_SetRemoteError(t *testing.T) {
	local := newStub()
	remote := newErrorCache()
	remote.setErr = fmt.Errorf("remote set failure")
	tl := NewTwoLevel(local, remote, time.Minute)
	ctx := context.Background()

	err := tl.Set(ctx, "key", []byte("val"), time.Minute)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "two-level remote set")
	assert.Contains(t, err.Error(), "remote set failure")
}

func TestTwoLevel_DeleteLocalError(t *testing.T) {
	local := newErrorCache()
	local.deleteErr = fmt.Errorf("local delete failure")
	remote := newStub()
	tl := NewTwoLevel(local, remote, time.Minute)
	ctx := context.Background()

	err := tl.Delete(ctx, "key")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "two-level local delete")
	assert.Contains(t, err.Error(), "local delete failure")
}

func TestTwoLevel_DeleteRemoteError(t *testing.T) {
	local := newStub()
	remote := newErrorCache()
	remote.deleteErr = fmt.Errorf("remote delete failure")
	tl := NewTwoLevel(local, remote, time.Minute)
	ctx := context.Background()

	err := tl.Delete(ctx, "key")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "two-level remote delete")
	assert.Contains(t, err.Error(), "remote delete failure")
}

func TestTwoLevel_ExistsLocalError(t *testing.T) {
	local := newErrorCache()
	local.existsErr = fmt.Errorf("local exists failure")
	remote := newStub()
	tl := NewTwoLevel(local, remote, time.Minute)
	ctx := context.Background()

	_, err := tl.Exists(ctx, "key")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "two-level local exists")
	assert.Contains(t, err.Error(), "local exists failure")
}

func TestTwoLevel_ExistsRemoteError(t *testing.T) {
	local := newStub()
	remote := newErrorCache()
	remote.existsErr = fmt.Errorf("remote exists failure")
	tl := NewTwoLevel(local, remote, time.Minute)
	ctx := context.Background()

	_, err := tl.Exists(ctx, "key")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "two-level remote exists")
	assert.Contains(t, err.Error(), "remote exists failure")
}

func TestTwoLevel_CloseLocalError(t *testing.T) {
	local := newErrorCache()
	local.closeErr = fmt.Errorf("local close failure")
	remote := newStub()
	tl := NewTwoLevel(local, remote, time.Minute)

	err := tl.Close()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "local close failure")
}

func TestTwoLevel_CloseRemoteError(t *testing.T) {
	local := newStub()
	remote := newErrorCache()
	remote.closeErr = fmt.Errorf("remote close failure")
	tl := NewTwoLevel(local, remote, time.Minute)

	err := tl.Close()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "remote close failure")
}

func TestTwoLevel_CloseBothErrors(t *testing.T) {
	local := newErrorCache()
	local.closeErr = fmt.Errorf("local close failure")
	remote := newErrorCache()
	remote.closeErr = fmt.Errorf("remote close failure")
	tl := NewTwoLevel(local, remote, time.Minute)

	err := tl.Close()
	require.Error(t, err)
	// Local error takes precedence
	assert.Contains(t, err.Error(), "local close failure")
}

func TestTwoLevel_SetWithShorterTTL(t *testing.T) {
	local := newStub()
	remote := newStub()
	tl := NewTwoLevel(local, remote, 10*time.Minute)
	ctx := context.Background()

	// TTL shorter than l1TTL should be used for local
	err := tl.Set(ctx, "key", []byte("val"), 2*time.Minute)
	require.NoError(t, err)

	assert.Equal(t, []byte("val"), local.data["key"])
	assert.Equal(t, []byte("val"), remote.data["key"])
}

// --- WriteThrough error tests ---

func TestWriteThrough_GetCacheError(t *testing.T) {
	c := newErrorCache()
	c.getErr = fmt.Errorf("cache get failure")
	src := newStubDS()
	wt := NewWriteThrough(c, src)
	ctx := context.Background()

	_, err := wt.Get(ctx, "key")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cache get failure")
}

func TestWriteThrough_GetSourceLoadError(t *testing.T) {
	c := newStub()
	src := newErrorDS()
	src.loadErr = fmt.Errorf("source load failure")
	wt := NewWriteThrough(c, src)
	ctx := context.Background()

	_, err := wt.Get(ctx, "key")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "source load failure")
}

func TestWriteThrough_SetSourceStoreError(t *testing.T) {
	c := newStub()
	src := newErrorDS()
	src.storeErr = fmt.Errorf("source store failure")
	wt := NewWriteThrough(c, src)
	ctx := context.Background()

	err := wt.Set(ctx, "key", []byte("val"), time.Minute)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "write-through source store")
	assert.Contains(t, err.Error(), "source store failure")
}

func TestWriteThrough_SetCacheSetError(t *testing.T) {
	c := newErrorCache()
	c.setErr = fmt.Errorf("cache set failure")
	src := newStubDS()
	wt := NewWriteThrough(c, src)
	ctx := context.Background()

	err := wt.Set(ctx, "key", []byte("val"), time.Minute)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "write-through cache set")
	assert.Contains(t, err.Error(), "cache set failure")
}

func TestWriteThrough_DeleteSourceRemoveError(t *testing.T) {
	c := newStub()
	src := newErrorDS()
	src.removeErr = fmt.Errorf("source remove failure")
	wt := NewWriteThrough(c, src)
	ctx := context.Background()

	err := wt.Delete(ctx, "key")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "write-through source remove")
	assert.Contains(t, err.Error(), "source remove failure")
}

func TestWriteThrough_DeleteCacheDeleteError(t *testing.T) {
	c := newErrorCache()
	c.deleteErr = fmt.Errorf("cache delete failure")
	src := newStubDS()
	wt := NewWriteThrough(c, src)
	ctx := context.Background()

	err := wt.Delete(ctx, "key")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cache delete failure")
}

// --- WriteBack error tests ---

func TestWriteBack_GetCacheError(t *testing.T) {
	c := newErrorCache()
	c.getErr = fmt.Errorf("cache get failure")
	src := newStubDS()
	wb := NewWriteBack(c, src)
	ctx := context.Background()

	_, err := wb.Get(ctx, "key")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cache get failure")
}

func TestWriteBack_GetSourceLoadError(t *testing.T) {
	c := newStub()
	src := newErrorDS()
	src.loadErr = fmt.Errorf("source load failure")
	wb := NewWriteBack(c, src)
	ctx := context.Background()

	_, err := wb.Get(ctx, "key")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "source load failure")
}

func TestWriteBack_SetCacheSetError(t *testing.T) {
	c := newErrorCache()
	c.setErr = fmt.Errorf("cache set failure")
	src := newStubDS()
	wb := NewWriteBack(c, src)
	ctx := context.Background()

	err := wb.Set(ctx, "key", []byte("val"), time.Minute)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "write-back cache set")
	assert.Contains(t, err.Error(), "cache set failure")
}

func TestWriteBack_DeleteSourceRemoveError(t *testing.T) {
	c := newStub()
	src := newErrorDS()
	src.removeErr = fmt.Errorf("source remove failure")
	wb := NewWriteBack(c, src)
	ctx := context.Background()

	wb.Set(ctx, "key", []byte("val"), time.Minute)
	err := wb.Delete(ctx, "key")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "write-back source remove")
	assert.Contains(t, err.Error(), "source remove failure")
}

func TestWriteBack_DeleteCacheDeleteError(t *testing.T) {
	c := newErrorCache()
	c.deleteErr = fmt.Errorf("cache delete failure")
	src := newStubDS()
	wb := NewWriteBack(c, src)
	ctx := context.Background()

	err := wb.Delete(ctx, "key")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cache delete failure")
}

func TestWriteBack_FlushStoreError(t *testing.T) {
	c := newStub()
	src := newErrorDS()
	wb := NewWriteBack(c, src)
	ctx := context.Background()

	// First set succeeds (no store error during set for write-back)
	err := wb.Set(ctx, "key", []byte("val"), time.Minute)
	require.NoError(t, err)

	// Now enable store error
	src.storeErr = fmt.Errorf("store failure on flush")

	err = wb.Flush(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "write-back flush")
	assert.Contains(t, err.Error(), "store failure on flush")
}

func TestWriteBack_FlushEmptyDirty(t *testing.T) {
	c := newStub()
	src := newStubDS()
	wb := NewWriteBack(c, src)
	ctx := context.Background()

	// Flush with no dirty entries should succeed
	err := wb.Flush(ctx)
	require.NoError(t, err)
}

func TestWriteBack_MultipleEntriesFlush(t *testing.T) {
	c := newStub()
	src := newStubDS()
	wb := NewWriteBack(c, src)
	ctx := context.Background()

	// Set multiple keys
	for i := 0; i < 5; i++ {
		key := fmt.Sprintf("key%d", i)
		val := fmt.Sprintf("val%d", i)
		err := wb.Set(ctx, key, []byte(val), time.Minute)
		require.NoError(t, err)
	}

	assert.Equal(t, 5, wb.DirtyCount())

	err := wb.Flush(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, wb.DirtyCount())

	// Verify all entries in source
	for i := 0; i < 5; i++ {
		key := fmt.Sprintf("key%d", i)
		val := fmt.Sprintf("val%d", i)
		assert.Equal(t, []byte(val), src.data[key])
	}
}

// --- CacheAside error tests ---

func TestCacheAside_GetCacheError(t *testing.T) {
	c := newErrorCache()
	c.getErr = fmt.Errorf("cache get failure")
	src := newStubDS()
	ca := NewCacheAside(c, src)
	ctx := context.Background()

	_, err := ca.Get(ctx, "key")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cache get failure")
}

func TestCacheAside_GetSourceLoadError(t *testing.T) {
	c := newStub()
	src := newErrorDS()
	src.loadErr = fmt.Errorf("source load failure")
	ca := NewCacheAside(c, src)
	ctx := context.Background()

	_, err := ca.Get(ctx, "key")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "source load failure")
}

func TestCacheAside_SetSourceStoreError(t *testing.T) {
	c := newStub()
	src := newErrorDS()
	src.storeErr = fmt.Errorf("source store failure")
	ca := NewCacheAside(c, src)
	ctx := context.Background()

	err := ca.Set(ctx, "key", []byte("val"), time.Minute)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cache-aside source store")
	assert.Contains(t, err.Error(), "source store failure")
}

func TestCacheAside_SetCacheDeleteError(t *testing.T) {
	c := newErrorCache()
	c.deleteErr = fmt.Errorf("cache delete failure")
	src := newStubDS()
	ca := NewCacheAside(c, src)
	ctx := context.Background()

	err := ca.Set(ctx, "key", []byte("val"), time.Minute)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cache delete failure")
}

func TestCacheAside_DeleteSourceRemoveError(t *testing.T) {
	c := newStub()
	src := newErrorDS()
	src.removeErr = fmt.Errorf("source remove failure")
	ca := NewCacheAside(c, src)
	ctx := context.Background()

	err := ca.Delete(ctx, "key")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cache-aside source remove")
	assert.Contains(t, err.Error(), "source remove failure")
}

func TestCacheAside_DeleteCacheDeleteError(t *testing.T) {
	c := newErrorCache()
	c.deleteErr = fmt.Errorf("cache delete failure")
	src := newStubDS()
	ca := NewCacheAside(c, src)
	ctx := context.Background()

	err := ca.Delete(ctx, "key")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cache delete failure")
}

// --- ConsistentHash additional edge case tests ---

func TestConsistentHash_HashWrapAround(t *testing.T) {
	ch := NewConsistentHash(10)
	ch.AddNode("node1")

	// Generate many keys to exercise the wrap-around case (idx >= len(ch.ring))
	foundWrapAround := false
	for i := 0; i < 10000; i++ {
		key := fmt.Sprintf("test-key-%d", i)
		node := ch.GetNode(key)
		if node == "node1" {
			foundWrapAround = true
		}
	}
	assert.True(t, foundWrapAround, "should find node via wrap-around path")
}

func TestConsistentHash_RemoveNonExistentNode(t *testing.T) {
	ch := NewConsistentHash(10)
	ch.AddNode("node1")
	ch.AddNode("node2")

	// Removing a node that doesn't exist should be safe
	ch.RemoveNode("nonexistent")
	assert.Equal(t, 2, ch.NodeCount())
}

func TestConsistentHash_AddSameNodeTwice(t *testing.T) {
	ch := NewConsistentHash(10)
	ch.AddNode("node1")
	ch.AddNode("node1")

	// Adding the same node twice adds more virtual nodes
	// NodeCount should still show 1 unique physical node
	assert.Equal(t, 1, ch.NodeCount())
	// But the ring should have 20 entries (10 * 2)
	assert.Equal(t, 20, len(ch.ring))
}

func TestConsistentHash_NegativeReplicas(t *testing.T) {
	ch := NewConsistentHash(-5)
	assert.Equal(t, 100, ch.replicas)
}

// --- Strategy interface compliance tests ---

func TestStrategy_WriteThrough_Interface(t *testing.T) {
	var s Strategy = NewWriteThrough(newStub(), newStubDS())
	assert.Equal(t, "write-through", s.Name())
}

func TestStrategy_WriteBack_Interface(t *testing.T) {
	var s Strategy = NewWriteBack(newStub(), newStubDS())
	assert.Equal(t, "write-back", s.Name())
}

func TestStrategy_CacheAside_Interface(t *testing.T) {
	var s Strategy = NewCacheAside(newStub(), newStubDS())
	assert.Equal(t, "cache-aside", s.Name())
}

func TestWriteBack_GetMissReturnsNil(t *testing.T) {
	// Test the case where both cache miss and source returns nil
	c := newStub()
	src := newStubDS()
	wb := NewWriteBack(c, src)
	ctx := context.Background()

	// Get a key that doesn't exist anywhere
	got, err := wb.Get(ctx, "nonexistent")
	require.NoError(t, err)
	assert.Nil(t, got)

	// Also verify the cache was NOT populated (because source returned nil)
	_, exists := c.data["nonexistent"]
	assert.False(t, exists)
}
