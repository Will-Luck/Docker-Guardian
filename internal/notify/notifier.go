package notify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/smtp"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/Will-Luck/Docker-Guardian/internal/config"
	"github.com/Will-Luck/Docker-Guardian/internal/logging"
	"github.com/Will-Luck/Docker-Guardian/internal/metrics"
)

// Notifier is the interface for sending notifications from the guardian.
type Notifier interface {
	Startup(text string)
	Action(text string)
	Skip(text string)
	Close()
}

// Dispatcher sends notifications to all configured services.
type Dispatcher struct {
	cfg      *config.Config
	log      *logging.Logger
	client   *http.Client
	resolved []string
	wg       sync.WaitGroup

	// Rate limiting: per container+event key → last notification time
	rateMu    sync.Mutex
	rateLimit map[string]time.Time
}

// NewDispatcher creates a notification dispatcher from config.
func NewDispatcher(cfg *config.Config, log *logging.Logger) *Dispatcher {
	return &Dispatcher{
		cfg: cfg,
		log: log,
		client: &http.Client{
			Timeout: time.Duration(cfg.CurlTimeout) * time.Second,
		},
		resolved:  cfg.ResolvedNotifyEvents(),
		rateLimit: make(map[string]time.Time),
	}
}

// Close waits for in-flight notification goroutines to finish, with a 10-second timeout.
func (d *Dispatcher) Close() {
	done := make(chan struct{})
	go func() {
		d.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		d.log.Warn("notification shutdown timed out after 10s, some notifications may have been lost")
	}
}

// ConfiguredServices returns a human-readable list of configured notification services.
func (d *Dispatcher) ConfiguredServices() string {
	var services []string
	if d.cfg.WebhookURL != "" {
		services = append(services, "webhook")
	}
	if d.cfg.AppriseURL != "" {
		services = append(services, "apprise")
	}
	if d.cfg.GotifyURL != "" {
		services = append(services, "gotify")
	}
	if d.cfg.DiscordWebhook != "" {
		services = append(services, "discord")
	}
	if d.cfg.SlackWebhook != "" {
		services = append(services, "slack")
	}
	if d.cfg.TelegramToken != "" {
		services = append(services, "telegram")
	}
	if d.cfg.PushoverToken != "" {
		services = append(services, "pushover")
	}
	if d.cfg.PushbulletToken != "" {
		services = append(services, "pushbullet")
	}
	if d.cfg.LunaSeaWebhook != "" {
		services = append(services, "lunasea")
	}
	if d.cfg.EmailSMTP != "" {
		services = append(services, "email")
	}
	if len(services) == 0 {
		return "none"
	}
	return strings.Join(services, " ")
}

func (d *Dispatcher) hasEvent(event string) bool {
	for _, e := range d.resolved {
		if e == event {
			return true
		}
	}
	return false
}

// isRateLimited checks if a notification for this key is rate-limited.
// Returns true if the notification should be suppressed.
func (d *Dispatcher) isRateLimited(key string) bool {
	if d.cfg.NotifyRateLimit <= 0 {
		return false
	}

	d.rateMu.Lock()
	defer d.rateMu.Unlock()

	window := time.Duration(d.cfg.NotifyRateLimit) * time.Second
	if last, ok := d.rateLimit[key]; ok {
		if time.Since(last) < window {
			return true
		}
	}
	d.rateLimit[key] = time.Now()
	return false
}

// Startup sends a startup notification.
func (d *Dispatcher) Startup(text string) {
	if !d.hasEvent("startup") {
		return
	}
	d.dispatch(text, false)
}

// Action sends an action notification (success or failure).
// Action events use retry on failure.
func (d *Dispatcher) Action(text string) {
	if strings.Contains(text, "Failed") || strings.Contains(text, "[CRITICAL]") {
		if !d.hasEvent("actions") && !d.hasEvent("failures") {
			return
		}
	} else {
		if !d.hasEvent("actions") {
			return
		}
	}

	// Rate limit using the first ~50 chars as a key (contains container name)
	key := text
	if len(key) > 50 {
		key = key[:50]
	}
	if d.isRateLimited(key) {
		return
	}

	d.dispatch(text, true)
}

// Skip sends a skip notification.
func (d *Dispatcher) Skip(text string) {
	if !d.hasEvent("skips") {
		return
	}
	d.dispatch(text, false)
}

