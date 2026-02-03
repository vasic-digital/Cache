# API Reference - digital.vasic.cache

## Package `cache` (`digital.vasic.cache/pkg/cache`)

Core interfaces, typed wrappers, configuration, statistics, and eviction policy enumerations.

### Interfaces

#### `Cache`

The core interface that all cache backends must implement. Operates on raw byte slices.

```go
type Cache interface {
    Get(ctx context.Context, key string) ([]byte, error)
    Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
    Delete(ctx context.Context, key string) error
    Exists(ctx context.Context, key string) (bool, error)
    Close() error
}
```

| Method | Parameters | Returns | Description |
|--------|-----------|---------|-------------|
| `Get` | `ctx context.Context`, `key string` | `[]byte, error` | Retrieves a value by key. Returns `nil, nil` on cache miss. |
| `Set` | `ctx context.Context`, `key string`, `value []byte`, `ttl time.Duration` | `error` | Stores a value with optional TTL. Zero TTL means no expiration (implementation-defined). |
| `Delete` | `ctx context.Context`, `key string` | `error` | Removes a single key. Deleting a non-existent key is not an error. |
| `Exists` | `ctx context.Context`, `key string` | `bool, error` | Reports whether the key is present in the cache. |
| `Close` | (none) | `error` | Releases any resources held by the cache. |

---

### Types

#### `EvictionPolicy`

```go
type EvictionPolicy int
```

Determines how entries are evicted when the cache reaches capacity.

| Constant | Value | Description |
|----------|-------|-------------|
| `LRU` | 0 | Least Recently Used -- evicts the entry not accessed for the longest time |
| `LFU` | 1 | Least Frequently Used -- evicts the entry with the fewest accesses |
| `FIFO` | 2 | First In, First Out -- evicts the oldest entry |

**Methods:**

| Method | Signature | Description |
|--------|-----------|-------------|
| `String` | `(p EvictionPolicy) String() string` | Returns the human-readable name (`"LRU"`, `"LFU"`, `"FIFO"`). |

---

#### `Config`

```go
type Config struct {
    DefaultTTL     time.Duration
    MaxSize        int
    EvictionPolicy EvictionPolicy
}
```

General cache configuration.

| Field | Type | Description |
|-------|------|-------------|
| `DefaultTTL` | `time.Duration` | Applied when a `Set` call specifies zero TTL. |
| `MaxSize` | `int` | Maximum number of entries (0 = unlimited). |
| `EvictionPolicy` | `EvictionPolicy` | Eviction strategy when `MaxSize` is reached. |

---

#### `Stats`

```go
type Stats struct {
    Hits      int64 `json:"hits"`
    Misses    int64 `json:"misses"`
    Evictions int64 `json:"evictions"`
    Size      int64 `json:"size"`
}
```

Runtime cache statistics.

| Field | Type | Description |
|-------|------|-------------|
| `Hits` | `int64` | Number of cache hits. |
| `Misses` | `int64` | Number of cache misses. |
| `Evictions` | `int64` | Number of evicted entries. |
| `Size` | `int64` | Current number of entries. |

**Methods:**

| Method | Signature | Returns | Description |
|--------|-----------|---------|-------------|
| `HitRate` | `(s *Stats) HitRate() float64` | `float64` | Hit ratio as a percentage (0--100). Returns 0 when there have been no requests. |

---

#### `TypedCache[T]`

```go
type TypedCache[T any] struct { /* unexported */ }
```

Generic wrapper around `Cache` that handles JSON serialization/deserialization for a concrete type `T`.

**Methods:**

