// Package notify provides per-user notification profile loading/saving
// and per-channel message delivery (Dooray, Slack, Discord, Teams).
//
// Import-cycle constraint: this package MUST NOT import internal/eventbus.
// All HTTP calls are made directly via net/http.
package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const notificationsFile = "notifications.json"

// NotificationProfile holds the per-user notification preferences.
// It is stored as notifications.json in the soul directory.
type NotificationProfile struct {
	// Channel is the target platform: dooray|slack|discord|teams
	Channel string `json:"channel"`
	// Events is the list of workflow events that trigger a notification:
	// plan.created|checkpoint.ready|finish.complete|run.task_started
	Events []string `json:"events"`
	// WebhookSecretKey is the secrets-store key whose value is the webhook URL.
	// Convention: notification.{channel}.webhook
	WebhookSecretKey string `json:"webhook_secret_key"`
}

// LoadProfile reads the notification profile from soulDir/notifications.json.
// Returns nil, nil when the file does not exist (not-configured is not an error).
func LoadProfile(soulDir string) (*NotificationProfile, error) {
	path := filepath.Join(soulDir, notificationsFile)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read notifications profile: %w", err)
	}

	var p NotificationProfile
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("parse notifications profile: %w", err)
	}
	return &p, nil
}

// Save writes the notification profile to soulDir/notifications.json.
// It creates the directory if it does not exist.
func (p *NotificationProfile) Save(soulDir string) error {
	if err := os.MkdirAll(soulDir, 0o750); err != nil {
		return fmt.Errorf("create soul dir: %w", err)
	}
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal notifications profile: %w", err)
	}
	path := filepath.Join(soulDir, notificationsFile)
	if err := os.WriteFile(path, data, 0o640); err != nil {
		return fmt.Errorf("write notifications profile: %w", err)
	}
	return nil
}

// Sender delivers a text message to an external platform.
type Sender interface {
	Send(ctx context.Context, message string) error
}

// NewSender constructs the appropriate Sender for the given channel type.
// webhookURL must be the resolved webhook URL (not a secret key).
// Returns an error for unknown channel types.
func NewSender(channel, webhookURL string) (Sender, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	switch channel {
	case "dooray":
		return &dooraySender{url: webhookURL, client: client}, nil
	case "slack":
		return &slackSender{url: webhookURL, client: client}, nil
	case "discord":
		return &discordSender{url: webhookURL, client: client}, nil
	case "teams":
		return &teamsSender{url: webhookURL, client: client}, nil
	default:
		return nil, fmt.Errorf("notify: unknown channel %q (want dooray|slack|discord|teams)", channel)
	}
}

// ---------------------------------------------------------------------------
// postJSON posts body to url and drains the response.
// ---------------------------------------------------------------------------

func postJSON(ctx context.Context, client *http.Client, url string, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("POST: %w", err)
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
	resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("server returned HTTP %d", resp.StatusCode)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Dooray sender
// ---------------------------------------------------------------------------

type dooraySender struct {
	url    string
	client *http.Client
}

func (s *dooraySender) Send(ctx context.Context, message string) error {
	payload := map[string]string{"text": message}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("dooray: marshal: %w", err)
	}
	if err := postJSON(ctx, s.client, s.url, body); err != nil {
		return fmt.Errorf("dooray: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Slack sender
// ---------------------------------------------------------------------------

type slackSender struct {
	url    string
	client *http.Client
}

func (s *slackSender) Send(ctx context.Context, message string) error {
	payload := map[string]string{"text": message}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("slack: marshal: %w", err)
	}
	if err := postJSON(ctx, s.client, s.url, body); err != nil {
		return fmt.Errorf("slack: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Discord sender
// ---------------------------------------------------------------------------

type discordSender struct {
	url    string
	client *http.Client
}

func (s *discordSender) Send(ctx context.Context, message string) error {
	payload := map[string]string{"content": message}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("discord: marshal: %w", err)
	}
	if err := postJSON(ctx, s.client, s.url, body); err != nil {
		return fmt.Errorf("discord: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Microsoft Teams sender
// ---------------------------------------------------------------------------

type teamsSender struct {
	url    string
	client *http.Client
}

func (s *teamsSender) Send(ctx context.Context, message string) error {
	payload := map[string]string{"@type": "MessageCard", "text": message}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("teams: marshal: %w", err)
	}
	if err := postJSON(ctx, s.client, s.url, body); err != nil {
		return fmt.Errorf("teams: %w", err)
	}
	return nil
}
