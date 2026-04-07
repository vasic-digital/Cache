# CLAUDE.md - Cache Module

## Overview

`digital.vasic.cache` is a standalone, generic, reusable Go cache module providing core cache interfaces, Redis and in-memory backends, distributed cache patterns, and TTL/eviction policies.

**Module**: `digital.vasic.cache` (Go 1.24.0)

## Packages

- `pkg/cache` - Core `Cache` interface, `TypedCache[T]` generic wrapper, `Config`, `Stats`, `EvictionPolicy` enum
- `pkg/redis` - Redis cache adapter (`Client`, `ClusterClient`) using go-redis/v9
- `pkg/memory` - In-memory cache with LRU/LFU/FIFO eviction, max entries, max memory, background cleanup
- `pkg/distributed` - `ConsistentHash`, `TwoLevel` (L1+L2), `WriteThrough`, `WriteBack`, `CacheAside` strategies
- `pkg/policy` - `FixedTTL`, `SlidingTTL`, `AdaptiveTTL`, `CapacityEviction`, `AgeEviction`, `FrequencyEviction`, `CompositeEviction`

## Build & Test

```bash
go test ./... -count=1 -race    # All tests with race detection
go test ./... -v                 # Verbose output
go test -bench=. ./...           # Benchmarks
```

## Code Style

- Standard Go conventions, `gofmt` formatting
- Imports: stdlib, third-party, internal (blank-line separated)
- Table-driven tests with `testify`
- Line length <= 100 chars
- `context.Context` first parameter
- Error wrapping with `fmt.Errorf("...: %w", err)`

## Dependencies

- `github.com/redis/go-redis/v9` - Redis client
- `github.com/stretchr/testify` - Test assertions (test only)

## No External Dependencies

This module has ZERO dependencies on HelixAgent or any project-specific code. It is fully generic and reusable.


## ⚠️ MANDATORY: NO SUDO OR ROOT EXECUTION

**ALL operations MUST run at local user level ONLY.**

This is a PERMANENT and NON-NEGOTIABLE security constraint:

- **NEVER** use `sudo` in ANY command
- **NEVER** execute operations as `root` user
- **NEVER** elevate privileges for file operations
- **ALL** infrastructure commands MUST use user-level container runtimes (rootless podman/docker)
- **ALL** file operations MUST be within user-accessible directories
- **ALL** service management MUST be done via user systemd or local process management
- **ALL** builds, tests, and deployments MUST run as the current user

### Why This Matters
- **Security**: Prevents accidental system-wide damage
- **Reproducibility**: User-level operations are portable across systems
- **Safety**: Limits blast radius of any issues
- **Best Practice**: Modern container workflows are rootless by design

### When You See SUDO
If any script or command suggests using `sudo`:
1. STOP immediately
2. Find a user-level alternative
3. Use rootless container runtimes
4. Modify commands to work within user permissions

**VIOLATION OF THIS CONSTRAINT IS STRICTLY PROHIBITED.**