| Method | Signature | Returns | Description |
|--------|-----------|---------|-------------|
| `Get` | `(tc *TypedCache[T]) Get(ctx context.Context, key string)` | `T, bool, error` | Retrieves and deserializes a value. Returns zero value of `T` and `false` on miss. |
| `Set` | `(tc *TypedCache[T]) Set(ctx context.Context, key string, value T, ttl time.Duration)` | `error` | Serializes and stores a value. |
| `Delete` | `(tc *TypedCache[T]) Delete(ctx context.Context, key string)` | `error` | Removes a key from the underlying cache. |
| `Exists` | `(tc *TypedCache[T]) Exists(ctx context.Context, key string)` | `bool, error` | Checks whether a key exists. |
| `Close` | `(tc *TypedCache[T]) Close()` | `error` | Closes the underlying cache. |

---

### Functions

| Function | Signature | Returns | Description |
|----------|-----------|---------|-------------|
| `DefaultConfig` | `DefaultConfig() *Config` | `*Config` | Returns a `Config` with defaults: 30m TTL, 10000 max size, LRU eviction. |
| `NewTypedCache` | `NewTypedCache[T any](c Cache) *TypedCache[T]` | `*TypedCache[T]` | Wraps an existing `Cache` with typed Get/Set methods. |

---

## Package `redis` (`digital.vasic.cache/pkg/redis`)

Redis-backed cache implementation using `github.com/redis/go-redis/v9`.

### Types

#### `Config`

```go
type Config struct {
    Addr         string
    Password     string
    DB           int
    PoolSize     int
    MinIdleConns int
    DialTimeout  time.Duration
    ReadTimeout  time.Duration
    WriteTimeout time.Duration
}
```

Connection parameters for a single Redis instance.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `Addr` | `string` | `"localhost:6379"` | Host:port address. |
| `Password` | `string` | `""` | Redis AUTH password. Empty means no auth. |
| `DB` | `int` | `0` | Redis database number (0--15). |
| `PoolSize` | `int` | `10` | Maximum number of socket connections. |
| `MinIdleConns` | `int` | `2` | Minimum idle connections maintained. |
| `DialTimeout` | `time.Duration` | `5s` | Timeout for establishing new connections. |
| `ReadTimeout` | `time.Duration` | `3s` | Timeout for socket reads. |
| `WriteTimeout` | `time.Duration` | `3s` | Timeout for socket writes. |

---

#### `ClusterConfig`

```go
type ClusterConfig struct {
    Addrs        []string
    Password     string
    PoolSize     int
    MinIdleConns int
    DialTimeout  time.Duration
    ReadTimeout  time.Duration
    WriteTimeout time.Duration
}
```

Connection parameters for a Redis Cluster.

| Field | Type | Description |
|-------|------|-------------|
| `Addrs` | `[]string` | Seed list of cluster node addresses. |
| `Password` | `string` | Redis AUTH password. |
| `PoolSize` | `int` | Pool size per cluster node. |
| `MinIdleConns` | `int` | Minimum idle connections per cluster node. |
| `DialTimeout` | `time.Duration` | Timeout for new connections. |
| `ReadTimeout` | `time.Duration` | Timeout for socket reads. |
| `WriteTimeout` | `time.Duration` | Timeout for socket writes. |

---

#### `Client`

```go
type Client struct { /* unexported */ }
```

Implements `cache.Cache` using a single Redis instance.

**Methods:**

| Method | Signature | Returns | Description |
|--------|-----------|---------|-------------|
| `Get` | `(c *Client) Get(ctx context.Context, key string)` | `[]byte, error` | Retrieves a value. Returns `nil, nil` on miss. |
| `Set` | `(c *Client) Set(ctx context.Context, key string, value []byte, ttl time.Duration)` | `error` | Stores a value with TTL. Zero TTL means no expiration. |
| `Delete` | `(c *Client) Delete(ctx context.Context, key string)` | `error` | Removes a key. |
| `Exists` | `(c *Client) Exists(ctx context.Context, key string)` | `bool, error` | Reports whether the key exists. |
| `Close` | `(c *Client) Close()` | `error` | Closes the Redis connection. |
| `HealthCheck` | `(c *Client) HealthCheck(ctx context.Context)` | `error` | Pings Redis and returns an error if unreachable. |
| `Underlying` | `(c *Client) Underlying()` | `*redis.Client` | Returns the raw go-redis client for advanced operations. |

