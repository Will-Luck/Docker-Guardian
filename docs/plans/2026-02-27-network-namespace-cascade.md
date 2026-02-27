# Network Namespace Cascade Restart Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Automatically restart containers that share a network namespace (`--network=container:X`) when the parent container restarts, with a periodic exec-ping safety net.

**Architecture:** Extend Guardian's existing dependency management (`dependency.go`) with two new capabilities: (1) event-driven cascade restart when a parent container's `start` event fires, and (2) periodic network health checking via `docker exec ping` for shared-namespace containers. Both layers follow the existing restart pattern (shouldSkip -> tracker.ShouldRestart -> docker op -> RecordRestart -> postRestartScript).

**Tech Stack:** Go 1.24, moby client v0.2.2, Docker Engine API

---

### Task 1: Add Config Fields

**Files:**
- Modify: `internal/config/config.go` (struct ~line 14, Load ~line 90)
- Test: `internal/config/config_test.go`

**Step 1: Write the failing test**

```go
func TestCascadeRestartDefaults(t *testing.T) {
	cfg := Load()
	assert.True(t, cfg.CascadeRestart)
	assert.Equal(t, 15, cfg.CascadeSettleDelay)
	assert.True(t, cfg.NetworkHealthcheck)
	assert.Equal(t, "8.8.8.8", cfg.NetworkHealthcheckTarget)
}

func TestCascadeRestartFromEnv(t *testing.T) {
	t.Setenv("AUTOHEAL_CASCADE_RESTART", "false")
	t.Setenv("AUTOHEAL_CASCADE_SETTLE_DELAY", "30")
	t.Setenv("AUTOHEAL_NETWORK_HEALTHCHECK", "false")
	t.Setenv("AUTOHEAL_NETWORK_HEALTHCHECK_TARGET", "1.1.1.1")
	cfg := Load()
	assert.False(t, cfg.CascadeRestart)
	assert.Equal(t, 30, cfg.CascadeSettleDelay)
	assert.False(t, cfg.NetworkHealthcheck)
	assert.Equal(t, "1.1.1.1", cfg.NetworkHealthcheckTarget)
}
```

**Step 2: Run test to verify it fails**

