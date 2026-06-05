package guardian

import (
	"context"
	"testing"
	"time"

	"github.com/Will-Luck/Docker-Guardian/internal/config"
	"github.com/Will-Luck/Docker-Guardian/internal/docker"
	"github.com/Will-Luck/Docker-Guardian/internal/logging"
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
