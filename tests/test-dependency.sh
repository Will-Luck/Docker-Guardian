#!/usr/bin/env bash
# Test: Dependency orphan recovery
# Creates a parent + dependent (network=container:parent), stops the dependent
# (simulating it being killed when parent restarted), then verifies Docker-Guardian
# detects the orphaned dependent and starts it once parent is confirmed running.

set -euo pipefail

PASS=0
FAIL=0
GUARDIAN_IMAGE="${GUARDIAN_IMAGE:-docker-guardian}"

cleanup() {
  echo "Cleaning up..."
  docker rm -f dg-test-parent dg-test-dependent dg-test-guardian 2>/dev/null || true
}
trap cleanup EXIT

echo "=== Test: Dependency Orphan Recovery ==="

cleanup

# Start parent container
echo "Starting parent container..."
docker run -d \
  --name dg-test-parent \
  alpine:3.20 sh -c 'echo "parent running"; sleep 3600'

sleep 2

# Start dependent using parent's network namespace
echo "Starting dependent container (network=container:parent)..."
docker run -d \
  --name dg-test-dependent \
  --network=container:dg-test-parent \
  alpine:3.20 sh -c 'echo "dependent running"; sleep 3600'

sleep 2

# Verify both running
if [ "$(docker inspect -f '{{.State.Status}}' dg-test-parent)" = "running" ] && \
   [ "$(docker inspect -f '{{.State.Status}}' dg-test-dependent)" = "running" ]; then
  echo "PASS: Both containers running"
  PASS=$((PASS + 1))
else
  echo "FAIL: Containers not running before test"
  FAIL=$((FAIL + 1))
fi

# Stop the dependent (simulates being killed during parent restart)
# Parent stays running — this is the "orphaned dependent" scenario
echo "Stopping dependent (simulating orphan scenario)..."
docker stop -t 1 dg-test-dependent

sleep 2

# Verify: parent running, dependent exited
if [ "$(docker inspect -f '{{.State.Status}}' dg-test-parent)" = "running" ] && \
   [ "$(docker inspect -f '{{.State.Status}}' dg-test-dependent)" = "exited" ]; then
  echo "PASS: Parent running, dependent exited (orphan scenario)"
  PASS=$((PASS + 1))
else
  echo "FAIL: Expected parent=running, dependent=exited"
  FAIL=$((FAIL + 1))
fi

# Start Docker-Guardian with short interval, no grace period
echo "Starting Docker-Guardian..."
docker run -d \
  --name dg-test-guardian \
  -e AUTOHEAL_CONTAINER_LABEL=all \
  -e AUTOHEAL_INTERVAL=3 \
  -e AUTOHEAL_GRACE_PERIOD=0 \
  -e AUTOHEAL_WATCHTOWER_COOLDOWN=0 \
  -e AUTOHEAL_DEPENDENCY_START_DELAY=2 \
  -v /var/run/docker.sock:/var/run/docker.sock \
  "$GUARDIAN_IMAGE"

# Wait for guardian to detect and start the orphan
echo "Waiting for guardian to detect orphan (up to 20s)..."
RECOVERED=false
for i in $(seq 1 20); do
  DEP_STATE=$(docker inspect -f '{{.State.Status}}' dg-test-dependent 2>/dev/null || echo "unknown")
  if [ "$DEP_STATE" = "running" ]; then
    echo "PASS: Dependent recovered by guardian (took ${i}s)"
    PASS=$((PASS + 1))
    RECOVERED=true
    break
  fi
  sleep 1
done

if [ "$RECOVERED" = false ]; then
  echo "FAIL: Dependent not recovered within 20s"
  FAIL=$((FAIL + 1))
  echo "Guardian logs:"
  docker logs dg-test-guardian 2>&1 | tail -20
fi

echo ""
echo "=== Dependency Test Results: ${PASS} passed, ${FAIL} failed ==="
exit "$FAIL"
