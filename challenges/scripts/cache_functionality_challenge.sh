#!/usr/bin/env bash
# cache_functionality_challenge.sh - Validates Cache module core functionality and structure
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MODULE_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd)"
MODULE_NAME="Cache"

PASS=0
FAIL=0
TOTAL=0

pass() { PASS=$((PASS+1)); TOTAL=$((TOTAL+1)); echo "  PASS: $1"; }
fail() { FAIL=$((FAIL+1)); TOTAL=$((TOTAL+1)); echo "  FAIL: $1"; }

echo "=== ${MODULE_NAME} Functionality Challenge ==="
echo ""

# Test 1: Required packages exist
echo "Test: Required packages exist"
pkgs_ok=true
for pkg in cache redis memory policy distributed; do
    if [ ! -d "${MODULE_DIR}/pkg/${pkg}" ]; then
        fail "Missing package: pkg/${pkg}"
        pkgs_ok=false
    fi
done
if [ "$pkgs_ok" = true ]; then
    pass "All required packages present (cache, redis, memory, policy, distributed)"
fi

# Test 2: Cache interface is defined
echo "Test: Cache interface is defined"
if grep -rq "type Cache interface" "${MODULE_DIR}/pkg/cache/"; then
    pass "Cache interface is defined in pkg/cache"
else
    fail "Cache interface not found in pkg/cache"
fi

# Test 3: TTLPolicy interface is defined
echo "Test: TTLPolicy interface is defined"
if grep -rq "type TTLPolicy interface" "${MODULE_DIR}/pkg/policy/"; then
    pass "TTLPolicy interface is defined in pkg/policy"
else
    fail "TTLPolicy interface not found in pkg/policy"
fi

# Test 4: Redis client implementation exists
echo "Test: Redis client implementation exists"
if grep -rq "type Client struct" "${MODULE_DIR}/pkg/redis/"; then
    pass "Redis Client struct exists in pkg/redis"
else
    fail "Redis Client struct not found in pkg/redis"
fi

# Test 5: In-memory cache implementation exists
echo "Test: In-memory cache implementation exists"
if grep -rq "type\s\+\w\+\s\+struct" "${MODULE_DIR}/pkg/memory/"; then
    pass "In-memory cache implementation exists in pkg/memory"
else
    fail "No struct implementation found in pkg/memory"
fi

# Test 6: Distributed cache support exists
echo "Test: Distributed cache support exists"
if grep -rq "type\s\+\w\+\s\+struct" "${MODULE_DIR}/pkg/distributed/"; then
    pass "Distributed cache implementation exists in pkg/distributed"
else
    fail "No struct implementation found in pkg/distributed"
fi

# Test 7: Eviction policy support
echo "Test: Eviction policy support exists"
if grep -rq "EvictionPolicy\|Evict\|LRU\|LFU\|TTL" "${MODULE_DIR}/pkg/policy/"; then
    pass "Eviction policy support found"
else
    fail "No eviction policy support found"
fi

# Test 8: Cache operations include Get/Set/Delete
echo "Test: Cache operations include Get/Set/Delete"
ops_found=0
for op in Get Set Delete; do
    if grep -rq "${op}" "${MODULE_DIR}/pkg/cache/"; then
        ops_found=$((ops_found + 1))
    fi
done
if [ "$ops_found" -ge 3 ]; then
    pass "Cache operations Get/Set/Delete found"
else
    fail "Missing cache operations (found ${ops_found}/3)"
fi

# Test 9: Config struct exists
echo "Test: Config struct exists"
if grep -rq "type Config struct" "${MODULE_DIR}/pkg/"; then
    pass "Config struct exists"
else
    fail "Config struct not found"
fi

# Test 10: Tests directory or test files exist
echo "Test: Tests directory exists"
if [ -d "${MODULE_DIR}/tests" ] || [ "$(find "${MODULE_DIR}/pkg" -name "*_test.go" | wc -l)" -gt 0 ]; then
    pass "Tests exist"
else
    fail "No tests found"
fi

echo ""
echo "=== Results: ${PASS}/${TOTAL} passed, ${FAIL} failed ==="
[ "${FAIL}" -eq 0 ] && exit 0 || exit 1
