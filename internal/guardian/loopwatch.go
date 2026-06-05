package guardian

import (
	"sync"
	"time"
)

// loopWatch detects crash loops by counting container "die" events within a
// rolling window. It targets containers restarted by Docker's own policy
// (which guardian does not itself restart), invisible to the healthcheck path.
type loopWatch struct {
	mu        sync.Mutex
	threshold int
	window    time.Duration
	deaths    map[string][]time.Time
	alerted   map[string]bool
}

func newLoopWatch(threshold, windowSecs int) *loopWatch {
	return &loopWatch{
		threshold: threshold,
		window:    time.Duration(windowSecs) * time.Second,
		deaths:    make(map[string][]time.Time),
		alerted:   make(map[string]bool),
	}
}

// recordDeath logs a death for name at now and returns true exactly once per
// loop episode, when deaths within the window reach the threshold. After the
// window goes quiet the episode re-arms, so a later relapse alerts again.
func (lw *loopWatch) recordDeath(name string, now time.Time) bool {
	if lw.threshold <= 0 {
		return false
	}
	lw.mu.Lock()
	defer lw.mu.Unlock()

	cutoff := now.Add(-lw.window)
	kept := lw.deaths[name][:0]
	for _, t := range lw.deaths[name] {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	if len(kept) == 0 { // previous episode aged out -> re-arm
		delete(lw.alerted, name)
	}
	kept = append(kept, now)
	lw.deaths[name] = kept

	if len(kept) >= lw.threshold && !lw.alerted[name] {
		lw.alerted[name] = true
		return true
	}
	return false
}
