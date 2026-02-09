package docker

import (
	"context"
	"io"
	"time"

	"github.com/moby/moby/api/types/events"
	"github.com/moby/moby/client"
)

// ContainerEvents returns container events within a time window.
// If orchestrationOnly is true, only destroy+create events are returned
// (the Watchtower signature).
func (c *Client) ContainerEvents(ctx context.Context, since time.Time, until time.Time, orchestrationOnly bool) ([]events.Message, error) {
	opts := client.EventsListOptions{
		Since:   since.Format(time.RFC3339Nano),
		Until:   until.Format(time.RFC3339Nano),
		Filters: make(client.Filters).Add("type", "container"),
	}
	if orchestrationOnly {
		opts.Filters = opts.Filters.Add("event", "destroy", "create")
	}

	result := c.api.Events(ctx, opts)

	var msgs []events.Message
	for {
		select {
		case msg, ok := <-result.Messages:
			if !ok {
				return msgs, nil
			}
			msgs = append(msgs, msg)
		case err := <-result.Err:
			if err == io.EOF {
				return msgs, nil
			}
			return msgs, err
		}
	}
}
