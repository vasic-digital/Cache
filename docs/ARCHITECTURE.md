# Architecture - digital.vasic.cache

## Design Goals

1. **Generic and reusable** -- zero dependencies on any application-specific code
2. **Interface-driven** -- all backends satisfy a single `Cache` interface
3. **Composable** -- distributed patterns wrap any `Cache` implementation
4. **Thread-safe** -- all types are safe for concurrent use
5. **Minimal dependencies** -- only `go-redis/v9` at runtime; `testify` for tests

## Package Layout

```
digital.vasic.cache/
  pkg/
    cache/         -- Core interface + types (leaf package)
    redis/         -- Redis backend (depends on cache)
    memory/        -- In-memory backend (depends on cache)
    distributed/   -- Distributed patterns (depends on cache)
    policy/        -- TTL and eviction policies (leaf package)
```

The `cache` and `policy` packages are leaf packages with no internal imports. All backend and pattern packages depend only on the `cache` package for the `Cache` interface definition.

## Design Patterns

### Strategy Pattern

The Strategy pattern is used in two places:

**1. Eviction Policies (pkg/memory)**

The in-memory cache delegates eviction to one of three strategies (`LRU`, `LFU`, `FIFO`), selected at construction time via the `EvictionPolicy` enum. The `evict()` method dispatches to the appropriate strategy method:

```
Config.EvictionPolicy = LRU  -->  evictLRU()   (remove from back of list)
Config.EvictionPolicy = LFU  -->  evictLFU()   (scan for min access count)
Config.EvictionPolicy = FIFO -->  evictFIFO()  (remove from front of list)
```

**2. Write Strategies (pkg/distributed)**

The `Strategy` interface abstracts the read/write coordination between cache and backing store. Three concrete strategies are provided:

| Strategy | Write Path | Read Path (on miss) |
|----------|-----------|---------------------|
| `WriteThrough` | Store in source, then cache (synchronous) | Load from source, populate cache |
| `WriteBack` | Write to cache, mark dirty (async flush) | Load from source, populate cache |
| `CacheAside` | Store in source, invalidate cache | Load from source, populate cache |

Each strategy implements `Name()`, `Get()`, `Set()`, and `Delete()` with different consistency/latency tradeoffs.

### Decorator Pattern

`TypedCache[T]` is a decorator around the `Cache` interface. It adds JSON serialization/deserialization without altering the underlying cache behavior:

```
TypedCache[User]
    |
    +-- inner: cache.Cache (any backend)
    |
    Set(key, User{...}) --> json.Marshal --> inner.Set(key, []byte)
    Get(key) --> inner.Get(key) --> json.Unmarshal --> User{...}
```

The decorator preserves the full `Cache` contract (`Get`, `Set`, `Delete`, `Exists`, `Close`) and adds typed convenience on top.

### Proxy Pattern

The `TwoLevel` cache acts as a proxy in front of two `Cache` instances (L1 local, L2 remote):

```
Client --> TwoLevel.Get(key)
              |
              +--> L1.Get(key)  -- hit? return
              |
              +--> L2.Get(key)  -- hit? promote to L1, return
              |
              +--> nil (miss)
```

On writes, the proxy fans out to both levels. On reads, it transparently handles promotion from L2 to L1. The caller does not need to know about the multi-level topology.

### Composite Pattern

`CompositeEviction` in `pkg/policy` composes multiple `EvictionDecider` instances into a single decider. An entry is evicted if ANY sub-policy triggers. This allows building complex eviction rules from simple building blocks:

```
CompositeEviction
    |
    +-- CapacityEviction(0.9)   -- triggers at 90% capacity
    +-- AgeEviction(24h)        -- triggers for entries older than 24h
    +-- FrequencyEviction(5)    -- triggers for entries with < 5 accesses
```

## Concurrency Model

### In-Memory Cache

