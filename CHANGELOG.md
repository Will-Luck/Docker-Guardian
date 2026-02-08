# Changelog

## [1.2.0] - 2026-02-08

### Added
- **Native multi-service notifications**: Gotify, Discord, Slack, Telegram, Pushover, Pushbullet, LunaSea, and Email — all via pure `curl`, no extra dependencies
- `NOTIFY_EVENTS` env var — controls which events trigger notifications: `startup`, `actions`, `failures`, `skips`, `debug`, or numbered (`1`-`5`)
- Event filtering: `notify_webhook()` checks event category before dispatching, `notify_skip()` for skip events, `notify_startup()` for boot confirmation
- Debug mode (`NOTIFY_EVENTS=debug`): logs every dispatch with `[notify] → service: message` to console
- Startup notification: sends boot confirmation when `startup` event is enabled
- Skip notifications: sends notifications for orchestration, grace period, and backup skip events when `skips` event is enabled
- Test suite: `test-notifications.sh`

### Changed
- `notify_webhook()` refactored into dispatcher pattern: event filtering → `_dispatch_notification()` → per-service `send_to_*()` functions
- Existing `WEBHOOK_URL` and `APPRISE_URL` still work (backward compatible, routed through new dispatcher)

## [1.1.0] - 2026-02-08

### Added
- **Watchtower awareness**: detects active orchestration (Watchtower, manual recreates) via Docker events API and pauses monitoring during the cooldown window
- `AUTOHEAL_WATCHTOWER_COOLDOWN` env var (default 300s) — cooldown window after orchestration events
- `AUTOHEAL_WATCHTOWER_SCOPE` env var (default `all`) — skip all containers or only affected ones
- `AUTOHEAL_WATCHTOWER_EVENTS` env var (default `orchestration`) — watch destroy+create events only or all lifecycle events
- Per-cycle orchestration event caching (single API call, same pattern as backup check)
- Test suite: `test-watchtower.sh`

### Changed
- README examples sanitised (generic container names instead of real service names)
- Grace period description updated (now positioned as fallback for non-Watchtower tools)

## [1.0.0] - 2026-02-08

### Added
- **Dependency monitoring**: auto-detects and recovers containers orphaned when their network parent restarts (exit code 128 with `--network=container:X`)
- **Backup awareness**: skips containers managed by backup tools (docker-volume-backup) during active backups
- **Grace period**: skips recently-stopped containers to avoid interfering with Watchtower, manual maintenance, or other orchestration
- Guard functions shared between unhealthy restart and dependency recovery paths
- Per-cycle backup status caching (single API call per cycle)
- Configurable start delay before restarting orphaned dependents
- Double-verification: re-checks parent and dependent state after delay
- Test suite: `test-dependency.sh`, `test-backup.sh`, `test-grace.sh`
- GitHub Actions CI: multi-arch build (amd64/arm64) + push to GHCR

### Changed
- Alpine base image bumped from 3.18 to 3.20
- Apprise notification title changed from "Autoheal" to "Docker-Guardian"
- POSIX-compliant `${CONTAINER_NAME#/}` instead of bash-only `${CONTAINER_NAME:1}`

### Inherited
- All original docker-autoheal functionality (unhealthy container restarts, webhooks, Apprise, TLS, per-container stop timeout)
