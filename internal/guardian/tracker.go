package guardian

import (
	"fmt"
	"sync"
	"time"

	"github.com/Will-Luck/Docker-Guardian/internal/clock"
)

// TrackerConfig holds circuit breaker / backoff settings.
type TrackerConfig struct {
	BackoffMultiplier float64       // multiplicative factor for each retry (default 2)
	BackoffMax        time.Duration // cap on backoff delay (default 300s)
	BackoffResetAfter time.Duration // healthy for this long resets backoff (default 600s)
	RestartBudget     int           // max restarts per window (0 = unlimited)
	RestartWindow     time.Duration // rolling window for budget (default 300s)
}

// DefaultTrackerConfig returns sensible defaults.
func DefaultTrackerConfig() TrackerConfig {
	return TrackerConfig{
		BackoffMultiplier: 2,
		BackoffMax:        300 * time.Second,
		BackoffResetAfter: 600 * time.Second,
		RestartBudget:     5,
		RestartWindow:     300 * time.Second,
	}
}

// ContainerHistory tracks restart history for a single container.
type ContainerHistory struct {
	Restarts       []time.Time   // timestamps of recent restarts
	BackoffUntil   time.Time     // next allowed restart time
	BackoffDelay   time.Duration // current backoff delay
	CircuitOpen    bool          // true = budget exhausted
	UnhealthyCount int           // consecutive unhealthy detections
}

// SkipReason describes why a restart was suppressed.
type SkipReason string

const (
	SkipNone    SkipReason = ""
	SkipBackoff SkipReason = "backoff"
	SkipCircuit SkipReason = "circuit"
)

// RestartTracker implements per-container circuit breaker and exponential backoff.
type RestartTracker struct {
	mu      sync.Mutex
	history map[string]*ContainerHistory
	cfg     TrackerConfig
	clock   clock.Clock
}

// NewRestartTracker creates a tracker with the given config.
func NewRestartTracker(cfg TrackerConfig, clk clock.Clock) *RestartTracker {
	return &RestartTracker{
		history: make(map[string]*ContainerHistory),
		cfg:     cfg,
		clock:   clk,
	}
}

// ShouldRestart checks if a restart is allowed for the given container.
// Returns (allowed, skipReason).
func (rt *RestartTracker) ShouldRestart(id string) (bool, SkipReason) {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	h := rt.getOrCreate(id)
	now := rt.clock.Now()

	// Prune old restarts outside the window
	rt.pruneOld(h)

	// Check circuit breaker (budget exhausted)
	if h.CircuitOpen {
		return false, SkipCircuit
	}

	// Check backoff
	if now.Before(h.BackoffUntil) {
		return false, SkipBackoff
	}

	// Check budget
	if rt.cfg.RestartBudget > 0 && len(h.Restarts) >= rt.cfg.RestartBudget {
		h.CircuitOpen = true
		return false, SkipCircuit
	}

	return true, SkipNone
}

// RecordRestart records that a restart was performed for the given container.
// Advances the backoff delay for next time.
func (rt *RestartTracker) RecordRestart(id string) {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	h := rt.getOrCreate(id)
	now := rt.clock.Now()

	h.Restarts = append(h.Restarts, now)

	// Calculate next backoff
	if h.BackoffDelay == 0 {
		h.BackoffDelay = 10 * time.Second // initial backoff
	} else {
		h.BackoffDelay = time.Duration(float64(h.BackoffDelay) * rt.cfg.BackoffMultiplier)
	}
	if h.BackoffDelay > rt.cfg.BackoffMax {
		h.BackoffDelay = rt.cfg.BackoffMax
	}
	h.BackoffUntil = now.Add(h.BackoffDelay)
}

// RecordUnhealthy increments the unhealthy counter for a container.
// Returns true if the threshold is reached and action should be taken.
func (rt *RestartTracker) RecordUnhealthy(id string, threshold int) bool {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	h := rt.getOrCreate(id)
	h.UnhealthyCount++
	return h.UnhealthyCount >= threshold
}

// UnhealthyCount returns the current consecutive unhealthy count for a container.
func (rt *RestartTracker) UnhealthyCount(id string) int {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	if h, ok := rt.history[id]; ok {
		return h.UnhealthyCount
	}
	return 0
}

// ResetUnhealthy clears the unhealthy counter for a container.
func (rt *RestartTracker) ResetUnhealthy(id string) {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	if h, ok := rt.history[id]; ok {
		h.UnhealthyCount = 0
	}
}

// Reset clears backoff and restart history for a container (e.g. when it becomes healthy).
func (rt *RestartTracker) Reset(id string) {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	delete(rt.history, id)
}

// IsCircuitOpen returns true if the circuit is open for the given container.
func (rt *RestartTracker) IsCircuitOpen(id string) bool {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	h, ok := rt.history[id]
	if !ok {
		return false
	}
	return h.CircuitOpen
}

// BackoffRemaining returns the time remaining in backoff for a container.
func (rt *RestartTracker) BackoffRemaining(id string) time.Duration {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	h, ok := rt.history[id]
	if !ok {
		return 0
	}
	remaining := h.BackoffUntil.Sub(rt.clock.Now())
	if remaining < 0 {
		return 0
	}
	return remaining
}

// CircuitOpenCount returns the number of containers with open circuits.
func (rt *RestartTracker) CircuitOpenCount() int {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	count := 0
	for _, h := range rt.history {
		if h.CircuitOpen {
			count++
		}
	}
	return count
}

// FormatSkipReason returns a human-readable string for a skip reason.
func (rt *RestartTracker) FormatSkipReason(id, name string, reason SkipReason) string {
	switch reason {
	case SkipBackoff:
		remaining := rt.BackoffRemaining(id)
		return fmt.Sprintf("Container %s in backoff (%.0fs remaining)", name, remaining.Seconds())
	case SkipCircuit:
		return fmt.Sprintf("Container %s circuit open (restart budget exhausted)", name)
	default:
		return ""
	}
}

func (rt *RestartTracker) getOrCreate(id string) *ContainerHistory {
	h, ok := rt.history[id]
	if !ok {
		h = &ContainerHistory{}
		rt.history[id] = h
	}
	return h
}

func (rt *RestartTracker) pruneOld(h *ContainerHistory) {
	if rt.cfg.RestartWindow <= 0 {
		return
	}
	cutoff := rt.clock.Now().Add(-rt.cfg.RestartWindow)
	i := 0
	for _, t := range h.Restarts {
		if t.After(cutoff) {
			h.Restarts[i] = t
			i++
		}
	}
	h.Restarts = h.Restarts[:i]
}
