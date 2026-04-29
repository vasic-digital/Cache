# AGENTS.md - Cache Module Multi-Agent Coordination Guide

## Overview

This document provides guidance for AI agents and multi-agent systems working with the `digital.vasic.cache` module. It describes package responsibilities, coordination boundaries, and safe modification patterns.

## Module Identity

- **Module path**: `digital.vasic.cache`
- **Go version**: 1.24.0
- **Purpose**: Standalone, generic, reusable cache library with no project-specific dependencies
- **External dependencies**: `github.com/redis/go-redis/v9` (runtime), `github.com/stretchr/testify` (test only)

## Package Ownership Map

Each package has a clear responsibility boundary. Agents should respect these boundaries and avoid cross-cutting changes without coordinating across affected packages.

| Package | Path | Responsibility | Key Interfaces |
|---------|------|---------------|----------------|
| cache | `pkg/cache/` | Core interface definitions, typed wrapper, config, stats, eviction policy enum | `Cache`, `TypedCache[T]`, `EvictionPolicy` |
| redis | `pkg/redis/` | Redis single-instance and cluster backend | `Client`, `ClusterClient` |
| memory | `pkg/memory/` | Thread-safe in-memory backend with LRU/LFU/FIFO eviction | `Cache` (implements `cache.Cache`) |
| distributed | `pkg/distributed/` | Distributed patterns: consistent hashing, two-level cache, write strategies | `Strategy`, `DataSource`, `ConsistentHash`, `TwoLevel` |
| policy | `pkg/policy/` | TTL policies and eviction decision logic | `TTLPolicy`, `EvictionDecider` |

## Dependency Graph

```
pkg/cache        (no internal dependencies -- leaf package)
    ^
    |
pkg/redis        (depends on: pkg/cache interface only via go-redis)
pkg/memory       (depends on: pkg/cache)
pkg/distributed  (depends on: pkg/cache)
pkg/policy       (no internal dependencies -- leaf package)
```

The `pkg/cache` and `pkg/policy` packages are leaf packages with no internal imports. All other packages depend only on `pkg/cache` for the `Cache` interface.

## Agent Coordination Rules

### Rule 1: Interface Changes Require Full Coordination

If an agent modifies the `Cache` interface in `pkg/cache/cache.go`, all implementing packages (`pkg/redis`, `pkg/memory`, `pkg/distributed`) must be updated simultaneously. This is a breaking change.

**Affected files on `Cache` interface change:**
- `pkg/cache/cache.go` -- interface definition
- `pkg/redis/redis.go` -- `Client` and `ClusterClient` implementations
- `pkg/memory/memory.go` -- `Cache` implementation
- `pkg/distributed/distributed.go` -- `TwoLevel` implementation
- All corresponding `_test.go` files

### Rule 2: Backend Changes Are Isolated

Changes to `pkg/redis` or `pkg/memory` internals do not affect other packages as long as they continue to satisfy the `Cache` interface. An agent working on Redis-specific features does not need to coordinate with the memory package agent.

### Rule 3: Policy Package Is Independent

The `pkg/policy` package defines `TTLPolicy` and `EvictionDecider` interfaces that are consumed by application code, not by other packages in this module. Changes to policy are self-contained.

### Rule 4: Distributed Package Composes, Does Not Extend

The `pkg/distributed` package composes `cache.Cache` instances (via `TwoLevel`, `WriteThrough`, `WriteBack`, `CacheAside`). It does not subclass or embed them. Agents adding new strategies should follow the same composition pattern.

### Rule 5: Strategy Interface Is Separate From Cache Interface

`distributed.Strategy` is a distinct interface from `cache.Cache`. Agents should not conflate the two. Strategy implementations (`WriteThrough`, `WriteBack`, `CacheAside`) wrap a `cache.Cache` but are not themselves `cache.Cache` implementations.

## Safe Modification Patterns

### Adding a New Cache Backend

1. Create a new package under `pkg/` (e.g., `pkg/memcached/`)
2. Implement the `cache.Cache` interface
3. Add a compile-time interface check: `var _ cache.Cache = (*Client)(nil)`
4. Write table-driven tests with `testify`
5. No changes required in any other package

