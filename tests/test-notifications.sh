#!/usr/bin/env bash
# Test: Notification services and event filtering
# Tests startup logging, event resolution, debug dispatch output, and graceful failure
# Does NOT test actual delivery (no real notification endpoints in CI)

set -euo pipefail

PASS=0
FAIL=0
GUARDIAN_IMAGE="${GUARDIAN_IMAGE:-docker-guardian}"

cleanup() {
  echo "Cleaning up..."
  docker rm -f dg-test-notify-guardian dg-test-notify-parent dg-test-notify-dep 2>/dev/null || true
}
trap cleanup EXIT

echo "=== Test: Notification Services ==="

cleanup

# ── Test 1: Startup logging shows configured services ────────────────

echo ""
echo "--- Test 1: Startup logging shows configured Gotify service ---"

docker run -d \
  --name dg-test-notify-guardian \
  -e AUTOHEAL_CONTAINER_LABEL=all \
  -e AUTOHEAL_INTERVAL=3 \
  -e AUTOHEAL_GRACE_PERIOD=0 \
  -e AUTOHEAL_WATCHTOWER_COOLDOWN=0 \
  -e NOTIFY_GOTIFY_URL=http://fake-gotify:8080 \
  -e NOTIFY_GOTIFY_TOKEN=fake-token \
  -e NOTIFY_EVENTS=actions \
  -v /var/run/docker.sock:/var/run/docker.sock \
  "$GUARDIAN_IMAGE"

sleep 3

if docker logs dg-test-notify-guardian 2>&1 | grep -q "NOTIFICATIONS=.*gotify"; then
  echo "PASS: Startup logs show gotify as configured notifier"
  PASS=$((PASS + 1))
else
  echo "FAIL: Startup logs do not show gotify"
  FAIL=$((FAIL + 1))
  docker logs dg-test-notify-guardian 2>&1 | head -20
fi

# ── Test 2: Event resolution logged correctly ────────────────────────

echo ""
echo "--- Test 2: NOTIFY_EVENTS resolution logged correctly ---"

if docker logs dg-test-notify-guardian 2>&1 | grep -q "NOTIFY_EVENTS=actions (resolved: actions)"; then
  echo "PASS: Event resolution logged correctly"
  PASS=$((PASS + 1))
else
  echo "FAIL: Event resolution not logged correctly"
  FAIL=$((FAIL + 1))
  docker logs dg-test-notify-guardian 2>&1 | grep "NOTIFY_EVENTS" || echo "(no NOTIFY_EVENTS line found)"
fi

# ── Test 3: No tokens leaked in logs ─────────────────────────────────

echo ""
echo "--- Test 3: No tokens leaked in startup logs ---"

if docker logs dg-test-notify-guardian 2>&1 | grep -q "fake-token"; then
  echo "FAIL: Token value leaked in logs"
  FAIL=$((FAIL + 1))
else
  echo "PASS: No tokens leaked in startup logs"
  PASS=$((PASS + 1))
fi

docker rm -f dg-test-notify-guardian 2>/dev/null || true

# ── Test 4: Debug mode shows dispatch lines ──────────────────────────

echo ""
echo "--- Test 4: Debug mode shows [notify] dispatch lines ---"

# Create parent + dependent for triggering an action notification
docker run -d \
  --name dg-test-notify-parent \
  alpine:3.20 sh -c 'sleep 3600'

sleep 2

docker run -d \
  --name dg-test-notify-dep \
  --network=container:dg-test-notify-parent \
  alpine:3.20 sh -c 'sleep 3600'

sleep 2

# Stop dependent to create orphan scenario
docker stop -t 1 dg-test-notify-dep

# Start Guardian with debug mode + fake gotify
docker run -d \
  --name dg-test-notify-guardian \
  -e AUTOHEAL_CONTAINER_LABEL=all \
  -e AUTOHEAL_INTERVAL=3 \
  -e AUTOHEAL_GRACE_PERIOD=0 \
  -e AUTOHEAL_WATCHTOWER_COOLDOWN=0 \
  -e AUTOHEAL_DEPENDENCY_START_DELAY=1 \
  -e NOTIFY_GOTIFY_URL=http://fake-gotify:8080 \
  -e NOTIFY_GOTIFY_TOKEN=fake-token \
  -e NOTIFY_EVENTS=debug \
  -v /var/run/docker.sock:/var/run/docker.sock \
  "$GUARDIAN_IMAGE"

# Wait for Guardian to detect and recover the orphan
RECOVERED=false
for i in $(seq 1 15); do
  DEP_STATE=$(docker inspect -f '{{.State.Status}}' dg-test-notify-dep 2>/dev/null || echo "unknown")
  if [ "$DEP_STATE" = "running" ]; then
    RECOVERED=true
    break
  fi
  sleep 1
done

if [ "$RECOVERED" = true ]; then
  # Check debug dispatch line appeared
  sleep 2  # Give logs time to flush
  if docker logs dg-test-notify-guardian 2>&1 | grep -q "\[notify\] .* gotify:"; then
    echo "PASS: Debug mode shows [notify] → gotify: dispatch line"
    PASS=$((PASS + 1))
  else
    echo "FAIL: Debug mode did not show [notify] dispatch line"
    FAIL=$((FAIL + 1))
    docker logs dg-test-notify-guardian 2>&1 | tail -20
  fi
else
  echo "FAIL: Orphan not recovered (cannot test debug dispatch)"
  FAIL=$((FAIL + 1))
  docker logs dg-test-notify-guardian 2>&1 | tail -20
fi

# ── Test 5: Startup notification in debug mode ───────────────────────

echo ""
echo "--- Test 5: Startup notification dispatched in debug mode ---"

if docker logs dg-test-notify-guardian 2>&1 | grep -q "\[notify\] .* gotify: Docker-Guardian started"; then
  echo "PASS: Startup notification dispatched via gotify in debug mode"
  PASS=$((PASS + 1))
else
  echo "FAIL: Startup notification not dispatched"
  FAIL=$((FAIL + 1))
  docker logs dg-test-notify-guardian 2>&1 | grep "notify" || echo "(no notify lines found)"
fi

# ── Test 6: Guardian survives unreachable notification URLs ──────────

echo ""
echo "--- Test 6: Guardian survives unreachable notification URLs ---"

# Guardian should still be running despite fake-gotify being unreachable
GUARDIAN_STATE=$(docker inspect -f '{{.State.Status}}' dg-test-notify-guardian 2>/dev/null || echo "unknown")
if [ "$GUARDIAN_STATE" = "running" ]; then
  echo "PASS: Guardian still running with unreachable notification URL"
  PASS=$((PASS + 1))
else
  echo "FAIL: Guardian crashed with unreachable notification URL (state=$GUARDIAN_STATE)"
  FAIL=$((FAIL + 1))
fi

echo ""
echo "=== Notification Test Results: ${PASS} passed, ${FAIL} failed ==="
exit "$FAIL"
