package guardian

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Will-Luck/Docker-Guardian/internal/config"
	"github.com/Will-Luck/Docker-Guardian/internal/docker"
	"github.com/Will-Luck/Docker-Guardian/internal/logging"
	"github.com/moby/moby/api/types/container"
)

func TestCheckDependencyOrphans_StartsOrphan(t *testing.T) {
	cfg := &config.Config{
		MonitorDependencies:  true,
		DependencyStartDelay: 0,
		WatchtowerCooldown:   0,
		GracePeriod:          0,
		BackupLabel:          "",
	}
	dock := newMockDocker()
	notif := &mockNotifier{}
	clk := newMockClock(time.Now())

	parentID := "parent1234567890abcdef"

	dock.exitedContainers = []container.Summary{
		{ID: "orphan01234567890abcdef"},
	}
	dock.inspectResults["orphan01234567890abcdef"] = container.InspectResponse{
		Name: "/orphan-app",
		HostConfig: &container.HostConfig{
			NetworkMode: container.NetworkMode("container:" + parentID),
		},
		Config: &container.Config{
			Labels: map[string]string{},
		},
		State: &container.State{
			ExitCode: 1,
		},
	}
	dock.statusResults[parentID] = "running"
	dock.statusResults["orphan01234567890abcdef"] = "exited"

	g := &Guardian{
		cfg:      cfg,
		docker:   dock,
		notifier: notif,
		log:      logging.New(false),
		clock:    clk,
		tracker:  NewRestartTracker(DefaultTrackerConfig(), clk),
	}

	g.checkDependencyOrphans(context.Background())

	if len(dock.startCalls) != 1 {
		t.Fatalf("expected 1 start call, got %d", len(dock.startCalls))
	}
	if dock.startCalls[0] != "orphan01234567890abcdef" {
		t.Errorf("started wrong container: %s", dock.startCalls[0])
	}
	if len(notif.actions) != 1 || !strings.Contains(notif.actions[0], "Successfully started") {
		t.Errorf("expected success notification, got %v", notif.actions)
	}
}

func TestCheckDependencyOrphans_SkipsNonDependents(t *testing.T) {
	cfg := &config.Config{
		MonitorDependencies:  true,
		DependencyStartDelay: 0,
		WatchtowerCooldown:   0,
		GracePeriod:          0,
		BackupLabel:          "",
	}
	dock := newMockDocker()
	notif := &mockNotifier{}
	clk := newMockClock(time.Now())

	dock.exitedContainers = []container.Summary{
		{ID: "standalone1234567890ab"},
	}
	dock.inspectResults["standalone1234567890ab"] = container.InspectResponse{
		Name: "/standalone",
		HostConfig: &container.HostConfig{
			NetworkMode: "bridge",
		},
		Config: &container.Config{},
		State:  &container.State{},
	}

	g := &Guardian{
		cfg:      cfg,
		docker:   dock,
		notifier: notif,
		log:      logging.New(false),
		clock:    clk,
		tracker:  NewRestartTracker(DefaultTrackerConfig(), clk),
	}

	g.checkDependencyOrphans(context.Background())

	if len(dock.startCalls) != 0 {
		t.Error("should not start non-dependent container")
	}
}

func TestCheckDependencyOrphans_SkipsWhenParentNotRunning(t *testing.T) {
	cfg := &config.Config{
		MonitorDependencies:  true,
		DependencyStartDelay: 0,
		WatchtowerCooldown:   0,
		GracePeriod:          0,
		BackupLabel:          "",
	}
	dock := newMockDocker()
	notif := &mockNotifier{}
	clk := newMockClock(time.Now())

	parentID := "parent1234567890abcdef"

	dock.exitedContainers = []container.Summary{
		{ID: "orphan01234567890abcdef"},
	}
	dock.inspectResults["orphan01234567890abcdef"] = container.InspectResponse{
		Name: "/orphan-app",
		HostConfig: &container.HostConfig{
			NetworkMode: container.NetworkMode("container:" + parentID),
		},
		Config: &container.Config{},
		State:  &container.State{},
	}
	dock.statusResults[parentID] = "exited" // Parent not running

	g := &Guardian{
		cfg:      cfg,
		docker:   dock,
		notifier: notif,
		log:      logging.New(false),
		clock:    clk,
		tracker:  NewRestartTracker(DefaultTrackerConfig(), clk),
	}

	g.checkDependencyOrphans(context.Background())

	if len(dock.startCalls) != 0 {
		t.Error("should not start orphan when parent is not running")
	}
}

