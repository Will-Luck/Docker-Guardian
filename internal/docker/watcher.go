package docker

import (
	"context"
	"time"

	"github.com/moby/moby/api/types/events"
	"github.com/moby/moby/client"
)

// ContainerEvent represents a processed Docker container event.
type ContainerEvent struct {
	ContainerID   string
	ContainerName string
	Action        string // "health_status", "die", "start", "destroy", "create"
	HealthStatus  string // "unhealthy", "healthy" (only for health_status events)
	Timestamp     time.Time
}

// WatcherAPI defines the interface for watching Docker events.
type WatcherAPI interface {
	Watch(ctx context.Context) <-chan ContainerEvent
}

// Watcher subscribes to the Docker event stream and emits ContainerEvents.
type Watcher struct {
	api            *client.Client
	reconnectMax   time.Duration
	livenessWindow time.Duration
}

// NewWatcher creates a Watcher connected to the Docker event stream.
func NewWatcher(c *Client) *Watcher {
	return &Watcher{
		api:            c.api,
		reconnectMax:   30 * time.Second,
		livenessWindow: 60 * time.Second,
	}
}

// Watch starts watching Docker events. It reconnects automatically on disconnect.
// Returns a channel of ContainerEvents. The channel is closed when ctx is cancelled.
func (w *Watcher) Watch(ctx context.Context) <-chan ContainerEvent {
	ch := make(chan ContainerEvent, 64)

	go func() {
		defer close(ch)
		backoff := time.Second

		for {
			if ctx.Err() != nil {
				return
			}

			w.streamEvents(ctx, ch)

			// If we get here, stream disconnected. Reconnect with backoff.
			if ctx.Err() != nil {
				return
			}

			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return
			}

			// Exponential backoff: 1s → 2s → 4s → 8s → ... → 30s max
			backoff *= 2
			if backoff > w.reconnectMax {
				backoff = w.reconnectMax
			}
		}
	}()

	return ch
}

func (w *Watcher) streamEvents(ctx context.Context, ch chan<- ContainerEvent) {
	opts := client.EventsListOptions{
		Filters: make(client.Filters).
			Add("type", "container").
			Add("event", "health_status", "die", "start", "destroy", "create"),
	}

	result := w.api.Events(ctx, opts)

	// Reset backoff on successful connection
	for {
		select {
		case msg, ok := <-result.Messages:
			if !ok {
				return // stream closed
			}
			evt := parseEvent(msg)
			if evt != nil {
				select {
				case ch <- *evt:
				case <-ctx.Done():
					return
				}
			}
		case <-result.Err:
			return // stream error — caller will reconnect
		case <-ctx.Done():
			return
		}
	}
}

func parseEvent(msg events.Message) *ContainerEvent {
	evt := &ContainerEvent{
		ContainerID:   msg.Actor.ID,
		ContainerName: msg.Actor.Attributes["name"],
		Action:        string(msg.Action),
		Timestamp:     time.Unix(msg.Time, msg.TimeNano%1e9),
	}

	// Docker sends health_status events as "health_status: unhealthy" or "health_status: healthy"
	if action := string(msg.Action); len(action) > 15 && action[:14] == "health_status:" {
		evt.Action = "health_status"
		evt.HealthStatus = action[15:] // skip "health_status: "
	} else if action == "health_status" {
		// Some Docker versions use attributes
		evt.HealthStatus = msg.Actor.Attributes["health_status"]
	}

	return evt
}
