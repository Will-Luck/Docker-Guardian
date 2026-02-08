package guardian

import (
	"context"
	"testing"
	"time"

	"github.com/Will-Luck/Docker-Guardian/internal/config"
	"github.com/Will-Luck/Docker-Guardian/internal/logging"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/events"
)

func newTestGuardian(cfg *config.Config, dock *mockDocker, notif *mockNotifier, clk *mockClock) *Guardian {
	return &Guardian{
		cfg:      cfg,
		docker:   dock,
		notifier: notif,
		log:      logging.New(false),
		clock:    clk,
		tracker:  NewRestartTracker(DefaultTrackerConfig(), clk),
	}
}

func TestShouldSkip_NoGuards(t *testing.T) {
	cfg := &config.Config{
		WatchtowerCooldown: 0,
		GracePeriod:        0,
		BackupLabel:        "",
	}
	dock := newMockDocker()
	notif := &mockNotifier{}
	clk := newMockClock(time.Now())

	g := newTestGuardian(cfg, dock, notif, clk)
	if g.shouldSkip(context.Background(), "abcdef123456", "test-container", nil) {
		t.Error("should not skip when no guards are configured")
	}
}

func TestShouldSkip_GracePeriod(t *testing.T) {
	now := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	clk := newMockClock(now)

	cfg := &config.Config{
		GracePeriod:        60,
		WatchtowerCooldown: 0,
		BackupLabel:        "",
	}
	dock := newMockDocker()
	notif := &mockNotifier{}

	// Container stopped 30s ago — within grace period
	dock.finishedAtResults["abcdef123456"] = now.Add(-30 * time.Second)

	g := newTestGuardian(cfg, dock, notif, clk)
	if !g.shouldSkip(context.Background(), "abcdef123456", "test-container", nil) {
		t.Error("should skip container within grace period")
	}
	if len(notif.skips) != 1 {
		t.Errorf("expected 1 skip notification, got %d", len(notif.skips))
	}
}

func TestShouldSkip_GracePeriodExpired(t *testing.T) {
	now := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	clk := newMockClock(now)

	cfg := &config.Config{
		GracePeriod:        60,
		WatchtowerCooldown: 0,
		BackupLabel:        "",
	}
	dock := newMockDocker()
	notif := &mockNotifier{}

	// Container stopped 90s ago — outside grace period
	dock.finishedAtResults["abcdef123456"] = now.Add(-90 * time.Second)

	g := newTestGuardian(cfg, dock, notif, clk)
	if g.shouldSkip(context.Background(), "abcdef123456", "test-container", nil) {
		t.Error("should not skip container outside grace period")
	}
}

func TestShouldSkip_OrchestrationAll(t *testing.T) {
	cfg := &config.Config{
		WatchtowerCooldown: 300,
		WatchtowerScope:    "all",
		WatchtowerEvents:   "orchestration",
		GracePeriod:        0,
		BackupLabel:        "",
	}
	dock := newMockDocker()
	notif := &mockNotifier{}
	clk := newMockClock(time.Now())

	dock.containerEvents = []events.Message{{Action: "create"}}

	g := newTestGuardian(cfg, dock, notif, clk)
	if !g.shouldSkip(context.Background(), "abcdef123456", "test-container", nil) {
		t.Error("should skip when orchestration events exist with scope=all")
	}
}

func TestShouldSkip_OrchestrationAffected(t *testing.T) {
	cfg := &config.Config{
		WatchtowerCooldown: 300,
		WatchtowerScope:    "affected",
		WatchtowerEvents:   "orchestration",
		GracePeriod:        0,
		BackupLabel:        "",
	}
	dock := newMockDocker()
	notif := &mockNotifier{}
	clk := newMockClock(time.Now())

	// Event for a different container
	dock.containerEvents = []events.Message{
		{Action: "create", Actor: events.Actor{Attributes: map[string]string{"name": "other-container"}}},
	}

	g := newTestGuardian(cfg, dock, notif, clk)

	// test-container should NOT be skipped — it's not in the events
	if g.shouldSkip(context.Background(), "abcdef123456", "test-container", nil) {
		t.Error("should not skip unaffected container with scope=affected")
	}

	// Reset cache for next check
	g.orchestratorCached = false

	// Now check the affected container
	if !g.shouldSkip(context.Background(), "bbbbbb123456", "other-container", nil) {
		t.Error("should skip affected container with scope=affected")
	}
}

func TestShouldSkip_BackupRunning(t *testing.T) {
	cfg := &config.Config{
		WatchtowerCooldown: 0,
		GracePeriod:        0,
		BackupLabel:        "docker-volume-backup.stop-during-backup",
		BackupContainer:    "my-backup",
	}
	dock := newMockDocker()
	notif := &mockNotifier{}
	clk := newMockClock(time.Now())

	// Backup container running
	dock.runningContainers = []container.Summary{
		{Names: []string{"/my-backup"}},
	}

	labels := map[string]string{
		"docker-volume-backup.stop-during-backup": "true",
	}

	g := newTestGuardian(cfg, dock, notif, clk)
	if !g.shouldSkip(context.Background(), "abcdef123456", "test-container", labels) {
		t.Error("should skip backup-managed container while backup is running")
	}
}

func TestShouldSkip_BackupNotRunning(t *testing.T) {
	cfg := &config.Config{
		WatchtowerCooldown: 0,
		GracePeriod:        0,
		BackupLabel:        "docker-volume-backup.stop-during-backup",
		BackupContainer:    "my-backup",
	}
	dock := newMockDocker()
	notif := &mockNotifier{}
	clk := newMockClock(time.Now())

	// No backup container running
	dock.runningContainers = []container.Summary{}

	labels := map[string]string{
		"docker-volume-backup.stop-during-backup": "true",
	}

	g := newTestGuardian(cfg, dock, notif, clk)
	if g.shouldSkip(context.Background(), "abcdef123456", "test-container", labels) {
		t.Error("should not skip when backup is not running")
	}
}
