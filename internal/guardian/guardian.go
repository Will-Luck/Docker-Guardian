package guardian

import (
	"context"
	"sync"
	"time"

	"github.com/Will-Luck/Docker-Guardian/internal/clock"
	"github.com/Will-Luck/Docker-Guardian/internal/config"
	"github.com/Will-Luck/Docker-Guardian/internal/docker"
	"github.com/Will-Luck/Docker-Guardian/internal/logging"
	"github.com/Will-Luck/Docker-Guardian/internal/notify"
	"github.com/moby/moby/api/types/events"
)

// Guardian orchestrates container health monitoring.
type Guardian struct {
	cfg      *config.Config
	docker   docker.API
	notifier notify.Notifier
	log      *logging.Logger
	clock    clock.Clock

	// Circuit breaker
	tracker *RestartTracker

	// Event debouncing
	debounceMu     sync.Mutex
	debounceTimers map[string]*time.Timer

	// Orchestration tracking (event-driven replacement for per-cycle cache)
	orchestrationMu     sync.Mutex
	orchestrationEvents map[string]time.Time // container name → latest event time

	// Per-cycle caches (used during full scans)
	orchestratorEvents []events.Message
	orchestratorCached bool
	backupRunning      *bool
	cycle              int
}

// New creates a Guardian instance.
func New(cfg *config.Config, client docker.API, notifier notify.Notifier, log *logging.Logger) *Guardian {
	clk := clock.Real{}
	tcfg := TrackerConfig{
		BackoffMultiplier: cfg.BackoffMultiplier,
		BackoffMax:        time.Duration(cfg.BackoffMax) * time.Second,
		BackoffResetAfter: time.Duration(cfg.BackoffResetAfter) * time.Second,
		RestartBudget:     cfg.RestartBudget,
		RestartWindow:     time.Duration(cfg.RestartWindow) * time.Second,
	}
	return &Guardian{
		cfg:                 cfg,
		docker:              client,
		notifier:            notifier,
		log:                 log,
		clock:               clk,
		tracker:             NewRestartTracker(tcfg, clk),
		debounceTimers:      make(map[string]*time.Timer),
		orchestrationEvents: make(map[string]time.Time),
	}
}

// NewWithClock creates a Guardian with a custom clock (for testing).
func NewWithClock(cfg *config.Config, client docker.API, notifier notify.Notifier, log *logging.Logger, clk clock.Clock) *Guardian {
	g := New(cfg, client, notifier, log)
	g.clock = clk
	g.tracker.clock = clk
	return g
}

// Run starts the event-driven monitoring loop.
// If a Watcher is available (via docker.Client), it uses the event stream.
// Otherwise, it falls back to the polling loop for compatibility.
func (g *Guardian) Run(ctx context.Context) error {
	// Check if we can get a watcher
	if client, ok := g.docker.(*docker.Client); ok {
		return g.runEventDriven(ctx, client)
	}
	// Fallback to polling (for tests with mock docker)
	return g.runPolling(ctx)
}

func (g *Guardian) runEventDriven(ctx context.Context, client *docker.Client) error {
	watcher := docker.NewWatcher(client)
	eventCh := watcher.Watch(ctx)

	// Initial full scan on startup
	g.fullScan(ctx)

	for {
		select {
		case evt, ok := <-eventCh:
			if !ok {
				return nil // watcher closed (context cancelled)
			}
			g.handleEvent(ctx, evt)
		case <-ctx.Done():
			return nil
		}
	}
}

func (g *Guardian) runPolling(ctx context.Context) error {
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

// fullScan does a complete check of all containers.
// Called on startup and after event stream reconnection.
func (g *Guardian) fullScan(ctx context.Context) {
	g.cycle++
	g.orchestratorCached = false
	g.backupRunning = nil

	g.checkUnhealthy(ctx)
	g.checkDependencyOrphans(ctx)
}

// handleEvent processes a single Docker event with debouncing.
func (g *Guardian) handleEvent(ctx context.Context, evt docker.ContainerEvent) {
	switch evt.Action {
	case "health_status":
		if evt.HealthStatus == "unhealthy" {
			g.debounce(ctx, evt.ContainerID, func() {
				g.checkContainerByID(ctx, evt.ContainerID)
			})
		} else if evt.HealthStatus == "healthy" {
			g.tracker.Reset(evt.ContainerID)
		}

	case "die":
		g.debounce(ctx, "dep:"+evt.ContainerID, func() {
			g.checkOrphanedDependents(ctx, evt.ContainerID)
		})

	case "create", "destroy":
		g.recordOrchestrationActivity(evt)

	case "start":
		// No action needed — tracked for potential future use
	}
}

// debounce ensures only one action per container within the debounce window.
func (g *Guardian) debounce(ctx context.Context, key string, fn func()) {
	debounceWindow := time.Duration(g.cfg.Interval) * time.Second
	if debounceWindow <= 0 {
		debounceWindow = 5 * time.Second
	}

	g.debounceMu.Lock()
	if timer, ok := g.debounceTimers[key]; ok {
		timer.Stop()
	}
	g.debounceTimers[key] = time.AfterFunc(debounceWindow, func() {
		if ctx.Err() == nil {
			fn()
		}
		g.debounceMu.Lock()
		delete(g.debounceTimers, key)
		g.debounceMu.Unlock()
	})
	g.debounceMu.Unlock()
}

// checkContainerByID inspects and potentially restarts a single container.
func (g *Guardian) checkContainerByID(ctx context.Context, containerID string) {
	// Use the regular unhealthy check — it re-queries and filters
	g.backupRunning = nil
	g.orchestratorCached = false
	g.checkUnhealthy(ctx)
}

// checkOrphanedDependents checks if any dependents of the given container need starting.
func (g *Guardian) checkOrphanedDependents(ctx context.Context, _ string) {
	g.backupRunning = nil
	g.orchestratorCached = false
	g.checkDependencyOrphans(ctx)
}

// recordOrchestrationActivity records a create/destroy event for orchestration tracking.
func (g *Guardian) recordOrchestrationActivity(evt docker.ContainerEvent) {
	g.orchestrationMu.Lock()
	g.orchestrationEvents[evt.ContainerName] = evt.Timestamp
	g.orchestrationMu.Unlock()

	// Clean up old entries
	go g.pruneOrchestrationEvents()
}

func (g *Guardian) pruneOrchestrationEvents() {
	cutoff := g.clock.Now().Add(-time.Duration(g.cfg.WatchtowerCooldown) * time.Second)
	g.orchestrationMu.Lock()
	for name, ts := range g.orchestrationEvents {
		if ts.Before(cutoff) {
			delete(g.orchestrationEvents, name)
		}
	}
	g.orchestrationMu.Unlock()
}

// EventStreamConnected returns whether we're using event-driven mode.
// Used by metrics.
func (g *Guardian) EventStreamConnected() bool {
	_, ok := g.docker.(*docker.Client)
	return ok
}

// Tracker returns the restart tracker (for metrics).
func (g *Guardian) Tracker() *RestartTracker {
	return g.tracker
}

// UnhealthyCount returns the count from the last check (for metrics).
// This is a simple accessor — the real metric instrumentation happens in Phase 5.
func (g *Guardian) UnhealthyCount(ctx context.Context) int {
	containers, err := g.docker.UnhealthyContainers(ctx, g.cfg.ContainerLabel, g.cfg.OnlyMonitorRunning)
	if err != nil {
		return 0
	}
	return len(containers)
}
