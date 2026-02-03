# User Guide - digital.vasic.cache

## Installation

```bash
go get digital.vasic.cache
```

Requires Go 1.24 or later.

## Quick Start

```go
package main

import (
    "context"
    "fmt"
    "time"

    "digital.vasic.cache/pkg/memory"
)

func main() {
    ctx := context.Background()

    mc := memory.New(memory.DefaultConfig())
    defer mc.Close()

    _ = mc.Set(ctx, "greeting", []byte("hello, world"), 5*time.Minute)

    data, _ := mc.Get(ctx, "greeting")
    fmt.Println(string(data)) // hello, world
}
```

---

## In-Memory Cache

The `pkg/memory` package provides a thread-safe, in-memory cache with configurable eviction policies, entry limits, memory limits, and automatic background cleanup of expired entries.

### Basic Usage

```go
import "digital.vasic.cache/pkg/memory"

mc := memory.New(&memory.Config{
    MaxEntries:      10000,
    MaxMemoryBytes:  100 * 1024 * 1024, // 100 MB
    DefaultTTL:      5 * time.Minute,
    CleanupInterval: time.Minute,
    EvictionPolicy:  cache.LRU,
})
defer mc.Close()
```

### Eviction Policies

The `EvictionPolicy` enum is defined in `pkg/cache`:

| Policy | Constant | Behavior |
|--------|----------|----------|
| Least Recently Used | `cache.LRU` | Evicts the entry that has not been accessed for the longest time |
| Least Frequently Used | `cache.LFU` | Evicts the entry with the fewest accesses |
| First In, First Out | `cache.FIFO` | Evicts the oldest entry regardless of access pattern |

```go
import "digital.vasic.cache/pkg/cache"

// LFU eviction
mc := memory.New(&memory.Config{
    MaxEntries:     5000,
    EvictionPolicy: cache.LFU,
    DefaultTTL:     10 * time.Minute,
})
```

### Monitoring Statistics

```go
stats := mc.Stats()
fmt.Printf("Hits: %d, Misses: %d, Hit Rate: %.1f%%\n",
    stats.Hits, stats.Misses, stats.HitRate())
fmt.Printf("Entries: %d, Memory: %s\n",
    mc.Len(), memory.FormatSize(mc.MemoryUsed()))
```

### Flushing All Entries

```go
mc.Flush()
```

---

## Redis Cache

The `pkg/redis` package provides a Redis-backed cache supporting both single-instance and cluster deployments. It wraps `github.com/redis/go-redis/v9`.

### Single Instance

```go
import "digital.vasic.cache/pkg/redis"

client := redis.New(&redis.Config{
    Addr:         "localhost:6379",
    Password:     "secret",
    DB:           0,
    PoolSize:     10,
    MinIdleConns: 2,
    DialTimeout:  5 * time.Second,
    ReadTimeout:  3 * time.Second,
    WriteTimeout: 3 * time.Second,
})
defer client.Close()

ctx := context.Background()
err := client.Set(ctx, "user:123", []byte(`{"name":"Alice"}`), 10*time.Minute)
data, err := client.Get(ctx, "user:123")
```

### Redis Cluster

```go
cluster := redis.NewCluster(&redis.ClusterConfig{
    Addrs:    []string{"redis-1:7000", "redis-2:7001", "redis-3:7002"},
    Password: "secret",
    PoolSize: 10,
})
defer cluster.Close()

err := cluster.Set(ctx, "session:abc", []byte("data"), 30*time.Minute)
```

### Health Checks

Both `Client` and `ClusterClient` expose a `HealthCheck` method that pings Redis:

```go
if err := client.HealthCheck(ctx); err != nil {
    log.Fatalf("Redis is unreachable: %v", err)
}
```

### Accessing the Underlying Client

For advanced operations not covered by the `Cache` interface, access the raw go-redis client:

```go
rdb := client.Underlying()
pipe := rdb.Pipeline()
// ... pipeline operations ...
```

---

## Typed Cache

`TypedCache[T]` is a generic wrapper that handles JSON serialization and deserialization automatically. It works with any `Cache` backend.

```go
import "digital.vasic.cache/pkg/cache"

type User struct {
    ID   int    `json:"id"`
    Name string `json:"name"`
}

// Wrap any cache.Cache implementation
tc := cache.NewTypedCache[User](memoryCache)

// Set serializes to JSON automatically
err := tc.Set(ctx, "user:1", User{ID: 1, Name: "Alice"}, 5*time.Minute)

// Get deserializes from JSON automatically
user, found, err := tc.Get(ctx, "user:1")
if found {
    fmt.Println(user.Name) // Alice
}

// Exists and Delete work the same as the underlying cache
exists, err := tc.Exists(ctx, "user:1")
err = tc.Delete(ctx, "user:1")
```

