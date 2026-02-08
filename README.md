# Docker-Guardian

Dependency-aware container recovery for Docker. Forked from [willfarrell/docker-autoheal](https://github.com/willfarrell/docker-autoheal) (MIT).

Adds four features to the battle-tested autoheal base:

1. **Dependency Monitoring** — auto-detects and recovers containers orphaned when their network parent restarts
2. **Watchtower Awareness** — detects active orchestration (Watchtower updates, etc.) via Docker events and pauses monitoring during the cooldown window
3. **Backup Awareness** — skips containers managed by backup tools during active backups
4. **Grace Period** — avoids interfering with manual maintenance windows

All original autoheal functionality (unhealthy container restarts, webhooks, Apprise notifications) is preserved.

## Quick Start

```bash
docker run -d \
  --name docker-guardian \
  --restart=always \
  -e AUTOHEAL_CONTAINER_LABEL=all \
  -v /var/run/docker.sock:/var/run/docker.sock \
  ghcr.io/lucknet/docker-guardian
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

### Dependency Monitoring

Auto-detects network dependencies via Docker API — **no labels needed**. Every poll cycle:

1. Queries exited containers
2. Filters to those using `--network=container:X` network mode
3. Checks if exit code is 128 (killed by parent exit)
4. Verifies parent is running
5. Waits configurable delay (parent initialisation time)
6. Starts the orphaned dependent

Multi-level dependencies (A→B→C) resolve naturally over multiple poll cycles.

### Watchtower Awareness

Detects active orchestration (Watchtower, manual `docker-compose up`, etc.) by querying the Docker events API each poll cycle:

- Watches for container `destroy` and `create` events within a configurable cooldown window (default 300s)
- When events are found, pauses all monitoring until the cooldown expires
- Tracks the **actual** activity window — a 10-minute image pull produces events at start and end, so the cooldown tracks the real duration
- Cached per poll cycle (single API call)
- Configurable scope: skip all containers (default) or only affected ones
- Configurable events: orchestration only (default, avoids self-triggering) or all lifecycle events

Set `AUTOHEAL_WATCHTOWER_COOLDOWN=0` to disable.

### Backup Awareness

Prevents Docker-Guardian from interfering with backup tools like [docker-volume-backup](https://github.com/offen/docker-volume-backup):

- Auto-detects running backup containers by image name
- Skips containers labelled with `docker-volume-backup.stop-during-backup` while backup is active
- Cached per poll cycle (single API call)

### Grace Period

Skips recently-stopped containers to avoid fighting with:

- **Manual stops** for maintenance
- **Other orchestration tools** not covered by Watchtower awareness

Default: 300 seconds. Set to `0` to disable.

## Configuration

All configuration via environment variables, matching the upstream autoheal pattern:

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

### Original Autoheal Settings (unchanged)

| Variable | Default | Description |
|---|---|---|
| `AUTOHEAL_CONTAINER_LABEL` | `autoheal` | Label to filter monitored containers (`all` for all) |
| `AUTOHEAL_INTERVAL` | `5` | Poll interval in seconds |
| `AUTOHEAL_START_PERIOD` | `0` | Delay before first check |
| `AUTOHEAL_DEFAULT_STOP_TIMEOUT` | `10` | Default stop timeout for unhealthy restarts |
| `AUTOHEAL_ONLY_MONITOR_RUNNING` | `false` | Only monitor running containers for health |
| `DOCKER_SOCK` | `/var/run/docker.sock` | Docker socket path or `tcp://host:port` |
| `CURL_TIMEOUT` | `30` | API request timeout |
| `WEBHOOK_URL` | _(empty)_ | Webhook URL for notifications |
| `WEBHOOK_JSON_KEY` | `content` | JSON key for webhook payload |
| `APPRISE_URL` | _(empty)_ | Apprise notification URL |
| `POST_RESTART_SCRIPT` | _(empty)_ | Script to run after container restart/start |

## Decision Flowchart

```
Container exited?
├── Has healthcheck + unhealthy?
│   ├── Orchestration active (Watchtower)? → SKIP
│   ├── Within grace period? → SKIP
│   ├── Backup-managed + backup running? → SKIP
│   ├── State = restarting? → SKIP
│   └── Restart container ✓
│
└── NetworkMode = container:X?
    ├── Parent not running? → SKIP
    ├── Orchestration active (Watchtower)? → SKIP
    ├── Within grace period? → SKIP
    ├── Backup-managed + backup running? → SKIP
    ├── Wait start delay...
    ├── Parent still running? → Start container ✓
    └── Parent stopped? → SKIP
```

## Testing

Build and run the test suite:

```bash
docker build -t docker-guardian .
cd tests && ./test-all.sh
```

Individual tests:

```bash
./tests/test-dependency.sh   # Dependency orphan recovery
./tests/test-backup.sh       # Backup awareness
./tests/test-grace.sh        # Grace period behaviour
./tests/test-watchtower.sh   # Watchtower/orchestration awareness
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
| Dependency orphan recovery | No | Yes |
| Watchtower/orchestration awareness | No | Yes |
| Backup awareness | No | Yes |
| Grace period | No | Yes |
| Alpine version | 3.18 | 3.20 |
| Webhook notifications | Yes | Yes (extended to new features) |

## Licence

MIT — see [LICENSE](LICENSE). Original work by [Will Farrell](https://github.com/willfarrell).
