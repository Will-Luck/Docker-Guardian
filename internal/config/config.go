package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
)

// Config holds all Docker-Guardian configuration from environment variables.
// Every field maps 1:1 to the shell version's env vars for backward compatibility.
type Config struct {
	// Docker connection
	DockerSock  string
	CurlTimeout int // seconds (kept for env var compat, used as HTTP timeout)

	// Core autoheal
	ContainerLabel     string // "all" or label name
	StartPeriod        int    // seconds
	Interval           int    // seconds
	DefaultStopTimeout int    // seconds
	OnlyMonitorRunning bool

	// Docker-Guardian extensions
	MonitorDependencies  bool
	DependencyStartDelay int // seconds

	// Cascade restart (network namespace dependents)
	CascadeRestart           bool
	CascadeSettleDelay       int // seconds to wait after parent starts before cascading
	NetworkHealthcheck       bool
	NetworkHealthcheckTarget string // IP to ping for health checks

	BackupLabel        string
	BackupContainer    string
	BackupTimeout      int    // seconds (0 = disabled)
	GracePeriod        int    // seconds
	WatchtowerCooldown int    // seconds
	WatchtowerScope    string // "all" or "affected"
	WatchtowerEvents   string // "orchestration" or "all"

	// Unhealthy threshold
	UnhealthyThreshold int // consecutive unhealthy checks before action (1 = immediate)

	// Circuit breaker / backoff
	BackoffMultiplier float64
	BackoffMax        int // seconds
	BackoffResetAfter int // seconds
	RestartBudget     int
	RestartWindow     int // seconds

	// Crash-loop / down detection (Docker-policy restarts guardian does not perform)
	RestartLoopThreshold int    // deaths within window before a crash-loop alert
	RestartLoopWindow    int    // seconds
	DownGrace            int    // seconds a meant-to-be-up container may stay down before a down alert
	ExpectedUp           string // comma-separated container names to down-watch even if restart=no

	// Post-restart script
	PostRestartScript string

	// Notification events
	NotifyEvents    string
	NotifyRateLimit int    // seconds (0 = unlimited)
	NotifyHostname  string // prepended to all notifications as [hostname]

	// Notification services
	WebhookURL     string
	WebhookJSONKey string
	AppriseURL     string

	GotifyURL   string
	GotifyToken string

	DiscordWebhook string
	SlackWebhook   string

	TelegramToken  string
	TelegramChatID string

	PushoverToken string
	PushoverUser  string

	PushbulletToken string
	LunaSeaWebhook  string

	EmailSMTP string
	EmailFrom string
	EmailTo   string
	EmailUser string
	EmailPass string

	// Metrics
	MetricsPort int

	// Logging
	LogJSON bool
}

