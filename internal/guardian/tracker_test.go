package guardian

import (
	"testing"
	"time"
)

func TestTracker_AllowsFirstRestart(t *testing.T) {
	clk := newMockClock(time.Now())
	rt := NewRestartTracker(DefaultTrackerConfig(), clk)

	allowed, reason := rt.ShouldRestart("abc123")
	if !allowed {
		t.Errorf("first restart should be allowed, got reason=%s", reason)
	}
}

func TestTracker_BackoffAfterRestart(t *testing.T) {
	clk := newMockClock(time.Now())
	rt := NewRestartTracker(DefaultTrackerConfig(), clk)

	rt.RecordRestart("abc123")

	// Immediately after, should be in backoff
	allowed, reason := rt.ShouldRestart("abc123")
	if allowed {
		t.Error("should be in backoff after restart")
	}
	if reason != SkipBackoff {
		t.Errorf("expected backoff reason, got %s", reason)
	}

	// Advance past initial 10s backoff
	clk.Advance(11 * time.Second)

	allowed, reason = rt.ShouldRestart("abc123")
	if !allowed {
		t.Errorf("should be allowed after backoff expires, got reason=%s", reason)
	}
}

func TestTracker_ExponentialBackoff(t *testing.T) {
	clk := newMockClock(time.Now())
	rt := NewRestartTracker(DefaultTrackerConfig(), clk)

	// First restart: 10s backoff
	rt.RecordRestart("abc123")
	if rt.BackoffRemaining("abc123") > 10*time.Second || rt.BackoffRemaining("abc123") < 9*time.Second {
		t.Errorf("expected ~10s backoff, got %v", rt.BackoffRemaining("abc123"))
	}

	// Advance past first backoff
	clk.Advance(11 * time.Second)

	// Second restart: 20s backoff
	rt.RecordRestart("abc123")
	remaining := rt.BackoffRemaining("abc123")
	if remaining > 20*time.Second || remaining < 19*time.Second {
		t.Errorf("expected ~20s backoff, got %v", remaining)
	}

	// Advance past second backoff
	clk.Advance(21 * time.Second)

	// Third restart: 40s backoff
	rt.RecordRestart("abc123")
	remaining = rt.BackoffRemaining("abc123")
	if remaining > 40*time.Second || remaining < 39*time.Second {
		t.Errorf("expected ~40s backoff, got %v", remaining)
	}
}

func TestTracker_BackoffMax(t *testing.T) {
	clk := newMockClock(time.Now())
	cfg := DefaultTrackerConfig()
	cfg.BackoffMax = 30 * time.Second
	rt := NewRestartTracker(cfg, clk)

	// Restart 5 times, each time advancing past backoff
	for i := 0; i < 5; i++ {
		rt.RecordRestart("abc123")
		clk.Advance(cfg.BackoffMax + time.Second)
	}

	// Backoff should be capped at BackoffMax
	rt.RecordRestart("abc123")
	remaining := rt.BackoffRemaining("abc123")
	if remaining > cfg.BackoffMax {
		t.Errorf("backoff should be capped at %v, got %v", cfg.BackoffMax, remaining)
	}
}

func TestTracker_BudgetExhausted(t *testing.T) {
	clk := newMockClock(time.Now())
	cfg := DefaultTrackerConfig()
	cfg.RestartBudget = 3
	cfg.RestartWindow = 3600 * time.Second // large window so restarts aren't pruned
	cfg.BackoffMax = 10 * time.Second      // small max so we advance less
	rt := NewRestartTracker(cfg, clk)

	// Record 3 restarts (budget = 3), advancing past backoff each time
	for i := 0; i < 3; i++ {
		rt.RecordRestart("abc123")
		clk.Advance(cfg.BackoffMax + time.Second)
	}

	// 4th restart should be denied — circuit open
	allowed, reason := rt.ShouldRestart("abc123")
	if allowed {
		t.Error("should deny restart when budget exhausted")
	}
	if reason != SkipCircuit {
		t.Errorf("expected circuit reason, got %s", reason)
	}
	if !rt.IsCircuitOpen("abc123") {
		t.Error("circuit should be open")
	}
}

func TestTracker_BudgetUnlimited(t *testing.T) {
	clk := newMockClock(time.Now())
	cfg := DefaultTrackerConfig()
	cfg.RestartBudget = 0 // unlimited
	rt := NewRestartTracker(cfg, clk)

	for i := 0; i < 20; i++ {
		rt.RecordRestart("abc123")
		clk.Advance(cfg.BackoffMax + time.Second) // advance past max backoff
	}

	allowed, _ := rt.ShouldRestart("abc123")
	if !allowed {
		t.Error("should allow restarts when budget is unlimited")
	}
}

func TestTracker_Reset(t *testing.T) {
	clk := newMockClock(time.Now())
	cfg := DefaultTrackerConfig()
	cfg.RestartBudget = 2
	rt := NewRestartTracker(cfg, clk)

	// Exhaust budget
	rt.RecordRestart("abc123")
	rt.RecordRestart("abc123")
	clk.Advance(11 * time.Second)

	allowed, _ := rt.ShouldRestart("abc123")
	if allowed {
		t.Error("should deny after budget exhausted")
	}

	// Reset (container became healthy)
	rt.Reset("abc123")

	allowed, _ = rt.ShouldRestart("abc123")
	if !allowed {
		t.Error("should allow after reset")
	}
}

func TestTracker_PruneOldRestarts(t *testing.T) {
	clk := newMockClock(time.Now())
	cfg := DefaultTrackerConfig()
	cfg.RestartBudget = 3
	cfg.RestartWindow = 60 * time.Second
	rt := NewRestartTracker(cfg, clk)

	// Record 3 restarts
	for i := 0; i < 3; i++ {
		rt.RecordRestart("abc123")
		clk.Advance(11 * time.Second)
	}

	// Advance past the window so old restarts are pruned
	clk.Advance(60 * time.Second)

	// Should be allowed now — old restarts pruned
	allowed, _ := rt.ShouldRestart("abc123")
	if !allowed {
		t.Error("should allow after old restarts are pruned")
	}
}

func TestTracker_CircuitOpenCount(t *testing.T) {
	clk := newMockClock(time.Now())
	cfg := DefaultTrackerConfig()
	cfg.RestartBudget = 1
	rt := NewRestartTracker(cfg, clk)

	rt.RecordRestart("a")
	clk.Advance(11 * time.Second)
	rt.ShouldRestart("a") // triggers circuit open

	rt.RecordRestart("b")
	clk.Advance(11 * time.Second)
	rt.ShouldRestart("b") // triggers circuit open

	if rt.CircuitOpenCount() != 2 {
		t.Errorf("expected 2 open circuits, got %d", rt.CircuitOpenCount())
	}
}

func TestContainerAction(t *testing.T) {
	tests := []struct {
		labels   map[string]string
		expected string
	}{
		{nil, "restart"},
		{map[string]string{}, "restart"},
		{map[string]string{"autoheal.action": "restart"}, "restart"},
		{map[string]string{"autoheal.action": "stop"}, "stop"},
		{map[string]string{"autoheal.action": "notify"}, "notify"},
		{map[string]string{"autoheal.action": "none"}, "none"},
		{map[string]string{"autoheal.action": "invalid"}, "restart"},
	}

	for _, tt := range tests {
		got := containerAction(tt.labels)
		if got != tt.expected {
			t.Errorf("containerAction(%v) = %q, want %q", tt.labels, got, tt.expected)
		}
	}
}
