package guardian

import (
	"testing"
	"time"
)

func TestLoopWatch_BelowThresholdNoAlert(t *testing.T) {
	lw := newLoopWatch(5, 300)
	base := time.Unix(1_000_000, 0)
	for i := 0; i < 4; i++ {
		if lw.recordDeath("radarr", base.Add(time.Duration(i)*time.Second)) {
			t.Fatalf("alert fired at death %d, expected none below threshold", i+1)
		}
	}
}

func TestLoopWatch_AtThresholdAlertsOnce(t *testing.T) {
	lw := newLoopWatch(5, 300)
	base := time.Unix(1_000_000, 0)
	fired := 0
	for i := 0; i < 10; i++ {
		if lw.recordDeath("radarr", base.Add(time.Duration(i)*time.Second)) {
			fired++
		}
	}
	if fired != 1 {
		t.Fatalf("expected exactly 1 alert for a sustained loop, got %d", fired)
	}
}

func TestLoopWatch_ReArmsAfterQuietWindow(t *testing.T) {
	lw := newLoopWatch(5, 300)
	base := time.Unix(1_000_000, 0)
	for i := 0; i < 5; i++ {
		lw.recordDeath("radarr", base.Add(time.Duration(i)*time.Second))
	}
	relapse := base.Add(400 * time.Second)
	fired := 0
	for i := 0; i < 5; i++ {
		if lw.recordDeath("radarr", relapse.Add(time.Duration(i)*time.Second)) {
			fired++
		}
	}
	if fired != 1 {
		t.Fatalf("expected 1 alert on relapse after quiet window, got %d", fired)
	}
}