### Adding a New Write Strategy

1. Add the strategy struct and constructor to `pkg/distributed/distributed.go`
2. Implement the `Strategy` interface (`Name`, `Get`, `Set`, `Delete`)
3. Add a compile-time check: `var _ Strategy = (*NewStrategy)(nil)`
4. Write tests in `pkg/distributed/distributed_test.go`

### Adding a New TTL Policy

1. Add the policy struct and constructor to `pkg/policy/policy.go`
2. Implement the `TTLPolicy` interface (`GetTTL`)
3. Add a compile-time check: `var _ TTLPolicy = (*NewPolicy)(nil)`
4. Write tests in `pkg/policy/policy_test.go`

### Adding a New Eviction Decider

1. Add the decider struct to `pkg/policy/policy.go`
2. Implement `EvictionDecider` (`ShouldEvict`)
3. Add a compile-time check
4. Write tests -- ensure compatibility with `CompositeEviction`

## Testing Coordination

All tests use `testify` with table-driven patterns. Agents should:

- Run `go test ./... -count=1 -race` after any change
- Ensure zero race conditions (the memory cache uses `sync.RWMutex` and `atomic` operations)
- Never introduce mocks for the `Cache` interface in production code
- Test files live alongside source files (`*_test.go`)

## Concurrency Notes

- `pkg/memory` uses `sync.RWMutex` for map access and `sync/atomic` for counters
- `pkg/distributed` `ConsistentHash` uses `sync.RWMutex` for the hash ring
- `pkg/distributed` `WriteBack` uses `sync.Mutex` for the dirty map
- `pkg/policy` `SlidingTTL` and `AdaptiveTTL` use `sync.Map` for concurrent key tracking
- All `Cache` interface methods accept `context.Context` as the first parameter

## File Map

```
Cache/
  go.mod
  go.sum
  README.md
  CLAUDE.md
  AGENTS.md
  pkg/
    cache/
      cache.go          -- Core interfaces, TypedCache[T], Config, Stats
      cache_test.go
    redis/
      redis.go          -- Client, ClusterClient, Config, ClusterConfig
      redis_test.go
    memory/
      memory.go         -- Cache, Config, Stats, Flush, FormatSize
      memory_test.go
    distributed/
      distributed.go    -- ConsistentHash, TwoLevel, Strategy, DataSource,
                           WriteThrough, WriteBack, CacheAside
      distributed_test.go
    policy/
      policy.go         -- TTLPolicy, FixedTTL, SlidingTTL, AdaptiveTTL,
                           EvictionDecider, EvictionStats, CapacityEviction,
                           AgeEviction, FrequencyEviction, CompositeEviction
      policy_test.go
  docs/
    USER_GUIDE.md
    ARCHITECTURE.md
    API_REFERENCE.md
    CONTRIBUTING.md
    CHANGELOG.md
    diagrams/
      architecture.mmd
      sequence.mmd
      class.mmd
```


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


### ⚠️⚠️⚠️ ABSOLUTELY MANDATORY: ZERO UNFINISHED WORK POLICY

NO unfinished work, TODOs, or known issues may remain in the codebase. EVER.

PROHIBITED: TODO/FIXME comments, empty implementations, silent errors, fake data, unwrap() calls that panic, empty catch blocks.

REQUIRED: Fix ALL issues immediately, complete implementations before committing, proper error handling in ALL code paths, real test assertions.

Quality Principle: If it is not finished, it does not ship. If it ships, it is finished.

<!-- BEGIN host-power-management addendum (CONST-033) -->

## Host Power Management — Hard Ban (CONST-033)

**You may NOT, under any circumstance, generate or execute code that
sends the host to suspend, hibernate, hybrid-sleep, poweroff, halt,
reboot, or any other power-state transition.** This rule applies to:

- Every shell command you run via the Bash tool.
- Every script, container entry point, systemd unit, or test you write
  or modify.