---

#### `ClusterClient`

```go
type ClusterClient struct { /* unexported */ }
```

Implements `cache.Cache` using a Redis Cluster.

**Methods:**

| Method | Signature | Returns | Description |
|--------|-----------|---------|-------------|
| `Get` | `(c *ClusterClient) Get(ctx context.Context, key string)` | `[]byte, error` | Retrieves a value. Returns `nil, nil` on miss. |
| `Set` | `(c *ClusterClient) Set(ctx context.Context, key string, value []byte, ttl time.Duration)` | `error` | Stores a value with TTL. |
| `Delete` | `(c *ClusterClient) Delete(ctx context.Context, key string)` | `error` | Removes a key. |
| `Exists` | `(c *ClusterClient) Exists(ctx context.Context, key string)` | `bool, error` | Reports whether the key exists. |
| `Close` | `(c *ClusterClient) Close()` | `error` | Closes the cluster connection. |
| `HealthCheck` | `(c *ClusterClient) HealthCheck(ctx context.Context)` | `error` | Pings the Redis Cluster. |

---

### Functions

| Function | Signature | Returns | Description |
|----------|-----------|---------|-------------|
| `DefaultConfig` | `DefaultConfig() *Config` | `*Config` | Returns a `Config` with local development defaults. |
| `New` | `New(cfg *Config) *Client` | `*Client` | Creates a new Redis cache client. Passing `nil` uses `DefaultConfig()`. |
| `NewCluster` | `NewCluster(cfg *ClusterConfig) *ClusterClient` | `*ClusterClient` | Creates a Redis Cluster cache client. Passing `nil` defaults to `localhost:7000-7002`. |

---

## Package `memory` (`digital.vasic.cache/pkg/memory`)

Thread-safe in-memory cache with configurable eviction, entry limits, memory limits, and background cleanup.

### Types

#### `Config`

```go
type Config struct {
    MaxEntries      int
    MaxMemoryBytes  int64
    DefaultTTL      time.Duration
    CleanupInterval time.Duration
    EvictionPolicy  cache.EvictionPolicy
}
```

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `MaxEntries` | `int` | `10000` | Maximum number of entries. 0 means unlimited. |
| `MaxMemoryBytes` | `int64` | `0` (unlimited) | Maximum memory budget in bytes. |
| `DefaultTTL` | `time.Duration` | `30m` | Applied when `Set` is called with zero TTL. |
| `CleanupInterval` | `time.Duration` | `1m` | How often expired entries are removed by the background goroutine. |
| `EvictionPolicy` | `cache.EvictionPolicy` | `cache.LRU` | Which entries to evict on capacity overflow. |

---

#### `Cache`

```go
type Cache struct { /* unexported */ }
```

Implements `cache.Cache`. Thread-safe via `sync.RWMutex` and `sync/atomic`.

**Methods:**

| Method | Signature | Returns | Description |
|--------|-----------|---------|-------------|
| `Get` | `(c *Cache) Get(_ context.Context, key string)` | `[]byte, error` | Retrieves a value. Returns `nil, nil` on miss. Updates access tracking for LRU/LFU. |
| `Set` | `(c *Cache) Set(_ context.Context, key string, value []byte, ttl time.Duration)` | `error` | Stores a copy of the value. Triggers eviction if capacity/memory limits are exceeded. |
| `Delete` | `(c *Cache) Delete(_ context.Context, key string)` | `error` | Removes a key. |
| `Exists` | `(c *Cache) Exists(_ context.Context, key string)` | `bool, error` | Reports whether a key is present and not expired. |
| `Close` | `(c *Cache) Close()` | `error` | Stops the background cleanup goroutine. |
| `Stats` | `(c *Cache) Stats()` | `*cache.Stats` | Returns current hit/miss/eviction/size statistics. |
| `Len` | `(c *Cache) Len()` | `int` | Returns the current number of entries (including expired ones not yet cleaned). |
| `MemoryUsed` | `(c *Cache) MemoryUsed()` | `int64` | Returns approximate memory used by cached values in bytes. |
| `Flush` | `(c *Cache) Flush()` | (none) | Removes all entries from the cache. |

