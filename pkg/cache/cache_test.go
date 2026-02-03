package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- EvictionPolicy tests ---

func TestEvictionPolicy_String(t *testing.T) {
	tests := []struct {
		name     string
		policy   EvictionPolicy
		expected string
	}{
		{name: "LRU", policy: LRU, expected: "LRU"},
		{name: "LFU", policy: LFU, expected: "LFU"},
		{name: "FIFO", policy: FIFO, expected: "FIFO"},
		{name: "unknown", policy: EvictionPolicy(99), expected: "EvictionPolicy(99)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.policy.String())
		})
	}
}

// --- Config tests ---

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	assert.Equal(t, 30*time.Minute, cfg.DefaultTTL)
	assert.Equal(t, 10000, cfg.MaxSize)
	assert.Equal(t, LRU, cfg.EvictionPolicy)
}

// --- Stats tests ---

func TestStats_HitRate(t *testing.T) {
	tests := []struct {
		name     string
		stats    Stats
		expected float64
	}{
		{name: "no requests", stats: Stats{Hits: 0, Misses: 0}, expected: 0},
		{name: "all hits", stats: Stats{Hits: 100, Misses: 0}, expected: 100},
		{name: "all misses", stats: Stats{Hits: 0, Misses: 100}, expected: 0},
		{name: "half hits", stats: Stats{Hits: 50, Misses: 50}, expected: 50},
		{name: "75% hits", stats: Stats{Hits: 75, Misses: 25}, expected: 75},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.InDelta(t, tt.expected, tt.stats.HitRate(), 0.01)
		})
	}
}

// --- stubCache for TypedCache tests ---

type stubCache struct {
	data map[string][]byte
}

func newStubCache() *stubCache {
	return &stubCache{data: make(map[string][]byte)}
}

func (s *stubCache) Get(_ context.Context, key string) ([]byte, error) {
	v, ok := s.data[key]
	if !ok {
		return nil, nil
	}
	return v, nil
}

func (s *stubCache) Set(_ context.Context, key string, value []byte, _ time.Duration) error {
	s.data[key] = value
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

func (s *stubCache) Close() error {
	return nil
}

// errCache always returns errors
type errCache struct{}

func (e *errCache) Get(_ context.Context, _ string) ([]byte, error) {
	return nil, fmt.Errorf("get error")
}
func (e *errCache) Set(_ context.Context, _ string, _ []byte, _ time.Duration) error {
	return fmt.Errorf("set error")
}
func (e *errCache) Delete(_ context.Context, _ string) error {
	return fmt.Errorf("delete error")
}
func (e *errCache) Exists(_ context.Context, _ string) (bool, error) {
	return false, fmt.Errorf("exists error")
}
func (e *errCache) Close() error { return nil }

// --- TypedCache tests ---

type testItem struct {
	Name  string `json:"name"`
	Value int    `json:"value"`
}

func TestTypedCache_SetAndGet(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		value testItem
	}{
		{
			name:  "simple item",
			key:   "item1",
			value: testItem{Name: "alpha", Value: 42},
		},
		{
			name:  "empty name",
			key:   "item2",
			value: testItem{Name: "", Value: 0},
		},
		{
			name:  "large value",
			key:   "item3",
			value: testItem{Name: "big", Value: 999999},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tc := NewTypedCache[testItem](newStubCache())
			ctx := context.Background()

			err := tc.Set(ctx, tt.key, tt.value, time.Minute)
			require.NoError(t, err)

			got, found, err := tc.Get(ctx, tt.key)
			require.NoError(t, err)
			assert.True(t, found)
			assert.Equal(t, tt.value, got)
		})
	}
}

func TestTypedCache_GetMiss(t *testing.T) {
	tc := NewTypedCache[testItem](newStubCache())
	ctx := context.Background()

	got, found, err := tc.Get(ctx, "nonexistent")
	require.NoError(t, err)
	assert.False(t, found)
	assert.Equal(t, testItem{}, got)
}

func TestTypedCache_GetUnmarshalError(t *testing.T) {
	stub := newStubCache()
	stub.data["bad"] = []byte("not valid json{{{")
	tc := NewTypedCache[testItem](stub)
	ctx := context.Background()

	_, _, err := tc.Get(ctx, "bad")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "typed cache unmarshal")
}

func TestTypedCache_GetError(t *testing.T) {
	tc := NewTypedCache[testItem](&errCache{})
	ctx := context.Background()

	_, _, err := tc.Get(ctx, "key")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "typed cache get")
}

func TestTypedCache_Delete(t *testing.T) {
	tc := NewTypedCache[testItem](newStubCache())
	ctx := context.Background()

	err := tc.Set(ctx, "key", testItem{Name: "x", Value: 1}, time.Minute)
	require.NoError(t, err)

	err = tc.Delete(ctx, "key")
	require.NoError(t, err)

	_, found, err := tc.Get(ctx, "key")
	require.NoError(t, err)
	assert.False(t, found)
}

func TestTypedCache_Exists(t *testing.T) {
	tc := NewTypedCache[testItem](newStubCache())
	ctx := context.Background()

	exists, err := tc.Exists(ctx, "key")
	require.NoError(t, err)
	assert.False(t, exists)

	err = tc.Set(ctx, "key", testItem{Name: "x", Value: 1}, time.Minute)
	require.NoError(t, err)

	exists, err = tc.Exists(ctx, "key")
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestTypedCache_Close(t *testing.T) {
	tc := NewTypedCache[testItem](newStubCache())
	assert.NoError(t, tc.Close())
}

func TestTypedCache_SetMarshalError(t *testing.T) {
	// json.Marshal cannot fail on a simple struct, but we can test
	// that the path works by verifying the data round-trips.
	stub := newStubCache()
	tc := NewTypedCache[testItem](stub)
	ctx := context.Background()

	err := tc.Set(ctx, "key", testItem{Name: "test", Value: 1}, time.Minute)
	require.NoError(t, err)

	// Verify the stored bytes are valid JSON
	raw := stub.data["key"]
	var decoded testItem
	require.NoError(t, json.Unmarshal(raw, &decoded))
	assert.Equal(t, "test", decoded.Name)
}

// unmarshalableType is a type that cannot be marshaled to JSON
type unmarshalableType struct {
	Ch chan int `json:"ch"`
}

func TestTypedCache_SetMarshalErrorPath(t *testing.T) {
	stub := newStubCache()
	tc := NewTypedCache[unmarshalableType](stub)
	ctx := context.Background()

	// Channels cannot be marshaled to JSON
	err := tc.Set(ctx, "key", unmarshalableType{Ch: make(chan int)}, time.Minute)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "typed cache marshal")
}
