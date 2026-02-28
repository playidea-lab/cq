//go:build c3_eventbus

package eventbushandler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/changmin/c4-core/internal/eventbus"
	"github.com/changmin/c4-core/internal/mcp"
)

// RegisterDoorayRespondTool registers the c4_dooray_respond MCP tool.
// It allows a Standby Worker to post a reply directly to a Dooray slash-command
// response URL obtained from task metadata (dooray_response_url).
func RegisterDoorayRespondTool(reg *mcp.Registry) {
	reg.Register(mcp.ToolSchema{
		Name:        "c4_dooray_respond",
		Description: "Post a reply to a Dooray slash-command via its response_url. Only *.dooray.com URLs are accepted (SSRF protection).",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"response_url": map[string]any{
					"type":        "string",
					"description": "Dooray response URL (must be *.dooray.com)",
				},
				"text": map[string]any{
					"type":        "string",
					"description": "Reply text to send to Dooray",
				},
				"response_type": map[string]any{
					"type":        "string",
					"enum":        []string{"ephemeral", "inChannel"},
					"description": "Response visibility: ephemeral (default) or inChannel",
				},
			},
			"required": []string{"response_url", "text"},
		},
	}, func(raw json.RawMessage) (any, error) {
		var args struct {
			ResponseURL  string `json:"response_url"`
			Text         string `json:"text"`
			ResponseType string `json:"response_type"`
		}
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("parse args: %w", err)
		}
		if args.ResponseURL == "" {
			return nil, fmt.Errorf("response_url is required")
		}
		if args.Text == "" {
			return nil, fmt.Errorf("text is required")
		}

		// Validate URL: must be *.dooray.com (SSRF protection)
		if err := eventbus.ValidateDoorayResponseURL(args.ResponseURL); err != nil {
			return map[string]any{
				"success": false,
				"error":   err.Error(),
			}, nil
		}

		responseType := args.ResponseType
		if responseType == "" {
			responseType = "ephemeral"
		}

		payload := map[string]any{
			"text":         args.Text,
			"responseType": responseType,
		}
		body, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("marshal payload: %w", err)
		}

		client := &http.Client{Timeout: 10 * time.Second}
		req, err := http.NewRequest("POST", args.ResponseURL, bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("create request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			return map[string]any{
				"success": false,
				"error":   fmt.Sprintf("POST failed: %v", err),
			}, nil
		}
		defer func() {
			io.Copy(io.Discard, io.LimitReader(resp.Body, 4096)) //nolint:errcheck
			resp.Body.Close()
		}()

		if resp.StatusCode >= 400 {
			return map[string]any{
				"success": false,
				"error":   fmt.Sprintf("Dooray returned HTTP %d", resp.StatusCode),
			}, nil
		}

		return map[string]any{
			"success": true,
		}, nil
	})
}
