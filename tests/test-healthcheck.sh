#!/usr/bin/env bash
# Test: Unhealthy container restart (core autoheal feature)
# Starts a container with a failing healthcheck, verifies Docker-Guardian
# detects it as unhealthy and restarts it.

set -euo pipefail

PASS=0
FAIL=0
GUARDIAN_IMAGE="${GUARDIAN_IMAGE:-docker-guardian}"

cleanup() {
  echo "Cleaning up..."
  docker rm -f dg-test-hc-target dg-test-guardian-hc 2>/dev/null || true
}
trap cleanup EXIT

echo "=== Test: Unhealthy Container Restart ==="

cleanup

# Start a container with a healthcheck that always fails
echo "Starting container with failing healthcheck..."
docker run -d \
  --name dg-test-hc-target \
  --health-cmd="exit 1" \
  --health-interval=2s \
  --health-retries=1 \
  --health-start-period=0s \
  alpine:3.20 sh -c 'sleep 3600'

# Wait for container to become unhealthy
echo "Waiting for container to become unhealthy..."
UNHEALTHY=false
for i in $(seq 1 15); do
  HC_STATE=$(docker inspect -f '{{.State.Health.Status}}' dg-test-hc-target 2>/dev/null || echo "unknown")
  if [ "$HC_STATE" = "unhealthy" ]; then
    echo "Container is unhealthy after ${i}s"
    UNHEALTHY=true
    break
  fi
  sleep 1
done

if [ "$UNHEALTHY" = false ]; then
  echo "FAIL: Container did not become unhealthy within 15s (state=$HC_STATE)"
  FAIL=$((FAIL + 1))
  echo ""
  echo "=== Healthcheck Test Results: ${PASS} passed, ${FAIL} failed ==="
  exit "$FAIL"
fi

echo "PASS: Container reached unhealthy state"
PASS=$((PASS + 1))

# Record the container start time before Guardian acts
STARTED_AT_BEFORE=$(docker inspect -f '{{.State.StartedAt}}' dg-test-hc-target 2>/dev/null)

# Start Docker-Guardian with label=all, short interval
echo "Starting Docker-Guardian..."
docker run -d \
  --name dg-test-guardian-hc \
  -e AUTOHEAL_CONTAINER_LABEL=all \
  -e AUTOHEAL_INTERVAL=3 \
  -e AUTOHEAL_GRACE_PERIOD=0 \
  -e AUTOHEAL_WATCHTOWER_COOLDOWN=0 \
  -v /var/run/docker.sock:/var/run/docker.sock \
  "$GUARDIAN_IMAGE"

# Wait for Guardian to restart the unhealthy container
echo "Waiting for Guardian to restart the unhealthy container (up to 20s)..."
RESTARTED=false
for i in $(seq 1 20); do
  STARTED_AT_NOW=$(docker inspect -f '{{.State.StartedAt}}' dg-test-hc-target 2>/dev/null || echo "unknown")
  if [ "$STARTED_AT_NOW" != "$STARTED_AT_BEFORE" ] && [ "$STARTED_AT_NOW" != "unknown" ]; then
    echo "PASS: Container was restarted by Guardian (took ${i}s)"
    PASS=$((PASS + 1))
    RESTARTED=true
    break
  fi
  sleep 1
done

if [ "$RESTARTED" = false ]; then
  echo "FAIL: Container was not restarted within 20s"
  FAIL=$((FAIL + 1))
  echo "Guardian logs:"
  docker logs dg-test-guardian-hc 2>&1 | tail -20
fi

# Verify Guardian logged the restart
if docker logs dg-test-guardian-hc 2>&1 | grep -q "found to be unhealthy"; then
  echo "PASS: Guardian logs confirm unhealthy detection"
  PASS=$((PASS + 1))
else
  echo "FAIL: Guardian logs missing unhealthy detection message"
  FAIL=$((FAIL + 1))
  docker logs dg-test-guardian-hc 2>&1 | tail -20
fi

echo ""
echo "=== Healthcheck Test Results: ${PASS} passed, ${FAIL} failed ==="
exit "$FAIL"
