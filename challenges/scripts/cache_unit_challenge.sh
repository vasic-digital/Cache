#!/usr/bin/env bash
# cache_unit_challenge.sh - Validates Cache module unit tests
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MODULE_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd)"
MODULE_NAME="Cache"

PASS=0
FAIL=0
TOTAL=0

pass() { PASS=$((PASS+1)); TOTAL=$((TOTAL+1)); echo "  PASS: $1"; }
fail() { FAIL=$((FAIL+1)); TOTAL=$((TOTAL+1)); echo "  FAIL: $1"; }

echo "=== ${MODULE_NAME} Unit Test Challenge ==="
echo ""

# Test 1: Test files exist
echo "Test: Test files exist"
test_count=$(find "${MODULE_DIR}" -name "*_test.go" | wc -l)
if [ "${test_count}" -gt 0 ]; then
    pass "Found ${test_count} test files"
else
    fail "No test files found"
fi

# Test 2: Tests exist in each package
echo "Test: Test coverage across packages"
pkgs_with_tests=0
for pkg_dir in "${MODULE_DIR}"/pkg/*/; do
    pkg_name=$(basename "$pkg_dir")
    pkg_tests=$(find "$pkg_dir" -name "*_test.go" | wc -l)
    if [ "$pkg_tests" -gt 0 ]; then
        pkgs_with_tests=$((pkgs_with_tests + 1))
    fi
done
if [ "$pkgs_with_tests" -ge 3 ]; then
    pass "At least 3 packages have tests (found ${pkgs_with_tests})"
else
    fail "Only ${pkgs_with_tests} packages have tests (expected at least 3)"
fi

# Test 3: Unit tests pass
echo "Test: Unit tests pass"
if (cd "${MODULE_DIR}" && GOMAXPROCS=2 nice -n 19 go test -short -count=1 -p 1 ./... 2>&1); then
    pass "Unit tests pass"
else
    fail "Unit tests failed"
fi

# Test 4: No race conditions (short mode)
echo "Test: Race detector clean"
if (cd "${MODULE_DIR}" && GOMAXPROCS=2 nice -n 19 go test -short -race -count=1 -p 1 ./... 2>&1); then
    pass "No race conditions detected"
else
    fail "Race conditions detected"
fi

echo ""
echo "=== Results: ${PASS}/${TOTAL} passed, ${FAIL} failed ==="
[ "${FAIL}" -eq 0 ] && exit 0 || exit 1
