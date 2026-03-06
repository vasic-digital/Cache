# Examples

## 1. Two-Level Cache (Local + Redis)

Use in-memory as L1 and Redis as L2 for fast local reads with shared remote state.

```go
package main

import (
    "context"
    "fmt"
    "time"

    "digital.vasic.cache/pkg/cache"
    "digital.vasic.cache/pkg/distributed"
    "digital.vasic.cache/pkg/memory"
    "digital.vasic.cache/pkg/redis"
)

func main() {
    local := memory.New(&memory.Config{
        MaxEntries:     1000,
        EvictionPolicy: cache.LRU,
    })
    defer local.Close()

    remote := redis.New(&redis.Config{Addr: "localhost:6379"})
    defer remote.Close()

    twoLevel := distributed.NewTwoLevel(local, remote, 5*time.Minute)
    defer twoLevel.Close()

    ctx := context.Background()

    // Write goes to both levels
    twoLevel.Set(ctx, "config:theme", []byte("dark"), 1*time.Hour)

    // Read checks local first, promotes from remote on L1 miss
    data, _ := twoLevel.Get(ctx, "config:theme")
    fmt.Println(string(data)) // "dark"
}
```

## 2. Write Strategies

Compare write-through, write-back, and cache-aside patterns.

```go
package main

import (
    "context"
    "fmt"

    "digital.vasic.cache/pkg/distributed"
    "digital.vasic.cache/pkg/memory"
)

// mapSource is a trivial DataSource for demonstration.
type mapSource struct{ data map[string][]byte }

func (s *mapSource) Load(_ context.Context, key string) ([]byte, error) {
    return s.data[key], nil
}
func (s *mapSource) Store(_ context.Context, key string, value []byte) error {
    s.data[key] = value; return nil
}
func (s *mapSource) Remove(_ context.Context, key string) error {
    delete(s.data, key); return nil
}

func main() {
    c := memory.New(nil)
    defer c.Close()
    src := &mapSource{data: map[string][]byte{}}
    ctx := context.Background()

    // Write-through: writes to source then cache
    wt := distributed.NewWriteThrough(c, src)
    wt.Set(ctx, "key1", []byte("value1"), 0)
    data, _ := wt.Get(ctx, "key1")
    fmt.Printf("write-through: %s\n", data)

    // Write-back: writes to cache, flushes later
    wb := distributed.NewWriteBack(c, src)
    wb.Set(ctx, "key2", []byte("value2"), 0)
    fmt.Printf("dirty entries: %d\n", wb.DirtyCount())
    wb.Flush(ctx) // persist to source
    fmt.Printf("dirty entries after flush: %d\n", wb.DirtyCount())

    // Cache-aside: writes to source, invalidates cache
    ca := distributed.NewCacheAside(c, src)
    ca.Set(ctx, "key3", []byte("value3"), 0)
    data, _ = ca.Get(ctx, "key3") // loads from source, populates cache
    fmt.Printf("cache-aside: %s\n", data)
}
```

## 3. Adaptive TTL Policy

Keys accessed frequently get longer TTLs.

```go
package main

import (
    "fmt"
    "time"

    "digital.vasic.cache/pkg/policy"
)

func main() {
    adaptive := policy.NewAdaptiveTTL(1*time.Minute, 30*time.Minute)

    // Simulate accesses
    for i := 0; i < 50; i++ {
        adaptive.RecordAccess("hot-key")
    }
    adaptive.RecordAccess("cold-key")

    hotTTL := adaptive.GetTTL("hot-key")
    coldTTL := adaptive.GetTTL("cold-key")

    fmt.Printf("hot-key  TTL: %s (accesses: %d)\n", hotTTL, adaptive.AccessCount("hot-key"))
    fmt.Printf("cold-key TTL: %s (accesses: %d)\n", coldTTL, adaptive.AccessCount("cold-key"))
    // hot-key gets ~15.5 min, cold-key gets ~1.3 min
}
```
