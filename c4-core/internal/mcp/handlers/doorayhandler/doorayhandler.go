package doorayhandler

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/changmin/c4-core/internal/mcp"
	"github.com/changmin/c4-core/internal/serve"
)

// Register registers Dooray MCP tools for polling and replying.
func Register(reg *mcp.Registry, poller *serve.DoorayPollerComponent) {
	if poller == nil {
		return
	}

	reg.Register(mcp.ToolSchema{
		Name:        "c4_dooray_poll",
		Description: "Check for pending Dooray messages. Returns messages from the Hub pending queue.",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}, func(rawArgs json.RawMessage) (any, error) {
		msgs := poller.PopAll()
		if len(msgs) == 0 {
			return map[string]any{"messages": []any{}, "count": 0}, nil
		}
		result := make([]map[string]any, len(msgs))
		for i, m := range msgs {
			result[i] = map[string]any{
				"sender":  m.SenderName,
				"text":    m.Text,
				"channel": m.ChannelID,
				"time":    m.ReceivedAt.Format(time.RFC3339),
			}
		}
		return map[string]any{"messages": result, "count": len(result)}, nil
	})

	reg.Register(mcp.ToolSchema{
		Name:        "c4_dooray_reply",
		Description: "Send a reply message to Dooray via Incoming Webhook.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"text": map[string]any{
					"type":        "string",
					"description": "Reply text to send to Dooray",
				},
			},
			"required": []string{"text"},
		},
	}, func(rawArgs json.RawMessage) (any, error) {
		var args struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal(rawArgs, &args); err != nil {
			return nil, fmt.Errorf("parse args: %w", err)
		}
		if args.Text == "" {
			return nil, fmt.Errorf("text is required")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := poller.Reply(ctx, args.Text); err != nil {
			return nil, fmt.Errorf("reply failed: %w", err)
		}
		return map[string]any{"ok": true, "sent": args.Text}, nil
	})
}
