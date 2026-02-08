# Features

## Circuit Breaker & Restart Policy

Prevents restart storms when a container is fundamentally broken:

- **Exponential backoff** — delays between restarts increase: 10s → 20s → 40s → ... up to a configurable max
- **Restart budget** — maximum restarts per rolling time window (default: 5 per 300s)
- **Circuit open** — when budget exhausted, Guardian stops restarting and sends a CRITICAL notification
- **Auto-reset** — backoff resets after a container stays healthy for a configurable duration

## Event-Driven Detection

Docker-Guardian subscribes to the Docker event stream for real-time detection:

- Reacts to `health_status: unhealthy` events within seconds (no polling delay)
- Detects container `die` events for instant orphan dependency recovery
- Tracks `create`/`destroy` events for orchestration awareness
- Resets backoff when `health_status: healthy` is received
- Auto-reconnects with exponential backoff if the event stream drops
- Falls back to polling if event stream is unavailable

## Dependency Monitoring

Auto-detects network dependencies via Docker API — **no labels needed**. On each event or poll cycle:

1. Queries exited containers
2. Filters to those using `--network=container:X` network mode
3. Checks if exit code is 128 (killed by parent exit)
4. Verifies parent is running
5. Waits configurable delay (parent initialisation time)
6. Starts the orphaned dependent

Multi-level dependencies (A→B→C) resolve naturally over multiple cycles.

## Watchtower Awareness

Detects active orchestration (Watchtower, manual `docker-compose up`, etc.) via Docker events:

- Watches for container `destroy` and `create` events within a configurable cooldown window (default 300s)
- When events are found, pauses all monitoring until the cooldown expires
- Configurable scope: skip all containers (default) or only affected ones
- Configurable events: orchestration only (default, avoids self-triggering) or all lifecycle events

Set `AUTOHEAL_WATCHTOWER_COOLDOWN=0` to disable.

## Backup Awareness

Prevents Docker-Guardian from interfering with backup tools like [docker-volume-backup](https://github.com/offen/docker-volume-backup):

- Auto-detects running backup containers by image name
- Skips containers labelled with `docker-volume-backup.stop-during-backup` while backup is active

## Grace Period

Skips recently-stopped containers to avoid fighting with:

- **Manual stops** for maintenance
- **Other orchestration tools** not covered by Watchtower awareness

Default: 300 seconds. Set to `0` to disable.

## Prometheus Metrics

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

## Decision Flowchart

```
Container event received
├── health_status: unhealthy
│   ├── autoheal=False or action=none? → IGNORE
│   ├── State = paused? → SKIP
│   ├── State = restarting? → SKIP
│   ├── Below unhealthy threshold? → SKIP (count N/M)
│   ├── Orchestration active (Watchtower)? → SKIP
│   ├── Within grace period? → SKIP
│   ├── Backup-managed + backup running? → SKIP
│   ├── action=notify? → NOTIFY ONLY
│   ├── Circuit breaker open (budget exhausted)? → NOTIFY [CRITICAL]
│   ├── Backoff active? → SKIP (wait for backoff)
│   ├── action=stop? → Stop container (quarantine)
│   └── Restart container
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
│   ├── Parent still running? → Start container
│   └── Parent stopped? → SKIP
│
└── create/destroy
    └── Record orchestration activity
```