func (d *Dispatcher) dispatch(text string, retry bool) {
	if d.cfg.NotifyHostname != "" {
		text = "[" + d.cfg.NotifyHostname + "] " + text
	}

	if d.hasEvent("debug") {
		now := time.Now().Format("2006-01-02T15:04:05-0700")
		services := d.ConfiguredServices()
		for _, svc := range strings.Split(services, " ") {
			if svc != "none" {
				fmt.Printf("%s [notify] → %s: %s\n", now, svc, text)
			}
		}
	}

	if d.cfg.WebhookURL != "" {
		d.wg.Add(1)
		go func() {
			defer d.wg.Done()
			d.sendWithRetry("webhook", retry, func() error {
				return d.sendJSON(d.cfg.WebhookURL, map[string]string{d.cfg.WebhookJSONKey: text})
			})
		}()
	}
	if d.cfg.AppriseURL != "" {
		d.wg.Add(1)
		go func() {
			defer d.wg.Done()
			d.sendWithRetry("apprise", retry, func() error {
				return d.sendJSON(d.cfg.AppriseURL, map[string]string{"title": "Docker-Guardian", "body": text})
			})
		}()
	}
	if d.cfg.GotifyURL != "" {
		d.wg.Add(1)
		go func() {
			defer d.wg.Done()
			d.sendWithRetry("gotify", retry, func() error {
				return d.sendJSON(d.cfg.GotifyURL+"/message?token="+d.cfg.GotifyToken,
					map[string]any{"title": "Docker-Guardian", "message": text, "priority": 5})
			})
		}()
	}
	if d.cfg.DiscordWebhook != "" {
		d.wg.Add(1)
		go func() {
			defer d.wg.Done()
			d.sendWithRetry("discord", retry, func() error {
				return d.sendJSON(d.cfg.DiscordWebhook, map[string]any{
					"embeds": []map[string]any{{"title": "Docker-Guardian", "description": text, "color": 3066993}},
				})
			})
		}()
	}
	if d.cfg.SlackWebhook != "" {
		d.wg.Add(1)
		go func() {
			defer d.wg.Done()
			d.sendWithRetry("slack", retry, func() error {
				return d.sendJSON(d.cfg.SlackWebhook, map[string]string{"text": "*Docker-Guardian*\n" + text})
			})
		}()
	}
	if d.cfg.TelegramToken != "" {
		d.wg.Add(1)
		go func() {
			defer d.wg.Done()
			d.sendWithRetry("telegram", retry, func() error {
				return d.sendJSON("https://api.telegram.org/bot"+d.cfg.TelegramToken+"/sendMessage",
					map[string]string{"chat_id": d.cfg.TelegramChatID, "text": "Docker-Guardian: " + text})
			})
		}()
	}
	if d.cfg.PushoverToken != "" {
		d.wg.Add(1)
		go func() {
			defer d.wg.Done()
			d.sendWithRetry("pushover", retry, func() error {
				return d.sendForm("https://api.pushover.net/1/messages.json", map[string]string{
					"token": d.cfg.PushoverToken, "user": d.cfg.PushoverUser,
					"title": "Docker-Guardian", "message": text,
				})
			})
		}()
	}
	if d.cfg.PushbulletToken != "" {
		d.wg.Add(1)
		go func() {
			defer d.wg.Done()
			d.sendWithRetry("pushbullet", retry, func() error {
				return d.sendJSONWithHeader("https://api.pushbullet.com/v2/pushes",
					"Access-Token", d.cfg.PushbulletToken,
					map[string]string{"type": "note", "title": "Docker-Guardian", "body": text})
			})
		}()
	}
	if d.cfg.LunaSeaWebhook != "" {
		d.wg.Add(1)
		go func() {
			defer d.wg.Done()
			d.sendWithRetry("lunasea", retry, func() error {
				return d.sendJSON(d.cfg.LunaSeaWebhook, map[string]string{"title": "Docker-Guardian", "body": text})
			})
		}()
	}
	if d.cfg.EmailSMTP != "" {
		d.wg.Add(1)
		go func() {
			defer d.wg.Done()
			d.sendWithRetry("email", retry, func() error {
				return d.sendEmail(text)
			})
		}()
	}
}

// sendWithRetry retries a send function up to 3 times with exponential backoff.
// Only retries if retry=true. Tracks metrics per service.
func (d *Dispatcher) sendWithRetry(service string, retry bool, fn func() error) {
	maxAttempts := 1
	if retry {
		maxAttempts = 3
	}

	delays := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second}

	for attempt := 0; attempt < maxAttempts; attempt++ {
		if err := fn(); err == nil {
			metrics.NotificationsTotal.WithLabelValues(service, "success").Inc()
			return
		}
		// Only retry if we have attempts left
		if attempt < maxAttempts-1 {
			time.Sleep(delays[attempt])
		}
	}
	metrics.NotificationsTotal.WithLabelValues(service, "failure").Inc()
}

func (d *Dispatcher) sendJSON(targetURL string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		d.log.Error("failed to marshal notification payload", "error", err)
		return err
	}
	resp, err := d.client.Post(targetURL, "application/json", bytes.NewReader(body))
	if err != nil {
		d.log.Warn("notification send failed", "url", targetURL, "error", err)
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		d.log.Warn("notification returned non-2xx status", "url", targetURL, "status", resp.StatusCode)
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return nil
}

func (d *Dispatcher) sendJSONWithHeader(targetURL, headerKey, headerVal string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		d.log.Warn("failed to marshal notification payload", "error", err)
		return err
	}
	req, err := http.NewRequest(http.MethodPost, targetURL, bytes.NewReader(body))
	if err != nil {
		d.log.Warn("failed to create notification request", "error", err)
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(headerKey, headerVal)
	resp, err := d.client.Do(req)
	if err != nil {
		d.log.Warn("notification send failed", "url", targetURL, "error", err)
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		d.log.Warn("notification returned non-2xx status", "url", targetURL, "status", resp.StatusCode)
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return nil
}

func (d *Dispatcher) sendForm(endpoint string, fields map[string]string) error {
	vals := url.Values{}
	for k, v := range fields {
		vals.Set(k, v)
	}
	resp, err := d.client.Post(endpoint, "application/x-www-form-urlencoded", strings.NewReader(vals.Encode()))
	if err != nil {
		d.log.Warn("notification send failed", "url", endpoint, "error", err)
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		d.log.Warn("notification returned non-2xx status", "url", endpoint, "status", resp.StatusCode)
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return nil
}

func (d *Dispatcher) sendEmail(text string) error {
	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: Docker-Guardian Alert\r\n\r\n%s",
		d.cfg.EmailFrom, d.cfg.EmailTo, text)

	auth := smtp.PlainAuth("", d.cfg.EmailUser, d.cfg.EmailPass, strings.Split(d.cfg.EmailSMTP, ":")[0])
	err := smtp.SendMail(d.cfg.EmailSMTP, auth, d.cfg.EmailFrom, []string{d.cfg.EmailTo}, []byte(msg))
	if err != nil {
		d.log.Warn("email notification failed", "error", err)
		return err
	}
	return nil
}
