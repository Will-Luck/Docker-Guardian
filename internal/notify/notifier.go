package notify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/smtp"
	"strings"
	"time"

	"github.com/Will-Luck/Docker-Guardian/internal/config"
	"github.com/Will-Luck/Docker-Guardian/internal/logging"
)

// Dispatcher sends notifications to all configured services.
type Dispatcher struct {
	cfg      *config.Config
	log      *logging.Logger
	client   *http.Client
	resolved []string
}

// NewDispatcher creates a notification dispatcher from config.
func NewDispatcher(cfg *config.Config, log *logging.Logger) *Dispatcher {
	return &Dispatcher{
		cfg: cfg,
		log: log,
		client: &http.Client{
			Timeout: time.Duration(cfg.CurlTimeout) * time.Second,
		},
		resolved: cfg.ResolvedNotifyEvents(),
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

// Startup sends a startup notification.
func (d *Dispatcher) Startup(text string) {
	if !d.hasEvent("startup") {
		return
	}
	d.dispatch(text)
}

// Action sends an action notification (success or failure).
func (d *Dispatcher) Action(text string) {
	if strings.Contains(text, "Failed") {
		if !d.hasEvent("actions") && !d.hasEvent("failures") {
			return
		}
	} else {
		if !d.hasEvent("actions") {
			return
		}
	}
	d.dispatch(text)
}

// Skip sends a skip notification.
func (d *Dispatcher) Skip(text string) {
	if !d.hasEvent("skips") {
		return
	}
	d.dispatch(text)
}

func (d *Dispatcher) dispatch(text string) {
	if d.hasEvent("debug") {
		now := time.Now().Format("2006-01-02T15:04:05-0700")
		services := d.ConfiguredServices()
		for _, svc := range strings.Split(services, " ") {
			if svc != "none" {
				d.log.Info(fmt.Sprintf("%s [notify] → %s: %s", now, svc, text))
			}
		}
	}

	if d.cfg.WebhookURL != "" {
		go d.sendJSON(d.cfg.WebhookURL, map[string]string{d.cfg.WebhookJSONKey: text})
	}
	if d.cfg.AppriseURL != "" {
		go d.sendJSON(d.cfg.AppriseURL, map[string]string{"title": "Docker-Guardian", "body": text})
	}
	if d.cfg.GotifyURL != "" {
		go d.sendJSON(d.cfg.GotifyURL+"/message?token="+d.cfg.GotifyToken,
			map[string]any{"title": "Docker-Guardian", "message": text, "priority": 5})
	}
	if d.cfg.DiscordWebhook != "" {
		go d.sendJSON(d.cfg.DiscordWebhook, map[string]any{
			"embeds": []map[string]any{{"title": "Docker-Guardian", "description": text, "color": 3066993}},
		})
	}
	if d.cfg.SlackWebhook != "" {
		go d.sendJSON(d.cfg.SlackWebhook, map[string]string{"text": "*Docker-Guardian*\n" + text})
	}
	if d.cfg.TelegramToken != "" {
		go d.sendJSON("https://api.telegram.org/bot"+d.cfg.TelegramToken+"/sendMessage",
			map[string]string{"chat_id": d.cfg.TelegramChatID, "text": "Docker-Guardian: " + text})
	}
	if d.cfg.PushoverToken != "" {
		go d.sendForm("https://api.pushover.net/1/messages.json", map[string]string{
			"token": d.cfg.PushoverToken, "user": d.cfg.PushoverUser,
			"title": "Docker-Guardian", "message": text,
		})
	}
	if d.cfg.PushbulletToken != "" {
		go d.sendJSONWithHeader("https://api.pushbullet.com/v2/pushes",
			"Access-Token", d.cfg.PushbulletToken,
			map[string]string{"type": "note", "title": "Docker-Guardian", "body": text})
	}
	if d.cfg.LunaSeaWebhook != "" {
		go d.sendJSON(d.cfg.LunaSeaWebhook, map[string]string{"title": "Docker-Guardian", "body": text})
	}
	if d.cfg.EmailSMTP != "" {
		go d.sendEmail(text)
	}
}

func (d *Dispatcher) sendJSON(url string, payload any) {
	body, err := json.Marshal(payload)
	if err != nil {
		d.log.Error("failed to marshal notification payload", "error", err)
		return
	}
	resp, err := d.client.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		d.log.Debug("notification send failed", "url", url, "error", err)
		return
	}
	resp.Body.Close()
}

func (d *Dispatcher) sendJSONWithHeader(url, headerKey, headerVal string, payload any) {
	body, err := json.Marshal(payload)
	if err != nil {
		return
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(headerKey, headerVal)
	resp, err := d.client.Do(req)
	if err != nil {
		return
	}
	resp.Body.Close()
}

func (d *Dispatcher) sendForm(url string, fields map[string]string) {
	var parts []string
	for k, v := range fields {
		parts = append(parts, k+"="+v)
	}
	body := strings.Join(parts, "&")
	resp, err := d.client.Post(url, "application/x-www-form-urlencoded", strings.NewReader(body))
	if err != nil {
		return
	}
	resp.Body.Close()
}

func (d *Dispatcher) sendEmail(text string) {
	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: Docker-Guardian Alert\r\n\r\n%s",
		d.cfg.EmailFrom, d.cfg.EmailTo, text)

	auth := smtp.PlainAuth("", d.cfg.EmailUser, d.cfg.EmailPass, strings.Split(d.cfg.EmailSMTP, ":")[0])
	err := smtp.SendMail(d.cfg.EmailSMTP, auth, d.cfg.EmailFrom, []string{d.cfg.EmailTo}, []byte(msg))
	if err != nil {
		d.log.Debug("email notification failed", "error", err)
	}
}
