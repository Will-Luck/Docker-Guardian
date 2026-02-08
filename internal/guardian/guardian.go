package guardian

import (
	"context"
	"time"

	"github.com/Will-Luck/Docker-Guardian/internal/config"
	"github.com/Will-Luck/Docker-Guardian/internal/docker"
	"github.com/Will-Luck/Docker-Guardian/internal/logging"
	"github.com/Will-Luck/Docker-Guardian/internal/notify"
	"github.com/moby/moby/api/types/events"
)

// Guardian orchestrates the main monitoring loop.
type Guardian struct {
	cfg        *config.Config
	docker     *docker.Client
	dispatcher *notify.Dispatcher
	log        *logging.Logger
	cycle      int

	// Per-cycle caches (reset at start of each cycle)
	orchestratorEvents []events.Message
	orchestratorCached bool
	backupRunning      *bool
}

// New creates a Guardian instance.
func New(cfg *config.Config, client *docker.Client, dispatcher *notify.Dispatcher, log *logging.Logger) *Guardian {
	return &Guardian{
		cfg:        cfg,
		docker:     client,
		dispatcher: dispatcher,
		log:        log,
	}
}

// Run starts the main monitoring loop, returning when the context is cancelled.
func (g *Guardian) Run(ctx context.Context) error {
	for {
		g.cycle++
		g.orchestratorCached = false
		g.backupRunning = nil

		g.checkUnhealthy(ctx)
		g.checkDependencyOrphans(ctx)

		select {
		case <-time.After(time.Duration(g.cfg.Interval) * time.Second):
		case <-ctx.Done():
			return nil
		}
	}
}
