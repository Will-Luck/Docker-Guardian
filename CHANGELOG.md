# Changelog

## [Unreleased]

### Added
- `docs/roadmap.md`: release ladder v2.4.2 to v2.7.0, CI hygiene track, and the four-phase Watchtower integration design.

## [2.4.1] - 2026-06-05

### Fixed
- **Network healthcheck restart-loop on ICMP-blocked networks**: restarts now require a successful-ping baseline plus `AUTOHEAL_NETWORK_HEALTHCHECK_FAILURES` (default 3) consecutive genuine failures. Previously a single failed ping restarted the container immediately, so environments that block outbound ICMP (GitHub Actions runners, strict corporate egress) restart-looped every `container:X` dependent. Containers whose ping has never succeeded are left alone with a one-time warning.
- **Exec errors no longer count as ping failures**: probes that could not run (container stopping, daemon error) are ignored instead of triggering a restart. Previously this raced `docker stop` and could start a container the operator had just stopped, bypassing the grace period - the cause of the grace-period test failure in GitHub CI.

### Changed
- Integration test scripts set `AUTOHEAL_NETWORK_HEALTHCHECK=false` (none of them test it) so results don't depend on the runner's ICMP egress policy, and dump guardian logs to `/tmp/dg-test-logs/` for the CI failure artifact.
- CI `setup-go` uses `check-latest: true` so govulncheck runs on the newest Go patch release (clears GO-2026-5037 / GO-2026-5039, fixed in go1.25.11).

## [2.4.0] - 2026-06-05

### Added
- **Crash-loop detection**: alerts when a container restarts via Docker's own restart policy `AUTOHEAL_RESTARTLOOP_THRESHOLD` (default 5) times within `AUTOHEAL_RESTARTLOOP_WINDOW` seconds (default 300). Catches loops on containers that have no Docker healthcheck, which the autoheal path cannot see. One alert per loop episode, re-armed after a quiet window.
- **Down-container detection**: alerts when a container whose restart policy is `unless-stopped`/`always` (or whose name is listed in `AUTOHEAL_EXPECTED_UP`) stays down longer than `AUTOHEAL_DOWN_GRACE` seconds (default 120). One alert per down episode, cleared on recovery.
- **`alerts` notification category** and `Notifier.Alert()`: enable proactive crash-loop/down alerts with `NOTIFY_EVENTS=actions,alerts`. Reuses the existing Gotify/Discord/Slack/etc. dispatcher and rate-limiting.
- **Network-namespace cascade restart**: when a parent providing the network namespace (`network_mode: container:X`, e.g. a VPN sidecar) restarts, its dependents are restarted too. Controlled by `AUTOHEAL_CASCADE_RESTART` and `AUTOHEAL_CASCADE_SETTLE_DELAY`.
- **Network healthcheck**: periodically pings `AUTOHEAL_NETWORK_HEALTHCHECK_TARGET` from inside monitored containers and restarts on failure. Controlled by `AUTOHEAL_NETWORK_HEALTHCHECK`.

### Changed
- CI split into `ci.yml` + `release.yml`; GitHub-only steps (upload-artifact, trivy) are skipped on the Gitea runner.
- LICENSE copyright holder updated.

### Fixed
- Network healthcheck false positive when the `ping` binary is missing from the container image.
- Bumped Go 1.24 to 1.25 to clear govulncheck CVEs in `net/url`.

## [2.3.0] - 2026-02-10

### Changed
- **Complete rewrite from shell to Go**, shipped as a multi-arch image; legacy v1 shell code removed and the build context tightened. README split into a slim overview plus a `docs/` reference.
- Backup-container detection replaced with a time-based timeout.

### Added
- GitHub Releases with cross-compiled multi-arch binaries and README badges.

### Fixed
- Orchestration event filter now keys on `event` rather than `action`.
- Custom-label test SIGPIPE failure under `pipefail`; gofmt formatting in config and guards.

## [2.2.0] - 2026-02-08

### Added
- **Timezone support**: `TZ` env var works out of the box (tzdata added to Alpine image). Fixes upstream [#143](https://github.com/willfarrell/docker-autoheal/issues/143)
- **Skip paused containers**: Paused containers reporting unhealthy are no longer restarted. Fixes upstream [#98](https://github.com/willfarrell/docker-autoheal/issues/98)
- **Custom hostname in notifications**: `NOTIFY_HOSTNAME` env var prepends `[hostname]` to all notification messages. Fixes upstream [#118](https://github.com/willfarrell/docker-autoheal/issues/118)
- **Per-container notification filtering**: `autoheal.notify=false` label suppresses notifications while still performing the configured action. Fixes upstream [#140](https://github.com/willfarrell/docker-autoheal/issues/140)
- **Healthcheck output in notifications**: Restart notifications now include the last healthcheck output (truncated to 200 chars) for immediate context. Fixes upstream [#81](https://github.com/willfarrell/docker-autoheal/issues/81)
- **Unhealthy threshold**: `AUTOHEAL_UNHEALTHY_THRESHOLD` env var requires N consecutive unhealthy checks before action (default 1 = immediate, preserving existing behaviour). Fixes upstream [#78](https://github.com/willfarrell/docker-autoheal/issues/78)

## [1.2.0] - 2026-02-08

### Added
- **Native multi-service notifications**: Gotify, Discord, Slack, Telegram, Pushover, Pushbullet, LunaSea, and Email - all via pure `curl`, no extra dependencies
- `NOTIFY_EVENTS` env var - controls which events trigger notifications: `startup`, `actions`, `failures`, `skips`, `debug`, or numbered (`1`-`5`)
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
- `AUTOHEAL_WATCHTOWER_COOLDOWN` env var (default 300s) - cooldown window after orchestration events
- `AUTOHEAL_WATCHTOWER_SCOPE` env var (default `all`) - skip all containers or only affected ones
- `AUTOHEAL_WATCHTOWER_EVENTS` env var (default `orchestration`) - watch destroy+create events only or all lifecycle events
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
