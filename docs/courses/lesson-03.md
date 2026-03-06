# Lesson 3: TTL Policies and Service Caching

## Objectives

- Choose between fixed, sliding, and adaptive TTL policies
- Compose eviction deciders for custom eviction logic
- Use the service wrapper for cache-aside with statistics

## Concepts

### TTL Policies

All implement `TTLPolicy`:

```go
type TTLPolicy interface {
    GetTTL(key string) time.Duration
}
```

- **FixedTTL** -- same duration for every key
- **SlidingTTL** -- resets on each access via `Touch(key)`, expires if idle for `BaseTTL`
- **AdaptiveTTL** -- scales between `MinTTL` and `MaxTTL` based on access frequency

### Eviction Deciders

All implement `EvictionDecider`:

```go
type EvictionDecider interface {
    ShouldEvict(key string, stats EvictionStats) bool
}
```

- **CapacityEviction** -- evicts when entry count exceeds a threshold percentage
- **AgeEviction** -- evicts entries older than `MaxAge`
- **FrequencyEviction** -- evicts entries with fewer than `MinAccesses`
- **CompositeEviction** -- OR-combines multiple deciders

### Service Wrapper

`service.Wrapper` provides the cache-aside pattern with key prefixing and hit/miss tracking.

## Code Walkthrough

### Adaptive TTL

```go
adaptive := policy.NewAdaptiveTTL(1*time.Minute, 30*time.Minute)

// Record accesses
adaptive.RecordAccess("popular-key") // call on each access

// TTL scales with access count (linear up to 100 accesses)
ttl := adaptive.GetTTL("popular-key")
```

### Sliding TTL

```go
sliding := policy.NewSlidingTTL(15 * time.Minute)

sliding.Touch("session:abc") // reset the TTL window
if sliding.ShouldExpire("session:abc") {
    // idle for more than 15 minutes
}
```

### Composite eviction

```go
eviction := policy.NewCompositeEviction(
    policy.NewCapacityEviction(0.9),      // start at 90% capacity
    policy.NewAgeEviction(24*time.Hour),   // max 24h old
    policy.NewFrequencyEviction(3),        // at least 3 accesses
)

shouldEvict := eviction.ShouldEvict("key", stats)
```

### Service cache wrapper

```go
wrapper := service.New(myCache, service.Config{
    DefaultTTL: 5 * time.Minute,
    KeyPrefix:  "users:",
})

// GetOrLoad: cache-aside pattern
user, err := wrapper.GetOrLoad(ctx, "42", func(ctx context.Context, key string) (interface{}, error) {
    return db.FindUser(ctx, key)
})

// Invalidate on update
wrapper.Invalidate(ctx, "42")

// Check statistics
stats := wrapper.GetStats()
fmt.Printf("Hits: %d, Misses: %d, Errors: %d\n", stats.Hits, stats.Misses, stats.Errors)
```

## Summary

TTL policies and eviction deciders give fine-grained control over cache entry lifetime. The service wrapper encapsulates the cache-aside pattern, keeping caching logic out of business code while providing visibility through statistics.
