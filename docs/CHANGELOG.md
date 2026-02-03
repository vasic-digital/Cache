# Changelog

All notable changes to the `digital.vasic.cache` module will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.1.0] - 2026-02-03

### Added

- **pkg/cache**: Core `Cache` interface with `Get`, `Set`, `Delete`, `Exists`, `Close` methods operating on raw byte slices.
- **pkg/cache**: `TypedCache[T]` generic wrapper with automatic JSON serialization/deserialization.
- **pkg/cache**: `Config` struct with `DefaultConfig()` providing sensible defaults (30m TTL, 10000 max size, LRU eviction).
- **pkg/cache**: `Stats` struct with `HitRate()` method for runtime cache statistics.
- **pkg/cache**: `EvictionPolicy` enum (`LRU`, `LFU`, `FIFO`) with `String()` method.
- **pkg/redis**: `Client` implementing `cache.Cache` for single Redis instance via go-redis/v9.
- **pkg/redis**: `ClusterClient` implementing `cache.Cache` for Redis Cluster via go-redis/v9.
- **pkg/redis**: `Config` and `ClusterConfig` structs with `DefaultConfig()`.
- **pkg/redis**: `HealthCheck` method on both `Client` and `ClusterClient`.
- **pkg/redis**: `Underlying()` method on `Client` for access to the raw go-redis client.
- **pkg/memory**: Thread-safe in-memory `Cache` with LRU, LFU, and FIFO eviction policies.
- **pkg/memory**: Configurable `MaxEntries` and `MaxMemoryBytes` limits with automatic eviction.
- **pkg/memory**: Background cleanup goroutine for expired entry removal.
- **pkg/memory**: `Stats()`, `Len()`, `MemoryUsed()`, and `Flush()` methods.
- **pkg/memory**: `FormatSize()` utility function for human-readable byte sizes.
- **pkg/distributed**: `ConsistentHash` with virtual nodes for uniform key distribution.
- **pkg/distributed**: `TwoLevel` cache (L1 local + L2 remote) implementing `cache.Cache` with automatic L2-to-L1 promotion.
- **pkg/distributed**: `Strategy` interface for cache write coordination with backing stores.
- **pkg/distributed**: `DataSource` interface abstracting backing data stores.
- **pkg/distributed**: `WriteThrough` strategy -- synchronous writes to source and cache.
- **pkg/distributed**: `WriteBack` strategy -- deferred writes with dirty tracking and `Flush`.
- **pkg/distributed**: `CacheAside` strategy -- lazy loading with cache invalidation on writes.
- **pkg/policy**: `TTLPolicy` interface with `FixedTTL`, `SlidingTTL`, and `AdaptiveTTL` implementations.
- **pkg/policy**: `SlidingTTL` with `Touch`, `ShouldExpire`, and `Remove` for session-style expiration.
- **pkg/policy**: `AdaptiveTTL` with `RecordAccess`, `Reset`, and `AccessCount` for frequency-based TTL scaling.
- **pkg/policy**: `EvictionDecider` interface with `CapacityEviction`, `AgeEviction`, `FrequencyEviction`, and `CompositeEviction` implementations.
- **pkg/policy**: `EvictionStats` struct providing cache metrics for eviction decisions.
- Unit tests for all packages with race detection support.
- Documentation: README, CLAUDE.md, AGENTS.md, User Guide, Architecture, API Reference, Contributing Guide, Mermaid diagrams.
