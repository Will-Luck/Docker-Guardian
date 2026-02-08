#!/usr/bin/env bash
# Test: Grace period
# Creates an orphaned dependent scenario, verifies guardian skips it within
# the grace window, then waits for grace period to expire and verifies recovery.

set -euo pipefail

PASS=0
FAIL=0
GUARDIAN_IMAGE="${GUARDIAN_IMAGE:-docker-guardian}"
GRACE_PERIOD=15  # Short for testing

cleanup() {
  echo "Cleaning up..."
  docker rm -f dg-test-grace-parent dg-test-grace-dependent dg-test-guardian-grace 2>/dev/null || true
}
trap cleanup EXIT

echo "=== Test: Grace Period ==="

cleanup

# Create parent + dependent pair
echo "Starting parent container..."
docker run -d \
  --name dg-test-grace-parent \
  alpine:3.20 sh -c 'sleep 3600'

sleep 2

echo "Starting dependent container..."
docker run -d \
  --name dg-test-grace-dependent \
  --network=container:dg-test-grace-parent \
  alpine:3.20 sh -c 'sleep 3600'

sleep 2

# Start guardian with grace period BEFORE stopping the dependent
echo "Starting Docker-Guardian (grace_period=${GRACE_PERIOD}s)..."
docker run -d \
  --name dg-test-guardian-grace \
  -e AUTOHEAL_CONTAINER_LABEL=all \
  -e AUTOHEAL_INTERVAL=3 \
  -e AUTOHEAL_GRACE_PERIOD=$GRACE_PERIOD \
  -e AUTOHEAL_WATCHTOWER_COOLDOWN=0 \
  -e AUTOHEAL_DEPENDENCY_START_DELAY=1 \
  -v /var/run/docker.sock:/var/run/docker.sock \
  "$GUARDIAN_IMAGE"

sleep 2

# Stop the dependent (creates orphan scenario)
echo "Stopping dependent (creating orphan with timestamp for grace period)..."
docker stop -t 1 dg-test-grace-dependent

sleep 2

# Check within first 8 seconds — dependent should still be stopped (grace period active)
echo "Checking within grace period (waiting 6s)..."
sleep 6
DEP_STATE=$(docker inspect -f '{{.State.Status}}' dg-test-grace-dependent 2>/dev/null || echo "unknown")
if [ "$DEP_STATE" = "exited" ]; then
  echo "PASS: Dependent still stopped within grace period"
  PASS=$((PASS + 1))
else
  echo "FAIL: Dependent was restarted within grace period (state=$DEP_STATE)"
  FAIL=$((FAIL + 1))
fi

# Wait for grace period to fully expire (we've used ~8s, need $GRACE_PERIOD total)
REMAINING=$((GRACE_PERIOD - 8 + 5))
echo "Waiting ${REMAINING}s for grace period to expire + guardian to act..."
sleep "$REMAINING"

# Now check — dependent should have been started
RECOVERED=false
for i in $(seq 1 15); do
  DEP_STATE=$(docker inspect -f '{{.State.Status}}' dg-test-grace-dependent 2>/dev/null || echo "unknown")
  if [ "$DEP_STATE" = "running" ]; then
    echo "PASS: Dependent started after grace period expired"
    PASS=$((PASS + 1))
    RECOVERED=true
    break
  fi
  sleep 1
done

if [ "$RECOVERED" = false ]; then
  echo "FAIL: Dependent not started after grace period (state=$DEP_STATE)"
  FAIL=$((FAIL + 1))
  echo "Guardian logs:"
  docker logs dg-test-guardian-grace 2>&1 | tail -20
fi

echo ""
echo "=== Grace Period Test Results: ${PASS} passed, ${FAIL} failed ==="
exit "$FAIL"
