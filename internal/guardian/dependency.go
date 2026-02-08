package guardian

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// checkDependencyOrphans finds exited containers whose parent (via container:X
// network mode) is still running, and starts them.
func (g *Guardian) checkDependencyOrphans(ctx context.Context) {
	if !g.cfg.MonitorDependencies {
		return
	}

	exited, err := g.docker.ExitedContainers(ctx)
	if err != nil {
		g.log.Error("failed to list exited containers", "error", err)
		return
	}

	for _, c := range exited {
		info, err := g.docker.InspectContainer(ctx, c.ID)
		if err != nil {
			continue
		}

		networkMode := string(info.HostConfig.NetworkMode)
		if !strings.HasPrefix(networkMode, "container:") {
			continue
		}

		parentID := strings.TrimPrefix(networkMode, "container:")
		parentStatus, err := g.docker.ContainerStatus(ctx, parentID)
		if err != nil || parentStatus != "running" {
			continue
		}

		shortID := c.ID[:12]
		name := strings.TrimPrefix(info.Name, "/")
		exitCode := info.State.ExitCode
		labels := info.Config.Labels

		if g.shouldSkip(ctx, c.ID, name, labels) {
			continue
		}

		now := time.Now().Format("02-01-2006 15:04:05")
		g.log.Info(fmt.Sprintf("%s Container %s (%s) exited (code %d, orphaned dependent) - parent %s is running",
			now, name, shortID, exitCode, parentID[:12]))

		if g.cfg.DependencyStartDelay > 0 {
			g.log.Info(fmt.Sprintf("%s Waiting %ds before starting %s...", now, g.cfg.DependencyStartDelay, name))

			select {
			case <-time.After(time.Duration(g.cfg.DependencyStartDelay) * time.Second):
			case <-ctx.Done():
				return
			}

			// Re-check parent
			parentStatus, err = g.docker.ContainerStatus(ctx, parentID)
			if err != nil || parentStatus != "running" {
				g.log.Info("parent no longer running after delay, skipping", "container", name, "parent", parentID[:12])
				continue
			}
		}

		// Re-check container hasn't auto-recovered
		currentStatus, err := g.docker.ContainerStatus(ctx, c.ID)
		if err == nil && currentStatus != "exited" {
			g.log.Info(fmt.Sprintf("%s Container %s (%s) is now %s - no action needed", now, name, shortID, currentStatus))
			continue
		}

		g.log.Info(fmt.Sprintf("%s Starting orphaned dependent %s (%s)...", now, name, shortID))
		if err := g.docker.StartContainer(ctx, c.ID); err != nil {
			g.log.Error("failed to start container", "container", name, "id", shortID, "error", err)
			g.dispatcher.Action(fmt.Sprintf("Container %s (%s) orphaned (parent running). Failed to start!", name, shortID))
		} else {
			g.log.Info(fmt.Sprintf("%s Successfully started %s (%s)", now, name, shortID))
			g.dispatcher.Action(fmt.Sprintf("Container %s (%s) orphaned (parent running). Successfully started!", name, shortID))
		}

		g.runPostRestartScript(name, shortID, "orphaned", 0)
	}
}
