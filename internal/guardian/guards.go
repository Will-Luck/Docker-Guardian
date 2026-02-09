package guardian

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/Will-Luck/Docker-Guardian/internal/metrics"
)

// shouldSkip returns true if this container should be skipped due to
// orchestration activity, grace period, or backup awareness.
func (g *Guardian) shouldSkip(ctx context.Context, containerID, containerName string, labels map[string]string) bool {
	shortID := containerID[:12]
	cleanName := strings.TrimPrefix(containerName, "/")

	// Orchestrator/Watchtower cooldown
	if g.cfg.WatchtowerCooldown > 0 {
		g.fetchOrchestrationEvents(ctx)

		if g.cfg.WatchtowerScope == "affected" {
			if g.isContainerInOrchestration(cleanName) {
				now := g.clock.Now().Format("02-01-2006 15:04:05")
				fmt.Printf("%s Container %s (%s) affected by orchestration activity within %ds - skipping\n",
					now, cleanName, shortID, g.cfg.WatchtowerCooldown)
				g.notifier.Skip(fmt.Sprintf("Container %s (%s) skipped - orchestration activity", cleanName, shortID))
				metrics.SkipsTotal.WithLabelValues(cleanName, "orchestration").Inc()
				return true
			}
		} else {
			if g.isOrchestratorActive() {
				now := g.clock.Now().Format("02-01-2006 15:04:05")
				fmt.Printf("%s Container %s (%s) skipped - orchestration activity detected within %ds\n",
					now, cleanName, shortID, g.cfg.WatchtowerCooldown)
				g.notifier.Skip(fmt.Sprintf("Container %s (%s) skipped - orchestration activity", cleanName, shortID))
				metrics.SkipsTotal.WithLabelValues(cleanName, "orchestration").Inc()
				return true
			}
		}
	}

	// Grace period
	if g.cfg.GracePeriod > 0 {
		finishedAt, err := g.docker.ContainerFinishedAt(ctx, containerID)
		if err == nil {
			age := g.clock.Since(finishedAt)
			if age < time.Duration(g.cfg.GracePeriod)*time.Second {
				now := g.clock.Now().Format("02-01-2006 15:04:05")
				fmt.Printf("%s Container %s (%s) stopped within grace period (%ds) - skipping\n",
					now, cleanName, shortID, g.cfg.GracePeriod)
				g.notifier.Skip(fmt.Sprintf("Container %s (%s) skipped - grace period", cleanName, shortID))
				metrics.SkipsTotal.WithLabelValues(cleanName, "grace").Inc()
				return true
			}
		}
	}

	// Backup awareness — skip containers stopped within backup timeout
	if g.isBackupManaged(labels) && g.cfg.BackupTimeout > 0 {
		finishedAt, err := g.docker.ContainerFinishedAt(ctx, containerID)
		if err == nil {
			age := g.clock.Since(finishedAt)
			if age < time.Duration(g.cfg.BackupTimeout)*time.Second {
				now := g.clock.Now().Format("02-01-2006 15:04:05")
				fmt.Printf("%s Container %s (%s) managed by backup (stopped %s ago, timeout %ds) - skipping\n",
					now, cleanName, shortID, age.Round(time.Second), g.cfg.BackupTimeout)
				g.notifier.Skip(fmt.Sprintf("Container %s (%s) skipped - backup timeout", cleanName, shortID))
				metrics.SkipsTotal.WithLabelValues(cleanName, "backup").Inc()
				return true
			}
		}
	}

	return false
}

// fetchOrchestrationEvents queries Docker events once per cycle and caches the result.
// Also logs a summary line when events are detected.
func (g *Guardian) fetchOrchestrationEvents(ctx context.Context) {
	if g.orchestratorCached {
		return
	}
	g.orchestratorCached = true
	g.orchestratorEvents = nil

	now := g.clock.Now()
	since := now.Add(-time.Duration(g.cfg.WatchtowerCooldown) * time.Second)
	orchestrationOnly := g.cfg.WatchtowerEvents != "all"

	events, err := g.docker.ContainerEvents(ctx, since, now, orchestrationOnly)
	if err != nil {
		return
	}
	g.orchestratorEvents = events

	if len(events) > 0 {
		dateStr := g.clock.Now().Format("02-01-2006 15:04:05")
		fmt.Printf("%s Orchestration activity detected: %d container event(s) within %ds cooldown\n",
			dateStr, len(events), g.cfg.WatchtowerCooldown)
	}
}

// isOrchestratorActive returns true if any orchestration events were found this cycle.
func (g *Guardian) isOrchestratorActive() bool {
	return len(g.orchestratorEvents) > 0
}

// isContainerInOrchestration checks if this specific container had events this cycle.
func (g *Guardian) isContainerInOrchestration(containerName string) bool {
	for _, e := range g.orchestratorEvents {
		if e.Actor.Attributes["name"] == containerName {
			return true
		}
	}
	return false
}

// isBackupManaged returns true if the container has the backup label.
func (g *Guardian) isBackupManaged(labels map[string]string) bool {
	if g.cfg.BackupLabel == "" {
		return false
	}
	_, ok := labels[g.cfg.BackupLabel]
	return ok
}

// runPostRestartScript executes the POST_RESTART_SCRIPT if configured.
func (g *Guardian) runPostRestartScript(containerName, shortID, state string, timeout int) {
	if g.cfg.PostRestartScript == "" {
		return
	}
	go func() {
		cmd := exec.Command(g.cfg.PostRestartScript, containerName, shortID, state, fmt.Sprintf("%d", timeout)) //nolint:gosec // User-configured script path from POST_RESTART_SCRIPT env var
		if err := cmd.Run(); err != nil {
			g.log.Error("post-restart script failed", "error", err)
		}
	}()
}