// Load reads all configuration from environment variables with defaults
// matching the shell version exactly.
func Load() *Config {
	return &Config{
		DockerSock:  envStr("DOCKER_SOCK", "/var/run/docker.sock"),
		CurlTimeout: envInt("CURL_TIMEOUT", 30),

		ContainerLabel:     envStr("AUTOHEAL_CONTAINER_LABEL", "autoheal"),
		StartPeriod:        envInt("AUTOHEAL_START_PERIOD", 0),
		Interval:           envInt("AUTOHEAL_INTERVAL", 5),
		DefaultStopTimeout: envInt("AUTOHEAL_DEFAULT_STOP_TIMEOUT", 10),
		OnlyMonitorRunning: envBool("AUTOHEAL_ONLY_MONITOR_RUNNING", false),

		MonitorDependencies:  envBool("AUTOHEAL_MONITOR_DEPENDENCIES", true),
		DependencyStartDelay: envInt("AUTOHEAL_DEPENDENCY_START_DELAY", 5),

		CascadeRestart:           envBool("AUTOHEAL_CASCADE_RESTART", true),
		CascadeSettleDelay:       envInt("AUTOHEAL_CASCADE_SETTLE_DELAY", 15),
		NetworkHealthcheck:       envBool("AUTOHEAL_NETWORK_HEALTHCHECK", true),
		NetworkHealthcheckTarget: envStr("AUTOHEAL_NETWORK_HEALTHCHECK_TARGET", "8.8.8.8"),

		BackupLabel:        envStr("AUTOHEAL_BACKUP_LABEL", "docker-volume-backup.stop-during-backup"),
		BackupContainer:    envStr("AUTOHEAL_BACKUP_CONTAINER", ""),
		BackupTimeout:      envInt("AUTOHEAL_BACKUP_TIMEOUT", 600),
		GracePeriod:        envInt("AUTOHEAL_GRACE_PERIOD", 300),
		WatchtowerCooldown: envInt("AUTOHEAL_WATCHTOWER_COOLDOWN", 300),
		WatchtowerScope:    envStr("AUTOHEAL_WATCHTOWER_SCOPE", "all"),
		WatchtowerEvents:   envStr("AUTOHEAL_WATCHTOWER_EVENTS", "orchestration"),

		UnhealthyThreshold: envInt("AUTOHEAL_UNHEALTHY_THRESHOLD", 1),

		BackoffMultiplier: envFloat("AUTOHEAL_BACKOFF_MULTIPLIER", 2),
		BackoffMax:        envInt("AUTOHEAL_BACKOFF_MAX", 300),
		BackoffResetAfter: envInt("AUTOHEAL_BACKOFF_RESET_AFTER", 600),
		RestartBudget:     envInt("AUTOHEAL_RESTART_BUDGET", 5),
		RestartWindow:     envInt("AUTOHEAL_RESTART_WINDOW", 300),

		RestartLoopThreshold: envInt("AUTOHEAL_RESTARTLOOP_THRESHOLD", 5),
		RestartLoopWindow:    envInt("AUTOHEAL_RESTARTLOOP_WINDOW", 300),
		DownGrace:            envInt("AUTOHEAL_DOWN_GRACE", 120),
		ExpectedUp:           envStr("AUTOHEAL_EXPECTED_UP", ""),

		PostRestartScript: envStr("POST_RESTART_SCRIPT", ""),
		NotifyEvents:      envStr("NOTIFY_EVENTS", "actions"),
		NotifyRateLimit:   envInt("NOTIFY_RATE_LIMIT", 60),
		NotifyHostname:    envStr("NOTIFY_HOSTNAME", ""),

		WebhookURL:     envStr("WEBHOOK_URL", ""),
		WebhookJSONKey: envStr("WEBHOOK_JSON_KEY", "text"),
		AppriseURL:     envStr("APPRISE_URL", ""),

		GotifyURL:   envStr("NOTIFY_GOTIFY_URL", ""),
		GotifyToken: envStr("NOTIFY_GOTIFY_TOKEN", ""),

		DiscordWebhook: envStr("NOTIFY_DISCORD_WEBHOOK", ""),
		SlackWebhook:   envStr("NOTIFY_SLACK_WEBHOOK", ""),

		TelegramToken:  envStr("NOTIFY_TELEGRAM_TOKEN", ""),
		TelegramChatID: envStr("NOTIFY_TELEGRAM_CHAT_ID", ""),

		PushoverToken: envStr("NOTIFY_PUSHOVER_TOKEN", ""),
		PushoverUser:  envStr("NOTIFY_PUSHOVER_USER", ""),

		PushbulletToken: envStr("NOTIFY_PUSHBULLET_TOKEN", ""),
		LunaSeaWebhook:  envStr("NOTIFY_LUNASEA_WEBHOOK", ""),

		EmailSMTP: envStr("NOTIFY_EMAIL_SMTP", ""),
		EmailFrom: envStr("NOTIFY_EMAIL_FROM", ""),
		EmailTo:   envStr("NOTIFY_EMAIL_TO", ""),
		EmailUser: envStr("NOTIFY_EMAIL_USER", ""),
		EmailPass: envStr("NOTIFY_EMAIL_PASS", ""),

		MetricsPort: envInt("METRICS_PORT", 0),

		LogJSON: envBool("LOG_JSON", false),
	}
}

// PrintBanner outputs configuration to stdout in the shell-compatible format
// that acceptance tests grep for (e.g. "AUTOHEAL_CONTAINER_LABEL=autoheal").
func (c *Config) PrintBanner() {
	fmt.Println("AUTOHEAL_CONTAINER_LABEL=" + c.ContainerLabel)
	fmt.Println("AUTOHEAL_START_PERIOD=" + strconv.Itoa(c.StartPeriod))
	fmt.Println("AUTOHEAL_INTERVAL=" + strconv.Itoa(c.Interval))
	fmt.Println("AUTOHEAL_DEFAULT_STOP_TIMEOUT=" + strconv.Itoa(c.DefaultStopTimeout))
	fmt.Println("AUTOHEAL_ONLY_MONITOR_RUNNING=" + strconv.FormatBool(c.OnlyMonitorRunning))
	fmt.Println("AUTOHEAL_MONITOR_DEPENDENCIES=" + strconv.FormatBool(c.MonitorDependencies))
	fmt.Println("AUTOHEAL_DEPENDENCY_START_DELAY=" + strconv.Itoa(c.DependencyStartDelay))
	fmt.Println("AUTOHEAL_BACKUP_LABEL=" + c.BackupLabel)
	fmt.Println("AUTOHEAL_BACKUP_CONTAINER=" + c.BackupContainer)
	fmt.Println("AUTOHEAL_BACKUP_TIMEOUT=" + strconv.Itoa(c.BackupTimeout))
	fmt.Println("AUTOHEAL_GRACE_PERIOD=" + strconv.Itoa(c.GracePeriod))
	fmt.Println("AUTOHEAL_WATCHTOWER_COOLDOWN=" + strconv.Itoa(c.WatchtowerCooldown))
	fmt.Println("AUTOHEAL_WATCHTOWER_SCOPE=" + c.WatchtowerScope)
	fmt.Println("AUTOHEAL_WATCHTOWER_EVENTS=" + c.WatchtowerEvents)
	fmt.Println("AUTOHEAL_UNHEALTHY_THRESHOLD=" + strconv.Itoa(c.UnhealthyThreshold))
	fmt.Printf("AUTOHEAL_BACKOFF_MULTIPLIER=%g\n", c.BackoffMultiplier)
	fmt.Println("AUTOHEAL_BACKOFF_MAX=" + strconv.Itoa(c.BackoffMax))
	fmt.Println("AUTOHEAL_BACKOFF_RESET_AFTER=" + strconv.Itoa(c.BackoffResetAfter))
	fmt.Println("AUTOHEAL_RESTART_BUDGET=" + strconv.Itoa(c.RestartBudget))
	fmt.Println("AUTOHEAL_RESTART_WINDOW=" + strconv.Itoa(c.RestartWindow))
	fmt.Println("AUTOHEAL_RESTARTLOOP_THRESHOLD=" + strconv.Itoa(c.RestartLoopThreshold))
	fmt.Println("AUTOHEAL_RESTARTLOOP_WINDOW=" + strconv.Itoa(c.RestartLoopWindow))
	fmt.Println("AUTOHEAL_DOWN_GRACE=" + strconv.Itoa(c.DownGrace))
	fmt.Println("AUTOHEAL_EXPECTED_UP=" + c.ExpectedUp)
}