---

### Functions

| Function | Signature | Returns | Description |
|----------|-----------|---------|-------------|
| `DefaultConfig` | `DefaultConfig() *Config` | `*Config` | Returns a `Config` with defaults: 10000 entries, unlimited memory, 30m TTL, 1m cleanup, LRU. |
| `New` | `New(cfg *Config) *Cache` | `*Cache` | Creates a new in-memory cache and starts the background cleanup goroutine. Passing `nil` uses `DefaultConfig()`. |
| `FormatSize` | `FormatSize(bytes int64) string` | `string` | Returns a human-readable byte size string (e.g., `"1.50 MB"`, `"512 B"`). |

---

## Package `distributed` (`digital.vasic.cache/pkg/distributed`)

Distributed cache patterns: consistent hashing, two-level caching, and write strategies.

### Interfaces

#### `Strategy`

```go
type Strategy interface {
    Name() string
    Get(ctx context.Context, key string) ([]byte, error)
    Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
    Delete(ctx context.Context, key string) error
}
```

Defines the contract for cache write strategies that coordinate between a cache and a backing data store.

| Method | Returns | Description |
|--------|---------|-------------|
| `Name` | `string` | Returns the strategy name (e.g., `"write-through"`, `"write-back"`, `"cache-aside"`). |
| `Get` | `[]byte, error` | Retrieves a value using the strategy's read path. |
| `Set` | `error` | Writes a value using the strategy's write path. |
| `Delete` | `error` | Removes a value using the strategy's delete path. |

---

#### `DataSource`

```go
type DataSource interface {
    Load(ctx context.Context, key string) ([]byte, error)
    Store(ctx context.Context, key string, value []byte) error
    Remove(ctx context.Context, key string) error
}
```

Abstraction for the backing data store used by cache strategies.

| Method | Returns | Description |
|--------|---------|-------------|
| `Load` | `[]byte, error` | Retrieves data from the source. |
| `Store` | `error` | Persists data to the source. |
| `Remove` | `error` | Deletes data from the source. |

---

### Types

#### `ConsistentHash`

```go
type ConsistentHash struct { /* unexported */ }
```

Consistent hashing for distributing cache keys across nodes using virtual nodes (replicas).

**Methods:**

| Method | Signature | Returns | Description |
|--------|-----------|---------|-------------|
| `AddNode` | `(ch *ConsistentHash) AddNode(node string)` | (none) | Adds a physical node (with virtual replicas) to the hash ring. |
| `RemoveNode` | `(ch *ConsistentHash) RemoveNode(node string)` | (none) | Removes a node and all its virtual replicas from the ring. |
| `GetNode` | `(ch *ConsistentHash) GetNode(key string)` | `string` | Returns the node responsible for the given key. Returns `""` if no nodes exist. |
| `NodeCount` | `(ch *ConsistentHash) NodeCount()` | `int` | Returns the number of unique physical nodes. |

---

#### `TwoLevel`

```go
type TwoLevel struct { /* unexported */ }
```

Combines a local (L1) cache with a remote (L2) cache. Implements `cache.Cache`. Reads check L1 first; L2 hits are promoted to L1.

**Methods:**

