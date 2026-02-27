# Network Namespace Cascade Restart

## Problem

Containers using `--network=container:X` share the parent's network namespace. When the parent restarts (backup cycle, update, manual restart), Docker keeps dependent containers "running" but their network namespace is gone. They become zombies: process alive, network dead.

Guardian currently handles the case where dependents **crash** (exit code 128 in `dependency.go`). It does not handle the more common case where the parent **restarts** and dependents stay "running" with a broken namespace.

Real-world trigger: nightly backup stops and restarts `wireguard-pia`, breaking networking for 7 dependent containers (radarr, sonarr, lidarr, prowlarr, qbittorrent, sabnzbd, flaresolverr). Currently mitigated by a standalone systemd watcher script, which this feature replaces.

## Design

### Detection: Hybrid (event + healthcheck)

**Layer 1 -- Event-based cascade (fast path):**
- Guardian already watches Docker `start` events in `watcher.go`
- On a `start` event, query all running containers for `HostConfig.NetworkMode == container:<parent-id-or-name>`
- Wait for configurable settle delay (default 15s) for the parent's network to stabilise
- Cascade restart all dependents
- Circuit breaker and restart budget apply normally

**Layer 2 -- Exec ping safety net (periodic):**
- During Guardian's periodic full scan, identify containers using `--network=container:X`
- Run `docker exec <container> ping -c1 -W3 <target>` to verify network connectivity
- If ping fails and the container is "running", treat as network-unhealthy and restart
- Catches edge cases the event stream might miss (reconnection gaps, daemon restarts)

### Scope

Generic, not VPN-specific. Any container using `--network=container:X` is automatically covered. No labels needed for cascade behaviour; Guardian auto-discovers the relationship from Docker inspect.

### Configuration

| Env Var | Default | Purpose |
|---------|---------|---------|
| `AUTOHEAL_CASCADE_RESTART` | `true` | Enable/disable cascade restart on parent start events |
| `AUTOHEAL_CASCADE_SETTLE_DELAY` | `15` | Seconds to wait after parent starts before cascading |
| `AUTOHEAL_NETWORK_HEALTHCHECK` | `true` | Enable exec-ping safety net for shared-namespace containers |
| `AUTOHEAL_NETWORK_HEALTHCHECK_TARGET` | `8.8.8.8` | Ping target for network health checks |

### Codebase Changes

**`internal/docker/containers.go`:**
- `DependentsOf(ctx, parentID) ([]ContainerInfo, error)` -- query running containers where NetworkMode matches the parent
- `ExecPing(ctx, containerID, target string) error` -- exec a ping inside a container, return error if unreachable

**`internal/guardian/dependency.go`:**
- `handleParentRestart(ctx, parentID)` -- wait for settle delay, query dependents, cascade restart with logging and circuit breaker
- `checkNetworkHealth(ctx)` -- periodic scan: find all shared-namespace containers, exec ping, restart if unhealthy

**`internal/guardian/guardian.go`:**
- Wire `handleParentRestart` into `handleEvent()` for `start` events
- Wire `checkNetworkHealth` into the periodic full scan

**`internal/docker/watcher.go`:**
- No changes needed (already watches `start` events)

**`internal/guardian/config.go`:**
- Add 4 new config fields with env var parsing

### Interactions with Existing Features

- **Circuit breaker:** Cascade restarts count against the restart budget normally. If a parent is flapping, the circuit breaker prevents restart storms.
- **Watchtower cooldown:** If the parent restart was triggered by Watchtower/Sentinel, the cooldown window applies. Cascade restarts are deferred until the cooldown expires.
- **Grace period:** Dependent containers that were recently stopped (e.g. by backup) get grace period protection.
- **Notifications:** Cascade restarts trigger normal notification events (action type: "cascade-restart").

### Migration

Once deployed, the standalone `vpn-network-watcher.service` systemd unit is retired:
```bash
sudo systemctl stop vpn-network-watcher
sudo systemctl disable vpn-network-watcher
sudo rm /etc/systemd/system/vpn-network-watcher.service
```

### Testing

- Unit tests: mock Docker client, simulate parent start event, verify dependents queried and restarted
- Unit tests: mock exec ping failure, verify container flagged as network-unhealthy
- Integration test: create parent + dependent containers, restart parent, verify dependent gets restarted
- Edge cases: parent flapping (circuit breaker kicks in), dependent already stopped, parent has no dependents