Run: `cd /home/lns/Docker-Guardian && go test ./internal/config/ -run TestCascadeRestart -v`
Expected: FAIL (fields don't exist)

**Step 3: Write minimal implementation**

Add to Config struct (after `DependencyStartDelay` field, ~line 40):
```go
CascadeRestart         bool
CascadeSettleDelay     int
NetworkHealthcheck     bool
NetworkHealthcheckTarget string
```

Add to Load() function (after the `DependencyStartDelay` line, ~line 115):
```go
CascadeRestart:           envBool("AUTOHEAL_CASCADE_RESTART", true),
CascadeSettleDelay:       envInt("AUTOHEAL_CASCADE_SETTLE_DELAY", 15),
NetworkHealthcheck:       envBool("AUTOHEAL_NETWORK_HEALTHCHECK", true),
NetworkHealthcheckTarget: envString("AUTOHEAL_NETWORK_HEALTHCHECK_TARGET", "8.8.8.8"),
```

**Step 4: Run test to verify it passes**

Run: `cd /home/lns/Docker-Guardian && go test ./internal/config/ -run TestCascadeRestart -v`
Expected: PASS

**Step 5: Commit**

```bash
cd /home/lns/Docker-Guardian
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat(config): add cascade restart and network healthcheck settings"
```

---

### Task 2: Add DependentsOf() to Docker Client

**Files:**
- Modify: `internal/docker/interface.go` (~line 20)
- Modify: `internal/docker/containers.go` (append)
- Test: `internal/docker/containers_test.go`

**Step 1: Write the failing test**

```go
func TestDependentsOf(t *testing.T) {
	ctx := context.Background()
	cli := setupTestClient(t) // uses the existing test helper

	// Create a "parent" container
	parent := createTestContainer(t, cli, "test-parent", nil)
	defer removeTestContainer(t, cli, parent)

	// Create a "dependent" using parent's network
	dependent := createTestContainerWithNetwork(t, cli, "test-dependent", "container:"+parent)
	defer removeTestContainer(t, cli, dependent)

	deps, err := cli.DependentsOf(ctx, parent)
	require.NoError(t, err)
	assert.Len(t, deps, 1)
	assert.Contains(t, deps[0].Names[0], "test-dependent")
}

func TestDependentsOfNone(t *testing.T) {
	ctx := context.Background()
	cli := setupTestClient(t)

	parent := createTestContainer(t, cli, "test-loner", nil)
	defer removeTestContainer(t, cli, parent)

	deps, err := cli.DependentsOf(ctx, parent)
	require.NoError(t, err)
	assert.Empty(t, deps)
}
```

Note: If integration test helpers don't exist yet, write unit tests with a mock client instead. Check `internal/docker/` for existing test patterns.

**Step 2: Run test to verify it fails**

Run: `cd /home/lns/Docker-Guardian && go test ./internal/docker/ -run TestDependentsOf -v`
Expected: FAIL (method doesn't exist)

**Step 3: Write minimal implementation**

Add to `interface.go` API interface (~line 20):
```go
DependentsOf(ctx context.Context, parentID string) ([]container.Summary, error)
```

Add to `containers.go` (append at end of file):
```go
// DependentsOf returns all running containers whose NetworkMode is
// "container:<parentID>" or "container:<parentName>".
func (c *Client) DependentsOf(ctx context.Context, parentID string) ([]container.Summary, error) {
	running, err := c.RunningContainers(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing running containers: %w", err)
	}

	// Resolve parent name for matching (NetworkMode can use either ID or name).
	info, err := c.InspectContainer(ctx, parentID)
	if err != nil {
		return nil, fmt.Errorf("inspecting parent %s: %w", parentID, err)
	}
	parentName := strings.TrimPrefix(info.Name, "/")

	var deps []container.Summary
	for _, c2 := range running {
		if c2.ID == parentID {
			continue
		}
		inspect, err := c.InspectContainer(ctx, c2.ID)
		if err != nil {
			continue // container may have disappeared
		}
		mode := string(inspect.HostConfig.NetworkMode)
		if mode == "container:"+parentID || mode == "container:"+parentName {
			deps = append(deps, c2)
		}
	}
	return deps, nil
}
```

**Step 4: Run test to verify it passes**

Run: `cd /home/lns/Docker-Guardian && go test ./internal/docker/ -run TestDependentsOf -v`
Expected: PASS

**Step 5: Also update mock client if one exists**

Check `internal/docker/mock_*.go` or `internal/guardian/*_test.go` for a mock API implementation. Add the new method to keep the mock in sync with the interface.

**Step 6: Commit**

```bash
cd /home/lns/Docker-Guardian
git add internal/docker/
git commit -m "feat(docker): add DependentsOf() for network namespace queries"
```

---

### Task 3: Add ExecPing() to Docker Client

**Files:**
- Modify: `internal/docker/interface.go`
- Modify: `internal/docker/containers.go` (append)
- Test: `internal/docker/containers_test.go`

**Step 1: Write the failing test**

```go
func TestExecPing(t *testing.T) {
	ctx := context.Background()
	cli := setupTestClient(t)

	// Use a container with networking (e.g. alpine with ping)
	ctr := createTestContainer(t, cli, "test-ping", nil)
	defer removeTestContainer(t, cli, ctr)

	err := cli.ExecPing(ctx, ctr, "8.8.8.8")
	// May pass or fail depending on test env networking;
	// the key test is that ExecPing exists and doesn't panic.
	_ = err
}
```

**Step 2: Run test to verify it fails**

Run: `cd /home/lns/Docker-Guardian && go test ./internal/docker/ -run TestExecPing -v`
Expected: FAIL (method doesn't exist)

**Step 3: Write minimal implementation**

Add to `interface.go` API interface:
```go
ExecPing(ctx context.Context, containerID string, target string) error
```

Add to `containers.go`:
```go
// ExecPing runs "ping -c1 -W3 <target>" inside the container.
// Returns nil if the ping succeeds, error otherwise.
func (c *Client) ExecPing(ctx context.Context, containerID string, target string) error {
	execCfg := container.ExecOptions{
		Cmd:          []string{"ping", "-c", "1", "-W", "3", target},
		AttachStdout: false,
		AttachStderr: false,
	}

	resp, err := c.cli.ContainerExecCreate(ctx, containerID, execCfg)
	if err != nil {
		return fmt.Errorf("exec create: %w", err)
	}

	if err := c.cli.ContainerExecStart(ctx, resp.ID, container.ExecStartOptions{}); err != nil {
		return fmt.Errorf("exec start: %w", err)
	}

	// Wait for completion and check exit code.
	for {
		inspect, err := c.cli.ContainerExecInspect(ctx, resp.ID)
		if err != nil {
			return fmt.Errorf("exec inspect: %w", err)
		}
		if !inspect.Running {
			if inspect.ExitCode != 0 {
				return fmt.Errorf("ping exited with code %d", inspect.ExitCode)
			}
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
}
```

Note: Check the moby v0.2.2 API for exact `ExecCreate`/`ExecStart` signatures. The types may be `container.ExecOptions` or a separate `types` package. Verify imports.

**Step 4: Run test to verify it passes**

Run: `cd /home/lns/Docker-Guardian && go test ./internal/docker/ -run TestExecPing -v`
Expected: PASS

**Step 5: Update mock client**

Add `ExecPing` to the mock API.

**Step 6: Commit**

```bash
cd /home/lns/Docker-Guardian
git add internal/docker/
git commit -m "feat(docker): add ExecPing() for network health probing"
```

---

### Task 4: Cascade Restart on Parent Start Event

**Files:**
- Modify: `internal/guardian/dependency.go` (append new function)
- Modify: `internal/guardian/guardian.go` (~line 155, the "start" case in handleEvent)
- Test: `internal/guardian/dependency_test.go`

**Step 1: Write the failing test**

```go
func TestCascadeRestartOnParentStart(t *testing.T) {
	mockDocker := &MockAPI{}
	cfg := &config.Config{
		CascadeRestart:     true,
		CascadeSettleDelay: 0, // no delay in tests
		MonitorDependencies: true,
	}
	g := newTestGuardian(cfg, mockDocker)

	parentID := "parent-abc123"
	depSummary := []container.Summary{
		{ID: "dep-xyz789", Names: []string{"/sonarr"}},
	}

	mockDocker.On("DependentsOf", mock.Anything, parentID).Return(depSummary, nil)
	mockDocker.On("InspectContainer", mock.Anything, "dep-xyz789").Return(
		container.InspectResponse{
			ContainerJSONBase: container.ContainerJSONBase{
				ID: "dep-xyz789", Name: "/sonarr",
				State: &container.State{Status: "running"},
			},
		}, nil,
	)
	mockDocker.On("RestartContainer", mock.Anything, "dep-xyz789", mock.Anything).Return(nil)

	ctx := context.Background()
	g.handleCascadeRestart(ctx, parentID)

	mockDocker.AssertCalled(t, "DependentsOf", mock.Anything, parentID)
	mockDocker.AssertCalled(t, "RestartContainer", mock.Anything, "dep-xyz789", mock.Anything)
}

func TestCascadeRestartDisabled(t *testing.T) {
	mockDocker := &MockAPI{}
	cfg := &config.Config{
		CascadeRestart: false,
	}
	g := newTestGuardian(cfg, mockDocker)

	ctx := context.Background()
	g.handleCascadeRestart(ctx, "parent-abc123")

	mockDocker.AssertNotCalled(t, "DependentsOf", mock.Anything, mock.Anything)
}

func TestCascadeRestartNoDependents(t *testing.T) {
	mockDocker := &MockAPI{}
	cfg := &config.Config{
		CascadeRestart:     true,
		CascadeSettleDelay: 0,
		MonitorDependencies: true,
	}
	g := newTestGuardian(cfg, mockDocker)

	mockDocker.On("DependentsOf", mock.Anything, "loner-123").Return([]container.Summary{}, nil)

	ctx := context.Background()
	g.handleCascadeRestart(ctx, "loner-123")

	mockDocker.AssertNotCalled(t, "RestartContainer", mock.Anything, mock.Anything, mock.Anything)
}
```

Note: Adapt to the actual mock pattern used in existing tests. Check `internal/guardian/*_test.go` for `MockAPI`, `newTestGuardian`, etc.

**Step 2: Run test to verify it fails**

Run: `cd /home/lns/Docker-Guardian && go test ./internal/guardian/ -run TestCascadeRestart -v`
Expected: FAIL (handleCascadeRestart doesn't exist)

**Step 3: Write minimal implementation**

Add to `dependency.go`:
```go
// handleCascadeRestart restarts all containers sharing a network namespace
// with the given parent container. Called when a parent's "start" event fires.
// When a parent restarts, dependent containers keep running but their network
// namespace is gone -- they need to be restarted to pick up the new one.
func (g *Guardian) handleCascadeRestart(ctx context.Context, parentID string) {
	if !g.cfg.CascadeRestart || !g.cfg.MonitorDependencies {
		return
	}

	deps, err := g.docker.DependentsOf(ctx, parentID)
	if err != nil {
		g.log.Error("cascade: failed to query dependents", "parent", parentID, "error", err)
		return
	}
	if len(deps) == 0 {
		return
	}

	names := make([]string, len(deps))
	for i, d := range deps {
		names[i] = strings.TrimPrefix(d.Names[0], "/")
	}
	g.log.Info("cascade: parent restarted, restarting dependents",
		"parent", parentID, "dependents", names, "count", len(deps))

	// Settle delay: wait for parent's network (e.g. VPN tunnel) to stabilise.
	if g.cfg.CascadeSettleDelay > 0 {
		g.log.Debug("cascade: waiting for parent to settle",
			"delay", g.cfg.CascadeSettleDelay)
		select {
		case <-ctx.Done():
			return
		case <-g.clock.After(time.Duration(g.cfg.CascadeSettleDelay) * time.Second):
		}
	}

	for _, dep := range deps {
		name := strings.TrimPrefix(dep.Names[0], "/")
		shortID := dep.ID[:12]

		// Skip checks (orchestration cooldown, grace period, backup).
		if skip, reason := g.shouldSkip(ctx, dep.ID, name, nil); skip {
			g.log.Info("cascade: skipping dependent", "name", name, "reason", reason)
			continue
		}

		// Circuit breaker.
		if allowed, reason := g.tracker.ShouldRestart(dep.ID); !allowed {
			g.log.Info("cascade: circuit breaker active", "name", name, "reason", reason)
			continue
		}

		timeout := g.cfg.DefaultStopTimeout
		g.log.Info("cascade: restarting dependent", "name", name, "id", shortID)
		if err := g.docker.RestartContainer(ctx, dep.ID, timeout); err != nil {
			g.log.Error("cascade: restart failed", "name", name, "error", err)
			continue
		}

		g.tracker.RecordRestart(dep.ID)
		g.notify(ctx, notify.Event{
			Type:      "cascade_restart",
			Container: name,
			Message:   fmt.Sprintf("Restarted %s (parent network namespace changed)", name),
		})
		g.runPostRestartScript(name, shortID, "running", timeout)
		g.log.Info("cascade: restarted dependent", "name", name)
	}
}
```

Note: Check the exact signatures of `g.shouldSkip()`, `g.tracker.ShouldRestart()`, `g.notify()`, and `g.runPostRestartScript()`. Adapt the label parameter in shouldSkip (3rd arg) -- it may need the container's labels from inspect. Adapt the notify call to match the existing notify.Event struct.

**Step 4: Wire into handleEvent**

In `guardian.go` `handleEvent()` (~line 155), change the `"start"` case from no-op to:
```go
case "start":
	go g.handleCascadeRestart(ctx, evt.ContainerID)
```

The goroutine is needed because the settle delay would block event processing.

**Step 5: Run test to verify it passes**

Run: `cd /home/lns/Docker-Guardian && go test ./internal/guardian/ -run TestCascadeRestart -v`
Expected: PASS

**Step 6: Commit**

```bash
cd /home/lns/Docker-Guardian
git add internal/guardian/dependency.go internal/guardian/guardian.go internal/guardian/dependency_test.go
git commit -m "feat(guardian): cascade restart dependents on parent start event"
```

---

### Task 5: Network Healthcheck in Periodic Scan

**Files:**
- Modify: `internal/guardian/dependency.go` (append new function)
- Modify: `internal/guardian/guardian.go` (add call to fullScan, ~line 135)
- Test: `internal/guardian/dependency_test.go`

**Step 1: Write the failing test**

```go
func TestNetworkHealthcheckRestartsUnreachable(t *testing.T) {
	mockDocker := &MockAPI{}
	cfg := &config.Config{
		NetworkHealthcheck:       true,
		NetworkHealthcheckTarget: "8.8.8.8",
		MonitorDependencies:      true,
		CascadeRestart:           true,
	}
	g := newTestGuardian(cfg, mockDocker)

	running := []container.Summary{
		{ID: "vpn-parent", Names: []string{"/wireguard-pia"}},
		{ID: "dep-sonarr", Names: []string{"/sonarr"}},
	}
	mockDocker.On("RunningContainers", mock.Anything).Return(running, nil)

	// Parent: normal bridge network
	mockDocker.On("InspectContainer", mock.Anything, "vpn-parent").Return(
		container.InspectResponse{
			ContainerJSONBase: container.ContainerJSONBase{
				ID: "vpn-parent", Name: "/wireguard-pia",
				State: &container.State{Status: "running"},
				HostConfig: &container.HostConfig{NetworkMode: "bridge"},
			},
		}, nil,
	)
	// Dependent: shared namespace
	mockDocker.On("InspectContainer", mock.Anything, "dep-sonarr").Return(
		container.InspectResponse{
			ContainerJSONBase: container.ContainerJSONBase{
				ID: "dep-sonarr", Name: "/sonarr",
				State: &container.State{Status: "running"},
				HostConfig: &container.HostConfig{NetworkMode: "container:vpn-parent"},
			},
		}, nil,
	)

	// Ping fails -- network is broken
	mockDocker.On("ExecPing", mock.Anything, "dep-sonarr", "8.8.8.8").
		Return(fmt.Errorf("ping exited with code 1"))
	mockDocker.On("RestartContainer", mock.Anything, "dep-sonarr", mock.Anything).Return(nil)

	ctx := context.Background()
	g.checkNetworkHealth(ctx)

	mockDocker.AssertCalled(t, "ExecPing", mock.Anything, "dep-sonarr", "8.8.8.8")
	mockDocker.AssertCalled(t, "RestartContainer", mock.Anything, "dep-sonarr", mock.Anything)
}

func TestNetworkHealthcheckSkipsHealthy(t *testing.T) {
	mockDocker := &MockAPI{}
	cfg := &config.Config{
		NetworkHealthcheck:       true,
		NetworkHealthcheckTarget: "8.8.8.8",
		MonitorDependencies:      true,
	}
	g := newTestGuardian(cfg, mockDocker)

	running := []container.Summary{
		{ID: "vpn-parent", Names: []string{"/wireguard-pia"}},
		{ID: "dep-sonarr", Names: []string{"/sonarr"}},
	}
	mockDocker.On("RunningContainers", mock.Anything).Return(running, nil)
	mockDocker.On("InspectContainer", mock.Anything, "vpn-parent").Return(
		container.InspectResponse{
			ContainerJSONBase: container.ContainerJSONBase{
				ID: "vpn-parent", Name: "/wireguard-pia",
				HostConfig: &container.HostConfig{NetworkMode: "bridge"},
			},
		}, nil,
	)
	mockDocker.On("InspectContainer", mock.Anything, "dep-sonarr").Return(
		container.InspectResponse{
			ContainerJSONBase: container.ContainerJSONBase{
				ID: "dep-sonarr", Name: "/sonarr",
				HostConfig: &container.HostConfig{NetworkMode: "container:vpn-parent"},
			},
		}, nil,
	)

	// Ping succeeds -- network is fine
	mockDocker.On("ExecPing", mock.Anything, "dep-sonarr", "8.8.8.8").Return(nil)

	ctx := context.Background()
	g.checkNetworkHealth(ctx)

	mockDocker.AssertCalled(t, "ExecPing", mock.Anything, "dep-sonarr", "8.8.8.8")
	mockDocker.AssertNotCalled(t, "RestartContainer", mock.Anything, mock.Anything, mock.Anything)
}

func TestNetworkHealthcheckDisabled(t *testing.T) {
	mockDocker := &MockAPI{}
	cfg := &config.Config{
		NetworkHealthcheck: false,
	}
	g := newTestGuardian(cfg, mockDocker)

	ctx := context.Background()
	g.checkNetworkHealth(ctx)

	mockDocker.AssertNotCalled(t, "RunningContainers", mock.Anything)
}
```

Note: Adapt `container.InspectResponse` fields to match moby v0.2.2 struct layout. The agent found that `InspectResponse` no longer has `ContainerJSONBase` -- fields are directly on the struct. Verify the struct shape by reading the actual import.

**Step 2: Run test to verify it fails**

Run: `cd /home/lns/Docker-Guardian && go test ./internal/guardian/ -run TestNetworkHealthcheck -v`
Expected: FAIL (checkNetworkHealth doesn't exist)

**Step 3: Write minimal implementation**

Add to `dependency.go`:
```go
// checkNetworkHealth probes containers that share a network namespace to
// verify they still have working connectivity. This is the safety net for
// cases where the event-driven cascade (handleCascadeRestart) misses a
// parent restart.
func (g *Guardian) checkNetworkHealth(ctx context.Context) {
	if !g.cfg.NetworkHealthcheck || !g.cfg.MonitorDependencies {
		return
	}

	running, err := g.docker.RunningContainers(ctx)
	if err != nil {
		g.log.Error("network health: failed to list containers", "error", err)
		return
	}

	// Find containers using --network=container:X
	for _, c := range running {
		info, err := g.docker.InspectContainer(ctx, c.ID)
		if err != nil {
			continue
		}
		mode := string(info.HostConfig.NetworkMode)
		if !strings.HasPrefix(mode, "container:") {
			continue
		}

		name := strings.TrimPrefix(info.Name, "/")
		shortID := c.ID[:12]

		// Probe network connectivity.
		if err := g.docker.ExecPing(ctx, c.ID, g.cfg.NetworkHealthcheckTarget); err != nil {
			g.log.Warn("network health: ping failed, restarting",
				"name", name, "target", g.cfg.NetworkHealthcheckTarget, "error", err)

			if skip, reason := g.shouldSkip(ctx, c.ID, name, nil); skip {
				g.log.Info("network health: skipping", "name", name, "reason", reason)
				continue
			}
			if allowed, reason := g.tracker.ShouldRestart(c.ID); !allowed {
				g.log.Info("network health: circuit breaker", "name", name, "reason", reason)
				continue
			}

			timeout := g.cfg.DefaultStopTimeout
			if err := g.docker.RestartContainer(ctx, c.ID, timeout); err != nil {
				g.log.Error("network health: restart failed", "name", name, "error", err)
				continue
			}

			g.tracker.RecordRestart(c.ID)
			g.notify(ctx, notify.Event{
				Type:      "network_unhealthy",
				Container: name,
				Message:   fmt.Sprintf("Restarted %s (network unreachable)", name),
			})
			g.runPostRestartScript(name, shortID, "running", timeout)
		}
	}
}
```

**Step 4: Wire into fullScan**

In `guardian.go` `fullScan()` (~line 135), add after `checkDependencyOrphans()`:
```go
g.checkNetworkHealth(ctx)
```

**Step 5: Run test to verify it passes**

Run: `cd /home/lns/Docker-Guardian && go test ./internal/guardian/ -run TestNetworkHealthcheck -v`
Expected: PASS

**Step 6: Commit**

```bash
cd /home/lns/Docker-Guardian
git add internal/guardian/dependency.go internal/guardian/guardian.go internal/guardian/dependency_test.go
git commit -m "feat(guardian): add periodic network healthcheck for shared-namespace containers"
```

---

### Task 6: Update Mock API and Run Full Test Suite

**Files:**
- Modify: Mock API file (check `internal/guardian/mock_*.go` or test helpers)

**Step 1: Add new methods to mock**

Find the mock implementation of `docker.API` and add:
```go
func (m *MockAPI) DependentsOf(ctx context.Context, parentID string) ([]container.Summary, error) {
	args := m.Called(ctx, parentID)
	return args.Get(0).([]container.Summary), args.Error(1)
}

func (m *MockAPI) ExecPing(ctx context.Context, containerID string, target string) error {
	args := m.Called(ctx, containerID, target)
	return args.Error(0)
}
```

**Step 2: Run full test suite**

Run: `cd /home/lns/Docker-Guardian && go test ./... -v`
Expected: ALL PASS

**Step 3: Run linter**

Run: `cd /home/lns/Docker-Guardian && make lint`
Expected: PASS (no gofmt or lint issues)

**Step 4: Commit if any fixes were needed**

```bash
cd /home/lns/Docker-Guardian
git add -A
git commit -m "fix: update mock API and lint fixes for cascade restart"
```

---

### Task 7: Update Documentation

**Files:**
- Modify: `README.md` (env var table, features list)
- Modify: `CLAUDE.md` (if architecture reference tables exist)

**Step 1: Update README**

Add the 4 new env vars to the configuration table. Add "Network Namespace Cascade Restart" to the features list with a brief description.

**Step 2: Update CLAUDE.md**

If there's an architecture reference or config table in CLAUDE.md, add the new fields.

**Step 3: Commit**

```bash
cd /home/lns/Docker-Guardian
git add README.md CLAUDE.md
git commit -m "docs: add cascade restart and network healthcheck to README"
```

---

### Task 8: Integration Smoke Test

**This is a manual verification task, not automated.**

**Step 1: Build**

Run: `cd /home/lns/Docker-Guardian && make build`
Expected: Binary builds successfully

**Step 2: Verify with dry run (if supported)**

Check if Guardian has a dry-run or debug mode. If not, review logs by running with `AUTOHEAL_CONTAINER_LABEL=all` in a test environment.

**Step 3: Commit final state**

```bash
cd /home/lns/Docker-Guardian
git add -A
git commit -m "chore: integration smoke test verified"
```

---

## Dependency Graph

```
Task 1 (config) ──┬──> Task 4 (cascade event handler)
                   │
Task 2 (DependentsOf) ──┤
                        ├──> Task 6 (mock + full tests)
Task 3 (ExecPing) ──────┤
                        │
                        ├──> Task 5 (network healthcheck)
                        │
                        └──> Task 7 (docs) ──> Task 8 (smoke test)
```

Tasks 1, 2, 3 can be done in parallel. Tasks 4 and 5 depend on 1+2 and 1+3 respectively. Task 6 depends on all implementation tasks. Task 7 and 8 are sequential at the end.

## Notes

- Check moby v0.2.2 API for exact type names (the `InspectResponse` struct layout changed -- fields are directly on the struct, not in `ContainerJSONBase`).
- The `notify.Event` struct and `g.notify()` method may have a different signature -- check the existing notification pattern in `unhealthy.go`.
- `g.shouldSkip()` takes labels as 3rd arg (type `map[string]string`) -- pass nil or the container's actual labels from inspect.
- The `g.clock.After()` call in the settle delay uses Guardian's clock abstraction for testability.
