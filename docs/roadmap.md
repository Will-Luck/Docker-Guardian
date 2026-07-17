# Roadmap

Where Docker-Guardian goes after v2.4.1. Sequencing is driven by two dependencies:
dry-run should land early so every later action path inherits the gate, and the
`/status` endpoint introduces the HTTP surface that persistent history and the
Watchtower webhook receiver then reuse.

Evidence links point at the upstream [willfarrell/docker-autoheal](https://github.com/willfarrell/docker-autoheal)
issue tracker (abbreviated `#N` below), where most of these requests have sat
unanswered for years.

## v2.4.2 (patch) - housekeeping

- **Wire the three registered-but-unwritten metrics.** `events_processed_total`,
  `event_processing_duration_seconds`, and `event_stream_connected` are registered
  (`internal/metrics/prometheus.go`) but no code writes them. A
  `Guardian.EventStreamConnected()` helper already exists unused
  (`internal/guardian/guardian.go`).
- **`AUTOHEAL_BACKUP_CONTAINER`: implement or remove.** Loaded and documented
  (`internal/config/config.go`, `docs/configuration.md`) but never referenced by
  any logic; only `AUTOHEAL_BACKUP_LABEL` drives backup skips.
- **Opt-out consistency.** The `autoheal=False` skip (`internal/guardian/unhealthy.go`)
  matches capital-F only and can only ever fire in `all` mode. Accept `false`/`False`
  everywhere the label is read.
- **CHANGELOG hygiene.** Keep a permanent `[Unreleased]` section per
  [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## v2.5.0 - visibility and safety

- **Dry-run mode.** `AUTOHEAL_DRY_RUN=true` evaluates the full decision flow and
  logs/notifies what *would* happen without acting; per-container
  `autoheal.action=dryrun` for selective trials. Gate every action verb behind one
  check. (#92, #111, #132)
- **`/status` endpoint.** JSON on the existing metrics listener: per-container
  restart history, circuit-breaker state, quarantined/held containers, last probe
  result. Optional tiny embedded HTML page. Surfaces the currently-silent
  circuit-open state. (#104, #42)
- **Persistent history.** Opt-in bounded history file (mounted path) so restart
  counts and episodes survive Guardian's own restarts; feeds `/status`. (#104)
- **`test-notify` and `check-config` subcommands.** Fire a sample notification to
  every configured provider on demand; validate env/label config and exit non-zero
  on error. (#90, #110)
- **Watchtower integration, phases A and B** (see design below).

## v2.6.0 - probes and dependencies

- **Synthetic probes for containers without a HEALTHCHECK.** Label-driven
  `guardian.probe=http://:8080/health`, `tcp://:5432`, or `exec:<cmd>` with
  per-container interval/threshold, reusing the existing consecutive-failure and
  circuit-breaker machinery. Makes the large population of HEALTHCHECK-less images
  healable; no shipping competitor has this. (#91, #138)
- **Label-declared dependencies.** `guardian.depends-on=<name>` extends cascade and
  orphan recovery beyond shared network namespaces to ordinary bridge-network
  app-to-database relationships, with cycle detection. The most-requested upstream
  feature. (#49, #36)
- **Quiet windows.** `AUTOHEAL_QUIET_WINDOWS="Sun 02:00-04:00"` suppresses restarts
  during maintenance/backup windows; complements the grace period and backup-label
  skip. (#140)
- **Watchtower integration, phase C** (see design below).

## v2.7.0 - reach and polish

- **Notification templating.** `NOTIFY_TEMPLATE` Go template for body/title with
  container/event/output variables; default format unchanged. (#118, #134)
- **Digest notifications.** Optional roll-up ("3 containers restarted in the last
  5m") beyond per-container rate limiting. (#83, #140)
- **Remote hosts over mTLS/SSH.** `DOCKER_SOCK` already accepts `tcp://`; add
  ca/cert/key and `ssh://` so multiple nodes can be watched without exposing
  plaintext 2375.
- **Least-privilege posture.** Document running behind a Docker socket proxy,
  enumerate the API verbs Guardian actually needs, publish an SBOM. (#135, #130)
- **Watchtower integration, phase D** (opt-in, see design below).

## CI track (any time)

- **Weekly base-image rebuild and digest-pinned tags** so CVE fixes ship without
  code changes and semver tags never silently move. (#147, #96, #136, #130, #127)

## Watchtower integration design

Guardian currently *infers* update activity from create/destroy event bursts and
applies a blanket cooldown (`AUTOHEAL_WATCHTOWER_*`). The actively maintained
[nicholas-fedor/watchtower](https://github.com/nicholas-fedor/watchtower) fork
exposes enough surface to replace inference with facts:

- **A. Label awareness (S).** Read `com.centurylinklabs.watchtower.enable`,
  `.scope`, and `.monitor-only`, and detect the Watchtower container by image, so
  `affected`-scope suppression keys on what Watchtower actually manages.
- **B. Exact update windows (M).** Guardian exposes `/hooks/watchtower`; the user
  points `WATCHTOWER_NOTIFICATION_URL` (generic webhook) at it with
  [`--notification-report`](https://watchtower.nickfedor.com/dev/notifications/overview/)
  enabled. The session report (`.Scanned`/`.Updated`/`.Failed`) tells Guardian
  exactly when a session ran and which containers changed; the event-burst
  heuristic becomes fallback-only.
- **C. Update-aware healing (M).** When a container is recreated with a new image
  ID, apply a post-update settle grace; if it goes unhealthy or crash-loops within
  N minutes of an update, raise a distinct alert ("likely bad update: healthy on
  `<old image>`, failing on `<new image>`"), optionally quarantining instead of
  restart-looping.
- **D. Active remediation (S/M, opt-in, off by default).** As a last-resort action,
  call Watchtower's token-authenticated
  [HTTP API](https://watchtower.nickfedor.com/v1.12.3/advanced-features/http-api/)
  (`/v1/update?image=X`) to force a targeted re-pull. Handle HTTP 429 (session
  already running).

Sizes: S = small, M = medium, L = large (scope, not schedule).
