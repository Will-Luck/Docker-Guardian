package guardian

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Will-Luck/Docker-Guardian/internal/config"
	"github.com/Will-Luck/Docker-Guardian/internal/logging"
	"github.com/moby/moby/api/types/container"
)

func TestCheckUnhealthy_RestartsContainer(t *testing.T) {
	cfg := &config.Config{
		ContainerLabel:     "all",
		DefaultStopTimeout: 10,
		WatchtowerCooldown: 0,
		GracePeriod:        0,
		BackupLabel:        "",
	}
	dock := newMockDocker()
	notif := &mockNotifier{}
	clk := newMockClock(time.Now())

	dock.unhealthyContainers = []container.Summary{
		{
			ID:     "abcdef1234567890abcdef",
			Names:  []string{"/test-app"},
			State:  "running",
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

	g.checkUnhealthy(context.Background())

	if len(dock.restartCalls) != 1 {
		t.Fatalf("expected 1 restart call, got %d", len(dock.restartCalls))
	}
	if dock.restartCalls[0] != "abcdef1234567890abcdef" {
		t.Errorf("restarted wrong container: %s", dock.restartCalls[0])
	}
	if len(notif.actions) != 1 || !strings.Contains(notif.actions[0], "Successfully restarted") {
		t.Errorf("expected success notification, got %v", notif.actions)
	}
}

func TestCheckUnhealthy_SkipsOptedOut(t *testing.T) {
	cfg := &config.Config{
		ContainerLabel:     "all",
		DefaultStopTimeout: 10,
		WatchtowerCooldown: 0,
		GracePeriod:        0,
		BackupLabel:        "",
	}
	dock := newMockDocker()
	notif := &mockNotifier{}
	clk := newMockClock(time.Now())

	dock.unhealthyContainers = []container.Summary{
		{
			ID:     "abcdef1234567890abcdef",
			Names:  []string{"/opted-out"},
			State:  "running",
			Labels: map[string]string{"autoheal": "False"},
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

	g.checkUnhealthy(context.Background())

	if len(dock.restartCalls) != 0 {
		t.Error("should not restart opted-out container")
	}
}

func TestCheckUnhealthy_SkipsEmptyNames(t *testing.T) {
	cfg := &config.Config{
		ContainerLabel:     "all",
		DefaultStopTimeout: 10,
		WatchtowerCooldown: 0,
		GracePeriod:        0,
		BackupLabel:        "",
	}
	dock := newMockDocker()
	notif := &mockNotifier{}
	clk := newMockClock(time.Now())

	dock.unhealthyContainers = []container.Summary{
		{
			ID:     "abcdef1234567890abcdef",
			Names:  []string{},
			State:  "running",
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

	// Should not panic
	g.checkUnhealthy(context.Background())

	if len(dock.restartCalls) != 0 {
		t.Error("should not restart container with no names")
	}
}

func TestCheckUnhealthy_SkipsRestarting(t *testing.T) {
	cfg := &config.Config{
		ContainerLabel:     "all",
		DefaultStopTimeout: 10,
		WatchtowerCooldown: 0,
		GracePeriod:        0,
		BackupLabel:        "",
	}
	dock := newMockDocker()
	notif := &mockNotifier{}
	clk := newMockClock(time.Now())

	dock.unhealthyContainers = []container.Summary{
		{
			ID:     "abcdef1234567890abcdef",
			Names:  []string{"/restarting-app"},
			State:  "restarting",
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

	g.checkUnhealthy(context.Background())

	if len(dock.restartCalls) != 0 {
		t.Error("should not restart container already in restarting state")
	}
}

func TestCheckUnhealthy_RestartFailure(t *testing.T) {
	cfg := &config.Config{
		ContainerLabel:     "all",
		DefaultStopTimeout: 10,
		WatchtowerCooldown: 0,
		GracePeriod:        0,
		BackupLabel:        "",
	}
	dock := newMockDocker()
	notif := &mockNotifier{}
	clk := newMockClock(time.Now())

	dock.unhealthyContainers = []container.Summary{
		{
			ID:     "abcdef1234567890abcdef",
			Names:  []string{"/fail-app"},
			State:  "running",
			Labels: map[string]string{},
		},
	}
	dock.restartErr["abcdef1234567890abcdef"] = errors.New("restart failed")

	g := &Guardian{
		cfg:      cfg,
		docker:   dock,
		notifier: notif,
		log:      logging.New(false),
		clock:    clk,
		tracker:  NewRestartTracker(DefaultTrackerConfig(), clk),
	}

	g.checkUnhealthy(context.Background())

	if len(notif.actions) != 1 || !strings.Contains(notif.actions[0], "Failed") {
		t.Errorf("expected failure notification, got %v", notif.actions)
	}
}

func TestCheckUnhealthy_CustomTimeout(t *testing.T) {
	cfg := &config.Config{
		ContainerLabel:     "all",
		DefaultStopTimeout: 10,
		WatchtowerCooldown: 0,
		GracePeriod:        0,
		BackupLabel:        "",
	}
	dock := newMockDocker()
	notif := &mockNotifier{}
	clk := newMockClock(time.Now())

	dock.unhealthyContainers = []container.Summary{
		{
			ID:     "abcdef1234567890abcdef",
			Names:  []string{"/custom-timeout"},
			State:  "running",
			Labels: map[string]string{"autoheal.stop.timeout": "30"},
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

	g.checkUnhealthy(context.Background())

	if len(dock.restartCalls) != 1 {
		t.Fatalf("expected 1 restart, got %d", len(dock.restartCalls))
	}
}
