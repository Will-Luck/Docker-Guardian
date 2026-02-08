package docker

import (
	"context"
	"strings"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
)

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
