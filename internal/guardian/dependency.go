package guardian

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// handleCascadeRestart restarts all running containers sharing a network namespace
// with the given parent container. Called when a parent's "start" event fires.
func (g *Guardian) handleCascadeRestart(ctx context.Context, parentID string) {
	if !g.cfg.CascadeRestart || !g.cfg.MonitorDependencies {
		return
	}

	deps, err := g.docker.DependentsOf(ctx, parentID)
	if err != nil {
		g.log.Error("cascade: failed to query dependents", "parent", parentID, "error", err)
		return
	}
	if len(deps) == 0 {
		return
	}

	// Log what we found
	names := make([]string, len(deps))
	for i, d := range deps {
		names[i] = strings.TrimPrefix(d.Names[0], "/")
	}
	g.log.Info("cascade: parent restarted, restarting dependents",
		"parent", parentID, "dependents", names, "count", len(deps))

	// Settle delay: wait for parent's network to stabilise
	if g.cfg.CascadeSettleDelay > 0 {
		g.log.Debug("cascade: waiting for parent to settle", "delay", g.cfg.CascadeSettleDelay)
		select {
		case <-ctx.Done():
			return
		case <-g.clock.After(time.Duration(g.cfg.CascadeSettleDelay) * time.Second):
		}
	}

	for _, dep := range deps {
		name := strings.TrimPrefix(dep.Names[0], "/")
		shortID := dep.ID[:12]

		if g.shouldSkip(ctx, dep.ID, name, dep.Labels) {
			continue
		}

		if allowed, reason := g.tracker.ShouldRestart(dep.ID); !allowed {
			msg := g.tracker.FormatSkipReason(dep.ID, name, reason)
			now := g.clock.Now().Format("02-01-2006 15:04:05")
			fmt.Printf("%s cascade: %s\n", now, msg)
			continue
		}

		timeout := g.cfg.DefaultStopTimeout
		now := g.clock.Now().Format("02-01-2006 15:04:05")
		fmt.Printf("%s cascade: restarting dependent %s (%s) with %ds timeout\n",
			now, name, shortID, timeout)

		if err := g.docker.RestartContainer(ctx, dep.ID, timeout); err != nil {
			g.log.Error("cascade: restart failed", "name", name, "id", shortID, "error", err)
			g.notifier.Action(fmt.Sprintf("Cascade: container %s (%s) restart failed after parent %s restarted",
				name, shortID, parentID[:12]))
			continue
		}

		g.tracker.RecordRestart(dep.ID)
		g.notifier.Action(fmt.Sprintf("Cascade: container %s (%s) restarted after parent %s restarted",
			name, shortID, parentID[:12]))
		fmt.Printf("%s cascade: restarted %s (%s)\n", now, name, shortID)
		g.runPostRestartScript(name, shortID, "cascade", timeout)
	}
}

// checkNetworkHealth probes containers that share a network namespace to
// verify they still have working connectivity. Safety net for cases where
// the event-driven cascade misses a parent restart.
func (g *Guardian) checkNetworkHealth(ctx context.Context) {
	if !g.cfg.NetworkHealthcheck || !g.cfg.MonitorDependencies {
		return
	}

	running, err := g.docker.RunningContainers(ctx)
	if err != nil {
		g.log.Error("network health: failed to list containers", "error", err)
		return
	}

	for _, c := range running {
		info, err := g.docker.InspectContainer(ctx, c.ID)
		if err != nil {
			continue
		}
		mode := string(info.HostConfig.NetworkMode)
		if !strings.HasPrefix(mode, "container:") {
			continue
		}

		name := strings.TrimPrefix(info.Name, "/")
		shortID := c.ID[:12]

		// Probe network connectivity
		if err := g.docker.ExecPing(ctx, c.ID, g.cfg.NetworkHealthcheckTarget); err != nil {
			g.log.Warn("network health: ping failed, restarting",
				"name", name, "target", g.cfg.NetworkHealthcheckTarget, "error", err)

			if g.shouldSkip(ctx, c.ID, name, info.Config.Labels) {
				continue
			}

			if allowed, reason := g.tracker.ShouldRestart(c.ID); !allowed {
				msg := g.tracker.FormatSkipReason(c.ID, name, reason)
				now := g.clock.Now().Format("02-01-2006 15:04:05")
				fmt.Printf("%s network health: %s\n", now, msg)
				continue
			}

			timeout := g.cfg.DefaultStopTimeout
			now := g.clock.Now().Format("02-01-2006 15:04:05")
			fmt.Printf("%s network health: restarting %s (%s) with %ds timeout\n",
				now, name, shortID, timeout)

			if err := g.docker.RestartContainer(ctx, c.ID, timeout); err != nil {
				g.log.Error("network health: restart failed", "name", name, "id", shortID, "error", err)
				g.notifier.Action(fmt.Sprintf("Network health: container %s (%s) restart failed (ping to %s failed)",
					name, shortID, g.cfg.NetworkHealthcheckTarget))
				continue
			}

			g.tracker.RecordRestart(c.ID)
			g.notifier.Action(fmt.Sprintf("Network health: container %s (%s) restarted (ping to %s failed)",
				name, shortID, g.cfg.NetworkHealthcheckTarget))
			fmt.Printf("%s network health: restarted %s (%s)\n", now, name, shortID)
			g.runPostRestartScript(name, shortID, "network-health", timeout)
		}
	}
}

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

		now := g.clock.Now().Format("02-01-2006 15:04:05")
		fmt.Printf("%s Container %s (%s) exited (code %d, orphaned dependent) - parent %s is running\n",
			now, name, shortID, exitCode, parentID[:12])

		if g.cfg.DependencyStartDelay > 0 {
			fmt.Printf("%s Waiting %ds before starting %s...\n", now, g.cfg.DependencyStartDelay, name)

			select {
			case <-time.After(time.Duration(g.cfg.DependencyStartDelay) * time.Second):
			case <-ctx.Done():
				return
			}

			// Re-check parent
			parentStatus, err = g.docker.ContainerStatus(ctx, parentID)
			if err != nil || parentStatus != "running" {
				fmt.Printf("%s Parent %s no longer running after delay - skipping %s\n", now, parentID[:12], name)
				continue
			}
		}

		// Re-check container hasn't auto-recovered
		currentStatus, err := g.docker.ContainerStatus(ctx, c.ID)
		if err == nil && currentStatus != "exited" {
			fmt.Printf("%s Container %s (%s) is now %s - no action needed\n", now, name, shortID, currentStatus)
			continue
		}

		fmt.Printf("%s Starting orphaned dependent %s (%s)...\n", now, name, shortID)
		if err := g.docker.StartContainer(ctx, c.ID); err != nil {
			g.log.Error("failed to start container", "container", name, "id", shortID, "error", err)
			g.notifier.Action(fmt.Sprintf("Container %s (%s) orphaned (parent running). Failed to start!", name, shortID))
		} else {
			fmt.Printf("%s Successfully started %s (%s)\n", now, name, shortID)
			g.notifier.Action(fmt.Sprintf("Container %s (%s) orphaned (parent running). Successfully started!", name, shortID))
		}

		g.runPostRestartScript(name, shortID, "orphaned", 0)
	}
}
