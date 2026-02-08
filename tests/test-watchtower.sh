#!/usr/bin/env bash
# Test: Watchtower/orchestration awareness
# Phase 1: Verify Guardian SKIPS orphans when orchestration cooldown is active
# Phase 2: Verify Guardian RECOVERS orphans when cooldown is disabled (cooldown=0)
# This two-phase approach works reliably on busy hosts where Docker events are always present.

set -euo pipefail

PASS=0
FAIL=0
GUARDIAN_IMAGE="${GUARDIAN_IMAGE:-docker-guardian}"
COOLDOWN=10  # Short for testing

cleanup() {
  echo "Cleaning up..."
  docker rm -f dg-test-wt-parent dg-test-wt-dependent dg-test-wt-dummy dg-test-guardian-wt 2>/dev/null || true
}
trap cleanup EXIT

echo "=== Test: Watchtower Awareness ==="

cleanup

# Create parent + dependent pair
echo "Starting parent container..."
docker run -d \
  --name dg-test-wt-parent \
  alpine:3.20 sh -c 'sleep 3600'

sleep 2

echo "Starting dependent container..."
docker run -d \
  --name dg-test-wt-dependent \
  --network=container:dg-test-wt-parent \
  alpine:3.20 sh -c 'sleep 3600'

sleep 2

# ── Phase 1: Cooldown active → Guardian should SKIP ──────────────────

echo ""
echo "--- Phase 1: Orchestration cooldown active (should skip) ---"

# Simulate Watchtower activity: create+destroy a dummy container
# This generates destroy+create events within the cooldown window
echo "Simulating Watchtower activity (create+destroy dummy container)..."
docker run -d --name dg-test-wt-dummy alpine:3.20 sh -c 'sleep 5'
sleep 1
docker rm -f dg-test-wt-dummy

sleep 1

# Start Guardian with cooldown enabled
echo "Starting Docker-Guardian (watchtower_cooldown=${COOLDOWN}s)..."
docker run -d \
  --name dg-test-guardian-wt \
  -e AUTOHEAL_CONTAINER_LABEL=all \
  -e AUTOHEAL_INTERVAL=3 \
  -e AUTOHEAL_GRACE_PERIOD=0 \
  -e AUTOHEAL_WATCHTOWER_COOLDOWN=$COOLDOWN \
  -e AUTOHEAL_WATCHTOWER_SCOPE=all \
  -e AUTOHEAL_WATCHTOWER_EVENTS=orchestration \
  -e AUTOHEAL_DEPENDENCY_START_DELAY=1 \
  -v /var/run/docker.sock:/var/run/docker.sock \
  "$GUARDIAN_IMAGE"

sleep 2

# Stop the dependent (creates orphan scenario)
echo "Stopping dependent (creating orphan scenario)..."
docker stop -t 1 dg-test-wt-dependent

# Wait a few cycles — guardian should detect orchestration events and skip
echo "Waiting 8s (guardian should skip due to orchestration cooldown)..."
sleep 8

DEP_STATE=$(docker inspect -f '{{.State.Status}}' dg-test-wt-dependent 2>/dev/null || echo "unknown")
if [ "$DEP_STATE" = "exited" ]; then
  echo "PASS: Dependent still stopped during orchestration cooldown"
  PASS=$((PASS + 1))
else
  echo "FAIL: Dependent was restarted during orchestration cooldown (state=$DEP_STATE)"
  FAIL=$((FAIL + 1))
fi

# Verify Guardian logged the orchestration detection
if docker logs dg-test-guardian-wt 2>&1 | grep -q "Orchestration activity detected"; then
  echo "PASS: Guardian logs show orchestration activity detection"
  PASS=$((PASS + 1))
else
  echo "FAIL: Guardian did not log orchestration activity detection"
  FAIL=$((FAIL + 1))
fi

# Stop Guardian for Phase 2
docker rm -f dg-test-guardian-wt 2>/dev/null || true

# ── Phase 2: Cooldown disabled → Guardian should RECOVER ─────────────

echo ""
echo "--- Phase 2: Orchestration cooldown disabled (should recover) ---"

# Dependent should still be exited from Phase 1
echo "Starting Docker-Guardian (watchtower_cooldown=0, disabled)..."
docker run -d \
  --name dg-test-guardian-wt \
  -e AUTOHEAL_CONTAINER_LABEL=all \
  -e AUTOHEAL_INTERVAL=3 \
  -e AUTOHEAL_GRACE_PERIOD=0 \
  -e AUTOHEAL_WATCHTOWER_COOLDOWN=0 \
  -e AUTOHEAL_DEPENDENCY_START_DELAY=1 \
  -v /var/run/docker.sock:/var/run/docker.sock \
  "$GUARDIAN_IMAGE"

# Wait for guardian to recover the orphan
RECOVERED=false
for i in $(seq 1 15); do
  DEP_STATE=$(docker inspect -f '{{.State.Status}}' dg-test-wt-dependent 2>/dev/null || echo "unknown")
  if [ "$DEP_STATE" = "running" ]; then
    echo "PASS: Dependent recovered with cooldown disabled (took ${i}s)"
    PASS=$((PASS + 1))
    RECOVERED=true
    break
  fi
  sleep 1
done

if [ "$RECOVERED" = false ]; then
  echo "FAIL: Dependent not recovered with cooldown disabled (state=$DEP_STATE)"
  FAIL=$((FAIL + 1))
  echo "Guardian logs:"
  docker logs dg-test-guardian-wt 2>&1 | tail -20
fi

echo ""
echo "=== Watchtower Awareness Test Results: ${PASS} passed, ${FAIL} failed ==="
exit "$FAIL"