func TestCheckDependencyOrphans_Disabled(t *testing.T) {
	cfg := &config.Config{
		MonitorDependencies: false,
	}
	dock := newMockDocker()
	notif := &mockNotifier{}
	clk := newMockClock(time.Now())

	g := &Guardian{
		cfg:      cfg,
		docker:   dock,
		notifier: notif,
		log:      logging.New(false),
		clock:    clk,
		tracker:  NewRestartTracker(DefaultTrackerConfig(), clk),
	}

	g.checkDependencyOrphans(context.Background())

	if len(dock.startCalls) != 0 {
		t.Error("should not make any calls when dependencies disabled")
	}
}

func TestCheckDependencyOrphans_SkipsAutoRecovered(t *testing.T) {
	cfg := &config.Config{
		MonitorDependencies:  true,
		DependencyStartDelay: 0,
		WatchtowerCooldown:   0,
		GracePeriod:          0,
		BackupLabel:          "",
	}
	dock := newMockDocker()
	notif := &mockNotifier{}
	clk := newMockClock(time.Now())

	parentID := "parent1234567890abcdef"

	dock.exitedContainers = []container.Summary{
		{ID: "orphan01234567890abcdef"},
	}
	dock.inspectResults["orphan01234567890abcdef"] = container.InspectResponse{
		Name: "/orphan-app",
		HostConfig: &container.HostConfig{
			NetworkMode: container.NetworkMode("container:" + parentID),
		},
		Config: &container.Config{
			Labels: map[string]string{},
		},
		State: &container.State{
			ExitCode: 0,
		},
	}
	dock.statusResults[parentID] = "running"
	// Container has auto-recovered by the time we re-check
	dock.statusResults["orphan01234567890abcdef"] = "running"

	g := &Guardian{
		cfg:      cfg,
		docker:   dock,
		notifier: notif,
		log:      logging.New(false),
		clock:    clk,
		tracker:  NewRestartTracker(DefaultTrackerConfig(), clk),
	}

	g.checkDependencyOrphans(context.Background())

	if len(dock.startCalls) != 0 {
		t.Error("should not start auto-recovered container")
	}
}

// --- Cascade restart tests ---

func TestCascadeRestartOnParentStart(t *testing.T) {
	cfg := &config.Config{
		CascadeRestart:      true,
		MonitorDependencies: true,
		CascadeSettleDelay:  0, // no delay for fast tests
		DefaultStopTimeout:  10,
		WatchtowerCooldown:  0,
		GracePeriod:         0,
		BackupLabel:         "",
	}
	dock := newMockDocker()
	notif := &mockNotifier{}
	clk := newMockClock(time.Now())

	parentID := "parent1234567890abcdef"
	dep1ID := "dep001234567890abcdef"
	dep2ID := "dep002234567890abcdef"

	dock.dependentsOfResults[parentID] = []container.Summary{
		{
			ID:     dep1ID,
			Names:  []string{"/dep-app-1"},
			Labels: map[string]string{},
		},
		{
			ID:     dep2ID,
			Names:  []string{"/dep-app-2"},
			Labels: map[string]string{},
		},
	}

	g := &Guardian{
		cfg:                 cfg,
		docker:              dock,
		notifier:            notif,
		log:                 logging.New(false),
		clock:               clk,
		tracker:             NewRestartTracker(DefaultTrackerConfig(), clk),
		orchestrationEvents: make(map[string]time.Time),
	}

	g.handleCascadeRestart(context.Background(), parentID)

	dock.mu.Lock()
	defer dock.mu.Unlock()

	if len(dock.restartCalls) != 2 {
		t.Fatalf("expected 2 restart calls, got %d", len(dock.restartCalls))
	}
	if dock.restartCalls[0] != dep1ID {
		t.Errorf("expected first restart to be %s, got %s", dep1ID, dock.restartCalls[0])
	}
	if dock.restartCalls[1] != dep2ID {
		t.Errorf("expected second restart to be %s, got %s", dep2ID, dock.restartCalls[1])
	}

	notif.mu.Lock()
	defer notif.mu.Unlock()

	if len(notif.actions) != 2 {
		t.Fatalf("expected 2 action notifications, got %d: %v", len(notif.actions), notif.actions)
	}
	for _, a := range notif.actions {
		if !strings.Contains(a, "Cascade:") {
			t.Errorf("expected cascade notification, got %q", a)
		}
		if !strings.Contains(a, "restarted") {
			t.Errorf("expected 'restarted' in notification, got %q", a)
		}
	}
}

