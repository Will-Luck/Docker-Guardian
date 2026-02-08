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
		if g.cfg.WatchtowerScope == "affected" {
			if g.isContainerInOrchestration(ctx, cleanName) {
				g.log.Info(fmt.Sprintf("Container %s (%s) affected by orchestration activity - skipping", cleanName, shortID))
				g.dispatcher.Skip(fmt.Sprintf("Container %s (%s) skipped - orchestration activity", cleanName, shortID))
				return true
			}
		} else {
			if g.isOrchestratorActive(ctx) {
				g.log.Info(fmt.Sprintf("Container %s (%s) skipped - orchestration activity detected", cleanName, shortID))
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
				g.log.Info(fmt.Sprintf("Container %s (%s) stopped within grace period (%ds) - skipping", cleanName, shortID, g.cfg.GracePeriod))
				g.dispatcher.Skip(fmt.Sprintf("Container %s (%s) skipped - grace period", cleanName, shortID))
				return true
			}
		}
	}

	// Backup awareness
	if g.isBackupManaged(labels) && g.isBackupRunning(ctx) {
		g.log.Info(fmt.Sprintf("Container %s (%s) managed by backup (currently running) - skipping", cleanName, shortID))
		g.dispatcher.Skip(fmt.Sprintf("Container %s (%s) skipped - backup running", cleanName, shortID))
		return true
	}

	return false
}

// isOrchestratorActive checks if any container lifecycle events occurred
// within the watchtower cooldown window.
func (g *Guardian) isOrchestratorActive(ctx context.Context) bool {
	now := time.Now()
	since := now.Add(-time.Duration(g.cfg.WatchtowerCooldown) * time.Second)
	orchestrationOnly := g.cfg.WatchtowerEvents != "all"

	events, err := g.docker.ContainerEvents(ctx, since, now, orchestrationOnly)
	if err != nil {
		return false
	}
	return len(events) > 0
}

// isContainerInOrchestration checks if this specific container had events
// within the cooldown window.
func (g *Guardian) isContainerInOrchestration(ctx context.Context, containerName string) bool {
	now := time.Now()
	since := now.Add(-time.Duration(g.cfg.WatchtowerCooldown) * time.Second)
	orchestrationOnly := g.cfg.WatchtowerEvents != "all"

	events, err := g.docker.ContainerEvents(ctx, since, now, orchestrationOnly)
	if err != nil {
		return false
	}

	for _, e := range events {
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

// isBackupRunning returns true if a backup container is currently running.
func (g *Guardian) isBackupRunning(ctx context.Context) bool {
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