// ResolvedNotifyEvents returns the normalised event categories.
func (c *Config) ResolvedNotifyEvents() []string {
	raw := strings.TrimSpace(c.NotifyEvents)
	switch raw {
	case "all":
		return []string{"startup", "actions", "skips", "alerts"}
	case "debug":
		return []string{"startup", "actions", "skips", "alerts", "debug"}
	}

	var result []string
	for _, item := range strings.Split(raw, ",") {
		item = strings.TrimSpace(item)
		switch item {
		case "1", "startup":
			result = append(result, "startup")
		case "2", "actions":
			result = append(result, "actions")
		case "3", "failures":
			result = append(result, "failures")
		case "4", "skips":
			result = append(result, "skips")
		case "5", "debug":
			result = append(result, "startup", "actions", "skips", "alerts", "debug")
		case "6", "alerts":
			result = append(result, "alerts")
		case "all":
			result = append(result, "startup", "actions", "skips", "alerts")
		}
	}
	return result
}

// Validate checks configuration for invalid or dangerous values.
func (c *Config) Validate() error {
	var errs []error
	if c.Interval <= 0 {
		errs = append(errs, fmt.Errorf("AUTOHEAL_INTERVAL must be > 0, got %d", c.Interval))
	}
	if c.GracePeriod < 0 {
		errs = append(errs, fmt.Errorf("AUTOHEAL_GRACE_PERIOD must be >= 0, got %d", c.GracePeriod))
	}
	if c.UnhealthyThreshold < 1 {
		errs = append(errs, fmt.Errorf("AUTOHEAL_UNHEALTHY_THRESHOLD must be >= 1, got %d", c.UnhealthyThreshold))
	}
	if c.DefaultStopTimeout < 0 {
		errs = append(errs, fmt.Errorf("AUTOHEAL_DEFAULT_STOP_TIMEOUT must be >= 0, got %d", c.DefaultStopTimeout))
	}
	if c.WatchtowerScope != "all" && c.WatchtowerScope != "affected" {
		errs = append(errs, fmt.Errorf("AUTOHEAL_WATCHTOWER_SCOPE must be \"all\" or \"affected\", got %q", c.WatchtowerScope))
	}
	if c.WatchtowerEvents != "orchestration" && c.WatchtowerEvents != "all" {
		errs = append(errs, fmt.Errorf("AUTOHEAL_WATCHTOWER_EVENTS must be \"orchestration\" or \"all\", got %q", c.WatchtowerEvents))
	}
	for _, u := range []struct {
		name, val string
	}{
		{"WEBHOOK_URL", c.WebhookURL},
		{"APPRISE_URL", c.AppriseURL},
		{"NOTIFY_GOTIFY_URL", c.GotifyURL},
		{"NOTIFY_DISCORD_WEBHOOK", c.DiscordWebhook},
		{"NOTIFY_SLACK_WEBHOOK", c.SlackWebhook},
		{"NOTIFY_LUNASEA_WEBHOOK", c.LunaSeaWebhook},
	} {
		if u.val != "" {
			if _, err := url.Parse(u.val); err != nil {
				errs = append(errs, fmt.Errorf("%s is not a valid URL: %w", u.name, err))
			}
		}
	}
	return errors.Join(errs...)
}

func envStr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func envFloat(key string, def float64) float64 {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return def
	}
	return f
}

func envBool(key string, def bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}
