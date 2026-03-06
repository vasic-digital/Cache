# FAQ

## What eviction policies are supported?

The in-memory cache supports three eviction policies:

- **LRU** (Least Recently Used) -- evicts the entry that was accessed longest ago
- **LFU** (Least Frequently Used) -- evicts the entry with the fewest access counts
- **FIFO** (First In, First Out) -- evicts the oldest entry by insertion time

Set the policy via `memory.Config.EvictionPolicy` using the `cache.LRU`, `cache.LFU`, or `cache.FIFO` constants.

## How does the memory limit work?

When `MaxMemoryBytes` is set, the cache tracks the byte size of all stored values. Before inserting a new entry, it evicts entries (using the configured policy) until the memory budget is not exceeded. The `MemoryUsed()` method returns the current total.

## What happens on a cache miss with Get?

`Get` returns `(nil, nil)` on a cache miss -- not an error. This lets callers distinguish between "key not found" and actual errors. The `TypedCache[T]` wrapper returns `(zero, false, nil)` on miss.

## How does the two-level cache handle L1 expiry?

When a key expires in L1 (local), the next `Get` call falls through to L2 (remote). If found in L2, the value is promoted back to L1 with the configured `l1TTL`. This happens transparently to the caller.

## When should I use write-back vs. write-through?

Use **write-through** when you need strong consistency -- every write is persisted to the backing store immediately. Use **write-back** when you need lower write latency and can tolerate eventual consistency -- writes are batched and flushed periodically or on demand via `Flush()`.
