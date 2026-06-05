package docker

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
)

// ErrPingFailed marks a ping probe that ran and exited non-zero, as opposed
// to exec infrastructure errors (container stopping, missing binary, daemon
// hiccup). Callers use errors.Is to tell genuine connectivity failures apart
// from probes that never ran.
var ErrPingFailed = errors.New("ping failed")

// UnhealthyContainers returns containers with health status "unhealthy",
// optionally filtered by label and running status.
func (c *Client) UnhealthyContainers(ctx context.Context, label string, onlyRunning bool) ([]container.Summary, error) {
	opts := client.ContainerListOptions{
		Filters: make(client.Filters).Add("health", "unhealthy"),
	}
	if label != "all" {
		opts.Filters = opts.Filters.Add("label", label+"=true")
	}
	if onlyRunning {
		opts.Filters = opts.Filters.Add("status", "running")
	}
	result, err := c.api.ContainerList(ctx, opts)
	if err != nil {
		return nil, err
	}
	return result.Items, nil
}

// ExitedContainers returns all containers with status "exited".
func (c *Client) ExitedContainers(ctx context.Context) ([]container.Summary, error) {
	opts := client.ContainerListOptions{
		All:     true,
		Filters: make(client.Filters).Add("status", "exited"),
	}
	result, err := c.api.ContainerList(ctx, opts)
	if err != nil {
		return nil, err
	}
	return result.Items, nil
}

// RunningContainers returns all containers with status "running".
func (c *Client) RunningContainers(ctx context.Context) ([]container.Summary, error) {
	opts := client.ContainerListOptions{
		Filters: make(client.Filters).Add("status", "running"),
	}
	result, err := c.api.ContainerList(ctx, opts)
	if err != nil {
		return nil, err
	}
	return result.Items, nil
}

// InspectContainer returns full container details by ID.
func (c *Client) InspectContainer(ctx context.Context, id string) (container.InspectResponse, error) {
	result, err := c.api.ContainerInspect(ctx, id, client.ContainerInspectOptions{})
	if err != nil {
		return container.InspectResponse{}, err
	}
	return result.Container, nil
}

// RestartContainer restarts a container with the given timeout.
func (c *Client) RestartContainer(ctx context.Context, id string, timeout int) error {
	_, err := c.api.ContainerRestart(ctx, id, client.ContainerRestartOptions{Timeout: &timeout})
	return err
}

// StartContainer starts a stopped container.
func (c *Client) StartContainer(ctx context.Context, id string) error {
	_, err := c.api.ContainerStart(ctx, id, client.ContainerStartOptions{})
	return err
}

// StopContainer stops a running container with the given timeout.
func (c *Client) StopContainer(ctx context.Context, id string, timeout int) error {
	_, err := c.api.ContainerStop(ctx, id, client.ContainerStopOptions{Timeout: &timeout})
	return err
}

// ContainerStatus returns the current status string of a container.
func (c *Client) ContainerStatus(ctx context.Context, id string) (string, error) {
	info, err := c.api.ContainerInspect(ctx, id, client.ContainerInspectOptions{})
	if err != nil {
		return "", err
	}
	return string(info.Container.State.Status), nil
}

// ContainerHealthLog returns the output from the last healthcheck log entry.
// Returns empty string if no health log is available.
func (c *Client) ContainerHealthLog(ctx context.Context, id string) (string, error) {
	info, err := c.api.ContainerInspect(ctx, id, client.ContainerInspectOptions{})
	if err != nil {
		return "", err
	}
	health := info.Container.State.Health
	if health == nil || len(health.Log) == 0 {
		return "", nil
	}
	output := strings.TrimSpace(health.Log[len(health.Log)-1].Output)
	if len(output) > 200 {
		output = output[:200] + "..."
	}
	return output, nil
}

// ContainerFinishedAt returns when the container last stopped.
func (c *Client) ContainerFinishedAt(ctx context.Context, id string) (time.Time, error) {
	info, err := c.api.ContainerInspect(ctx, id, client.ContainerInspectOptions{})
	if err != nil {
		return time.Time{}, err
	}
	t, err := time.Parse(time.RFC3339Nano, info.Container.State.FinishedAt)
	if err != nil {
		return time.Time{}, err
	}
	return t, nil
}

// DependentsOf returns all running containers whose NetworkMode is
// "container:<parentID>" or "container:<parentName>".
func (c *Client) DependentsOf(ctx context.Context, parentID string) ([]container.Summary, error) {
	return dependentsOf(ctx, c, parentID)
}

// dependentsOf contains the core logic for DependentsOf, accepting an API
// interface so it can be tested with mocks.
func dependentsOf(ctx context.Context, api API, parentID string) ([]container.Summary, error) {
	running, err := api.RunningContainers(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing running containers: %w", err)
	}

	// Resolve parent name for matching (NetworkMode can use either ID or name).
	info, err := api.InspectContainer(ctx, parentID)
	if err != nil {
		return nil, fmt.Errorf("inspecting parent %s: %w", parentID, err)
	}
	parentName := strings.TrimPrefix(info.Name, "/")

	var deps []container.Summary
	for _, c2 := range running {
		if c2.ID == parentID {
			continue
		}
		inspect, err := api.InspectContainer(ctx, c2.ID)
		if err != nil {
			continue // container may have disappeared
		}
		mode := string(inspect.HostConfig.NetworkMode)
		if mode == "container:"+parentID || mode == "container:"+parentName {
			deps = append(deps, c2)
		}
	}
	return deps, nil
}

// ExecPing runs "ping -c1 -W3 <target>" inside the container.
// Returns nil if the ping succeeds, error otherwise.
func (c *Client) ExecPing(ctx context.Context, containerID string, target string) error {
	resp, err := c.api.ExecCreate(ctx, containerID, client.ExecCreateOptions{
		Cmd:          []string{"ping", "-c", "1", "-W", "3", target},
		AttachStdout: false,
		AttachStderr: false,
	})
	if err != nil {
		return fmt.Errorf("exec create: %w", err)
	}

	if _, err := c.api.ExecStart(ctx, resp.ID, client.ExecStartOptions{Detach: true}); err != nil {
		return fmt.Errorf("exec start: %w", err)
	}

	// Poll for completion and check exit code.
	for {
		inspect, err := c.api.ExecInspect(ctx, resp.ID, client.ExecInspectOptions{})
		if err != nil {
			return fmt.Errorf("exec inspect: %w", err)
		}
		if !inspect.Running {
			if inspect.ExitCode != 0 {
				return fmt.Errorf("%w: ping exited with code %d", ErrPingFailed, inspect.ExitCode)
			}
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
}
