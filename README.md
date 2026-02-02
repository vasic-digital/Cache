# Cache

A standalone, generic Go cache module providing core interfaces, multiple backends, distributed patterns, and configurable policies.

## Installation

```bash
go get digital.vasic.cache
```

## Packages

### pkg/cache - Core Interfaces

```go
import "digital.vasic.cache/pkg/cache"

// Cache interface implemented by all backends
type Cache interface {
    Get(ctx context.Context, key string) ([]byte, error)
    Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
    Delete(ctx context.Context, key string) error
    Exists(ctx context.Context, key string) (bool, error)
    Close() error
}

// Generic typed wrapper
tc := cache.NewTypedCache[MyStruct](underlyingCache)
val, found, err := tc.Get(ctx, "key")
```

### pkg/redis - Redis Backend

```go
import "digital.vasic.cache/pkg/redis"

client := redis.New(&redis.Config{
    Addr:     "localhost:6379",
    Password: "secret",
    DB:       0,
    PoolSize: 10,
})
defer client.Close()

err := client.Set(ctx, "key", []byte("value"), 5*time.Minute)
data, err := client.Get(ctx, "key")
err = client.HealthCheck(ctx)
```

### pkg/memory - In-Memory Backend

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

### pkg/distributed - Distributed Patterns

```go
import "digital.vasic.cache/pkg/distributed"

// Consistent hashing
ch := distributed.NewConsistentHash(100)
ch.AddNode("redis-1")
ch.AddNode("redis-2")
node := ch.GetNode("my-key")

// Two-level cache (local + remote)
tl := distributed.NewTwoLevel(localCache, remoteCache, 5*time.Minute)

// Write strategies
wt := distributed.NewWriteThrough(cacheInstance, dataSource)
wb := distributed.NewWriteBack(cacheInstance, dataSource)
ca := distributed.NewCacheAside(cacheInstance, dataSource)
```

### pkg/policy - TTL and Eviction Policies

```go
import "digital.vasic.cache/pkg/policy"

// TTL policies
fixed := policy.NewFixedTTL(5 * time.Minute)
sliding := policy.NewSlidingTTL(10 * time.Minute)
adaptive := policy.NewAdaptiveTTL(time.Minute, time.Hour)

// Eviction policies
capacity := policy.NewCapacityEviction(0.9)
age := policy.NewAgeEviction(24 * time.Hour)
freq := policy.NewFrequencyEviction(5)
composite := policy.NewCompositeEviction(capacity, age)
```

## Testing

```bash
go test ./... -count=1 -race
```

## License

See repository license.
