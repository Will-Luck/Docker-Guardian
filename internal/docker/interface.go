package docker

import (
	"context"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/events"
)

// API defines the subset of Docker operations used by Guardian.
// Implemented by Client for production, and by mocks for testing.
type API interface {
	UnhealthyContainers(ctx context.Context, label string, onlyRunning bool) ([]container.Summary, error)
	ExitedContainers(ctx context.Context) ([]container.Summary, error)
	RunningContainers(ctx context.Context) ([]container.Summary, error)
	InspectContainer(ctx context.Context, id string) (container.InspectResponse, error)
	RestartContainer(ctx context.Context, id string, timeout int) error
	StartContainer(ctx context.Context, id string) error
	StopContainer(ctx context.Context, id string, timeout int) error
	ContainerStatus(ctx context.Context, id string) (string, error)
	ContainerFinishedAt(ctx context.Context, id string) (time.Time, error)
	ContainerEvents(ctx context.Context, since, until time.Time, orchestrationOnly bool) ([]events.Message, error)
	Close() error
}

// Verify Client implements API at compile time.
var _ API = (*Client)(nil)
