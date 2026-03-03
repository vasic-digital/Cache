package benchmark

import (
	"context"
	"fmt"
	"testing"
	"time"

	"digital.vasic.cache/pkg/cache"
	"digital.vasic.cache/pkg/distributed"
	"digital.vasic.cache/pkg/memory"
	"digital.vasic.cache/pkg/policy"
)

func BenchmarkMemoryCache_Set(b *testing.B) {
	if testing.Short() {
		b.Skip("skipping benchmark test in short mode")
	}

	c := memory.New(&memory.Config{
		MaxEntries:      b.N + 1000,
		DefaultTTL:      5 * time.Minute,
		CleanupInterval: time.Minute,
		EvictionPolicy:  cache.LRU,
	})
	defer func() { _ = c.Close() }()

	ctx := context.Background()
	value := []byte("benchmark-value-with-some-real-content-to-measure")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("bench-key-%d", i)
		_ = c.Set(ctx, key, value, time.Minute)
	}
}

func BenchmarkMemoryCache_Get(b *testing.B) {
	if testing.Short() {
		b.Skip("skipping benchmark test in short mode")
	}

	c := memory.New(&memory.Config{
		MaxEntries:      10000,
		DefaultTTL:      5 * time.Minute,
		CleanupInterval: time.Minute,
		EvictionPolicy:  cache.LRU,
	})
	defer func() { _ = c.Close() }()

	ctx := context.Background()
	// Pre-populate
	for i := 0; i < 10000; i++ {
		key := fmt.Sprintf("bench-key-%d", i)
		_ = c.Set(ctx, key, []byte("benchmark-value"), time.Minute)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("bench-key-%d", i%10000)
		_, _ = c.Get(ctx, key)
	}
}

func BenchmarkMemoryCache_SetGet_Mixed(b *testing.B) {
	if testing.Short() {
		b.Skip("skipping benchmark test in short mode")
	}

	c := memory.New(&memory.Config{
		MaxEntries:      50000,
		DefaultTTL:      5 * time.Minute,
		CleanupInterval: time.Minute,
		EvictionPolicy:  cache.LRU,
	})
	defer func() { _ = c.Close() }()

	ctx := context.Background()
	value := []byte("mixed-benchmark-value")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("mixed-key-%d", i)
		_ = c.Set(ctx, key, value, time.Minute)
		_, _ = c.Get(ctx, key)
	}
}

func BenchmarkTypedCache_SetGet(b *testing.B) {
	if testing.Short() {
		b.Skip("skipping benchmark test in short mode")
	}

	type Item struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}

	c := memory.New(&memory.Config{
		MaxEntries:      b.N + 1000,
		DefaultTTL:      5 * time.Minute,
		CleanupInterval: time.Minute,
		EvictionPolicy:  cache.LRU,
	})
	defer func() { _ = c.Close() }()

	tc := cache.NewTypedCache[Item](c)
	ctx := context.Background()
	item := Item{Name: "benchmark", Value: 42}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("typed-key-%d", i)
		_ = tc.Set(ctx, key, item, time.Minute)
		_, _, _ = tc.Get(ctx, key)
	}
}

func BenchmarkConsistentHash_GetNode(b *testing.B) {
	if testing.Short() {
		b.Skip("skipping benchmark test in short mode")
	}

	ch := distributed.NewConsistentHash(150)
	for i := 0; i < 20; i++ {
		ch.AddNode(fmt.Sprintf("node-%d", i))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("hash-bench-key-%d", i)
		_ = ch.GetNode(key)
	}
}

func BenchmarkTwoLevel_Get(b *testing.B) {
	if testing.Short() {
		b.Skip("skipping benchmark test in short mode")
	}

	l1 := memory.New(&memory.Config{
		MaxEntries:      5000,
		DefaultTTL:      time.Minute,
		CleanupInterval: time.Minute,
		EvictionPolicy:  cache.LRU,
	})
	l2 := memory.New(&memory.Config{
		MaxEntries:      50000,
		DefaultTTL:      10 * time.Minute,
		CleanupInterval: time.Minute,
		EvictionPolicy:  cache.LRU,
	})
	tl := distributed.NewTwoLevel(l1, l2, time.Minute)
	defer func() { _ = tl.Close() }()

	ctx := context.Background()
	// Pre-populate L2 only
	for i := 0; i < 5000; i++ {
		key := fmt.Sprintf("tl-bench-%d", i)
		_ = l2.Set(ctx, key, []byte("tl-value"), 10*time.Minute)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("tl-bench-%d", i%5000)
		_, _ = tl.Get(ctx, key)
	}
}

func BenchmarkAdaptiveTTL_RecordAndGet(b *testing.B) {
	if testing.Short() {
		b.Skip("skipping benchmark test in short mode")
	}

	adaptive := policy.NewAdaptiveTTL(time.Second, 10*time.Second)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("adaptive-bench-%d", i%100)
		adaptive.RecordAccess(key)
		_ = adaptive.GetTTL(key)
	}
}

func BenchmarkMemoryCache_Eviction(b *testing.B) {
	if testing.Short() {
		b.Skip("skipping benchmark test in short mode")
	}

	c := memory.New(&memory.Config{
		MaxEntries:      100,
		DefaultTTL:      5 * time.Minute,
		CleanupInterval: time.Minute,
		EvictionPolicy:  cache.LRU,
	})
	defer func() { _ = c.Close() }()

	ctx := context.Background()
	value := []byte("eviction-bench-value")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("evict-bench-%d", i)
		_ = c.Set(ctx, key, value, time.Minute)
	}
}
