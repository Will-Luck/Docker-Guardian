# Docker-Guardian

Dependency-aware container recovery for Docker. Rewritten in Go from [willfarrell/docker-autoheal](https://github.com/willfarrell/docker-autoheal) (MIT).

Static Go binary on Alpine. Multi-arch (`amd64`/`arm64`). No runtime dependencies.

## Quick Start

```bash
docker run -d \
  --name docker-guardian \
  --restart=always \
  -e AUTOHEAL_CONTAINER_LABEL=all \
  -v /var/run/docker.sock:/var/run/docker.sock \
  ghcr.io/will-luck/docker-guardian
```

With notifications and metrics:

```bash
docker run -d \
  --name docker-guardian \
  --restart=always \
  -e AUTOHEAL_CONTAINER_LABEL=all \
  -e METRICS_PORT=9090 \
  -e NOTIFY_GOTIFY_URL=http://gotify:8080 \
  -e NOTIFY_GOTIFY_TOKEN=Axxxxxxxxx \
  -e NOTIFY_HOSTNAME=my-server \
  -e TZ=Europe/London \
  -p 9090:9090 \
  -v /var/run/docker.sock:/var/run/docker.sock \
  ghcr.io/will-luck/docker-guardian
```

## What it does

Docker-Guardian restarts unhealthy containers, same as docker-autoheal. On top of that:

- **Circuit breaker** — exponential backoff and restart budgets prevent restart storms
- **Dependency recovery** — auto-restarts containers orphaned when their `--network=container:X` parent dies (exit code 128)
- **Event-driven** — reacts to Docker events in real-time instead of polling
- **Orchestration awareness** — pauses during Watchtower updates and backup jobs
- **Notifications** — 9 native services (Gotify, Discord, Slack, Telegram, Pushover, Pushbullet, LunaSea, Email, Webhook) with rate limiting and retry
- **Prometheus metrics** — `/metrics` endpoint for observability
- **Per-container control** — action labels (`restart`, `stop`, `notify`, `none`), notification filtering, custom stop timeouts

All original autoheal functionality is preserved.

## The Problem

Containers sharing a network namespace (`--network=container:X`) all die with exit code 128 when the parent stops. Docker's restart policy brings back the parent, but dependents stay dead:

```
vpn-gateway (parent)
  ├── app-1  (--network=container:vpn-gateway)
  ├── app-2  (--network=container:vpn-gateway)
  └── app-3  (--network=container:vpn-gateway)
```

Docker-Guardian detects this and restarts them automatically.

## Key Configuration

| Variable | Default | Description |
|---|---|---|
| `AUTOHEAL_CONTAINER_LABEL` | `autoheal` | Label filter (`all` for all containers) |
| `AUTOHEAL_INTERVAL` | `5` | Poll interval in seconds |
| `AUTOHEAL_GRACE_PERIOD` | `300` | Skip recently-stopped containers (seconds) |
| `AUTOHEAL_WATCHTOWER_COOLDOWN` | `300` | Pause after orchestration activity (`0` to disable) |
| `AUTOHEAL_RESTART_BUDGET` | `5` | Max restarts per window before circuit opens |
| `AUTOHEAL_UNHEALTHY_THRESHOLD` | `1` | Consecutive unhealthy checks before action |
| `NOTIFY_HOSTNAME` | _(empty)_ | Prefix notifications with `[hostname]` |
| `METRICS_PORT` | `0` | Prometheus metrics port (`0` = disabled) |
| `TZ` | _(empty)_ | Timezone (e.g. `Europe/London`) |

Full reference: [docs/configuration.md](docs/configuration.md)

## Documentation

| Document | Contents |
|---|---|
| [Configuration](docs/configuration.md) | All env vars, container labels |
| [Features](docs/features.md) | Circuit breaker, dependencies, orchestration awareness, metrics, decision flowchart |
| [Notifications](docs/notifications.md) | 9 services, event filtering, hostname prefix, healthcheck output |
| [Development](docs/development.md) | Building, testing, differences from upstream |

## Licence

MIT — see [LICENSE](LICENSE). Original work by [Will Farrell](https://github.com/willfarrell).
