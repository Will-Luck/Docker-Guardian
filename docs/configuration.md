# Configuration

All configuration via environment variables, matching the upstream autoheal pattern.

## Container Labels

Control per-container behaviour:

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

# Suppress notifications for this container (still performs action)
docker run --label autoheal.notify=false ...

# Custom stop timeout per container
docker run --label autoheal.stop.timeout=30 ...
```

## Core Settings

| Variable | Default | Description |
|---|---|---|
| `AUTOHEAL_CONTAINER_LABEL` | `autoheal` | Label to filter monitored containers (`all` for all) |
| `AUTOHEAL_INTERVAL` | `5` | Poll interval in seconds (fallback when event stream unavailable) |
| `AUTOHEAL_START_PERIOD` | `0` | Delay before first check |
| `AUTOHEAL_DEFAULT_STOP_TIMEOUT` | `10` | Default stop timeout for unhealthy restarts |
| `AUTOHEAL_ONLY_MONITOR_RUNNING` | `false` | Only monitor running containers for health |
| `AUTOHEAL_UNHEALTHY_THRESHOLD` | `1` | Consecutive unhealthy checks before action (`1` = immediate) |
| `DOCKER_SOCK` | `/var/run/docker.sock` | Docker socket path or `tcp://host:port` |
| `CURL_TIMEOUT` | `30` | API request timeout |
| `TZ` | _(empty)_ | Timezone (e.g. `Europe/London`) — requires tzdata in image |

## Circuit Breaker Settings

| Variable | Default | Description |
|---|---|---|
| `AUTOHEAL_BACKOFF_MULTIPLIER` | `2` | Multiplier for exponential backoff between restarts |
| `AUTOHEAL_BACKOFF_MAX` | `300` | Maximum backoff delay in seconds |
| `AUTOHEAL_BACKOFF_RESET_AFTER` | `600` | Seconds a container must stay healthy before backoff resets |
| `AUTOHEAL_RESTART_BUDGET` | `5` | Maximum restarts per rolling window (`0` = unlimited) |
| `AUTOHEAL_RESTART_WINDOW` | `300` | Rolling window for restart budget in seconds |

## Dependency & Orchestration Settings

| Variable | Default | Description |
|---|---|---|
| `AUTOHEAL_MONITOR_DEPENDENCIES` | `true` | Enable dependency orphan recovery |
| `AUTOHEAL_DEPENDENCY_START_DELAY` | `5` | Seconds to wait before starting orphaned dependent |
| `AUTOHEAL_BACKUP_LABEL` | `docker-volume-backup.stop-during-backup` | Label marking backup-managed containers |
| `AUTOHEAL_BACKUP_CONTAINER` | _(empty)_ | Backup container name (empty = auto-detect by image) |
| `AUTOHEAL_GRACE_PERIOD` | `300` | Skip containers stopped within this many seconds |
| `AUTOHEAL_WATCHTOWER_COOLDOWN` | `300` | Skip if orchestration activity detected within this window. `0` to disable |
| `AUTOHEAL_WATCHTOWER_SCOPE` | `all` | `all` = skip every container. `affected` = only skip containers with events |
| `AUTOHEAL_WATCHTOWER_EVENTS` | `orchestration` | `orchestration` = `destroy`+`create` only. `all` = all lifecycle events |

## Notification Settings

| Variable | Default | Description |
|---|---|---|
| `NOTIFY_EVENTS` | `actions` | Notification event filter (see [notifications](notifications.md)) |
| `NOTIFY_RATE_LIMIT` | `60` | Minimum seconds between notifications per container (`0` = unlimited) |
| `NOTIFY_HOSTNAME` | _(empty)_ | Hostname prepended as `[hostname]` to all notifications |
| `METRICS_PORT` | `0` | Prometheus metrics port (`0` = disabled) |
| `POST_RESTART_SCRIPT` | _(empty)_ | Script to run after container restart/start |

For notification service env vars (Gotify, Discord, Slack, etc.), see [notifications](notifications.md).