- Every CLI suggestion, snippet, or example you emit.

**Forbidden invocations** (non-exhaustive — see CONST-033 in
`CONSTITUTION.md` for the full list):

- `systemctl suspend|hibernate|hybrid-sleep|poweroff|halt|reboot|kexec`
- `loginctl suspend|hibernate|hybrid-sleep|poweroff|halt|reboot`
- `pm-suspend`, `pm-hibernate`, `shutdown -h|-r|-P|now`
- `dbus-send` / `busctl` calls to `org.freedesktop.login1.Manager.Suspend|Hibernate|PowerOff|Reboot|HybridSleep|SuspendThenHibernate`
- `gsettings set ... sleep-inactive-{ac,battery}-type` to anything but `'nothing'` or `'blank'`

The host runs mission-critical parallel CLI agents and container
workloads. Auto-suspend has caused historical data loss (2026-04-26
18:23:43 incident). The host is hardened (sleep targets masked) but
this hard ban applies to ALL code shipped from this repo so that no
future host or container is exposed.

**Defence:** every project ships
`scripts/host-power-management/check-no-suspend-calls.sh` (static
scanner) and
`challenges/scripts/no_suspend_calls_challenge.sh` (challenge wrapper).
Both MUST be wired into the project's CI / `run_all_challenges.sh`.

**Full background:** `docs/HOST_POWER_MANAGEMENT.md` and `CONSTITUTION.md` (CONST-033).

<!-- END host-power-management addendum (CONST-033) -->



<!-- CONST-035 anti-bluff addendum (cascaded) -->

## CONST-035 — Anti-Bluff Tests & Challenges (mandatory; inherits from root)

Tests and Challenges in this submodule MUST verify the product, not
the LLM's mental model of the product. A test that passes when the
feature is broken is worse than a missing test — it gives false
confidence and lets defects ship to users. Functional probes at the
protocol layer are mandatory:

- TCP-open is the FLOOR, not the ceiling. Postgres → execute
  `SELECT 1`. Redis → `PING` returns `PONG`. ChromaDB → `GET
  /api/v1/heartbeat` returns 200. MCP server → TCP connect + valid
  JSON-RPC handshake. HTTP gateway → real request, real response,
  non-empty body.
- Container `Up` is NOT application healthy. A `docker/podman ps`
  `Up` status only means PID 1 is running; the application may be
  crash-looping internally.
- No mocks/fakes outside unit tests (already CONST-030; CONST-035
  raises the cost of a mock-driven false pass to the same severity
  as a regression).
- Re-verify after every change. Don't assume a previously-passing
  test still verifies the same scope after a refactor.
- Verification of CONST-035 itself: deliberately break the feature
  (e.g. `kill <service>`, swap a password). The test MUST fail. If
  it still passes, the test is non-conformant and MUST be tightened.

## CONST-033 clarification — distinguishing host events from sluggishness

Heavy container builds (BuildKit pulling many GB of layers, parallel
podman/docker compose-up across many services) can make the host
**appear** unresponsive — high load average, slow SSH, watchers
timing out. **This is NOT a CONST-033 violation.** Suspend / hibernate
/ logout are categorically different events. Distinguish via:

- `uptime` — recent boot? if so, the host actually rebooted.
- `loginctl list-sessions` — session(s) still active? if yes, no logout.
- `journalctl ... | grep -i 'will suspend\|hibernate'` — zero broadcasts
  since the CONST-033 fix means no suspend ever happened.
- `dmesg | grep -i 'killed process\|out of memory'` — OOM kills are
  also NOT host-power events; they're memory-pressure-induced and
  require their own separate fix (lower per-container memory limits,
  reduce parallelism).

A sluggish host under build pressure recovers when the build finishes;
a suspended host requires explicit unsuspend (and CONST-033 should
make that impossible by hardening `IdleAction=ignore` +
`HandleSuspendKey=ignore` + masked `sleep.target`,
`suspend.target`, `hibernate.target`, `hybrid-sleep.target`).