| Method | Signature | Returns | Description |
|--------|-----------|---------|-------------|
| `Get` | `(t *TwoLevel) Get(ctx context.Context, key string)` | `[]byte, error` | Checks L1, then L2. Promotes L2 hits to L1. |
| `Set` | `(t *TwoLevel) Set(ctx context.Context, key string, value []byte, ttl time.Duration)` | `error` | Writes to both L1 and L2. L1 TTL is capped at the configured `l1TTL`. |
| `Delete` | `(t *TwoLevel) Delete(ctx context.Context, key string)` | `error` | Removes from both L1 and L2. |
| `Exists` | `(t *TwoLevel) Exists(ctx context.Context, key string)` | `bool, error` | Checks L1, then L2. |
| `Close` | `(t *TwoLevel) Close()` | `error` | Closes both caches. Returns the first error encountered. |

---

#### `WriteThrough`

```go
type WriteThrough struct { /* unexported */ }
```

Implements `Strategy`. Writes to the backing store and cache synchronously on every `Set`.

**Methods:**

| Method | Signature | Returns | Description |
|--------|-----------|---------|-------------|
| `Name` | `(w *WriteThrough) Name()` | `string` | Returns `"write-through"`. |
| `Get` | `(w *WriteThrough) Get(ctx context.Context, key string)` | `[]byte, error` | Cache hit returns data; miss loads from source and populates cache. |
| `Set` | `(w *WriteThrough) Set(ctx context.Context, key string, value []byte, ttl time.Duration)` | `error` | Stores in source first, then cache. |
| `Delete` | `(w *WriteThrough) Delete(ctx context.Context, key string)` | `error` | Removes from source, then cache. |

---

#### `WriteBack`

```go
type WriteBack struct { /* unexported */ }
```

Implements `Strategy`. Writes to cache immediately; flushes dirty entries to the backing store on demand.

**Methods:**

| Method | Signature | Returns | Description |
|--------|-----------|---------|-------------|
| `Name` | `(w *WriteBack) Name()` | `string` | Returns `"write-back"`. |
| `Get` | `(w *WriteBack) Get(ctx context.Context, key string)` | `[]byte, error` | Cache hit returns data; miss loads from source and populates cache. |
| `Set` | `(w *WriteBack) Set(ctx context.Context, key string, value []byte, ttl time.Duration)` | `error` | Writes to cache and marks the entry as dirty. |
| `Delete` | `(w *WriteBack) Delete(ctx context.Context, key string)` | `error` | Removes from dirty map, then source, then cache. |
| `Flush` | `(w *WriteBack) Flush(ctx context.Context)` | `error` | Writes all dirty entries to the backing store. |
| `DirtyCount` | `(w *WriteBack) DirtyCount()` | `int` | Returns the number of entries pending flush. |

---

#### `CacheAside`

```go
type CacheAside struct { /* unexported */ }
```

Implements `Strategy`. Lazy-loading pattern: data is loaded into cache only on read miss; writes go to the source and invalidate the cache.

**Methods:**

| Method | Signature | Returns | Description |
|--------|-----------|---------|-------------|
| `Name` | `(ca *CacheAside) Name()` | `string` | Returns `"cache-aside"`. |
| `Get` | `(ca *CacheAside) Get(ctx context.Context, key string)` | `[]byte, error` | Cache hit returns data; miss loads from source and populates cache. |
| `Set` | `(ca *CacheAside) Set(ctx context.Context, key string, value []byte, _ time.Duration)` | `error` | Stores in source, then invalidates (deletes) the cache entry. TTL parameter is ignored. |
| `Delete` | `(ca *CacheAside) Delete(ctx context.Context, key string)` | `error` | Removes from source, then cache. |

---

### Functions

