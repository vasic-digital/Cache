# Architecture -- Cache

## Purpose

Standalone, generic Go cache module providing core cache interfaces, multiple backends (Redis, in-memory), distributed cache patterns (consistent hashing, two-level, write-through/back/aside), and configurable TTL/eviction policies (LRU, LFU, FIFO, adaptive TTL, capacity-based eviction).

## Structure

```
pkg/
  cache/         Core Cache interface, TypedCache[T] generic wrapper, Config, Stats, EvictionPolicy enum
  redis/         Redis cache adapter (Client, ClusterClient) using go-redis/v9
  memory/        In-memory cache with LRU/LFU/FIFO eviction, max entries, max memory, background cleanup
  distributed/   ConsistentHash, TwoLevel (L1+L2), WriteThrough, WriteBack, CacheAside strategies
  policy/        FixedTTL, SlidingTTL, AdaptiveTTL, CapacityEviction, AgeEviction, FrequencyEviction, CompositeEviction
```

## Key Components

- **`cache.Cache`** -- Core interface: Get, Set, Delete, Exists, Close
- **`cache.TypedCache[T]`** -- Generic typed wrapper for type-safe cache access
- **`memory.Cache`** -- In-memory backend with configurable max entries, max memory, eviction policy, and background cleanup goroutine
- **`redis.Client`** -- Redis backend with connection pooling and health check
- **`distributed.ConsistentHash`** -- Hash ring for distributing keys across cache nodes
- **`distributed.TwoLevel`** -- L1 (local) + L2 (remote) with configurable promotion TTL
- **`distributed.WriteThrough/WriteBack/CacheAside`** -- Cache write strategies backed by a DataSource interface
- **`policy.*`** -- Pluggable TTL and eviction policies

## Data Flow

```
Client -> TypedCache[T] -> Cache interface -> memory.Cache or redis.Client
                                                    |
                                     (distributed) TwoLevel -> L1 (memory) -> L2 (redis)
                                                    |
                                     (write strategy) WriteThrough -> Cache + DataSource
```

## Dependencies

- `github.com/redis/go-redis/v9` -- Redis client
- `github.com/stretchr/testify` -- Test assertions

## Testing Strategy

Table-driven tests with `testify` and race detection. In-memory backend tests cover eviction policies, TTL expiration, max entries/memory limits, and concurrent access. Distributed pattern tests use mock backends.
