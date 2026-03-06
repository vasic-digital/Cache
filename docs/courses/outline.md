# Course: High-Performance Caching in Go

## Module Overview

This course covers the `digital.vasic.cache` module, providing core cache interfaces, in-memory caching with LRU/LFU/FIFO eviction, Redis backends, distributed patterns (consistent hashing, two-level caching, write-through/write-back/cache-aside), and TTL/eviction policies. You will learn to build composable caching layers from simple building blocks.

## Prerequisites

- Intermediate Go knowledge (interfaces, generics, goroutines)
- Basic understanding of caching concepts (TTL, eviction, write strategies)
- Familiarity with Redis concepts (optional)
- Go 1.24+ installed

## Lessons

| # | Title | Duration |
|---|-------|----------|
| 1 | Core Interface and In-Memory Cache | 45 min |
| 2 | Redis and Distributed Patterns | 50 min |
| 3 | TTL Policies and Service Caching | 40 min |

## Source Files

- `pkg/cache/` -- Core `Cache` interface, `TypedCache[T]`, config, stats
- `pkg/memory/` -- In-memory cache with LRU/LFU/FIFO eviction
- `pkg/redis/` -- Redis single and cluster backends
- `pkg/distributed/` -- Consistent hash, two-level cache, write strategies
- `pkg/policy/` -- TTL policies and eviction deciders
- `pkg/service/` -- Service-layer caching utilities