func TestCascadeRestartDisabled(t *testing.T) {
	cfg := &config.Config{
		CascadeRestart:      false,
		MonitorDependencies: true,
	}
	dock := newMockDocker()
	notif := &mockNotifier{}
	clk := newMockClock(time.Now())

	parentID := "parent1234567890abcdef"

	// Even if dependents exist, they shouldn't be queried
	dock.dependentsOfResults[parentID] = []container.Summary{
		{
			ID:     "dep001234567890abcdef",
			Names:  []string{"/dep-app-1"},
			Labels: map[string]string{},
		},
	}

	g := &Guardian{
		cfg:      cfg,
		docker:   dock,
		notifier: notif,
		log:      logging.New(false),
		clock:    clk,
		tracker:  NewRestartTracker(DefaultTrackerConfig(), clk),
	}

	g.handleCascadeRestart(context.Background(), parentID)

	dock.mu.Lock()
	defer dock.mu.Unlock()

	if len(dock.restartCalls) != 0 {
		t.Error("should not restart anything when cascade is disabled")
	}
}

func TestCascadeRestartDisabledWhenMonitorDependenciesFalse(t *testing.T) {
	cfg := &config.Config{
		CascadeRestart:      true,
		MonitorDependencies: false,
	}
	dock := newMockDocker()
	notif := &mockNotifier{}
	clk := newMockClock(time.Now())

	parentID := "parent1234567890abcdef"
	dock.dependentsOfResults[parentID] = []container.Summary{
		{
			ID:     "dep001234567890abcdef",
			Names:  []string{"/dep-app-1"},
			Labels: map[string]string{},
		},
	}

	g := &Guardian{
		cfg:      cfg,
		docker:   dock,
		notifier: notif,
		log:      logging.New(false),
		clock:    clk,
		tracker:  NewRestartTracker(DefaultTrackerConfig(), clk),
	}

	g.handleCascadeRestart(context.Background(), parentID)

	dock.mu.Lock()
	defer dock.mu.Unlock()

	if len(dock.restartCalls) != 0 {
		t.Error("should not restart anything when MonitorDependencies is false")
	}
}

func TestCascadeRestartNoDependents(t *testing.T) {
	cfg := &config.Config{
		CascadeRestart:      true,
		MonitorDependencies: true,
		CascadeSettleDelay:  0,
		DefaultStopTimeout:  10,
		WatchtowerCooldown:  0,
		GracePeriod:         0,
		BackupLabel:         "",
	}
	dock := newMockDocker()
	notif := &mockNotifier{}
	clk := newMockClock(time.Now())

	parentID := "parent1234567890abcdef"
	// No dependents registered — DependentsOf returns empty slice

	g := &Guardian{
		cfg:      cfg,
		docker:   dock,
		notifier: notif,
		log:      logging.New(false),
		clock:    clk,
		tracker:  NewRestartTracker(DefaultTrackerConfig(), clk),
	}

	g.handleCascadeRestart(context.Background(), parentID)

	dock.mu.Lock()
	defer dock.mu.Unlock()

	if len(dock.restartCalls) != 0 {
		t.Error("should not restart anything when parent has no dependents")
	}

	notif.mu.Lock()
	defer notif.mu.Unlock()

	if len(notif.actions) != 0 {
		t.Error("should not send notifications when there are no dependents")
	}
}