The `memory.Cache` uses a `sync.RWMutex` for map and list access. Counters (`hits`, `misses`, `evictions`, `memUsed`) use `sync/atomic` operations. The background cleanup goroutine acquires a write lock periodically to remove expired entries. It is stopped via `context.WithCancel` when `Close()` is called.

Data safety: `Get` returns a copy of the stored byte slice, and `Set` stores a copy of the input. This prevents callers from accidentally mutating cached data.

### Consistent Hash Ring

The `ConsistentHash` struct uses `sync.RWMutex`. `AddNode` and `RemoveNode` acquire write locks. `GetNode` acquires a read lock. The ring is sorted after each `AddNode` call.

### Write-Back Dirty Tracking

`WriteBack` uses a `sync.Mutex` for the dirty map. `Flush` swaps the dirty map atomically (lock, copy reference, replace with new empty map, unlock) and then writes outside the lock.

### Policy Access Tracking

`SlidingTTL` and `AdaptiveTTL` use `sync.Map` for per-key access tracking. `AdaptiveTTL.RecordAccess` uses `atomic.AddInt64` for lock-free counter increments.

## Data Flow

### Cache-Aside Read Path

```
1. Application calls CacheAside.Get(ctx, key)
2. Cache backend is checked (cache.Cache.Get)
3. On hit: return cached data
4. On miss: call DataSource.Load(ctx, key)
5. If source returns data: populate cache with cache.Cache.Set
6. Return data to application
```

### Write-Through Write Path

```
1. Application calls WriteThrough.Set(ctx, key, value, ttl)
2. DataSource.Store(ctx, key, value) -- write to source first
3. cache.Cache.Set(ctx, key, value, ttl) -- then write to cache
4. Both succeed: return nil
5. Source fails: return error (cache not updated)
```

### Write-Back Write Path

```
1. Application calls WriteBack.Set(ctx, key, value, ttl)
2. cache.Cache.Set(ctx, key, value, ttl) -- write to cache immediately
3. Record key in dirty map
4. Return nil (source NOT written yet)
5. Later: WriteBack.Flush(ctx) writes all dirty entries to DataSource.Store
```

### Two-Level Read Path

```
1. Application calls TwoLevel.Get(ctx, key)
2. L1 (local) cache checked
3. On L1 hit: return immediately
4. On L1 miss: check L2 (remote) cache
5. On L2 hit: promote to L1 (Set with l1TTL), return data
6. On L2 miss: return nil, nil
```

## Hashing Strategy

The consistent hash ring uses MD5 (first 4 bytes as uint32) for hash distribution. MD5 is used purely for uniform distribution, not for cryptographic purposes. Each physical node gets `replicas` virtual nodes on the ring (default: 100), which provides good distribution uniformity.

## Memory Management

The in-memory cache tracks approximate memory usage by summing value byte lengths. This does not include map/list overhead, key string sizes, or struct padding. Two enforcement mechanisms exist:

1. **MaxEntries** -- hard cap on entry count; triggers eviction on overflow
2. **MaxMemoryBytes** -- soft cap on value bytes; triggers eviction loop until under budget

Eviction removes one entry at a time. For memory-based eviction, the loop continues until the budget is satisfied or the cache is empty.

## Error Handling

All errors are wrapped with descriptive context using `fmt.Errorf("operation context: %w", err)`. This preserves the error chain for `errors.Is` and `errors.As` inspection.

Cache misses are represented as `nil, nil` (not as errors), following the convention that a missing key is a normal condition, not an exceptional one.

## Extensibility

The module is designed for extension through new implementations:

- **New backends**: Implement `cache.Cache` (5 methods)
- **New write strategies**: Implement `distributed.Strategy` (3 methods + `Name`)
- **New TTL policies**: Implement `policy.TTLPolicy` (1 method: `GetTTL`)
- **New eviction deciders**: Implement `policy.EvictionDecider` (1 method: `ShouldEvict`)

All interfaces are small and focused, following the Go principle of accepting interfaces and returning structs.