| Function | Signature | Returns | Description |
|----------|-----------|---------|-------------|
| `NewConsistentHash` | `NewConsistentHash(replicas int) *ConsistentHash` | `*ConsistentHash` | Creates a consistent hash ring. `replicas` controls virtual nodes per physical node (default: 100 if <= 0). |
| `NewTwoLevel` | `NewTwoLevel(local, remote cache.Cache, l1TTL time.Duration) *TwoLevel` | `*TwoLevel` | Creates a two-level cache. `l1TTL` defaults to 5 minutes if <= 0. |
| `NewWriteThrough` | `NewWriteThrough(c cache.Cache, src DataSource) *WriteThrough` | `*WriteThrough` | Creates a write-through strategy. |
| `NewWriteBack` | `NewWriteBack(c cache.Cache, src DataSource) *WriteBack` | `*WriteBack` | Creates a write-back strategy. |
| `NewCacheAside` | `NewCacheAside(c cache.Cache, src DataSource) *CacheAside` | `*CacheAside` | Creates a cache-aside strategy. |

---

## Package `policy` (`digital.vasic.cache/pkg/policy`)

TTL policies and eviction decision logic.

### Interfaces

#### `TTLPolicy`

```go
type TTLPolicy interface {
    GetTTL(key string) time.Duration
}
```

Determines the time-to-live for a cache key.

---

#### `EvictionDecider`

```go
type EvictionDecider interface {
    ShouldEvict(key string, stats EvictionStats) bool
}
```

Decides whether a specific entry should be evicted based on cache statistics.

---

### Types

#### `EvictionStats`

```go
type EvictionStats struct {
    TotalEntries     int
    MaxEntries       int
    MemoryUsed       int64
    MaxMemory        int64
    EntryAge         time.Duration
    EntryAccessCount int64
    EntryLastAccess  time.Time
}
```

Information provided to eviction deciders for their decisions.

| Field | Type | Description |
|-------|------|-------------|
| `TotalEntries` | `int` | Current number of entries in the cache. |
| `MaxEntries` | `int` | Configured maximum entry count. |
| `MemoryUsed` | `int64` | Approximate memory used in bytes. |
| `MaxMemory` | `int64` | Configured memory limit in bytes. |
| `EntryAge` | `time.Duration` | How long ago the entry was created. |
| `EntryAccessCount` | `int64` | How many times the entry has been accessed. |
| `EntryLastAccess` | `time.Time` | Time of the last access. |

---

#### `FixedTTL`

```go
type FixedTTL struct {
    Duration time.Duration
}
```

Implements `TTLPolicy`. Returns the same TTL for every key.

**Methods:**

| Method | Signature | Returns | Description |
|--------|-----------|---------|-------------|
| `GetTTL` | `(f *FixedTTL) GetTTL(_ string)` | `time.Duration` | Returns `Duration` regardless of key. |

---

#### `SlidingTTL`

```go
type SlidingTTL struct {
    BaseTTL time.Duration
    // unexported fields
}
```

Implements `TTLPolicy`. Extends the TTL each time a key is accessed via `Touch`.

**Methods:**

| Method | Signature | Returns | Description |
|--------|-----------|---------|-------------|
| `GetTTL` | `(s *SlidingTTL) GetTTL(_ string)` | `time.Duration` | Returns `BaseTTL`. |
| `Touch` | `(s *SlidingTTL) Touch(key string)` | (none) | Records an access, resetting the TTL window for the key. |
| `ShouldExpire` | `(s *SlidingTTL) ShouldExpire(key string)` | `bool` | Returns true if the key has not been touched within `BaseTTL`. |
| `Remove` | `(s *SlidingTTL) Remove(key string)` | (none) | Stops tracking the given key. |

---

#### `AdaptiveTTL`

```go
type AdaptiveTTL struct {
    MinTTL time.Duration
    MaxTTL time.Duration
    // unexported fields
}
```

Implements `TTLPolicy`. Adjusts TTL based on access frequency. Frequently accessed keys get longer TTLs (up to `MaxTTL`); infrequently accessed keys get shorter TTLs (down to `MinTTL`).

**Methods:**