func TestCascadeRestartCircuitBreaker(t *testing.T) {
	cfg := &config.Config{
		CascadeRestart:      true,
		MonitorDependencies: true,
		CascadeSettleDelay:  0,
		DefaultStopTimeout:  10,
		WatchtowerCooldown:  0,
		GracePeriod:         0,
		BackupLabel:         "",
	}
	dock := newMockDocker()
	notif := &mockNotifier{}
	clk := newMockClock(time.Now())

	parentID := "parent1234567890abcdef"
	depID := "dep001234567890abcdef"

	dock.dependentsOfResults[parentID] = []container.Summary{
		{
			ID:     depID,
			Names:  []string{"/dep-app-1"},
			Labels: map[string]string{},
		},
	}

	// Use a tracker with budget=1 so the circuit opens after one restart
	tcfg := TrackerConfig{
		BackoffMultiplier: 2,
		BackoffMax:        300 * time.Second,
		BackoffResetAfter: 600 * time.Second,
		RestartBudget:     1,
		RestartWindow:     300 * time.Second,
	}

	g := &Guardian{
		cfg:                 cfg,
		docker:              dock,
		notifier:            notif,
		log:                 logging.New(false),
		clock:               clk,
		tracker:             NewRestartTracker(tcfg, clk),
		orchestrationEvents: make(map[string]time.Time),
	}

	// Exhaust the budget: record a restart so the circuit opens
	g.tracker.RecordRestart(depID)

	g.handleCascadeRestart(context.Background(), parentID)

	dock.mu.Lock()
	defer dock.mu.Unlock()

	if len(dock.restartCalls) != 0 {
		t.Errorf("should not restart when circuit breaker is active, got %d restarts", len(dock.restartCalls))
	}
}

func TestCascadeRestartDependentsOfError(t *testing.T) {
	cfg := &config.Config{
		CascadeRestart:      true,
		MonitorDependencies: true,
		CascadeSettleDelay:  0,
		DefaultStopTimeout:  10,
		WatchtowerCooldown:  0,
		GracePeriod:         0,
		BackupLabel:         "",
	}
	dock := newMockDocker()
	notif := &mockNotifier{}
	clk := newMockClock(time.Now())

	parentID := "parent1234567890abcdef"
	dock.dependentsOfErr[parentID] = errors.New("docker API error")

	g := &Guardian{
		cfg:      cfg,
		docker:   dock,
		notifier: notif,
		log:      logging.New(false),
		clock:    clk,
		tracker:  NewRestartTracker(DefaultTrackerConfig(), clk),
	}

	g.handleCascadeRestart(context.Background(), parentID)

	dock.mu.Lock()
	defer dock.mu.Unlock()

	if len(dock.restartCalls) != 0 {
		t.Error("should not restart when DependentsOf returns error")
	}
}

