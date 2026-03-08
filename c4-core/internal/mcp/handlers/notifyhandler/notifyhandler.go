package notifyhandler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/changmin/c4-core/internal/mcp"
)

const configFile = "notifications.json"

// notifyConfig holds persisted notification channel configuration.
type notifyConfig struct {
	Channel    string `json:"channel"`
	WebhookURL string `json:"webhook_url"`
}

func configPath(projectDir string) string {
	return filepath.Join(projectDir, ".c4", configFile)
}

func loadConfig(projectDir string) (*notifyConfig, error) {
	data, err := os.ReadFile(configPath(projectDir))
	if os.IsNotExist(err) {
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
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(configPath(projectDir), data, 0600)
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
		Description: "Configure a notification channel (webhook). Stores channel name and webhook URL in .c4/notifications.json.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"channel": map[string]any{
					"type":        "string",
					"description": "Channel name (e.g. dooray, slack, discord, teams)",
				},
				"webhook_url": map[string]any{
					"type":        "string",
					"description": "Webhook URL for the channel",
				},
			},
			"required": []string{"channel", "webhook_url"},
		},
	}, func(args json.RawMessage) (any, error) {
		var p struct {
			Channel    string `json:"channel"`
			WebhookURL string `json:"webhook_url"`
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
		cfg := &notifyConfig{Channel: p.Channel, WebhookURL: p.WebhookURL}
		if err := saveConfig(projectDir, cfg); err != nil {
			return nil, fmt.Errorf("saving config: %w", err)
		}
		return map[string]any{"success": true, "channel": p.Channel}, nil
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
		}, nil
	})

	reg.Register(mcp.ToolSchema{
		Name:        "c4_notify",
		Description: "Send a notification message via the configured webhook channel.",
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
			},
			"required": []string{"message"},
		},
	}, func(args json.RawMessage) (any, error) {
		var p struct {
			Message string `json:"message"`
			Title   string `json:"title"`
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

		payload := map[string]any{"text": p.Message}
		if p.Title != "" {
			payload["title"] = p.Title
		}
		body, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}

		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Post(cfg.WebhookURL, "application/json", bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("webhook POST failed: %w", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 300 {
			return nil, fmt.Errorf("webhook returned status %d", resp.StatusCode)
		}
		return map[string]any{"sent": true, "channel": cfg.Channel, "status": resp.StatusCode}, nil
	})
}
