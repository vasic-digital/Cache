# digital.vasic.cache

A standalone Go caching module providing core interfaces, in-memory and Redis backends, distributed cache patterns, TTL/eviction policies, and service-layer caching wrappers.

## Key Features

- **Core Interface** -- `Cache` interface operating on raw bytes with `TypedCache[T]` generic wrapper for automatic JSON serialization
- **In-Memory Cache** -- LRU, LFU, and FIFO eviction policies, max entry/memory limits, background expiration cleanup
- **Redis Backend** -- Single-instance and cluster clients via go-redis/v9
- **Distributed Patterns** -- Consistent hashing, two-level caching (L1 + L2), write-through, write-back, and cache-aside strategies
- **TTL Policies** -- Fixed, sliding, and adaptive TTL with access-frequency scaling
- **Eviction Policies** -- Capacity, age, frequency, and composite eviction deciders
- **Service Wrapper** -- Cache-aside pattern with key prefixing, hit/miss statistics, and invalidation

## Installation

```bash
go get digital.vasic.cache
```

Requires Go 1.24+.

## Package Overview

| Package | Import Path | Purpose |
|---------|-------------|---------|
| `cache` | `digital.vasic.cache/pkg/cache` | Core `Cache` interface, `TypedCache[T]`, `Config`, `Stats`, eviction policy enum |
| `memory` | `digital.vasic.cache/pkg/memory` | In-memory cache with LRU/LFU/FIFO, max entries, max memory, cleanup |
| `redis` | `digital.vasic.cache/pkg/redis` | Redis single-instance and cluster clients |
| `distributed` | `digital.vasic.cache/pkg/distributed` | Consistent hashing, two-level cache, write strategies |
| `policy` | `digital.vasic.cache/pkg/policy` | TTL policies (fixed, sliding, adaptive) and eviction deciders |
| `service` | `digital.vasic.cache/pkg/service` | Service-layer cache-aside wrapper with statistics |

## Quick Example

```go
package main

import (
    "context"
    "fmt"
    "time"

    "digital.vasic.cache/pkg/cache"
    "digital.vasic.cache/pkg/memory"
)

func main() {
    c := memory.New(&memory.Config{
        MaxEntries:     1000,
        DefaultTTL:     5 * time.Minute,
        EvictionPolicy: cache.LRU,
    })
    defer c.Close()

    ctx := context.Background()
    c.Set(ctx, "user:1", []byte(`{"name":"Alice"}`), 0)

    data, _ := c.Get(ctx, "user:1")
    fmt.Println(string(data)) // {"name":"Alice"}

    stats := c.Stats()
    fmt.Printf("Hits: %d, Misses: %d, Hit Rate: %.1f%%\n",
        stats.Hits, stats.Misses, stats.HitRate())
}
```
