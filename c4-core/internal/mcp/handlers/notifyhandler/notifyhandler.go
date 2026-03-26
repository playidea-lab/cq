package notifyhandler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/changmin/c4-core/internal/botstore"
	"github.com/changmin/c4-core/internal/chat"
	"github.com/changmin/c4-core/internal/mcp"
	"github.com/changmin/c4-core/internal/notify"
)

const configFile = "notifications.json"

// notifyConfig holds persisted notification channel configuration.
// Stored in .c4/notifications.json (0o640, .c4/ is gitignored).
type notifyConfig struct {
	BotUsername string   `json:"bot_username"`
	Events      []string `json:"events,omitempty"` // if empty → all events pass
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

// Register registers c4_notification_set, c4_notification_get, c4_notify.
// router is optional: when non-nil, c4_notify also posts to c1_messages (sender_type=agent).
func Register(reg *mcp.Registry, projectDir string, router *chat.Router) {
	// Shared botstore instance — avoids re-resolving paths on every call.
	bs, bsErr := botstore.New(projectDir)

	reg.Register(mcp.ToolSchema{
		Name:        "cq_notification_set",
		Description: "Configure a notification channel (Telegram bot). Stores bot_username and optional event filter in .c4/notifications.json. The bot must be registered in the botstore.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"bot_username": map[string]any{
					"type":        "string",
					"description": "Telegram bot username registered in the botstore",
				},
				"events": map[string]any{
					"type":        "array",
					"description": "Workflow event types that trigger a notification. Empty = all events. e.g. [\"plan.created\",\"finish.complete\",\"checkpoint.ready\"]",
					"items":       map[string]any{"type": "string"},
				},
			},
			"required": []string{"bot_username"},
		},
	}, func(args json.RawMessage) (any, error) {
		var p struct {
			BotUsername string   `json:"bot_username"`
			Events      []string `json:"events"`
		}
		if err := json.Unmarshal(args, &p); err != nil {
			return nil, fmt.Errorf("invalid args: %w", err)
		}
		if p.BotUsername == "" {
			return nil, fmt.Errorf("bot_username is required")
		}
		// Validate bot exists in botstore.
		if bsErr != nil {
			return nil, fmt.Errorf("botstore: %w", bsErr)
		}
		if _, err := bs.Get(p.BotUsername); err != nil {
			return nil, fmt.Errorf("bot %q not found in botstore: %w", p.BotUsername, err)
		}
		cfg := &notifyConfig{BotUsername: p.BotUsername, Events: p.Events}
		if err := saveConfig(projectDir, cfg); err != nil {
			return nil, fmt.Errorf("saving config: %w", err)
		}
		return map[string]any{"success": true, "bot_username": p.BotUsername, "events": p.Events}, nil
	})

	reg.Register(mcp.ToolSchema{
		Name:        "cq_notification_get",
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
			"configured":   true,
			"bot_username": cfg.BotUsername,
			"events":       cfg.Events,
		}, nil
	})

	reg.Register(mcp.ToolSchema{
		Name:        "cq_notify",
		Description: "Send a notification message via the configured Telegram bot. If event is provided and events filter is configured, skips silently when event not in list.",
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

		// Resolve token and chat_id from botstore.
		if bsErr != nil {
			return nil, fmt.Errorf("botstore: %w", bsErr)
		}
		bot, err := bs.Get(cfg.BotUsername)
		if err != nil {
			return nil, fmt.Errorf("bot %q not found in botstore: %w", cfg.BotUsername, err)
		}
		if len(bot.AllowFrom) == 0 {
			return nil, fmt.Errorf("bot %q has no AllowFrom entries; cannot determine chat_id", cfg.BotUsername)
		}
		chatID := strconv.FormatInt(bot.AllowFrom[0], 10)

		text := p.Message
		if p.Title != "" {
			text = p.Title + "\n" + p.Message
		}

		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if err := notify.SendTelegram(ctx, bot.Token, chatID, text); err != nil {
			return nil, fmt.Errorf("telegram send failed: %w", err)
		}
		// Mirror to c1_messages (best-effort; errors do not fail the tool call).
		if router != nil {
			_ = router.Post("agent", text)
		}
		return map[string]any{"sent": true, "bot_username": cfg.BotUsername}, nil
	})
}
