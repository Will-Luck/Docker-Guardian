package guardian

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/Will-Luck/Docker-Guardian/internal/config"
	"github.com/Will-Luck/Docker-Guardian/internal/docker"
	"github.com/Will-Luck/Docker-Guardian/internal/logging"
	"github.com/moby/moby/api/types/container"
)

// newNetworkHealthGuardian builds a guardian with one running dependent
// (dep001...) sharing a network namespace with parent1234..., ready for
// checkNetworkHealth tests.
func newNetworkHealthGuardian(failures int) (*Guardian, *mockDocker, *mockNotifier) {
	cfg := &config.Config{
		NetworkHealthcheck:         true,
		NetworkHealthcheckTarget:   "8.8.8.8",
		NetworkHealthcheckFailures: failures,
		MonitorDependencies:        true,
		DefaultStopTimeout:         10,
		WatchtowerCooldown:         0,
		GracePeriod:                0,
		BackupLabel:                "",
	}
	dock := newMockDocker()
	notif := &mockNotifier{}
	clk := newMockClock(time.Now())

	depID := "dep001234567890abcdef"
	dock.runningContainers = []container.Summary{
		{ID: depID, Names: []string{"/dep-app"}},
	}
	dock.inspectResults[depID] = container.InspectResponse{
		Name: "/dep-app",
		HostConfig: &container.HostConfig{
			NetworkMode: container.NetworkMode("container:parent1234567890abcdef"),
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
	return g, dock, notif
}

// pingFailedErr mimics what Client.ExecPing returns when ping itself runs
// and exits non-zero.
func pingFailedErr() error {
	return fmt.Errorf("%w: ping exited with code 1", docker.ErrPingFailed)
}

// A container whose ping has never succeeded must not be restarted, no
// matter how many times the probe fails - on ICMP-blocked networks
// (e.g. GitHub Actions runners, strict corporate egress) the ping can
// never succeed, and restarting would loop every monitored dependent.
func TestNetworkHealthNoRestartWithoutPingBaseline(t *testing.T) {
	g, dock, notif := newNetworkHealthGuardian(1)
	depID := "dep001234567890abcdef"
	dock.execPingErr[depID] = pingFailedErr()

	for i := 0; i < 5; i++ {
		g.checkNetworkHealth(context.Background())
	}

	dock.mu.Lock()
	defer dock.mu.Unlock()
	if len(dock.restartCalls) != 0 {
		t.Fatalf("expected 0 restart calls without a ping baseline, got %d", len(dock.restartCalls))
	}

	notif.mu.Lock()
	defer notif.mu.Unlock()
	if len(notif.actions) != 0 {
		t.Fatalf("expected 0 action notifications, got %d: %v", len(notif.actions), notif.actions)
	}
}

// Once a container has pinged successfully (baseline), a restart fires
// only after the configured number of consecutive failures.
func TestNetworkHealthRestartsAfterConsecutiveFailures(t *testing.T) {
	g, dock, _ := newNetworkHealthGuardian(3)
	depID := "dep001234567890abcdef"

	// Baseline: first scan succeeds.
	g.checkNetworkHealth(context.Background())

	// Two failures: below threshold, no restart yet.
	dock.execPingErr[depID] = pingFailedErr()
	g.checkNetworkHealth(context.Background())
	g.checkNetworkHealth(context.Background())

	dock.mu.Lock()
	if len(dock.restartCalls) != 0 {
		dock.mu.Unlock()
		t.Fatalf("expected no restart below failure threshold, got %d", len(dock.restartCalls))
	}
	dock.mu.Unlock()

	// Third consecutive failure: restart fires.
	g.checkNetworkHealth(context.Background())

	dock.mu.Lock()
	defer dock.mu.Unlock()
	if len(dock.restartCalls) != 1 {
		t.Fatalf("expected 1 restart call on threshold, got %d", len(dock.restartCalls))
	}
	if dock.restartCalls[0] != depID {
		t.Errorf("restarted wrong container: %s", dock.restartCalls[0])
	}
}

// A successful ping resets the consecutive-failure counter.
func TestNetworkHealthFailureCountResetsOnSuccess(t *testing.T) {
	g, dock, _ := newNetworkHealthGuardian(3)
	depID := "dep001234567890abcdef"

	// Baseline.
	g.checkNetworkHealth(context.Background())

	// fail, fail, success, fail, fail - never 3 consecutive.
	dock.execPingErr[depID] = pingFailedErr()
	g.checkNetworkHealth(context.Background())
	g.checkNetworkHealth(context.Background())
	delete(dock.execPingErr, depID)
	g.checkNetworkHealth(context.Background())
	dock.execPingErr[depID] = pingFailedErr()
	g.checkNetworkHealth(context.Background())
	g.checkNetworkHealth(context.Background())

	dock.mu.Lock()
	defer dock.mu.Unlock()
	if len(dock.restartCalls) != 0 {
		t.Fatalf("expected 0 restarts when failures never reach threshold, got %d", len(dock.restartCalls))
	}
}

// Exec infrastructure errors (container stopping, daemon hiccup) are not
// ping failures and must not count towards the restart threshold. This is
// the race that restarted a deliberately-stopped container in CI.
func TestNetworkHealthInfraErrorNotCounted(t *testing.T) {
	g, dock, _ := newNetworkHealthGuardian(1)
	depID := "dep001234567890abcdef"

	// Baseline.
	g.checkNetworkHealth(context.Background())

	// Infra error, not wrapped in ErrPingFailed.
	dock.execPingErr[depID] = errors.New("exec create: container dep001 is not running")
	for i := 0; i < 3; i++ {
		g.checkNetworkHealth(context.Background())
	}

	dock.mu.Lock()
	defer dock.mu.Unlock()
	if len(dock.restartCalls) != 0 {
		t.Fatalf("expected 0 restarts for infra errors, got %d", len(dock.restartCalls))
	}
}