If you observe what looks like a suspend during heavy builds, the
correct first action is **not** "edit CONST-033" but `bash
challenges/scripts/host_no_auto_suspend_challenge.sh` to confirm the
hardening is intact. If hardening is intact AND no suspend
broadcast appears in journal, the perceived event was build-pressure
sluggishness, not a power transition.

<!-- BEGIN anti-bluff-testing addendum (Article XI) -->

## Article XI — Anti-Bluff Testing (MANDATORY)

**Inherited from the umbrella project's Constitution Article XI.
Tests and Challenges that pass without exercising real end-user
behaviour are forbidden in this submodule too.**

Every test, every Challenge, every HelixQA bank entry MUST:

1. **Assert on a concrete end-user-visible outcome** — rendered DOM,
   DB rows that a real query would return, files on disk, media that
   actually plays, search results that actually contain expected
   items. Not "no error" or "200 OK".
2. **Run against the real system below the assertion.** Mocks/stubs
   are permitted ONLY in unit tests (`*_test.go` under `go test
   -short` or language equivalent). Integration / E2E / Challenge /
   HelixQA tests use real containers, real databases, real
   renderers. Unreachable real-system → skip with `SKIP-OK:
   #<ticket>`, never silently pass.
3. **Include a matching negative.** Every positive assertion is
   paired with an assertion that fails when the feature is broken.
4. **Emit copy-pasteable evidence** — body, screenshot, frame, DB
   row, log excerpt. Boolean pass/fail is insufficient.
5. **Verify "fails when feature is removed."** Author runs locally
   with the feature commented out; the test MUST FAIL. If it still
   passes, it's a bluff — delete and rewrite.
6. **No blind shells.** No `&& echo PASS`, `|| true`, `tee` exit
   laundering, `if [ -f file ]` without content assertion.

**Challenges in this submodule** must replay the user journey
end-to-end through the umbrella project's deliverables — never via
raw `curl` or third-party scripts. Sub-1-second Challenges almost
always indicate a bluff.

**HelixQA banks** declare executable actions
(`adb_shell:`, `playwright:`, `http:`, `assertVisible:`,
`assertNotVisible:`), never prose. Stagnation guard from Article I
§1.3 applies — frame N+1 identical to frame N for >10 s = FAIL.

**PR requirement:** every PR adding/modifying a test or Challenge in
this submodule MUST include a fenced `## Anti-Bluff Verification`
block with: (a) command run, (b) pasted output, (c) proof the test
fails when the feature is broken (second run with feature
commented-out showing FAIL).

**Cross-reference:** umbrella `CONSTITUTION.md` Article XI
(§§ 11.1 — 11.8).

<!-- END anti-bluff-testing addendum (Article XI) -->

<!-- BEGIN user-mandate forensic anchor (Article XI §11.9) -->

## ⚠️ User-Mandate Forensic Anchor (Article XI §11.9 — 2026-04-29)

Inherited from the umbrella project. Verbatim user mandate:

> "We had been in position that all tests do execute with success
> and all Challenges as well, but in reality the most of the
> features does not work and can't be used! This MUST NOT be the
> case and execution of tests and Challenges MUST guarantee the
> quality, the completion and full usability by end users of the
> product!"

**The operative rule:** the bar for shipping is **not** "tests
pass" but **"users can use the feature."**

Every PASS in this codebase MUST carry positive evidence captured
during execution that the feature works for the end user. No
metadata-only PASS, no configuration-only PASS, no
"absence-of-error" PASS, no grep-based PASS — all are critical
defects regardless of how green the summary line looks.

Tests and Challenges (HelixQA) are bound equally. A Challenge that
scores PASS on a non-functional feature is the same class of
defect as a unit test that does.

**No false-success results are tolerable.** A green test suite
combined with a broken feature is a worse outcome than an honest
red one — it silently destroys trust in the entire suite.

Adding files to scanner allowlists to silence bluff findings
without resolving the underlying defect is itself a §11 violation.

**Full text:** umbrella `CONSTITUTION.md` Article XI §11.9.

<!-- END user-mandate forensic anchor (Article XI §11.9) -->
