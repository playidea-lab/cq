package notifyhandler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/changmin/c4-core/internal/mcp"
	"github.com/changmin/c4-core/internal/notify"
)

const configFile = "notifications.json"

// notifyConfig holds persisted notification channel configuration.
// Stored in .c4/notifications.json (0o640, .c4/ is gitignored).
type notifyConfig struct {
	Channel    string   `json:"channel"`
	WebhookURL string   `json:"webhook_url"`
	Events     []string `json:"events,omitempty"` // if empty → all events pass
}

func configPath(projectDir string) string {
	return filepath.Join(projectDir, ".c4", configFile)
}

func loadConfig(projectDir string) (*notifyConfig, error) {
	data, err := os.ReadFile(configPath(projectDir))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var cfg notifyConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func saveConfig(projectDir string, cfg *notifyConfig) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Join(projectDir, ".c4")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	return os.WriteFile(configPath(projectDir), data, 0o640)
}

func maskURL(u string) string {
	if u == "" {
		return ""
	}
	idx := strings.LastIndex(u, "/")
	if idx < 0 || idx >= len(u)-1 {
		return "****"
	}
	return u[:idx+1] + "****"
}

// Register registers c4_notification_set, c4_notification_get, c4_notify.
func Register(reg *mcp.Registry, projectDir string) {
	reg.Register(mcp.ToolSchema{
		Name:        "c4_notification_set",
		Description: "Configure a notification channel (webhook). Stores channel name, webhook URL, and optional event filter in .c4/notifications.json.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"channel": map[string]any{
					"type":        "string",
					"description": "Channel name: dooray|slack|discord|teams",
					"enum":        []string{"dooray", "slack", "discord", "teams"},
				},
				"webhook_url": map[string]any{
					"type":        "string",
					"description": "Webhook URL for the channel",
				},
				"events": map[string]any{
					"type":        "array",
					"description": "Workflow event types that trigger a notification. Empty = all events. e.g. [\"plan.created\",\"finish.complete\",\"checkpoint.ready\"]",
					"items":       map[string]any{"type": "string"},
				},
			},
			"required": []string{"channel", "webhook_url"},
		},
	}, func(args json.RawMessage) (any, error) {
		var p struct {
			Channel    string   `json:"channel"`
			WebhookURL string   `json:"webhook_url"`
			Events     []string `json:"events"`
		}
		if err := json.Unmarshal(args, &p); err != nil {
			return nil, fmt.Errorf("invalid args: %w", err)
		}
		if p.Channel == "" {
			return nil, fmt.Errorf("channel is required")
		}
		if p.WebhookURL == "" {
			return nil, fmt.Errorf("webhook_url is required")
		}
		// Validate channel is supported by notify package.
		if _, err := notify.NewSender(p.Channel, p.WebhookURL); err != nil {
			return nil, fmt.Errorf("unsupported channel: %w", err)
		}
		cfg := &notifyConfig{Channel: p.Channel, WebhookURL: p.WebhookURL, Events: p.Events}
		if err := saveConfig(projectDir, cfg); err != nil {
			return nil, fmt.Errorf("saving config: %w", err)
		}
		return map[string]any{"success": true, "channel": p.Channel, "events": p.Events}, nil
	})

	reg.Register(mcp.ToolSchema{
		Name:        "c4_notification_get",
		Description: "Get configured notification channel. Returns {configured:false} if not set.",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}, func(_ json.RawMessage) (any, error) {
		cfg, err := loadConfig(projectDir)
		if err != nil {
			return nil, err
		}
		if cfg == nil {
			return map[string]any{"configured": false}, nil
		}
		return map[string]any{
			"configured":  true,
			"channel":     cfg.Channel,
			"webhook_url": maskURL(cfg.WebhookURL),
			"events":      cfg.Events,
		}, nil
	})

	reg.Register(mcp.ToolSchema{
		Name:        "c4_notify",
		Description: "Send a notification message via the configured webhook channel. If event is provided and events filter is configured, skips silently when event not in list.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"message": map[string]any{
					"type":        "string",
					"description": "Message to send",
				},
				"title": map[string]any{
					"type":        "string",
					"description": "Optional title or subject",
				},
				"event": map[string]any{
					"type":        "string",
					"description": "Workflow event type (e.g. plan.created, finish.complete, checkpoint.ready). Used for event filter matching.",
				},
			},
			"required": []string{"message"},
		},
	}, func(args json.RawMessage) (any, error) {
		var p struct {
			Message string `json:"message"`
			Title   string `json:"title"`
			Event   string `json:"event"`
		}
		if err := json.Unmarshal(args, &p); err != nil {
			return nil, fmt.Errorf("invalid args: %w", err)
		}
		if p.Message == "" {
			return nil, fmt.Errorf("message is required")
		}

		cfg, err := loadConfig(projectDir)
		if err != nil {
			return nil, err
		}
		if cfg == nil {
			return nil, fmt.Errorf("no notification channel configured; use c4_notification_set first")
		}

		// Event filter: if events list is configured and event is provided,
		// skip silently when event is not in the list.
		if p.Event != "" && len(cfg.Events) > 0 {
			matched := false
			for _, e := range cfg.Events {
				if e == p.Event {
					matched = true
					break
				}
			}
			if !matched {
				return map[string]any{"sent": false, "skipped": true, "event": p.Event}, nil
			}
		}

		sender, err := notify.NewSender(cfg.Channel, cfg.WebhookURL)
		if err != nil {
			return nil, fmt.Errorf("creating sender: %w", err)
		}

		text := p.Message
		if p.Title != "" {
			text = p.Title + "\n" + p.Message
		}

		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if err := sender.Send(ctx, text); err != nil {
			return nil, fmt.Errorf("webhook send failed: %w", err)
		}
		return map[string]any{"sent": true, "channel": cfg.Channel}, nil
	})
}
