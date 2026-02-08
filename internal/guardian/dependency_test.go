package guardian

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Will-Luck/Docker-Guardian/internal/config"
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