func TestCascadeRestartPartialFailure(t *testing.T) {
	cfg := &config.Config{
		CascadeRestart:      true,
		MonitorDependencies: true,
		CascadeSettleDelay:  0,
		DefaultStopTimeout:  10,
		WatchtowerCooldown:  0,
		GracePeriod:         0,
		BackupLabel:         "",
	}
	dock := newMockDocker()
	notif := &mockNotifier{}
	clk := newMockClock(time.Now())

	parentID := "parent1234567890abcdef"
	dep1ID := "dep001234567890abcdef"
	dep2ID := "dep002234567890abcdef"

	dock.dependentsOfResults[parentID] = []container.Summary{
		{
			ID:     dep1ID,
			Names:  []string{"/dep-fail"},
			Labels: map[string]string{},
		},
		{
			ID:     dep2ID,
			Names:  []string{"/dep-ok"},
			Labels: map[string]string{},
		},
	}
	dock.restartErr[dep1ID] = errors.New("restart failed")

	g := &Guardian{
		cfg:                 cfg,
		docker:              dock,
		notifier:            notif,
		log:                 logging.New(false),
		clock:               clk,
		tracker:             NewRestartTracker(DefaultTrackerConfig(), clk),
		orchestrationEvents: make(map[string]time.Time),
	}

	g.handleCascadeRestart(context.Background(), parentID)

	dock.mu.Lock()
	defer dock.mu.Unlock()

	// Both should be attempted
	if len(dock.restartCalls) != 2 {
		t.Fatalf("expected 2 restart attempts, got %d", len(dock.restartCalls))
	}

	notif.mu.Lock()
	defer notif.mu.Unlock()

	// Should have 2 notifications: 1 failure + 1 success
	if len(notif.actions) != 2 {
		t.Fatalf("expected 2 notifications, got %d: %v", len(notif.actions), notif.actions)
	}
	if !strings.Contains(notif.actions[0], "failed") {
		t.Errorf("expected failure notification for dep1, got %q", notif.actions[0])
	}
	if !strings.Contains(notif.actions[1], "restarted") {
		t.Errorf("expected success notification for dep2, got %q", notif.actions[1])
	}
}

func TestCascadeRestartSettleDelay(t *testing.T) {
	cfg := &config.Config{
		CascadeRestart:      true,
		MonitorDependencies: true,
		CascadeSettleDelay:  15,
		DefaultStopTimeout:  10,
		WatchtowerCooldown:  0,
		GracePeriod:         0,
		BackupLabel:         "",
	}
	dock := newMockDocker()
	notif := &mockNotifier{}
	clk := newMockClock(time.Now())

	parentID := "parent1234567890abcdef"
	depID := "dep001234567890abcdef"

	dock.dependentsOfResults[parentID] = []container.Summary{
		{
			ID:     depID,
			Names:  []string{"/dep-app"},
			Labels: map[string]string{},
		},
	}

	g := &Guardian{
		cfg:                 cfg,
		docker:              dock,
		notifier:            notif,
		log:                 logging.New(false),
		clock:               clk,
		tracker:             NewRestartTracker(DefaultTrackerConfig(), clk),
		orchestrationEvents: make(map[string]time.Time),
	}

	// The mock clock's After() returns immediately (buffered channel),
	// so the settle delay is effectively skipped in tests.
	// This test verifies the code path completes without blocking.
	g.handleCascadeRestart(context.Background(), parentID)

	dock.mu.Lock()
	defer dock.mu.Unlock()

	if len(dock.restartCalls) != 1 {
		t.Fatalf("expected 1 restart call after settle delay, got %d", len(dock.restartCalls))
	}
	if dock.restartCalls[0] != depID {
		t.Errorf("restarted wrong container: %s", dock.restartCalls[0])
	}
}

// --- Network healthcheck tests ---

