package notify

import (
	"testing"

	"github.com/Will-Luck/Docker-Guardian/internal/config"
	"github.com/Will-Luck/Docker-Guardian/internal/logging"
)

func newTestDispatcher(cfg *config.Config) *Dispatcher {
	return NewDispatcher(cfg, logging.New(false))
}

func TestConfiguredServices(t *testing.T) {
	tests := []struct {
		name     string
		cfg      *config.Config
		expected string
	}{
		{"none", &config.Config{CurlTimeout: 5, NotifyEvents: "actions"}, "none"},
		{"gotify only", &config.Config{CurlTimeout: 5, NotifyEvents: "actions", GotifyURL: "http://example.com", GotifyToken: "tok"}, "gotify"},
		{"multiple", &config.Config{
			CurlTimeout:    5,
			NotifyEvents:   "actions",
			GotifyURL:      "http://example.com",
			GotifyToken:    "tok",
			DiscordWebhook: "http://discord.example.com",
		}, "gotify discord"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := newTestDispatcher(tt.cfg)
			got := d.ConfiguredServices()
			if got != tt.expected {
				t.Errorf("got %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestHasEvent(t *testing.T) {
	cfg := &config.Config{CurlTimeout: 5, NotifyEvents: "actions"}
	d := newTestDispatcher(cfg)

	if !d.hasEvent("actions") {
		t.Error("should have 'actions' event")
	}
	if d.hasEvent("startup") {
		t.Error("should not have 'startup' event")
	}
	if d.hasEvent("debug") {
		t.Error("should not have 'debug' event")
	}
}

func TestHasEventAll(t *testing.T) {
	cfg := &config.Config{CurlTimeout: 5, NotifyEvents: "all"}
	d := newTestDispatcher(cfg)

	if !d.hasEvent("startup") {
		t.Error("'all' should include startup")
	}
	if !d.hasEvent("actions") {
		t.Error("'all' should include actions")
	}
	if !d.hasEvent("skips") {
		t.Error("'all' should include skips")
	}
	if d.hasEvent("debug") {
		t.Error("'all' should not include debug")
	}
}

func TestHasEventDebug(t *testing.T) {
	cfg := &config.Config{CurlTimeout: 5, NotifyEvents: "debug"}
	d := newTestDispatcher(cfg)

	if !d.hasEvent("debug") {
		t.Error("'debug' should include debug")
	}
	if !d.hasEvent("startup") {
		t.Error("'debug' should include startup")
	}
}