---

## Distributed Cache Patterns

The `pkg/distributed` package provides patterns for scaling caches across multiple nodes and coordinating cache-to-storage interactions.

### Consistent Hashing

Distribute keys across multiple cache nodes with minimal redistribution when nodes are added or removed:

```go
import "digital.vasic.cache/pkg/distributed"

ch := distributed.NewConsistentHash(100) // 100 virtual nodes per physical node
ch.AddNode("redis-1:6379")
ch.AddNode("redis-2:6379")
ch.AddNode("redis-3:6379")

// Route a key to its owning node
node := ch.GetNode("user:12345") // e.g., "redis-2:6379"

// Check cluster size
fmt.Printf("Nodes: %d\n", ch.NodeCount())

// Remove a node (keys redistribute to remaining nodes)
ch.RemoveNode("redis-2:6379")
```

### Two-Level Cache (L1 + L2)

Combine a fast local cache (L1) with a shared remote cache (L2). Reads check L1 first; L2 hits are automatically promoted to L1:

```go
import (
    "digital.vasic.cache/pkg/distributed"
    "digital.vasic.cache/pkg/memory"
    "digital.vasic.cache/pkg/redis"
)

local := memory.New(&memory.Config{
    MaxEntries:     1000,
    DefaultTTL:     2 * time.Minute,
    CleanupInterval: 30 * time.Second,
    EvictionPolicy: cache.LRU,
})

remote := redis.New(&redis.Config{
    Addr: "redis:6379",
})

twoLevel := distributed.NewTwoLevel(local, remote, 5*time.Minute)
defer twoLevel.Close()

// Reads: check local -> miss -> check remote -> promote to local
data, err := twoLevel.Get(ctx, "key")

// Writes: write to both local and remote
err = twoLevel.Set(ctx, "key", []byte("value"), 10*time.Minute)
```

### Write Strategies

All write strategies implement the `Strategy` interface and require a `DataSource` (your backing store):

```go
// DataSource interface for your database/storage
type DataSource interface {
    Load(ctx context.Context, key string) ([]byte, error)
    Store(ctx context.Context, key string, value []byte) error
    Remove(ctx context.Context, key string) error
}
```

#### Write-Through

Every write goes to both the cache and the backing store synchronously. This ensures strong consistency at the cost of write latency:

```go
wt := distributed.NewWriteThrough(cacheInstance, myDataSource)

// Read: cache hit -> return; cache miss -> load from source -> populate cache
data, err := wt.Get(ctx, "key")

// Write: store in source first, then cache
err = wt.Set(ctx, "key", []byte("value"), 10*time.Minute)

// Delete: remove from source, then cache
err = wt.Delete(ctx, "key")
```

#### Write-Back

Writes go to the cache immediately. Dirty entries are tracked and flushed to the backing store on demand. This offers lower write latency but risks data loss if the cache is lost before flushing:

```go
wb := distributed.NewWriteBack(cacheInstance, myDataSource)

// Write goes to cache only; entry is marked dirty
err := wb.Set(ctx, "key", []byte("value"), 10*time.Minute)

// Check pending writes
fmt.Printf("Dirty entries: %d\n", wb.DirtyCount())

// Flush all dirty entries to the backing store
err = wb.Flush(ctx)
```

#### Cache-Aside (Lazy Loading)

Data is loaded into the cache only on read misses. Writes go directly to the backing store and invalidate the cache entry:

```go
ca := distributed.NewCacheAside(cacheInstance, myDataSource)

// Read: cache hit -> return; miss -> load from source -> populate cache
data, err := ca.Get(ctx, "key")

// Write: store in source, invalidate cache (next read will reload)
err = ca.Set(ctx, "key", []byte("value"), 0)
```

---

## TTL Policies

The `pkg/policy` package provides configurable time-to-live strategies.

### Fixed TTL

Every key gets the same TTL:

```go
import "digital.vasic.cache/pkg/policy"

fixed := policy.NewFixedTTL(5 * time.Minute)
ttl := fixed.GetTTL("any-key") // always 5m
```

### Sliding TTL

The TTL resets on each access. Use `Touch` to record accesses:

```go
sliding := policy.NewSlidingTTL(10 * time.Minute)

// Record an access
sliding.Touch("session:abc")

// Check if the key should be expired (no touch within BaseTTL)
if sliding.ShouldExpire("session:abc") {
    // key has been idle for more than 10 minutes
}

// Clean up tracking
sliding.Remove("session:abc")
```

