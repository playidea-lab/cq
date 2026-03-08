// Package notifyhandler registers the c4_notification_set MCP tool.
// It saves per-user notification preferences to the soul directory and
// stores the webhook URL in the encrypted secret store.
package notifyhandler

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/changmin/c4-core/internal/mcp"
	"github.com/changmin/c4-core/internal/notify"
	"github.com/changmin/c4-core/internal/secrets"
)

var validChannels = map[string]bool{
	"dooray": true,
	"slack":  true,
	"discord": true,
	"teams":  true,
}

var defaultEvents = []string{"plan.created", "checkpoint.ready", "finish.complete"}

// Opts holds dependencies for the notification handler.
type Opts struct {
	ProjectDir  string
	SecretStore *secrets.Store // must not be nil
}

// Register registers c4_notification_set on the given registry.
func Register(reg *mcp.Registry, opts *Opts) {
	if opts == nil || opts.SecretStore == nil {
		return
	}

	reg.Register(mcp.ToolSchema{
		Name:        "c4_notification_set",
		Description: "Save per-user notification channel configuration. The webhook URL is encrypted in the secret store; preferences are written to the soul directory.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"channel": map[string]any{
					"type":        "string",
					"enum":        []string{"dooray", "slack", "discord", "teams"},
					"description": "알람 채널 종류",
				},
				"webhook_url": map[string]any{
					"type":        "string",
					"description": "Webhook URL (암호화 저장됨)",
				},
				"events": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "string",
						"enum": []string{"plan.created", "checkpoint.ready", "finish.complete", "run.task_started"},
					},
					"description": "구독할 이벤트 목록. 기본값: [plan.created, checkpoint.ready, finish.complete]",
				},
			},
			"required": []string{"channel", "webhook_url"},
		},
	}, func(rawArgs json.RawMessage) (any, error) {
		var args struct {
			Channel    string   `json:"channel"`
			WebhookURL string   `json:"webhook_url"`
			Events     []string `json:"events"`
		}
		if err := json.Unmarshal(rawArgs, &args); err != nil {
			return nil, fmt.Errorf("invalid args: %w", err)
		}

		if !validChannels[args.Channel] {
			return nil, fmt.Errorf("invalid channel %q: must be one of dooray|slack|discord|teams", args.Channel)
		}
		if args.WebhookURL == "" {
			return nil, fmt.Errorf("webhook_url must not be empty")
		}

		events := args.Events
		if len(events) == 0 {
			events = defaultEvents
		}

		secretKey := "notification." + args.Channel + ".webhook"

		// Save webhook URL to secret store (encrypted).
		if err := opts.SecretStore.Set(secretKey, args.WebhookURL); err != nil {
			return nil, fmt.Errorf("save webhook secret: %w", err)
		}

		// Determine soul directory for current user.
		user := currentUser()
		soulDir := filepath.Join(opts.ProjectDir, ".c4", "souls", user)

		// Save notification profile to soul directory.
		profile := &notify.NotificationProfile{
			Channel:          args.Channel,
			Events:           events,
			WebhookSecretKey: secretKey,
		}
		if err := profile.Save(soulDir); err != nil {
			return nil, fmt.Errorf("save notification profile: %w", err)
		}

		return map[string]any{
			"success": true,
			"channel": args.Channel,
			"events":  events,
		}, nil
	})
}

func currentUser() string {
	if u := os.Getenv("USER"); u != "" {
		return u
	}
	if u := os.Getenv("USERNAME"); u != "" {
		return u
	}
	return "default"
}
