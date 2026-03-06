# Getting Started

## Install

```bash
go get digital.vasic.cache
```

## In-Memory Cache

```go
import (
    "digital.vasic.cache/pkg/cache"
    "digital.vasic.cache/pkg/memory"
)

c := memory.New(&memory.Config{
    MaxEntries:      10000,
    MaxMemoryBytes:  100 * 1024 * 1024, // 100 MB
    DefaultTTL:      30 * time.Minute,
    CleanupInterval: time.Minute,
    EvictionPolicy:  cache.LRU,
})
defer c.Close()
```

### Basic operations

```go
ctx := context.Background()

// Set with default TTL
c.Set(ctx, "key", []byte("value"), 0)

// Set with custom TTL
c.Set(ctx, "temp", []byte("data"), 10*time.Second)

// Get (returns nil, nil on miss)
data, err := c.Get(ctx, "key")

// Check existence
exists, err := c.Exists(ctx, "key")

// Delete
c.Delete(ctx, "key")
```

### Statistics

```go
stats := c.Stats()
fmt.Printf("Size: %d, Hits: %d, Misses: %d, Evictions: %d\n",
    stats.Size, stats.Hits, stats.Misses, stats.Evictions)
fmt.Printf("Hit Rate: %.1f%%\n", stats.HitRate())
fmt.Printf("Memory: %s\n", memory.FormatSize(c.MemoryUsed()))
```

## Typed Cache

Wrap any `Cache` with automatic JSON serialization for a concrete type:

```go
type User struct {
    Name  string `json:"name"`
    Email string `json:"email"`
}

tc := cache.NewTypedCache[User](c)

tc.Set(ctx, "user:1", User{Name: "Alice", Email: "alice@example.com"}, 0)

user, found, err := tc.Get(ctx, "user:1")
if found {
    fmt.Println(user.Name) // Alice
}
```

## Redis Cache

```go
import "digital.vasic.cache/pkg/redis"

client := redis.New(&redis.Config{
    Addr:     "localhost:6379",
    Password: "",
    DB:       0,
    PoolSize: 10,
})
defer client.Close()

// Same Cache interface
client.Set(ctx, "key", []byte("value"), 5*time.Minute)
data, _ := client.Get(ctx, "key")
```

For Redis Cluster:

```go
cluster := redis.NewCluster(&redis.ClusterConfig{
    Addrs: []string{"node1:7000", "node2:7001", "node3:7002"},
})
```

## Service Cache Wrapper

Cache-aside pattern for service layer calls:

```go
import "digital.vasic.cache/pkg/service"

wrapper := service.New(myCache, service.Config{
    DefaultTTL: 5 * time.Minute,
    KeyPrefix:  "svc:",
})

result, err := wrapper.GetOrLoad(ctx, "user:1", func(ctx context.Context, key string) (interface{}, error) {
    return db.GetUser(ctx, key) // only called on cache miss
})
```
