#!/usr/bin/env bash
# Test: Opt-out via autoheal=False label
# Verifies that containers with autoheal=False are NOT restarted by Guardian,
# even when unhealthy.

set -euo pipefail

PASS=0
FAIL=0
GUARDIAN_IMAGE="${GUARDIAN_IMAGE:-docker-guardian}"

cleanup() {
  echo "Cleaning up..."
  docker rm -f dg-test-optout-target dg-test-optout-monitored dg-test-guardian-optout 2>/dev/null || true
}
trap cleanup EXIT

echo "=== Test: Opt-Out via autoheal=False ==="

cleanup

# ── Test 1: Container with autoheal=False is NOT restarted ─────────

echo ""
echo "--- Test 1: Container with autoheal=False is skipped ---"

# Start an unhealthy container with autoheal=False
docker run -d \
  --name dg-test-optout-target \
  --label autoheal=False \
  --health-cmd="exit 1" \
  --health-interval=2s \
  --health-retries=1 \
  --health-start-period=0s \
  alpine:3.20 sh -c 'sleep 3600'

# Wait for it to become unhealthy
echo "Waiting for opt-out container to become unhealthy..."
for i in $(seq 1 15); do
  HC_STATE=$(docker inspect -f '{{.State.Health.Status}}' dg-test-optout-target 2>/dev/null || echo "unknown")
  if [ "$HC_STATE" = "unhealthy" ]; then
    break
  fi
  sleep 1
done

if [ "$HC_STATE" != "unhealthy" ]; then
  echo "FAIL: Container did not become unhealthy within 15s"
  FAIL=$((FAIL + 1))
  echo ""
  echo "=== Opt-Out Test Results: ${PASS} passed, ${FAIL} failed ==="
  exit "$FAIL"
fi

STARTED_AT_BEFORE=$(docker inspect -f '{{.State.StartedAt}}' dg-test-optout-target 2>/dev/null)

# Start Guardian
docker run -d \
  --name dg-test-guardian-optout \
  -e AUTOHEAL_CONTAINER_LABEL=all \
  -e AUTOHEAL_INTERVAL=3 \
  -e AUTOHEAL_GRACE_PERIOD=0 \
  -e AUTOHEAL_WATCHTOWER_COOLDOWN=0 \
  -v /var/run/docker.sock:/var/run/docker.sock \
  "$GUARDIAN_IMAGE"

# Wait sufficient time — Guardian should NOT restart the opted-out container
echo "Waiting 15s to confirm Guardian does NOT restart the opted-out container..."
sleep 15

STARTED_AT_AFTER=$(docker inspect -f '{{.State.StartedAt}}' dg-test-optout-target 2>/dev/null)

if [ "$STARTED_AT_BEFORE" = "$STARTED_AT_AFTER" ]; then
  echo "PASS: Opted-out container was NOT restarted"
  PASS=$((PASS + 1))
else
  echo "FAIL: Opted-out container was restarted despite autoheal=False"
  FAIL=$((FAIL + 1))
  docker logs dg-test-guardian-optout 2>&1 | tail -20
fi

# ── Test 2: A monitored container IS still restarted ───────────────

echo ""
echo "--- Test 2: Monitored container (no opt-out) is still restarted ---"

docker run -d \
  --name dg-test-optout-monitored \
  --health-cmd="exit 1" \
  --health-interval=2s \
  --health-retries=1 \
  --health-start-period=0s \
  alpine:3.20 sh -c 'sleep 3600'

# Wait for unhealthy
for i in $(seq 1 15); do
  HC_STATE=$(docker inspect -f '{{.State.Health.Status}}' dg-test-optout-monitored 2>/dev/null || echo "unknown")
  if [ "$HC_STATE" = "unhealthy" ]; then
    break
  fi
  sleep 1
done

STARTED_AT_MON=$(docker inspect -f '{{.State.StartedAt}}' dg-test-optout-monitored 2>/dev/null)

echo "Waiting for Guardian to restart the monitored container (up to 20s)..."
RESTARTED=false
for i in $(seq 1 20); do
  STARTED_NOW=$(docker inspect -f '{{.State.StartedAt}}' dg-test-optout-monitored 2>/dev/null || echo "unknown")
  if [ "$STARTED_NOW" != "$STARTED_AT_MON" ] && [ "$STARTED_NOW" != "unknown" ]; then
    echo "PASS: Monitored container was restarted (took ${i}s)"
    PASS=$((PASS + 1))
    RESTARTED=true
    break
  fi
  sleep 1
done

if [ "$RESTARTED" = false ]; then
  echo "FAIL: Monitored container was NOT restarted within 20s"
  FAIL=$((FAIL + 1))
  docker logs dg-test-guardian-optout 2>&1 | tail -20
fi

echo ""
echo "=== Opt-Out Test Results: ${PASS} passed, ${FAIL} failed ==="
exit "$FAIL"
