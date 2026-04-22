// SPDX-License-Identifier: Apache-2.0
//
// Stress tests for digital.vasic.cache/pkg/memory, modelled on the
// canonical P3 stress-test template in
// digital.vasic.buildcheck/pkg/buildcheck/stress_test.go.
//
// Run with:
//   GOMAXPROCS=2 nice -n 19 ionice -c 3 go test -race -run '^TestStress' \
//       ./pkg/memory/ -p 1 -count=1 -timeout 120s
package memory

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	stressGoroutines   = 8
	stressIterations   = 400
	stressMaxWallClock = 20 * time.Second
)

// TestStress_MemoryCache_ConcurrentMixed puts 8 goroutines through a
// mixed Get/Set/Delete/Exists workload for up to 400 iterations each
// or 20s wall-clock, whichever completes first. Validates no race, no
// goroutine leak, no panic on eviction contention.
func TestStress_MemoryCache_ConcurrentMixed(t *testing.T) {
	cfg := DefaultConfig()
	// Tighten to force eviction churn.
	cfg.MaxEntries = 64
	c := New(cfg)
	defer func() { _ = c.Close() }()

	ctx := context.Background()
	startGoroutines := runtime.NumGoroutine()
	var wg sync.WaitGroup
	var errCount atomic.Int64
	deadline := time.Now().Add(stressMaxWallClock)

	for g := 0; g < stressGoroutines; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < stressIterations; j++ {
				if time.Now().After(deadline) {
					return
				}
				key := fmt.Sprintf("g%d/k%d", id, j%32)
				val := []byte(fmt.Sprintf("v-%d-%d", id, j))
				if err := c.Set(ctx, key, val, 5*time.Second); err != nil {
					errCount.Add(1)
					continue
				}
				_, _ = c.Get(ctx, key)
				_, _ = c.Exists(ctx, key)
				if j%7 == 0 {
					_ = c.Delete(ctx, key)
				}
			}
		}(g)
	}
	wg.Wait()

	assert.Equal(t, int64(0), errCount.Load(), "Set should not error on well-formed inputs")
	// Cache size must stay bounded by MaxEntries (hard invariant).
	assert.LessOrEqual(t, c.Len(), cfg.MaxEntries,
		"cache length %d exceeds MaxEntries %d", c.Len(), cfg.MaxEntries)

	time.Sleep(100 * time.Millisecond)
	runtime.Gosched()
	endGoroutines := runtime.NumGoroutine()
	assert.LessOrEqual(t, endGoroutines-startGoroutines, 3,
		"goroutine leak: worker count grew by %d", endGoroutines-startGoroutines)
}

// TestStress_MemoryCache_TTLExpiryUnderLoad asserts the TTL-expiry
// path is race-safe when many goroutines write short-TTL entries that
// expire in-flight.
func TestStress_MemoryCache_TTLExpiryUnderLoad(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxEntries = 128
	c := New(cfg)
	defer func() { _ = c.Close() }()

	ctx := context.Background()
	var wg sync.WaitGroup
	deadline := time.Now().Add(5 * time.Second)

	for g := 0; g < stressGoroutines; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; ; j++ {
				if time.Now().After(deadline) {
					return
				}
				key := fmt.Sprintf("ttl-g%d/k%d", id, j%16)
				require.NoError(t, c.Set(ctx, key, []byte("x"), 10*time.Millisecond))
				time.Sleep(500 * time.Microsecond)
				_, _ = c.Get(ctx, key)
			}
		}(g)
	}
	wg.Wait()

	// Nothing more to assert — the guarantee is no race / no panic.
}

// BenchmarkStress_MemoryCache_Set establishes throughput baseline.
func BenchmarkStress_MemoryCache_Set(b *testing.B) {
	c := New(DefaultConfig())
	defer func() { _ = c.Close() }()
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = c.Set(ctx, fmt.Sprintf("bench-%d", i%128), []byte("val"), 0)
	}
}
