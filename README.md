# Docker-Guardian

Dependency-aware container recovery for Docker. Forked from [willfarrell/docker-autoheal](https://github.com/willfarrell/docker-autoheal) (MIT).

Adds to the battle-tested autoheal base:

1. **Circuit Breaker** — exponential backoff and restart budgets prevent restart storms; per-container action labels
2. **Event-Driven Detection** — reacts to Docker events in real-time instead of polling
3. **Prometheus Metrics** — `/metrics` endpoint for observability
4. **Dependency Monitoring** — auto-detects and recovers containers orphaned when their network parent restarts
5. **Watchtower Awareness** — detects active orchestration via Docker events and pauses monitoring during cooldown
6. **Backup Awareness** — skips containers managed by backup tools during active backups
7. **Grace Period** — avoids interfering with manual maintenance windows
8. **Multi-Service Notifications** — 9 native notification services with rate limiting and retry

All original autoheal functionality (unhealthy container restarts, webhooks, Apprise notifications) is preserved.

## Quick Start

```bash
docker run -d \
  --name docker-guardian \
  --restart=always \
  -e AUTOHEAL_CONTAINER_LABEL=all \
  -v /var/run/docker.sock:/var/run/docker.sock \
  ghcr.io/will-luck/docker-guardian
```

With Prometheus metrics and Gotify notifications:

```bash
docker run -d \
  --name docker-guardian \
  --restart=always \
  -e AUTOHEAL_CONTAINER_LABEL=all \
  -e METRICS_PORT=9090 \
  -e NOTIFY_GOTIFY_URL=http://gotify:8080 \
  -e NOTIFY_GOTIFY_TOKEN=Axxxxxxxxx \
  -p 9090:9090 \
  -v /var/run/docker.sock:/var/run/docker.sock \
  ghcr.io/will-luck/docker-guardian
```

## The Problem

Containers sharing a network namespace (`--network=container:X`) all die with exit code 128 when the parent container stops. Docker's restart policy brings back the parent, but **dependents remain dead** — even with `restart=unless-stopped`.

This is common with VPN or gateway containers where multiple services route through one tunnel:

```
vpn-gateway (parent)
  ├── app-1  (--network=container:vpn-gateway)
  ├── app-2  (--network=container:vpn-gateway)
  └── app-3  (--network=container:vpn-gateway)
```

When the gateway container restarts (Watchtower update, crash, etc.), all dependents exit 128 and stay down. Docker-Guardian detects this and restarts them automatically.

## Features

### Circuit Breaker & Restart Policy

Prevents restart storms when a container is fundamentally broken:

- **Exponential backoff** — delays between restarts increase: 10s → 20s → 40s → ... up to a configurable max
- **Restart budget** — maximum restarts per rolling time window (default: 5 per 300s)
- **Circuit open** — when budget exhausted, Guardian stops restarting and sends a CRITICAL notification
- **Auto-reset** — backoff resets after a container stays healthy for a configurable duration

### Per-Container Action Labels

Control what happens when a container is detected as unhealthy:

```bash
# Default: restart the container
docker run --label autoheal.action=restart ...

# Stop the container (quarantine) instead of restarting
docker run --label autoheal.action=stop ...

# Send notification only, don't touch the container
docker run --label autoheal.action=notify ...

# Completely ignore this container
docker run --label autoheal.action=none ...

# Opt out (alternative to action=none)
docker run --label autoheal=False ...

# Custom stop timeout per container
docker run --label autoheal.stop.timeout=30 ...
```

### Event-Driven Detection

Docker-Guardian subscribes to the Docker event stream for real-time detection:

- Reacts to `health_status: unhealthy` events within seconds (no polling delay)
- Detects container `die` events for instant orphan dependency recovery
- Tracks `create`/`destroy` events for orchestration awareness
- Resets backoff when `health_status: healthy` is received
- Auto-reconnects with exponential backoff if the event stream drops
- Falls back to polling if event stream is unavailable

### Prometheus Metrics

Enable with `METRICS_PORT`:

```bash
-e METRICS_PORT=9090 -p 9090:9090
```

Exposed metrics:

| Metric | Type | Labels | Description |
|---|---|---|---|
| `docker_guardian_restarts_total` | Counter | container, result | Restart attempts (success/failure) |
| `docker_guardian_skips_total` | Counter | container, reason | Skipped containers (orchestration/grace/backup/circuit/backoff) |
| `docker_guardian_notifications_total` | Counter | service, result | Notification delivery (success/failure per service) |
| `docker_guardian_events_processed_total` | Counter | action | Docker events processed by type |
| `docker_guardian_unhealthy_containers` | Gauge | — | Current unhealthy container count |
| `docker_guardian_circuit_open_containers` | Gauge | — | Containers with circuit breaker open |
| `docker_guardian_event_stream_connected` | Gauge | — | Event stream connection status (1/0) |
| `docker_guardian_restart_duration_seconds` | Histogram | container | Time taken for restart operations |
| `docker_guardian_event_processing_duration_seconds` | Histogram | — | Time taken to process each event |

### Dependency Monitoring

Auto-detects network dependencies via Docker API — **no labels needed**. On each event or poll cycle:

1. Queries exited containers
2. Filters to those using `--network=container:X` network mode
3. Checks if exit code is 128 (killed by parent exit)
4. Verifies parent is running
5. Waits configurable delay (parent initialisation time)
6. Starts the orphaned dependent

Multi-level dependencies (A→B→C) resolve naturally over multiple cycles.

### Watchtower Awareness

Detects active orchestration (Watchtower, manual `docker-compose up`, etc.) via Docker events:

- Watches for container `destroy` and `create` events within a configurable cooldown window (default 300s)
- When events are found, pauses all monitoring until the cooldown expires
- Configurable scope: skip all containers (default) or only affected ones
- Configurable events: orchestration only (default, avoids self-triggering) or all lifecycle events

Set `AUTOHEAL_WATCHTOWER_COOLDOWN=0` to disable.

### Backup Awareness

Prevents Docker-Guardian from interfering with backup tools like [docker-volume-backup](https://github.com/offen/docker-volume-backup):

- Auto-detects running backup containers by image name
- Skips containers labelled with `docker-volume-backup.stop-during-backup` while backup is active

### Grace Period

Skips recently-stopped containers to avoid fighting with:

- **Manual stops** for maintenance
- **Other orchestration tools** not covered by Watchtower awareness

Default: 300 seconds. Set to `0` to disable.

## Notifications

Docker-Guardian supports 9 notification services natively — just set the env vars for your service(s). Multiple services can be active simultaneously. Action notifications (restarts, failures) retry up to 3 times with exponential backoff. Rate limiting prevents notification floods (default: 1 per container per 60 seconds).

### Notification Services

| Service | Env Vars | Notes |
|---|---|---|
| **Gotify** | `NOTIFY_GOTIFY_URL`, `NOTIFY_GOTIFY_TOKEN` | POST to `{url}/message?token={token}` |
| **Discord** | `NOTIFY_DISCORD_WEBHOOK` | Full webhook URL from Discord settings |
| **Slack** | `NOTIFY_SLACK_WEBHOOK` | Full webhook URL from Slack app config |
| **Telegram** | `NOTIFY_TELEGRAM_TOKEN`, `NOTIFY_TELEGRAM_CHAT_ID` | Bot token from @BotFather |
| **Pushover** | `NOTIFY_PUSHOVER_TOKEN`, `NOTIFY_PUSHOVER_USER` | App token + user key |
| **Pushbullet** | `NOTIFY_PUSHBULLET_TOKEN` | Access token from account settings |
| **LunaSea** | `NOTIFY_LUNASEA_WEBHOOK` | Custom webhook URL |
| **Email** | `NOTIFY_EMAIL_SMTP`, `NOTIFY_EMAIL_FROM`, `NOTIFY_EMAIL_TO`, `NOTIFY_EMAIL_USER`, `NOTIFY_EMAIL_PASS` | SMTP. Format: `host:port` |
| **Webhook** | `WEBHOOK_URL`, `WEBHOOK_JSON_KEY` | Generic webhook (legacy) |

`APPRISE_URL` also still works for Apprise users.

### Notification Events (`NOTIFY_EVENTS`)

Controls which events trigger notifications. Accepts keywords or numbers, comma-separated. Default: `actions`.

| # | Keyword | Events | Default |
|---|---|---|---|
| 1 | `startup` | Guardian boot confirmation (test notification) | No |
| 2 | `actions` | Restart success/failure + orphan start success/failure + circuit breaker | **Yes** |
| 3 | `failures` | Only failure events (restart failed, start failed) | No |
| 4 | `skips` | Orchestration skip, backup skip, grace period skip | No |
| 5 | `debug` | All of the above + logs every notification dispatch to console | No |

**Examples:**

```bash
-e NOTIFY_EVENTS=actions            # default — success + failure
-e NOTIFY_EVENTS=actions,startup    # actions + boot test
-e NOTIFY_EVENTS=2,1               # same as above, numbered
-e NOTIFY_EVENTS=failures           # only failures
-e NOTIFY_EVENTS=all                # everything except debug (1-4)
-e NOTIFY_EVENTS=debug              # everything + console logging (5)
```

`failures` (3) is a subset of `actions` (2). If both are set, `actions` takes precedence.

## Configuration

All configuration via environment variables, matching the upstream autoheal pattern:

### Circuit Breaker Settings

| Variable | Default | Description |
|---|---|---|
| `AUTOHEAL_BACKOFF_MULTIPLIER` | `2` | Multiplier for exponential backoff between restarts |
| `AUTOHEAL_BACKOFF_MAX` | `300` | Maximum backoff delay in seconds |
| `AUTOHEAL_BACKOFF_RESET_AFTER` | `600` | Seconds a container must stay healthy before backoff resets |
| `AUTOHEAL_RESTART_BUDGET` | `5` | Maximum restarts per rolling window (`0` = unlimited) |
| `AUTOHEAL_RESTART_WINDOW` | `300` | Rolling window for restart budget in seconds |

### Docker-Guardian Settings

| Variable | Default | Description |
|---|---|---|
| `AUTOHEAL_MONITOR_DEPENDENCIES` | `true` | Enable dependency orphan recovery |
| `AUTOHEAL_DEPENDENCY_START_DELAY` | `5` | Seconds to wait before starting orphaned dependent |
| `AUTOHEAL_BACKUP_LABEL` | `docker-volume-backup.stop-during-backup` | Label marking backup-managed containers |
| `AUTOHEAL_BACKUP_CONTAINER` | _(empty)_ | Backup container name (empty = auto-detect by image) |
| `AUTOHEAL_GRACE_PERIOD` | `300` | Skip containers stopped within this many seconds |
| `AUTOHEAL_WATCHTOWER_COOLDOWN` | `300` | Skip containers if orchestration activity detected within this many seconds. `0` to disable |
| `AUTOHEAL_WATCHTOWER_SCOPE` | `all` | `all` = skip every container when orchestration detected. `affected` = only skip containers with events in the window |
| `AUTOHEAL_WATCHTOWER_EVENTS` | `orchestration` | `orchestration` = only `destroy`+`create` events (Watchtower signature). `all` = all container lifecycle events |

### Metrics & Notification Settings

| Variable | Default | Description |
|---|---|---|
| `METRICS_PORT` | `0` | Prometheus metrics port (`0` = disabled) |
| `NOTIFY_EVENTS` | `actions` | Notification event filter (see Notification Events above) |
| `NOTIFY_RATE_LIMIT` | `60` | Minimum seconds between notifications per container (`0` = unlimited) |

### Original Autoheal Settings (unchanged)

| Variable | Default | Description |
|---|---|---|
| `AUTOHEAL_CONTAINER_LABEL` | `autoheal` | Label to filter monitored containers (`all` for all) |
| `AUTOHEAL_INTERVAL` | `5` | Poll interval in seconds (fallback when event stream unavailable) |
| `AUTOHEAL_START_PERIOD` | `0` | Delay before first check |
| `AUTOHEAL_DEFAULT_STOP_TIMEOUT` | `10` | Default stop timeout for unhealthy restarts |
| `AUTOHEAL_ONLY_MONITOR_RUNNING` | `false` | Only monitor running containers for health |
| `DOCKER_SOCK` | `/var/run/docker.sock` | Docker socket path or `tcp://host:port` |
| `CURL_TIMEOUT` | `30` | API request timeout |
| `WEBHOOK_URL` | _(empty)_ | Generic webhook URL for notifications |
| `WEBHOOK_JSON_KEY` | `content` | JSON key for webhook payload |
| `APPRISE_URL` | _(empty)_ | Apprise notification URL |
| `POST_RESTART_SCRIPT` | _(empty)_ | Script to run after container restart/start |

## Decision Flowchart

```
Container event received
├── health_status: unhealthy
│   ├── autoheal=False or action=none? → IGNORE
│   ├── Orchestration active (Watchtower)? → SKIP
│   ├── Within grace period? → SKIP
│   ├── Backup-managed + backup running? → SKIP
│   ├── State = restarting? → SKIP
│   ├── action=notify? → NOTIFY ONLY
│   ├── Circuit breaker open (budget exhausted)? → NOTIFY [CRITICAL]
│   ├── Backoff active? → SKIP (wait for backoff)
│   ├── action=stop? → Stop container (quarantine)
│   └── Restart container ✓
│
├── health_status: healthy
│   └── Reset backoff for container
│
├── die (exit code 128, NetworkMode=container:X)
│   ├── Parent not running? → SKIP
│   ├── Orchestration active? → SKIP
│   ├── Within grace period? → SKIP
│   ├── Backup-managed + backup running? → SKIP
│   ├── Wait start delay...
│   ├── Parent still running? → Start container ✓
│   └── Parent stopped? → SKIP
│
└── create/destroy
    └── Record orchestration activity
```

## Testing

Build and run the full test suite (9 suites):

```bash
docker build -t docker-guardian .
GUARDIAN_IMAGE=docker-guardian bash tests/test-all.sh
```

Individual tests:

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

Unit tests:

```bash
go test -race -count=1 ./...
```

## Building

```bash
# Single architecture
docker build -t docker-guardian .

# Multi-arch
docker buildx build --platform linux/amd64,linux/arm64 -t docker-guardian .
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
| Alpine version | 3.18 | 3.20 |

## Licence

MIT — see [LICENSE](LICENSE). Original work by [Will Farrell](https://github.com/willfarrell).
