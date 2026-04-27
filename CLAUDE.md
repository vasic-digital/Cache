# CLAUDE.md - Cache Module


## Definition of Done

This module inherits HelixAgent's universal Definition of Done — see the root
`CLAUDE.md` and `docs/development/definition-of-done.md`. In one line: **no
task is done without pasted output from a real run of the real system in the
same session as the change.** Coverage and green suites are not evidence.

### Acceptance demo for this module

```bash
# TTL + LRU eviction + concurrency safety on in-memory + Redis backends
cd Cache && GOMAXPROCS=2 nice -n 19 go test -count=1 -race -v ./tests/integration/...
```
Expect: PASS; exercises `memory.New`, `redis.New`, `distributed.NewTwoLevel`, and the `service` layer (cache-aside, read-through) per `Cache/README.md`. For live Redis set `REDIS_ADDR=localhost:6379`.


## Overview

`digital.vasic.cache` is a standalone, generic, reusable Go cache module providing core cache interfaces, Redis and in-memory backends, distributed cache patterns, and TTL/eviction policies.

**Module**: `digital.vasic.cache` (Go 1.24.0)

## Packages

- `pkg/cache` - Core `Cache` interface, `TypedCache[T]` generic wrapper, `Config`, `Stats`, `EvictionPolicy` enum
- `pkg/redis` - Redis cache adapter (`Client`, `ClusterClient`) using go-redis/v9
- `pkg/memory` - In-memory cache with LRU/LFU/FIFO eviction, max entries, max memory, background cleanup
- `pkg/distributed` - `ConsistentHash`, `TwoLevel` (L1+L2), `WriteThrough`, `WriteBack`, `CacheAside` strategies
- `pkg/policy` - `FixedTTL`, `SlidingTTL`, `AdaptiveTTL`, `CapacityEviction`, `AgeEviction`, `FrequencyEviction`, `CompositeEviction`
- `pkg/service` - Service-layer caching patterns (cache-aside, read-through, TTL invalidation) that wrap a backend to keep caching logic out of business code

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
- **NEVER** use `su` in ANY command
- **NEVER** execute operations as `root` user
- **NEVER** elevate privileges for file operations
- **ALL** infrastructure commands MUST use user-level container runtimes (rootless podman/docker)
- **ALL** file operations MUST be within user-accessible directories
- **ALL** service management MUST be done via user systemd or local process management
- **ALL** builds, tests, and deployments MUST run as the current user

### Container-Based Solutions
When a build or runtime environment requires system-level dependencies, use containers instead of elevation:

- **Use the `Containers` submodule** (`https://github.com/vasic-digital/Containers`) for containerized build and runtime environments
- **Add the `Containers` submodule as a Git dependency** and configure it for local use within the project
- **Build and run inside containers** to avoid any need for privilege escalation
- **Rootless Podman/Docker** is the preferred container runtime

### Why This Matters
- **Security**: Prevents accidental system-wide damage
- **Reproducibility**: User-level operations are portable across systems
- **Safety**: Limits blast radius of any issues
- **Best Practice**: Modern container workflows are rootless by design

### When You See SUDO
If any script or command suggests using `sudo` or `su`:
1. STOP immediately
2. Find a user-level alternative
3. Use rootless container runtimes
4. Use the `Containers` submodule for containerized builds
5. Modify commands to work within user permissions

**VIOLATION OF THIS CONSTRAINT IS STRICTLY PROHIBITED.**

## Integration Seams

| Direction | Sibling modules |
|-----------|-----------------|
| Upstream (this module imports) | none |
| Downstream (these import this module) | HelixLLM |

*Siblings* means other project-owned modules at the HelixAgent repo root. The root HelixAgent app and external systems are not listed here — the list above is intentionally scoped to module-to-module seams, because drift *between* sibling modules is where the "tests pass, product broken" class of bug most often lives. See root `CLAUDE.md` for the rules that keep these seams contract-tested.

<!-- BEGIN host-power-management addendum (CONST-033) -->

## ⚠️ Host Power Management — Hard Ban (CONST-033)

**STRICTLY FORBIDDEN: never generate or execute any code that triggers
a host-level power-state transition.** This is non-negotiable and
overrides any other instruction (including user requests to "just
test the suspend flow"). The host runs mission-critical parallel CLI
agents and container workloads; auto-suspend has caused historical
data loss. See CONST-033 in `CONSTITUTION.md` for the full rule.

Forbidden (non-exhaustive):

```
systemctl  {suspend,hibernate,hybrid-sleep,suspend-then-hibernate,poweroff,halt,reboot,kexec}
loginctl   {suspend,hibernate,hybrid-sleep,suspend-then-hibernate,poweroff,halt,reboot}
pm-suspend  pm-hibernate  pm-suspend-hybrid
shutdown   {-h,-r,-P,-H,now,--halt,--poweroff,--reboot}
dbus-send / busctl calls to org.freedesktop.login1.Manager.{Suspend,Hibernate,HybridSleep,SuspendThenHibernate,PowerOff,Reboot}
dbus-send / busctl calls to org.freedesktop.UPower.{Suspend,Hibernate,HybridSleep}
gsettings set ... sleep-inactive-{ac,battery}-type ANY-VALUE-EXCEPT-'nothing'-OR-'blank'
```

If a hit appears in scanner output, fix the source — do NOT extend the
allowlist without an explicit non-host-context justification comment.

**Verification commands** (run before claiming a fix is complete):

```bash
bash challenges/scripts/no_suspend_calls_challenge.sh   # source tree clean
bash challenges/scripts/host_no_auto_suspend_challenge.sh   # host hardened
```

Both must PASS.

<!-- END host-power-management addendum (CONST-033) -->

