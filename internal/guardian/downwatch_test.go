package guardian

import (
	"context"
	"testing"
	"time"

	"github.com/Will-Luck/Docker-Guardian/internal/config"
	"github.com/Will-Luck/Docker-Guardian/internal/docker"
	"github.com/Will-Luck/Docker-Guardian/internal/logging"
	"github.com/moby/moby/api/types/container"
)

func TestHandleEvent_CrashLoopAlertsOnce(t *testing.T) {
	cfg := &config.Config{Interval: 5, RestartLoopThreshold: 5, RestartLoopWindow: 300, DownGrace: 120}
	n := &mockNotifier{}
	clk := newMockClock(time.Unix(1_000_000, 0))
	g := NewWithClock(cfg, newMockDocker(), n, logging.New(false), clk)

	for i := 0; i < 6; i++ {
		clk.Advance(time.Second)
		g.handleEvent(context.Background(), docker.ContainerEvent{
			ContainerID: "radarr-id", ContainerName: "radarr", Action: "die",
		})
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	if len(n.alerts) != 1 {
		t.Fatalf("expected exactly 1 crash-loop alert, got %d: %v", len(n.alerts), n.alerts)
	}
}

func inspectDown(name, policy string, running bool) container.InspectResponse {
	return container.InspectResponse{
		Name:       "/" + name,
		State:      &container.State{Running: running, ExitCode: 1},
		HostConfig: &container.HostConfig{RestartPolicy: container.RestartPolicy{Name: container.RestartPolicyMode(policy)}},
	}
}

func TestCheckDown_AlertsAfterGraceForUnlessStopped(t *testing.T) {
	cfg := &config.Config{Interval: 5, DownGrace: 120, RestartLoopThreshold: 5, RestartLoopWindow: 300}
	n := &mockNotifier{}
	dock := newMockDocker()
	dock.inspectResults["radarr-id"] = inspectDown("radarr", "unless-stopped", false)
	clk := newMockClock(time.Unix(1_000_000, 0))
	g := NewWithClock(cfg, dock, n, logging.New(false), clk)

	g.handleEvent(context.Background(), docker.ContainerEvent{ContainerID: "radarr-id", ContainerName: "radarr", Action: "die"})
	clk.Advance(119 * time.Second)
	g.checkDownContainers(context.Background())
	if got := alertCount(n); got != 0 {
		t.Fatalf("no alert expected before grace, got %d", got)
	}
	clk.Advance(2 * time.Second)
	g.checkDownContainers(context.Background())
	if got := alertCount(n); got != 1 {
		t.Fatalf("expected 1 down alert after grace, got %d", got)
	}
	g.checkDownContainers(context.Background())
	if got := alertCount(n); got != 1 {
		t.Fatalf("expected still 1 down alert (episode), got %d", got)
	}
}

func TestCheckDown_RecoveryBeforeGraceNoAlert(t *testing.T) {
	cfg := &config.Config{Interval: 5, DownGrace: 120, RestartLoopThreshold: 5, RestartLoopWindow: 300}
	n := &mockNotifier{}
	g := NewWithClock(cfg, newMockDocker(), n, logging.New(false), newMockClock(time.Unix(1_000_000, 0)))
	g.handleEvent(context.Background(), docker.ContainerEvent{ContainerID: "radarr-id", ContainerName: "radarr", Action: "die"})
	g.handleEvent(context.Background(), docker.ContainerEvent{ContainerID: "radarr-id", ContainerName: "radarr", Action: "start"})
	g.checkDownContainers(context.Background())
	if got := alertCount(n); got != 0 {
		t.Fatalf("recovery before grace must not alert, got %d", got)
	}
}

func TestCheckDown_RestartNoNotWatched(t *testing.T) {
	cfg := &config.Config{Interval: 5, DownGrace: 120, RestartLoopThreshold: 5, RestartLoopWindow: 300}
	n := &mockNotifier{}
	dock := newMockDocker()
	dock.inspectResults["backup-id"] = inspectDown("backup-all", "no", false)
	clk := newMockClock(time.Unix(1_000_000, 0))
	g := NewWithClock(cfg, dock, n, logging.New(false), clk)
	g.handleEvent(context.Background(), docker.ContainerEvent{ContainerID: "backup-id", ContainerName: "backup-all", Action: "die"})
	clk.Advance(200 * time.Second)
	g.checkDownContainers(context.Background())
	if got := alertCount(n); got != 0 {
		t.Fatalf("restart=no container must not alert unless in EXPECTED_UP, got %d", got)
	}
}

func alertCount(n *mockNotifier) int {
	n.mu.Lock()
	defer n.mu.Unlock()
	return len(n.alerts)
}
