#!/usr/bin/env bash
# Run all Docker-Guardian tests
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GUARDIAN_IMAGE="${GUARDIAN_IMAGE:-docker-guardian}"
TOTAL_PASS=0
TOTAL_FAIL=0

echo "============================================="
echo "  Docker-Guardian Test Suite"
echo "  Image: ${GUARDIAN_IMAGE}"
echo "============================================="
echo ""

run_test() {
  local test_name="$1"
  local test_script="$2"

  echo "───────────────────────────────────────────"
  echo "Running: $test_name"
  echo "───────────────────────────────────────────"

  if GUARDIAN_IMAGE="$GUARDIAN_IMAGE" bash "$SCRIPT_DIR/$test_script"; then
    echo ">>> $test_name: ALL PASSED"
    TOTAL_PASS=$((TOTAL_PASS + 1))
  else
    echo ">>> $test_name: SOME FAILURES"
    TOTAL_FAIL=$((TOTAL_FAIL + 1))
  fi
  echo ""
}

run_test "Healthcheck Restart" "test-healthcheck.sh"
run_test "Dependency Recovery" "test-dependency.sh"
run_test "Backup Awareness" "test-backup.sh"
run_test "Grace Period" "test-grace.sh"
run_test "Watchtower Awareness" "test-watchtower.sh"
run_test "Notifications" "test-notifications.sh"
run_test "Opt-Out Label" "test-opt-out.sh"
run_test "Circuit Breaker" "test-circuit-breaker.sh"
run_test "Custom Label Filter" "test-custom-label.sh"

echo "============================================="
echo "  Final Results: ${TOTAL_PASS} suites passed, ${TOTAL_FAIL} suites failed"
echo "============================================="

if [ "$TOTAL_FAIL" -gt 0 ]; then
  exit 1
fi
