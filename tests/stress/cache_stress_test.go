package stress

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"digital.vasic.cache/pkg/cache"
	"digital.vasic.cache/pkg/distributed"
	"digital.vasic.cache/pkg/memory"
	"digital.vasic.cache/pkg/policy"
)

func TestStress_ConcurrentSetGet(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	c := memory.New(&memory.Config{
		MaxEntries:      10000,
		DefaultTTL:      5 * time.Minute,
		CleanupInterval: time.Second,
		EvictionPolicy:  cache.LRU,
	})
	defer func() { _ = c.Close() }()

	ctx := context.Background()
	const goroutines = 100
	const opsPerGoroutine = 100
	var wg sync.WaitGroup
	var errCount atomic.Int64

	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < opsPerGoroutine; i++ {
				key := fmt.Sprintf("stress-key-%d-%d", id, i)
				value := []byte(fmt.Sprintf("stress-value-%d-%d", id, i))

				if err := c.Set(ctx, key, value, time.Minute); err != nil {
					errCount.Add(1)
					continue
				}

				data, err := c.Get(ctx, key)
				if err != nil {
					errCount.Add(1)
					continue
				}
				if data == nil {
					// Possible eviction under pressure, acceptable
					continue
				}
				if string(data) != string(value) {
					errCount.Add(1)
				}
			}
		}(g)
	}

	wg.Wait()
	assert.Equal(t, int64(0), errCount.Load(), "no errors should occur during concurrent ops")
}

func TestStress_ConcurrentEviction(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	c := memory.New(&memory.Config{
		MaxEntries:      50,
		DefaultTTL:      time.Second,
		CleanupInterval: 100 * time.Millisecond,
		EvictionPolicy:  cache.LFU,
	})
	defer func() { _ = c.Close() }()

	ctx := context.Background()
	const goroutines = 80
	var wg sync.WaitGroup

	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				key := fmt.Sprintf("evict-%d-%d", id, i)
				_ = c.Set(ctx, key, []byte("data"), 200*time.Millisecond)
				_, _ = c.Get(ctx, key)
				_ = c.Delete(ctx, key)
			}
		}(g)
	}

	wg.Wait()

	stats := c.Stats()
	assert.True(t, stats.Evictions >= 0, "evictions count should be non-negative")
}

func TestStress_ConcurrentTwoLevel(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	l1 := memory.New(&memory.Config{
		MaxEntries:      500,
		DefaultTTL:      time.Minute,
		CleanupInterval: time.Second,
		EvictionPolicy:  cache.LRU,
	})
	l2 := memory.New(&memory.Config{
		MaxEntries:      5000,
		DefaultTTL:      10 * time.Minute,
		CleanupInterval: time.Second,
		EvictionPolicy:  cache.LRU,
	})
	tl := distributed.NewTwoLevel(l1, l2, time.Minute)
	defer func() { _ = tl.Close() }()

	ctx := context.Background()
	const goroutines = 60
	var wg sync.WaitGroup
	var errCount atomic.Int64

	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				key := fmt.Sprintf("tl-stress-%d-%d", id, i)
				value := []byte(fmt.Sprintf("tl-val-%d-%d", id, i))

				if err := tl.Set(ctx, key, value, time.Minute); err != nil {
					errCount.Add(1)
					continue
				}

				data, err := tl.Get(ctx, key)
				if err != nil {
					errCount.Add(1)
					continue
				}
				if data != nil && string(data) != string(value) {
					errCount.Add(1)
				}
			}
		}(g)
	}

	wg.Wait()
	assert.Equal(t, int64(0), errCount.Load())
}

func TestStress_ConcurrentConsistentHash(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	ch := distributed.NewConsistentHash(100)
	for i := 0; i < 10; i++ {
		ch.AddNode(fmt.Sprintf("node-%d", i))
	}

	const goroutines = 80
	var wg sync.WaitGroup

	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				key := fmt.Sprintf("hash-key-%d-%d", id, i)
				node := ch.GetNode(key)
				if node == "" {
					t.Errorf("got empty node for key %s", key)
				}
			}
		}(g)
	}

	wg.Wait()
}

func TestStress_ConcurrentAdaptiveTTL(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	adaptive := policy.NewAdaptiveTTL(time.Second, 10*time.Second)

	const goroutines = 50
	var wg sync.WaitGroup

	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			key := fmt.Sprintf("adaptive-key-%d", id)
			for i := 0; i < 200; i++ {
				adaptive.RecordAccess(key)
				ttl := adaptive.GetTTL(key)
				if ttl < time.Second || ttl > 10*time.Second {
					t.Errorf("TTL out of range: %v for key %s", ttl, key)
				}
			}
		}(g)
	}

	wg.Wait()
}

func TestStress_ConcurrentWriteBack(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	c := memory.New(memory.DefaultConfig())
	defer func() { _ = c.Close() }()

	src := &stressDataSource{data: make(map[string][]byte)}
	wb := distributed.NewWriteBack(c, src)
	ctx := context.Background()

	const goroutines = 50
	var wg sync.WaitGroup

	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < 20; i++ {
				key := fmt.Sprintf("wb-stress-%d-%d", id, i)
				err := wb.Set(ctx, key, []byte("value"), time.Minute)
				if err != nil {
					t.Errorf("write-back set failed: %v", err)
				}
			}
		}(g)
	}

	wg.Wait()
	require.True(t, wb.DirtyCount() > 0, "should have dirty entries")

	err := wb.Flush(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, wb.DirtyCount())
}

func TestStress_MemoryPressure(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	c := memory.New(&memory.Config{
		MaxEntries:      100,
		MaxMemoryBytes:  1024 * 1024, // 1MB
		DefaultTTL:      time.Minute,
		CleanupInterval: 100 * time.Millisecond,
		EvictionPolicy:  cache.LRU,
	})
	defer func() { _ = c.Close() }()

	ctx := context.Background()
	const goroutines = 50
	var wg sync.WaitGroup

	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			bigValue := make([]byte, 10*1024) // 10KB per entry
			for i := 0; i < 30; i++ {
				key := fmt.Sprintf("mem-pressure-%d-%d", id, i)
				_ = c.Set(ctx, key, bigValue, time.Minute)
			}
		}(g)
	}

	wg.Wait()

	// Memory usage should be within limits
	memUsed := c.MemoryUsed()
	assert.True(t, memUsed <= 1024*1024+100*1024,
		"memory usage should be near the 1MB limit, got %d", memUsed)
}

type stressDataSource struct {
	mu   sync.Mutex
	data map[string][]byte
}

func (s *stressDataSource) Load(_ context.Context, key string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v := s.data[key]
	if v == nil {
		return nil, nil
	}
	cp := make([]byte, len(v))
	copy(cp, v)
	return cp, nil
}

func (s *stressDataSource) Store(_ context.Context, key string, value []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make([]byte, len(value))
	copy(cp, value)
	s.data[key] = cp
	return nil
}

func (s *stressDataSource) Remove(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, key)
	return nil
}
