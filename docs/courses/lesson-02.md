# Lesson 2: Redis and Distributed Patterns

## Objectives

- Connect to Redis single-instance and cluster
- Use consistent hashing for cache distribution
- Implement two-level caching and write strategies

## Concepts

### Redis Backend

The `redis` package wraps go-redis/v9 and implements the `Cache` interface. It supports both single-instance (`Client`) and cluster (`ClusterClient`) modes.

### Consistent Hashing

`ConsistentHash` distributes keys across nodes using virtual replicas for uniform distribution. Adding or removing a node only redistributes a fraction of keys.

### Two-Level Cache

`TwoLevel` combines a fast local cache (L1) with a shared remote cache (L2). Reads check L1 first and promote L2 hits. Writes go to both levels.

### Write Strategies

| Strategy | Write Path | Read Path |
|----------|-----------|-----------|
| WriteThrough | Source, then cache | Cache, miss loads from source |
| WriteBack | Cache only (flush later) | Cache, miss loads from source |
| CacheAside | Source only (invalidate cache) | Cache, miss loads from source |

## Code Walkthrough

### Redis single-instance

```go
client := redis.New(&redis.Config{
    Addr:     "localhost:6379",
    PoolSize: 10,
})
defer client.Close()

err := client.HealthCheck(ctx)
```

### Consistent hashing

```go
ch := distributed.NewConsistentHash(100) // 100 virtual nodes per physical node
ch.AddNode("cache-1")
ch.AddNode("cache-2")
ch.AddNode("cache-3")

node := ch.GetNode("user:42") // deterministic node selection
ch.RemoveNode("cache-2")      // only ~1/3 of keys are remapped
```

### Two-level cache

```go
twoLevel := distributed.NewTwoLevel(localCache, redisCache, 5*time.Minute)

// Get: L1 -> L2 -> promote to L1
data, _ := twoLevel.Get(ctx, "key")

// Set: both L1 and L2
twoLevel.Set(ctx, "key", data, 1*time.Hour)
```

### Write-back with flush

```go
wb := distributed.NewWriteBack(cacheBackend, dataSource)
wb.Set(ctx, "key", value, 0) // writes to cache, marks dirty
fmt.Println(wb.DirtyCount()) // 1
wb.Flush(ctx)                // persists all dirty entries
```

## Practice Exercise

1. Create a consistent hash ring with 3 nodes and 100 replicas. Map 1000 keys and count the distribution per node. Verify each node gets approximately 333 keys (within 10% variance).
2. Set up a `TwoLevel` cache with an in-memory L1 and a mock L2. Set a key in L2 only, then get it through `TwoLevel`. Verify the key is promoted to L1 on the next access.
3. Implement a `DataSource` interface and use `WriteBack`. Set 5 keys, verify `DirtyCount()` is 5, then call `Flush()` and verify the data source received all 5 writes and `DirtyCount()` is 0.
