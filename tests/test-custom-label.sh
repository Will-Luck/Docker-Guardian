#!/usr/bin/env bash
# Test: Custom label filtering
# Verifies that AUTOHEAL_CONTAINER_LABEL restricts monitoring to only
# containers with that label, ignoring unhealthy containers without it.

set -euo pipefail

PASS=0
FAIL=0
GUARDIAN_IMAGE="${GUARDIAN_IMAGE:-docker-guardian}"

cleanup() {
  echo "Cleaning up..."
  docker rm -f dg-test-label-monitored dg-test-label-ignored dg-test-guardian-label 2>/dev/null || true
}
trap cleanup EXIT

echo "=== Test: Custom Label Filtering ==="

cleanup

# ── Start two unhealthy containers ─────────────────────────────────
# One with our custom label, one without

echo "Starting container WITH custom label (my-monitor=true)..."
docker run -d \
  --name dg-test-label-monitored \
  --label my-monitor=true \
  --health-cmd="exit 1" \
  --health-interval=2s \
  --health-retries=1 \
  --health-start-period=0s \
  alpine:3.20 sh -c 'sleep 3600'

echo "Starting container WITHOUT custom label..."
docker run -d \
  --name dg-test-label-ignored \
  --health-cmd="exit 1" \
  --health-interval=2s \
  --health-retries=1 \
  --health-start-period=0s \
  alpine:3.20 sh -c 'sleep 3600'

# Wait for both to become unhealthy
echo "Waiting for both containers to become unhealthy..."
for i in $(seq 1 15); do
  S1=$(docker inspect -f '{{.State.Health.Status}}' dg-test-label-monitored 2>/dev/null || echo "unknown")
  S2=$(docker inspect -f '{{.State.Health.Status}}' dg-test-label-ignored 2>/dev/null || echo "unknown")
  if [ "$S1" = "unhealthy" ] && [ "$S2" = "unhealthy" ]; then
    echo "Both containers unhealthy after ${i}s"
    break
  fi
  sleep 1
done

if [ "$S1" != "unhealthy" ] || [ "$S2" != "unhealthy" ]; then
  echo "FAIL: One or both containers did not become unhealthy within 15s"
  FAIL=$((FAIL + 1))
  echo ""
  echo "=== Custom Label Test Results: ${PASS} passed, ${FAIL} failed ==="
  exit "$FAIL"
fi

STARTED_MONITORED=$(docker inspect -f '{{.State.StartedAt}}' dg-test-label-monitored 2>/dev/null)
STARTED_IGNORED=$(docker inspect -f '{{.State.StartedAt}}' dg-test-label-ignored 2>/dev/null)

# Start Guardian with custom label filter
echo "Starting Guardian with AUTOHEAL_CONTAINER_LABEL=my-monitor..."
docker run -d \
  --name dg-test-guardian-label \
  -e AUTOHEAL_CONTAINER_LABEL=my-monitor \
  -e AUTOHEAL_INTERVAL=3 \
  -e AUTOHEAL_GRACE_PERIOD=0 \
  -e AUTOHEAL_WATCHTOWER_COOLDOWN=0 \
  -v /var/run/docker.sock:/var/run/docker.sock \
  "$GUARDIAN_IMAGE"

# ── Test 1: Labelled container IS restarted ────────────────────────

echo ""
echo "--- Test 1: Labelled container is restarted ---"

RESTARTED=false
for i in $(seq 1 20); do
  STARTED_NOW=$(docker inspect -f '{{.State.StartedAt}}' dg-test-label-monitored 2>/dev/null || echo "unknown")
  if [ "$STARTED_NOW" != "$STARTED_MONITORED" ] && [ "$STARTED_NOW" != "unknown" ]; then
    echo "PASS: Labelled container was restarted (took ${i}s)"
    PASS=$((PASS + 1))
    RESTARTED=true
    break
  fi
  sleep 1
done

if [ "$RESTARTED" = false ]; then
  echo "FAIL: Labelled container was not restarted within 20s"
  FAIL=$((FAIL + 1))
  docker logs dg-test-guardian-label 2>&1 | tail -20
fi

# ── Test 2: Unlabelled container is NOT restarted ──────────────────

echo ""
echo "--- Test 2: Unlabelled container is ignored ---"

# Give it some extra time to be sure
sleep 10

STARTED_IGN_NOW=$(docker inspect -f '{{.State.StartedAt}}' dg-test-label-ignored 2>/dev/null)
if [ "$STARTED_IGN_NOW" = "$STARTED_IGNORED" ]; then
  echo "PASS: Unlabelled container was NOT restarted"
  PASS=$((PASS + 1))
else
  echo "FAIL: Unlabelled container was restarted despite missing label"
  FAIL=$((FAIL + 1))
  docker logs dg-test-guardian-label 2>&1 | tail -20
fi

# ── Test 3: Guardian logs confirm label filter ─────────────────────

echo ""
echo "--- Test 3: Guardian logs show label filter ---"

# Capture logs to variable to avoid pipefail+SIGPIPE: grep -q exits early on
# match, but docker logs may still be writing → SIGPIPE → exit 141 → pipefail
# reports failure even though grep found the match.
LABEL_LOGS=$(docker logs dg-test-guardian-label 2>&1 || true)
if grep -q "AUTOHEAL_CONTAINER_LABEL=my-monitor" <<< "$LABEL_LOGS"; then
  echo "PASS: Guardian logs confirm custom label filter"
  PASS=$((PASS + 1))
else
  echo "FAIL: Guardian logs do not show custom label"
  FAIL=$((FAIL + 1))
  head -10 <<< "$LABEL_LOGS"
fi

echo ""
echo "=== Custom Label Test Results: ${PASS} passed, ${FAIL} failed ==="
exit "$FAIL"
