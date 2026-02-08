#!/usr/bin/env bash
# Test: Circuit breaker stops restarting after budget is exhausted
# Uses a container that always fails its healthcheck. After the restart budget
# is hit, Guardian should stop restarting and log a CRITICAL circuit-open message.

set -euo pipefail

PASS=0
FAIL=0
GUARDIAN_IMAGE="${GUARDIAN_IMAGE:-docker-guardian}"

cleanup() {
  echo "Cleaning up..."
  docker rm -f dg-test-cb-target dg-test-guardian-cb 2>/dev/null || true
}
trap cleanup EXIT

echo "=== Test: Circuit Breaker ==="

cleanup

# Start a container that always fails its healthcheck
docker run -d \
  --name dg-test-cb-target \
  --health-cmd="exit 1" \
  --health-interval=2s \
  --health-retries=1 \
  --health-start-period=0s \
  alpine:3.20 sh -c 'sleep 3600'

# Wait for unhealthy
echo "Waiting for container to become unhealthy..."
for i in $(seq 1 15); do
  HC_STATE=$(docker inspect -f '{{.State.Health.Status}}' dg-test-cb-target 2>/dev/null || echo "unknown")
  if [ "$HC_STATE" = "unhealthy" ]; then
    echo "Container is unhealthy after ${i}s"
    break
  fi
  sleep 1
done

if [ "$HC_STATE" != "unhealthy" ]; then
  echo "FAIL: Container did not become unhealthy within 15s"
  FAIL=$((FAIL + 1))
  echo ""
  echo "=== Circuit Breaker Test Results: ${PASS} passed, ${FAIL} failed ==="
  exit "$FAIL"
fi

echo "PASS: Container reached unhealthy state"
PASS=$((PASS + 1))

# Start Guardian with a very low restart budget (2) and short window/backoff
# Budget=2 means after 2 restarts the circuit opens
docker run -d \
  --name dg-test-guardian-cb \
  -e AUTOHEAL_CONTAINER_LABEL=all \
  -e AUTOHEAL_INTERVAL=3 \
  -e AUTOHEAL_GRACE_PERIOD=0 \
  -e AUTOHEAL_WATCHTOWER_COOLDOWN=0 \
  -e AUTOHEAL_RESTART_BUDGET=2 \
  -e AUTOHEAL_RESTART_WINDOW=600 \
  -e AUTOHEAL_BACKOFF_MULTIPLIER=1 \
  -e AUTOHEAL_BACKOFF_MAX=1 \
  -e AUTOHEAL_BACKOFF_RESET_AFTER=600 \
  -e NOTIFY_EVENTS=actions \
  -v /var/run/docker.sock:/var/run/docker.sock \
  "$GUARDIAN_IMAGE"

# ── Test 1: Guardian restarts the container at least once ──────────

echo ""
echo "--- Test 1: Guardian restarts container at least once ---"

STARTED_AT_BEFORE=$(docker inspect -f '{{.State.StartedAt}}' dg-test-cb-target 2>/dev/null)

RESTARTED=false
for i in $(seq 1 30); do
  STARTED_NOW=$(docker inspect -f '{{.State.StartedAt}}' dg-test-cb-target 2>/dev/null || echo "unknown")
  if [ "$STARTED_NOW" != "$STARTED_AT_BEFORE" ] && [ "$STARTED_NOW" != "unknown" ]; then
    echo "PASS: Container was restarted at least once (took ${i}s)"
    PASS=$((PASS + 1))
    RESTARTED=true
    break
  fi
  sleep 1
done

if [ "$RESTARTED" = false ]; then
  echo "FAIL: Container was not restarted within 30s"
  FAIL=$((FAIL + 1))
  docker logs dg-test-guardian-cb 2>&1 | tail -30
  echo ""
  echo "=== Circuit Breaker Test Results: ${PASS} passed, ${FAIL} failed ==="
  exit "$FAIL"
fi

# ── Test 2: After budget exhausted, Guardian logs circuit open ─────

echo ""
echo "--- Test 2: Circuit breaker opens after budget exhausted ---"

# Wait long enough for Guardian to hit the budget (2 restarts) and see the circuit message
# With backoff_max=1 and budget=2, this should happen within ~30s
echo "Waiting up to 60s for circuit breaker to open..."
CIRCUIT_OPEN=false
for i in $(seq 1 60); do
  if docker logs dg-test-guardian-cb 2>&1 | grep -q "circuit open\|budget exhausted"; then
    echo "PASS: Guardian logged circuit breaker open (took ${i}s)"
    PASS=$((PASS + 1))
    CIRCUIT_OPEN=true
    break
  fi
  sleep 1
done

if [ "$CIRCUIT_OPEN" = false ]; then
  echo "FAIL: Guardian did not log circuit breaker open within 60s"
  FAIL=$((FAIL + 1))
  echo "Guardian logs:"
  docker logs dg-test-guardian-cb 2>&1 | tail -30
fi

# ── Test 3: Verify CRITICAL notification was logged ────────────────

echo ""
echo "--- Test 3: CRITICAL notification in logs ---"

if docker logs dg-test-guardian-cb 2>&1 | grep -q "\[CRITICAL\]"; then
  echo "PASS: CRITICAL notification logged for circuit breaker"
  PASS=$((PASS + 1))
else
  echo "FAIL: No CRITICAL notification found in logs"
  FAIL=$((FAIL + 1))
  docker logs dg-test-guardian-cb 2>&1 | tail -20
fi

# ── Test 4: Guardian itself is still running ───────────────────────

echo ""
echo "--- Test 4: Guardian is still healthy after circuit opens ---"

GUARDIAN_STATE=$(docker inspect -f '{{.State.Status}}' dg-test-guardian-cb 2>/dev/null || echo "unknown")
if [ "$GUARDIAN_STATE" = "running" ]; then
  echo "PASS: Guardian still running after circuit breaker activation"
  PASS=$((PASS + 1))
else
  echo "FAIL: Guardian stopped unexpectedly (state=$GUARDIAN_STATE)"
  FAIL=$((FAIL + 1))
fi

echo ""
echo "=== Circuit Breaker Test Results: ${PASS} passed, ${FAIL} failed ==="
exit "$FAIL"
