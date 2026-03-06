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

## Summary

Redis provides shared, persistent caching for multi-instance deployments. Consistent hashing distributes load evenly. Two-level caching and write strategies let you balance between latency, consistency, and throughput.
