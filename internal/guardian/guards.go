package guardian

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
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
				now := time.Now().Format("02-01-2006 15:04:05")
				fmt.Printf("%s Container %s (%s) affected by orchestration activity within %ds - skipping\n",
					now, cleanName, shortID, g.cfg.WatchtowerCooldown)
				g.dispatcher.Skip(fmt.Sprintf("Container %s (%s) skipped - orchestration activity", cleanName, shortID))
				return true
			}
		} else {
			if g.isOrchestratorActive() {
				now := time.Now().Format("02-01-2006 15:04:05")
				fmt.Printf("%s Container %s (%s) skipped - orchestration activity detected within %ds\n",
					now, cleanName, shortID, g.cfg.WatchtowerCooldown)
				g.dispatcher.Skip(fmt.Sprintf("Container %s (%s) skipped - orchestration activity", cleanName, shortID))
				return true
			}
		}
	}

	// Grace period
	if g.cfg.GracePeriod > 0 {
		finishedAt, err := g.docker.ContainerFinishedAt(ctx, containerID)
		if err == nil {
			age := time.Since(finishedAt)
			if age < time.Duration(g.cfg.GracePeriod)*time.Second {
				now := time.Now().Format("02-01-2006 15:04:05")
				fmt.Printf("%s Container %s (%s) stopped within grace period (%ds) - skipping\n",
					now, cleanName, shortID, g.cfg.GracePeriod)
				g.dispatcher.Skip(fmt.Sprintf("Container %s (%s) skipped - grace period", cleanName, shortID))
				return true
			}
		}
	}

	// Backup awareness
	if g.isBackupManaged(labels) && g.cachedBackupRunning(ctx) {
		now := time.Now().Format("02-01-2006 15:04:05")
		fmt.Printf("%s Container %s (%s) managed by backup (currently running) - skipping\n",
			now, cleanName, shortID)
		g.dispatcher.Skip(fmt.Sprintf("Container %s (%s) skipped - backup running", cleanName, shortID))
		return true
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

	now := time.Now()
	since := now.Add(-time.Duration(g.cfg.WatchtowerCooldown) * time.Second)
	orchestrationOnly := g.cfg.WatchtowerEvents != "all"

	events, err := g.docker.ContainerEvents(ctx, since, now, orchestrationOnly)
	if err != nil {
		return
	}
	g.orchestratorEvents = events

	if len(events) > 0 {
		dateStr := time.Now().Format("02-01-2006 15:04:05")
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

// cachedBackupRunning returns true if a backup container is currently running (cached per cycle).
func (g *Guardian) cachedBackupRunning(ctx context.Context) bool {
	if g.backupRunning != nil {
		return *g.backupRunning
	}

	result := g.checkBackupRunning(ctx)
	g.backupRunning = &result
	return result
}

func (g *Guardian) checkBackupRunning(ctx context.Context) bool {
	running, err := g.docker.RunningContainers(ctx)
	if err != nil {
		return false
	}

	for _, c := range running {
		if g.cfg.BackupContainer != "" {
			for _, name := range c.Names {
				if strings.Contains(name, g.cfg.BackupContainer) {
					return true
				}
			}
		} else {
			if strings.Contains(c.Image, "docker-volume-backup") {
				return true
			}
		}
	}
	return false
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