func TestNetworkHealthcheckRestartsUnreachable(t *testing.T) {
	cfg := &config.Config{
		NetworkHealthcheck:         true,
		NetworkHealthcheckTarget:   "8.8.8.8",
		NetworkHealthcheckFailures: 1,
		MonitorDependencies:        true,
		DefaultStopTimeout:         10,
		WatchtowerCooldown:         0,
		GracePeriod:                0,
		BackupLabel:                "",
	}
	dock := newMockDocker()
	notif := &mockNotifier{}
	clk := newMockClock(time.Now())

	parentID := "parent1234567890abcdef"
	depID := "dep001234567890abcdef"

	// Running container that shares network namespace with parent
	dock.runningContainers = []container.Summary{
		{ID: depID, Names: []string{"/dep-app"}},
	}
	dock.inspectResults[depID] = container.InspectResponse{
		Name: "/dep-app",
		HostConfig: &container.HostConfig{
			NetworkMode: container.NetworkMode("container:" + parentID),
		},
		Config: &container.Config{
			Labels: map[string]string{},
		},
		State: &container.State{},
	}

	g := &Guardian{
		cfg:                 cfg,
		docker:              dock,
		notifier:            notif,
		log:                 logging.New(false),
		clock:               clk,
		tracker:             NewRestartTracker(DefaultTrackerConfig(), clk),
		orchestrationEvents: make(map[string]time.Time),
	}

	// Baseline scan: ping succeeds, container is known-reachable.
	g.checkNetworkHealth(context.Background())

	// Ping fails — network is broken
	dock.execPingErr[depID] = fmt.Errorf("%w: ping exited with code 1", docker.ErrPingFailed)

	g.checkNetworkHealth(context.Background())

	dock.mu.Lock()
	defer dock.mu.Unlock()

	if len(dock.restartCalls) != 1 {
		t.Fatalf("expected 1 restart call, got %d", len(dock.restartCalls))
	}
	if dock.restartCalls[0] != depID {
		t.Errorf("restarted wrong container: %s", dock.restartCalls[0])
	}

	notif.mu.Lock()
	defer notif.mu.Unlock()

	if len(notif.actions) != 1 {
		t.Fatalf("expected 1 notification, got %d: %v", len(notif.actions), notif.actions)
	}
	if !strings.Contains(notif.actions[0], "Network health") {
		t.Errorf("expected network health notification, got %q", notif.actions[0])
	}
}

func TestNetworkHealthcheckSkipsHealthy(t *testing.T) {
	cfg := &config.Config{
		NetworkHealthcheck:       true,
		NetworkHealthcheckTarget: "8.8.8.8",
		MonitorDependencies:      true,
		DefaultStopTimeout:       10,
		WatchtowerCooldown:       0,
		GracePeriod:              0,
		BackupLabel:              "",
	}
	dock := newMockDocker()
	notif := &mockNotifier{}
	clk := newMockClock(time.Now())

	parentID := "parent1234567890abcdef"
	depID := "dep001234567890abcdef"

	dock.runningContainers = []container.Summary{
		{ID: depID, Names: []string{"/dep-app"}},
	}
	dock.inspectResults[depID] = container.InspectResponse{
		Name: "/dep-app",
		HostConfig: &container.HostConfig{
			NetworkMode: container.NetworkMode("container:" + parentID),
		},
		Config: &container.Config{
			Labels: map[string]string{},
		},
		State: &container.State{},
	}

	// Ping succeeds — no error in execPingErr means success

	g := &Guardian{
		cfg:                 cfg,
		docker:              dock,
		notifier:            notif,
		log:                 logging.New(false),
		clock:               clk,
		tracker:             NewRestartTracker(DefaultTrackerConfig(), clk),
		orchestrationEvents: make(map[string]time.Time),
	}

	g.checkNetworkHealth(context.Background())

	dock.mu.Lock()
	defer dock.mu.Unlock()

	if len(dock.restartCalls) != 0 {
		t.Errorf("should not restart healthy container, got %d restarts", len(dock.restartCalls))
	}
}

