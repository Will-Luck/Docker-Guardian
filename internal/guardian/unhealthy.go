package guardian

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Will-Luck/Docker-Guardian/internal/metrics"
)

// containerAction returns the action to take for a container based on its labels.
// Possible values: "restart" (default), "stop", "notify", "none".
func containerAction(labels map[string]string) string {
	if action, ok := labels["autoheal.action"]; ok {
		switch action {
		case "restart", "stop", "notify", "none":
			return action
		}
	}
	return "restart"
}

// checkUnhealthy finds unhealthy containers and handles them based on action labels.
func (g *Guardian) checkUnhealthy(ctx context.Context) {
	containers, err := g.docker.UnhealthyContainers(ctx, g.cfg.ContainerLabel, g.cfg.OnlyMonitorRunning)
	if err != nil {
		g.log.Error("failed to list unhealthy containers", "error", err)
		return
	}

	metrics.UnhealthyContainers.Set(float64(len(containers)))
	metrics.CircuitOpenContainers.Set(float64(g.tracker.CircuitOpenCount()))

	for _, c := range containers {
		// Skip containers opted out via label
		if c.Labels["autoheal"] == "False" {
			continue
		}

		if len(c.Names) == 0 {
			continue
		}

		id := c.ID
		shortID := id[:12]
		name := strings.TrimPrefix(c.Names[0], "/")

		// Check per-container action label
		action := containerAction(c.Labels)
		if action == "none" {
			continue
		}

		if string(c.State) == "restarting" {
			now := g.clock.Now().Format("02-01-2006 15:04:05")
			fmt.Printf("%s Container %s (%s) found to be restarting - don't restart\n", now, name, shortID)
			continue
		}

		if g.shouldSkip(ctx, id, name, c.Labels) {
			continue
		}

		// Handle notify-only action
		if action == "notify" {
			g.notifier.Action(fmt.Sprintf("Container %s (%s) found to be unhealthy (action=notify)", name, shortID))
			continue
		}

		// Circuit breaker check (for restart and stop actions)
		if allowed, reason := g.tracker.ShouldRestart(id); !allowed {
			msg := g.tracker.FormatSkipReason(id, name, reason)
			now := g.clock.Now().Format("02-01-2006 15:04:05")
			fmt.Printf("%s %s\n", now, msg)
			metrics.SkipsTotal.WithLabelValues(name, string(reason)).Inc()
			if reason == SkipCircuit {
				g.notifier.Action(fmt.Sprintf("[CRITICAL] %s", msg))
			}
			continue
		}

		timeout := g.cfg.DefaultStopTimeout
		if v, ok := c.Labels["autoheal.stop.timeout"]; ok {
			if parsed, err := strconv.Atoi(v); err == nil {
				timeout = parsed
			}
		}

		// Handle stop action (quarantine)
		if action == "stop" {
			now := g.clock.Now().Format("02-01-2006 15:04:05")
			fmt.Printf("%s Container %s (%s) found to be unhealthy - Stopping container (action=stop)\n", now, name, shortID)
			if err := g.docker.StopContainer(ctx, id, timeout); err != nil {
				g.log.Error("failed to stop container", "container", name, "id", shortID, "error", err)
				g.notifier.Action(fmt.Sprintf("Container %s (%s) found to be unhealthy. Failed to stop (quarantine)!", name, shortID))
				metrics.RestartsTotal.WithLabelValues(name, "failure").Inc()
			} else {
				g.notifier.Action(fmt.Sprintf("Container %s (%s) found to be unhealthy. Stopped (quarantined).", name, shortID))
				metrics.RestartsTotal.WithLabelValues(name, "success").Inc()
			}
			g.tracker.RecordRestart(id)
			continue
		}

		// Default: restart
		now := g.clock.Now().Format("02-01-2006 15:04:05")
		fmt.Printf("%s Container %s (%s) found to be unhealthy - Restarting container now with %ds timeout\n",
			now, name, shortID, timeout)

		start := time.Now()
		if err := g.docker.RestartContainer(ctx, id, timeout); err != nil {
			g.log.Error("failed to restart container", "container", name, "id", shortID, "error", err)
			g.notifier.Action(fmt.Sprintf("Container %s (%s) found to be unhealthy. Failed to restart the container!", name, shortID))
			metrics.RestartsTotal.WithLabelValues(name, "failure").Inc()
		} else {
			g.notifier.Action(fmt.Sprintf("Container %s (%s) found to be unhealthy. Successfully restarted the container!", name, shortID))
			metrics.RestartsTotal.WithLabelValues(name, "success").Inc()
		}
		metrics.RestartDuration.WithLabelValues(name).Observe(time.Since(start).Seconds())

		g.tracker.RecordRestart(id)
		g.runPostRestartScript(name, shortID, string(c.State), timeout)
	}
}
