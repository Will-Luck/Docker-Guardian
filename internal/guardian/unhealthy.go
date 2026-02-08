package guardian

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// checkUnhealthy finds unhealthy containers and restarts them.
func (g *Guardian) checkUnhealthy(ctx context.Context) {
	containers, err := g.docker.UnhealthyContainers(ctx, g.cfg.ContainerLabel, g.cfg.OnlyMonitorRunning)
	if err != nil {
		g.log.Error("failed to list unhealthy containers", "error", err)
		return
	}

	for _, c := range containers {
		// Skip containers opted out via label
		if c.Labels["autoheal"] == "False" {
			continue
		}

		id := c.ID
		shortID := id[:12]
		name := strings.TrimPrefix(c.Names[0], "/")

		if c.State == "restarting" {
			g.log.Info("container is restarting, skipping", "container", name, "id", shortID)
			continue
		}

		if g.shouldSkip(ctx, id, name, c.Labels) {
			continue
		}

		timeout := g.cfg.DefaultStopTimeout
		if v, ok := c.Labels["autoheal.stop.timeout"]; ok {
			if parsed, err := strconv.Atoi(v); err == nil {
				timeout = parsed
			}
		}

		now := time.Now().Format("02-01-2006 15:04:05")
		g.log.Info(fmt.Sprintf("%s Container %s (%s) found to be unhealthy - Restarting container now with %ds timeout",
			now, name, shortID, timeout))

		if err := g.docker.RestartContainer(ctx, id, timeout); err != nil {
			g.log.Error("failed to restart container", "container", name, "id", shortID, "error", err)
			g.dispatcher.Action(fmt.Sprintf("Container %s (%s) found to be unhealthy. Failed to restart the container!", name, shortID))
		} else {
			g.dispatcher.Action(fmt.Sprintf("Container %s (%s) found to be unhealthy. Successfully restarted the container!", name, shortID))
		}

		g.runPostRestartScript(name, shortID, string(c.State), timeout)
	}
}