func TestNetworkHealthcheckSkipsNoPingBinary(t *testing.T) {
	cfg := &config.Config{
		NetworkHealthcheck:       true,
		NetworkHealthcheckTarget: "8.8.8.8",
		MonitorDependencies:      true,
		DefaultStopTimeout:       10,
		WatchtowerCooldown:       0,
		GracePeriod:              0,
		BackupLabel:              "",
	}
	dock := newMockDocker()
	notif := &mockNotifier{}
	clk := newMockClock(time.Now())

	parentID := "parent1234567890abcdef"
	depID := "dep001234567890abcdef"

	dock.runningContainers = []container.Summary{
		{ID: depID, Names: []string{"/dep-app"}},
	}
	dock.inspectResults[depID] = container.InspectResponse{
		Name: "/dep-app",
		HostConfig: &container.HostConfig{
			NetworkMode: container.NetworkMode("container:" + parentID),
		},
		Config: &container.Config{
			Labels: map[string]string{},
		},
		State: &container.State{},
	}

	// Simulate the exact error Docker returns when ping isn't installed
	dock.execPingErr[depID] = errors.New(
		`exec start: Error response from daemon: OCI runtime exec failed: exec failed: ` +
			`unable to start container process: exec: "ping": executable file not found in $PATH`)

	g := &Guardian{
		cfg:                 cfg,
		docker:              dock,
		notifier:            notif,
		log:                 logging.New(false),
		clock:               clk,
		tracker:             NewRestartTracker(DefaultTrackerConfig(), clk),
		orchestrationEvents: make(map[string]time.Time),
	}

	g.checkNetworkHealth(context.Background())

	dock.mu.Lock()
	defer dock.mu.Unlock()

	if len(dock.restartCalls) != 0 {
		t.Errorf("should not restart container without ping binary, got %d restarts", len(dock.restartCalls))
	}

	notif.mu.Lock()
	defer notif.mu.Unlock()

	if len(notif.actions) != 0 {
		t.Errorf("should not send notifications for missing ping binary, got %v", notif.actions)
	}
}

func TestNetworkHealthcheckDisabled(t *testing.T) {
	cfg := &config.Config{
		NetworkHealthcheck:  false,
		MonitorDependencies: true,
	}
	dock := newMockDocker()
	notif := &mockNotifier{}
	clk := newMockClock(time.Now())

	g := &Guardian{
		cfg:      cfg,
		docker:   dock,
		notifier: notif,
		log:      logging.New(false),
		clock:    clk,
		tracker:  NewRestartTracker(DefaultTrackerConfig(), clk),
	}

	g.checkNetworkHealth(context.Background())

	dock.mu.Lock()
	defer dock.mu.Unlock()

	if len(dock.restartCalls) != 0 {
		t.Error("should not make any calls when healthcheck is disabled")
	}
}

func TestNetworkHealthcheckDisabledWhenMonitorDependenciesFalse(t *testing.T) {
	cfg := &config.Config{
		NetworkHealthcheck:  true,
		MonitorDependencies: false,
	}
	dock := newMockDocker()
	notif := &mockNotifier{}
	clk := newMockClock(time.Now())

	g := &Guardian{
		cfg:      cfg,
		docker:   dock,
		notifier: notif,
		log:      logging.New(false),
		clock:    clk,
		tracker:  NewRestartTracker(DefaultTrackerConfig(), clk),
	}

	g.checkNetworkHealth(context.Background())

	dock.mu.Lock()
	defer dock.mu.Unlock()

	if len(dock.restartCalls) != 0 {
		t.Error("should not make any calls when MonitorDependencies is false")
	}
}

func TestNetworkHealthcheckSkipsNonSharedNamespace(t *testing.T) {
	cfg := &config.Config{
		NetworkHealthcheck:       true,
		NetworkHealthcheckTarget: "8.8.8.8",
		MonitorDependencies:      true,
		DefaultStopTimeout:       10,
		WatchtowerCooldown:       0,
		GracePeriod:              0,
		BackupLabel:              "",
	}
	dock := newMockDocker()
	notif := &mockNotifier{}
	clk := newMockClock(time.Now())

	bridgeID := "bridge01234567890abcde"

	dock.runningContainers = []container.Summary{
		{ID: bridgeID, Names: []string{"/standalone-app"}},
	}
	dock.inspectResults[bridgeID] = container.InspectResponse{
		Name: "/standalone-app",
		HostConfig: &container.HostConfig{
			NetworkMode: "bridge",
		},
		Config: &container.Config{
			Labels: map[string]string{},
		},
		State: &container.State{},
	}

	g := &Guardian{
		cfg:                 cfg,
		docker:              dock,
		notifier:            notif,
		log:                 logging.New(false),
		clock:               clk,
		tracker:             NewRestartTracker(DefaultTrackerConfig(), clk),
		orchestrationEvents: make(map[string]time.Time),
	}

	g.checkNetworkHealth(context.Background())

	dock.mu.Lock()
	defer dock.mu.Unlock()

	if len(dock.restartCalls) != 0 {
		t.Error("should not restart container with bridge network mode")
	}
}