### Adaptive TTL

TTL scales with access frequency. Hot keys get longer TTLs; cold keys get shorter TTLs:

```go
adaptive := policy.NewAdaptiveTTL(1*time.Minute, 1*time.Hour)

// Record accesses
adaptive.RecordAccess("popular-key")
adaptive.RecordAccess("popular-key")
adaptive.RecordAccess("popular-key")

// TTL increases with access count (linear scale, capped at 100 accesses)
ttl := adaptive.GetTTL("popular-key")    // > 1 minute, approaching 1 hour
coldTTL := adaptive.GetTTL("rare-key")   // 1 minute (MinTTL)

// Query access count
count := adaptive.AccessCount("popular-key") // 3

// Reset tracking for a key
adaptive.Reset("popular-key")
```

---

## Eviction Policies

Eviction deciders determine whether specific entries should be evicted. They are used by application-level eviction loops, not directly by the in-memory cache (which has built-in LRU/LFU/FIFO).

### Capacity-Based Eviction

Evict when the cache exceeds a capacity threshold:

```go
capacity := policy.NewCapacityEviction(0.9) // trigger at 90% full

stats := policy.EvictionStats{
    TotalEntries: 9500,
    MaxEntries:   10000,
}
if capacity.ShouldEvict("some-key", stats) {
    // cache is >= 90% full
}
```

### Age-Based Eviction

Evict entries older than a maximum age:

```go
age := policy.NewAgeEviction(24 * time.Hour)

stats := policy.EvictionStats{
    EntryAge: 25 * time.Hour,
}
if age.ShouldEvict("old-key", stats) {
    // entry is older than 24 hours
}
```

### Frequency-Based Eviction

Evict entries that have been accessed fewer than a minimum number of times:

```go
freq := policy.NewFrequencyEviction(5)

stats := policy.EvictionStats{
    EntryAccessCount: 2,
}
if freq.ShouldEvict("cold-key", stats) {
    // entry has < 5 accesses
}
```

### Composite Eviction

Combine multiple eviction policies. An entry is evicted if ANY sub-policy triggers:

```go
composite := policy.NewCompositeEviction(
    policy.NewCapacityEviction(0.9),
    policy.NewAgeEviction(24 * time.Hour),
)

if composite.ShouldEvict("key", stats) {
    // either capacity exceeded OR entry is too old
}
```

---

## Cache Warming

Cache warming is the practice of pre-populating the cache with frequently needed data before requests arrive. This module does not include a built-in warming mechanism, but the pattern is straightforward with any backend:

```go
func warmCache(ctx context.Context, c cache.Cache, keys []string, loader func(string) ([]byte, error)) error {
    for _, key := range keys {
        data, err := loader(key)
        if err != nil {
            return fmt.Errorf("warming key %q: %w", key, err)
        }
        if err := c.Set(ctx, key, data, 0); err != nil {
            return fmt.Errorf("setting key %q: %w", key, err)
        }
    }
    return nil
}
```

For two-level caches, warm both levels:

```go
// Warm L2 (remote) first, then L1 (local) for hot keys
for _, key := range hotKeys {
    data, _ := loader(key)
    _ = remote.Set(ctx, key, data, 30*time.Minute)
    _ = local.Set(ctx, key, data, 5*time.Minute)
}
```

With write-through strategy, warming happens naturally:

```go
wt := distributed.NewWriteThrough(cacheInstance, myDataSource)

// Pre-load: read from source -> auto-populate cache
for _, key := range warmKeys {
    _, _ = wt.Get(ctx, key) // miss -> load from source -> cache
}
```

---

## Error Handling

All errors are wrapped with context using `fmt.Errorf("...: %w", err)`. You can use `errors.Is` and `errors.As` to inspect the underlying cause:

```go
import "errors"

data, err := client.Get(ctx, "key")
if err != nil {
    // The error message includes the operation context, e.g.:
    // "redis get \"key\": connection refused"
    var netErr *net.OpError
    if errors.As(err, &netErr) {
        // handle network error
    }
}
```

Cache misses are not errors. A `Get` call returns `nil, nil` when the key does not exist.

---

## Testing

Run the full test suite:

```bash
go test ./... -count=1 -race
```

Run tests for a specific package:

```bash
go test -v digital.vasic.cache/pkg/memory -count=1 -race
```

Run benchmarks:

```bash
go test -bench=. -benchmem ./...
```
