# Lesson 1: Core Interface and In-Memory Cache

## Objectives

- Understand the `Cache` interface and its byte-oriented design
- Use `TypedCache[T]` for type-safe caching
- Configure eviction policies and memory limits

## Concepts

### The Cache Interface

```go
type Cache interface {
    Get(ctx context.Context, key string) ([]byte, error)
    Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
    Delete(ctx context.Context, key string) error
    Exists(ctx context.Context, key string) (bool, error)
    Close() error
}
```

Operating on `[]byte` keeps the interface serialization-agnostic. Any encoder (JSON, protobuf, msgpack) can be used.

### TypedCache[T]

```go
tc := cache.NewTypedCache[MyStruct](c)
tc.Set(ctx, "key", myStruct, ttl) // JSON marshals automatically
val, found, err := tc.Get(ctx, "key") // JSON unmarshals automatically
```

### Eviction Policies

The in-memory cache supports LRU, LFU, and FIFO. When `MaxEntries` or `MaxMemoryBytes` is reached, the policy determines which entry to evict.

## Code Walkthrough

### Creating with LFU eviction

```go
c := memory.New(&memory.Config{
    MaxEntries:      5000,
    MaxMemoryBytes:  50 * 1024 * 1024, // 50 MB
    DefaultTTL:      10 * time.Minute,
    CleanupInterval: 30 * time.Second,
    EvictionPolicy:  cache.LFU,
})
defer c.Close()
```

### How eviction works internally

- **LRU**: entries are stored in a doubly-linked list. `Get` moves the entry to the front. Eviction removes from the back.
- **LFU**: each entry has an access counter (`accessCnt`). Eviction scans for the minimum.
- **FIFO**: new entries go to the back of the list. Eviction removes from the front.

### Background cleanup

The `cleanupLoop` goroutine runs every `CleanupInterval` and removes all expired entries. This prevents expired entries from consuming memory between access-time lazy deletions.

### Flush all entries

```go
c.Flush() // removes everything, resets memory counter
```

## Summary

The `Cache` interface provides a clean abstraction for any backend. The in-memory implementation offers configurable eviction, memory limits, and automatic cleanup, making it suitable for both development and production single-instance deployments.