func TestNetworkHealthcheckCircuitBreaker(t *testing.T) {
	cfg := &config.Config{
		NetworkHealthcheck:       true,
		NetworkHealthcheckTarget: "8.8.8.8",
		MonitorDependencies:      true,
		DefaultStopTimeout:       10,
		WatchtowerCooldown:       0,
		GracePeriod:              0,
		BackupLabel:              "",
	}
	dock := newMockDocker()
	notif := &mockNotifier{}
	clk := newMockClock(time.Now())

	parentID := "parent1234567890abcdef"
	depID := "dep001234567890abcdef"

	dock.runningContainers = []container.Summary{
		{ID: depID, Names: []string{"/dep-app"}},
	}
	dock.inspectResults[depID] = container.InspectResponse{
		Name: "/dep-app",
		HostConfig: &container.HostConfig{
			NetworkMode: container.NetworkMode("container:" + parentID),
		},
		Config: &container.Config{
			Labels: map[string]string{},
		},
		State: &container.State{},
	}
	dock.execPingErr[depID] = errors.New("ping: timeout")

	// Use a tracker with budget=1 so the circuit opens after one restart
	tcfg := TrackerConfig{
		BackoffMultiplier: 2,
		BackoffMax:        300 * time.Second,
		BackoffResetAfter: 600 * time.Second,
		RestartBudget:     1,
		RestartWindow:     300 * time.Second,
	}

	g := &Guardian{
		cfg:                 cfg,
		docker:              dock,
		notifier:            notif,
		log:                 logging.New(false),
		clock:               clk,
		tracker:             NewRestartTracker(tcfg, clk),
		orchestrationEvents: make(map[string]time.Time),
	}

	// Exhaust the budget
	g.tracker.RecordRestart(depID)

	g.checkNetworkHealth(context.Background())

	dock.mu.Lock()
	defer dock.mu.Unlock()

	if len(dock.restartCalls) != 0 {
		t.Errorf("should not restart when circuit breaker is active, got %d restarts", len(dock.restartCalls))
	}
}

func TestCascadeRestartContextCancelled(t *testing.T) {
	cfg := &config.Config{
		CascadeRestart:      true,
		MonitorDependencies: true,
		CascadeSettleDelay:  15,
		DefaultStopTimeout:  10,
		WatchtowerCooldown:  0,
		GracePeriod:         0,
		BackupLabel:         "",
	}
	dock := newMockDocker()
	notif := &mockNotifier{}
	// Use a special clock that blocks on After() to test context cancellation
	clk := &blockingClock{now: time.Now()}

	parentID := "parent1234567890abcdef"
	depID := "dep001234567890abcdef"

	dock.dependentsOfResults[parentID] = []container.Summary{
		{
			ID:     depID,
			Names:  []string{"/dep-app"},
			Labels: map[string]string{},
		},
	}

	g := &Guardian{
		cfg:                 cfg,
		docker:              dock,
		notifier:            notif,
		log:                 logging.New(false),
		clock:               clk,
		tracker:             NewRestartTracker(DefaultTrackerConfig(), clk),
		orchestrationEvents: make(map[string]time.Time),
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately — settle delay select should pick ctx.Done()

	g.handleCascadeRestart(ctx, parentID)

	dock.mu.Lock()
	defer dock.mu.Unlock()

	if len(dock.restartCalls) != 0 {
		t.Error("should not restart when context is cancelled during settle delay")
	}
}
