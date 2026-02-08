# Development

## Building

```bash
# Single architecture
docker build -t docker-guardian .

# Multi-arch
docker buildx build --platform linux/amd64,linux/arm64 -t docker-guardian .
```

## Testing

### Unit tests

```bash
go test -race -count=1 ./...
```

### Acceptance tests

Build the image first, then run the full suite (9 test suites):

```bash
docker build -t docker-guardian .
GUARDIAN_IMAGE=docker-guardian bash tests/test-all.sh
```

Individual suites:

```bash
./tests/test-healthcheck.sh       # Unhealthy container restart
./tests/test-dependency.sh        # Dependency orphan recovery
./tests/test-backup.sh            # Backup awareness
./tests/test-grace.sh             # Grace period behaviour
./tests/test-watchtower.sh        # Watchtower/orchestration awareness
./tests/test-notifications.sh     # Notification services and event filtering
./tests/test-opt-out.sh           # autoheal=False opt-out
./tests/test-circuit-breaker.sh   # Circuit breaker budget exhaustion
./tests/test-custom-label.sh      # Custom label filtering
```

## Differences from upstream

| Feature | docker-autoheal | Docker-Guardian |
|---|---|---|
| Unhealthy container restarts | Yes | Yes |
| Circuit breaker / restart policy | No | Yes |
| Per-container action labels | No | Yes |
| Event-driven detection | No | Yes |
| Prometheus metrics | No | Yes |
| Dependency orphan recovery | No | Yes |
| Watchtower/orchestration awareness | No | Yes |
| Backup awareness | No | Yes |
| Grace period | No | Yes |
| Notification rate limiting & retry | No | Yes |
| Notification services | Webhook, Apprise | Webhook, Apprise + 8 native services |
| Healthcheck output in notifications | No | Yes |
| Per-container notification filtering | No | Yes |
| Unhealthy threshold | No | Yes |
| Custom hostname in notifications | No | Yes |
| Timezone support (TZ) | No | Yes |
| Skip paused containers | No | Yes |
| Alpine version | 3.18 | 3.20 |
