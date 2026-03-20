package doorayhandler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/changmin/c4-core/internal/mcp"
)

// DoorayConfig holds Hub connection info for Dooray MCP tools.
type DoorayConfig struct {
	HubURL     string // Hub API base URL (e.g. https://piqsol-c5.fly.dev)
	WebhookURL string // Dooray Incoming Webhook URL for replies
}

// Register registers c4_dooray_poll and c4_dooray_reply MCP tools.
// These tools talk directly to the Hub API — no local poller needed.
func Register(reg *mcp.Registry, cfg DoorayConfig) {
	if cfg.HubURL == "" {
		return
	}

	client := &http.Client{Timeout: 10 * time.Second}

	reg.Register(mcp.ToolSchema{
		Name:        "c4_dooray_poll",
		Description: "Check for pending Dooray messages from Hub. Returns messages sent via /cq slash command.",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}, func(rawArgs json.RawMessage) (any, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, cfg.HubURL+"/v1/dooray/pending", nil)
		if err != nil {
			return nil, fmt.Errorf("create request: %w", err)
		}
		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("fetch pending: %w", err)
		}
		defer resp.Body.Close()

		var msgs []map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&msgs); err != nil {
			return nil, fmt.Errorf("parse response: %w", err)
		}

		return map[string]any{"messages": msgs, "count": len(msgs)}, nil
	})

	reg.Register(mcp.ToolSchema{
		Name:        "c4_dooray_reply",
		Description: "Send a reply to Dooray via Incoming Webhook. Use after receiving a message from c4_dooray_poll.",
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

		webhookURL := cfg.WebhookURL
		if webhookURL == "" {
			return nil, fmt.Errorf("dooray webhook URL not configured (set dooray.webhook_url secret)")
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		body, _ := json.Marshal(map[string]string{
			"botName": "CQ",
			"text":    args.Text,
		})
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("create request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("send reply: %w", err)
		}
		defer resp.Body.Close()
		io.Copy(io.Discard, io.LimitReader(resp.Body, 4096)) //nolint:errcheck

		return map[string]any{"ok": true}, nil
	})
}
