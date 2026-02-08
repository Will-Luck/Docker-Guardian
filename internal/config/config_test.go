package config

import (
	"os"
	"testing"
)

func TestResolvedNotifyEvents(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{"actions only", "actions", []string{"actions"}},
		{"all shorthand", "all", []string{"startup", "actions", "skips"}},
		{"debug shorthand", "debug", []string{"startup", "actions", "skips", "debug"}},
		{"numeric 2", "2", []string{"actions"}},
		{"numeric 5", "5", []string{"startup", "actions", "skips", "debug"}},
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