| Method | Signature | Returns | Description |
|--------|-----------|---------|-------------|
| `GetTTL` | `(a *AdaptiveTTL) GetTTL(key string)` | `time.Duration` | Returns a TTL between `MinTTL` and `MaxTTL` proportional to access count. Linear scaling capped at 100 accesses. |
| `RecordAccess` | `(a *AdaptiveTTL) RecordAccess(key string)` | (none) | Increments the access counter for a key. |
| `Reset` | `(a *AdaptiveTTL) Reset(key string)` | (none) | Clears the access count for a key. |
| `AccessCount` | `(a *AdaptiveTTL) AccessCount(key string)` | `int64` | Returns the current access count for a key. |

---

#### `CapacityEviction`

```go
type CapacityEviction struct {
    Threshold float64
}
```

Implements `EvictionDecider`. Evicts entries when the cache exceeds a fraction of its capacity.

**Methods:**

| Method | Signature | Returns | Description |
|--------|-----------|---------|-------------|
| `ShouldEvict` | `(c *CapacityEviction) ShouldEvict(_ string, stats EvictionStats)` | `bool` | Returns true if `TotalEntries / MaxEntries >= Threshold`. |

---

#### `AgeEviction`

```go
type AgeEviction struct {
    MaxAge time.Duration
}
```

Implements `EvictionDecider`. Evicts entries older than a maximum age.

**Methods:**

| Method | Signature | Returns | Description |
|--------|-----------|---------|-------------|
| `ShouldEvict` | `(a *AgeEviction) ShouldEvict(_ string, stats EvictionStats)` | `bool` | Returns true if `EntryAge > MaxAge`. |

---

#### `FrequencyEviction`

```go
type FrequencyEviction struct {
    MinAccesses int64
}
```

Implements `EvictionDecider`. Evicts entries accessed fewer than `MinAccesses` times.

**Methods:**

| Method | Signature | Returns | Description |
|--------|-----------|---------|-------------|
| `ShouldEvict` | `(f *FrequencyEviction) ShouldEvict(_ string, stats EvictionStats)` | `bool` | Returns true if `EntryAccessCount < MinAccesses`. |

---

#### `CompositeEviction`

```go
type CompositeEviction struct {
    // unexported fields
}
```

Implements `EvictionDecider`. Combines multiple eviction policies. An entry is evicted if ANY sub-policy triggers.

**Methods:**

| Method | Signature | Returns | Description |
|--------|-----------|---------|-------------|
| `ShouldEvict` | `(c *CompositeEviction) ShouldEvict(key string, stats EvictionStats)` | `bool` | Returns true if any sub-policy returns true. |

---

### Functions

| Function | Signature | Returns | Description |
|----------|-----------|---------|-------------|
| `NewFixedTTL` | `NewFixedTTL(d time.Duration) *FixedTTL` | `*FixedTTL` | Creates a fixed TTL policy. |
| `NewSlidingTTL` | `NewSlidingTTL(baseTTL time.Duration) *SlidingTTL` | `*SlidingTTL` | Creates a sliding TTL policy. |
| `NewAdaptiveTTL` | `NewAdaptiveTTL(minTTL, maxTTL time.Duration) *AdaptiveTTL` | `*AdaptiveTTL` | Creates an adaptive TTL policy. Swaps min/max if reversed. |
| `NewCapacityEviction` | `NewCapacityEviction(threshold float64) *CapacityEviction` | `*CapacityEviction` | Creates a capacity-based eviction policy. Defaults to 0.9 if threshold is <= 0 or > 1.0. |
| `NewAgeEviction` | `NewAgeEviction(maxAge time.Duration) *AgeEviction` | `*AgeEviction` | Creates an age-based eviction policy. |
| `NewFrequencyEviction` | `NewFrequencyEviction(minAccesses int64) *FrequencyEviction` | `*FrequencyEviction` | Creates a frequency-based eviction policy. |
| `NewCompositeEviction` | `NewCompositeEviction(policies ...EvictionDecider) *CompositeEviction` | `*CompositeEviction` | Creates a composite eviction policy from multiple sub-policies. |
