// Package notifyhandler registers c4_notification_set and c4_notification_get MCP tools.
package notifyhandler

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/changmin/c4-core/internal/mcp"
	"github.com/changmin/c4-core/internal/notify"
)

// Register registers c4_notification_set and c4_notification_get.
// projectDir is the root of the project (.c4 sibling).
// userID identifies the soul directory; if empty, "default" is used.
func Register(reg *mcp.Registry, projectDir, userID string) {
	if userID == "" {
		userID = "default"
	}
	soulDir := filepath.Join(projectDir, ".c4", "souls", userID)

	registerSet(reg, soulDir)
	registerGet(reg, soulDir)
}

func registerSet(reg *mcp.Registry, soulDir string) {
	reg.Register(mcp.ToolSchema{
		Name:        "c4_notification_set",
		Description: "Set per-user notification preferences (channel, events, webhook secret key). Stored in soul JSON.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"channel": map[string]any{
					"type":        "string",
					"description": "Target platform: dooray|slack|discord|teams",
					"enum":        []string{"dooray", "slack", "discord", "teams"},
				},
				"events": map[string]any{
					"type":        "array",
					"description": "Workflow events that trigger a notification",
					"items":       map[string]any{"type": "string"},
				},
				"webhook_secret_key": map[string]any{
					"type":        "string",
					"description": "Secret store key whose value is the webhook URL (e.g. notification.dooray.webhook)",
				},
			},
			"required": []string{"channel", "webhook_secret_key"},
		},
	}, func(args json.RawMessage) (any, error) {
		return handleSet(soulDir, args)
	})
}

func registerGet(reg *mcp.Registry, soulDir string) {
	reg.Register(mcp.ToolSchema{
		Name:        "c4_notification_get",
		Description: "현재 유저의 알람 설정 조회. soul JSON에서 preference를 읽어 반환 (webhook URL 제외).",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}, func(args json.RawMessage) (any, error) {
		return handleGet(soulDir)
	})
}

func handleSet(soulDir string, rawArgs json.RawMessage) (any, error) {
	var params struct {
		Channel          string   `json:"channel"`
		Events           []string `json:"events"`
		WebhookSecretKey string   `json:"webhook_secret_key"`
	}
	if err := json.Unmarshal(rawArgs, &params); err != nil {
		return nil, fmt.Errorf("notification_set: parse args: %w", err)
	}
	if params.Channel == "" {
		return nil, fmt.Errorf("notification_set: channel is required")
	}
	if params.WebhookSecretKey == "" {
		return nil, fmt.Errorf("notification_set: webhook_secret_key is required")
	}

	p := &notify.NotificationProfile{
		Channel:          params.Channel,
		Events:           params.Events,
		WebhookSecretKey: params.WebhookSecretKey,
	}
	if err := p.Save(soulDir); err != nil {
		return nil, fmt.Errorf("notification_set: save: %w", err)
	}
	return map[string]any{"ok": true}, nil
}

func handleGet(soulDir string) (any, error) {
	p, err := notify.LoadProfile(soulDir)
	if err != nil {
		return nil, fmt.Errorf("notification_get: load: %w", err)
	}
	if p == nil {
		return map[string]any{"configured": false}, nil
	}
	return map[string]any{
		"configured":         true,
		"channel":            p.Channel,
		"events":             p.Events,
		"webhook_secret_key": p.WebhookSecretKey,
	}, nil
}
