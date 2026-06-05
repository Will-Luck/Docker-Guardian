package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolvedNotifyEvents(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{"actions only", "actions", []string{"actions"}},
		{"all shorthand", "all", []string{"startup", "actions", "skips", "alerts"}},
		{"debug shorthand", "debug", []string{"startup", "actions", "skips", "alerts", "debug"}},
		{"numeric 2", "2", []string{"actions"}},
		{"numeric 5", "5", []string{"startup", "actions", "skips", "alerts", "debug"}},
		{"csv", "startup,actions,skips", []string{"startup", "actions", "skips"}},
		{"failures category", "failures", []string{"failures"}},
		{"mixed csv", "1,2", []string{"startup", "actions"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{NotifyEvents: tt.input}
			got := cfg.ResolvedNotifyEvents()
			if len(got) != len(tt.expected) {
				t.Fatalf("got %v, want %v", got, tt.expected)
			}
			for i := range got {
				if got[i] != tt.expected[i] {
					t.Errorf("index %d: got %q, want %q", i, got[i], tt.expected[i])
				}
			}
		})
	}
}

func TestCascadeRestartDefaults(t *testing.T) {
	cfg := Load()
	assert.True(t, cfg.CascadeRestart)
	assert.Equal(t, 15, cfg.CascadeSettleDelay)
	assert.True(t, cfg.NetworkHealthcheck)
	assert.Equal(t, "8.8.8.8", cfg.NetworkHealthcheckTarget)
	assert.Equal(t, 3, cfg.NetworkHealthcheckFailures)
}

func TestCascadeRestartFromEnv(t *testing.T) {
	t.Setenv("AUTOHEAL_CASCADE_RESTART", "false")
	t.Setenv("AUTOHEAL_CASCADE_SETTLE_DELAY", "30")
	t.Setenv("AUTOHEAL_NETWORK_HEALTHCHECK", "false")
	t.Setenv("AUTOHEAL_NETWORK_HEALTHCHECK_TARGET", "1.1.1.1")
	t.Setenv("AUTOHEAL_NETWORK_HEALTHCHECK_FAILURES", "5")
	cfg := Load()
	assert.False(t, cfg.CascadeRestart)
	assert.Equal(t, 30, cfg.CascadeSettleDelay)
	assert.False(t, cfg.NetworkHealthcheck)
	assert.Equal(t, "1.1.1.1", cfg.NetworkHealthcheckTarget)
	assert.Equal(t, 5, cfg.NetworkHealthcheckFailures)
}

func TestEnvStr(t *testing.T) {
	const key = "DG_TEST_ENV_STR"
	os.Setenv(key, "custom")
	defer os.Unsetenv(key)

	if got := envStr(key, "default"); got != "custom" {
		t.Errorf("got %q, want %q", got, "custom")
	}
	if got := envStr("DG_TEST_MISSING", "fallback"); got != "fallback" {
		t.Errorf("got %q, want %q", got, "fallback")
	}
}

func TestEnvInt(t *testing.T) {
	const key = "DG_TEST_ENV_INT"

	os.Setenv(key, "42")
	defer os.Unsetenv(key)
	if got := envInt(key, 0); got != 42 {
		t.Errorf("got %d, want 42", got)
	}

	os.Setenv(key, "notanumber")
	if got := envInt(key, 99); got != 99 {
		t.Errorf("got %d, want 99 (default on parse failure)", got)
	}
}

func TestEnvBool(t *testing.T) {
	const key = "DG_TEST_ENV_BOOL"

	os.Setenv(key, "true")
	defer os.Unsetenv(key)
	if got := envBool(key, false); !got {
		t.Errorf("got false, want true")
	}

	os.Setenv(key, "invalid")
	if got := envBool(key, true); !got {
		t.Errorf("got false, want true (default on parse failure)")
	}
}

func TestResolvedNotifyEvents_Alerts(t *testing.T) {
	if got := (&Config{NotifyEvents: "actions,alerts"}).ResolvedNotifyEvents(); !contains(got, "alerts") {
		t.Fatalf("expected alerts in %v", got)
	}
	if got := (&Config{NotifyEvents: "all"}).ResolvedNotifyEvents(); !contains(got, "alerts") {
		t.Fatalf("expected alerts included in 'all', got %v", got)
	}
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
