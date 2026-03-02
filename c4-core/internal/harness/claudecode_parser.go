package harness

import (
	"encoding/json"

	"github.com/changmin/c4-core/internal/c1push"
)

// claudeCodeLine represents a single JSONL line in a Claude Code journal file.
// Claude Code emits lines with type "user" or "assistant" (and "summary" for meta).
type claudeCodeLine struct {
	Type    string          `json:"type"`
	UUID    string          `json:"uuid"`
	IsMeta  bool            `json:"isMeta"`
	Message json.RawMessage `json:"message"`
}

// ParseClaudeCodeLine parses a single JSON line from a Claude Code journal file
// and returns a PushMessage. Returns nil if the line should be skipped (meta, unparseable).
func ParseClaudeCodeLine(data []byte) (*c1push.PushMessage, error) {
	var line claudeCodeLine
	if err := json.Unmarshal(data, &line); err != nil {
		return nil, err
	}

	// Skip meta lines (summaries, system internal events).
	if line.IsMeta {
		return nil, nil
	}

	// Determine sender type.
	senderType := line.Type
	if senderType == "" {
		return nil, nil
	}

	// Extract content from message field.
	content := extractContent(line.Message)
	if content == "" {
		return nil, nil
	}

	return &c1push.PushMessage{
		SenderName: senderType,
		SenderType: senderType,
		Content:    content,
	}, nil
}

// extractContent attempts to get a text content string from a Claude Code message payload.
// The message field can be {"content": "string"} or {"content": [{"type":"text","text":"..."}]}.
func extractContent(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}

	// Try simple string content first.
	var msg struct {
		Content json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(raw, &msg); err != nil || len(msg.Content) == 0 {
		return ""
	}

	// Try string.
	var s string
	if err := json.Unmarshal(msg.Content, &s); err == nil {
		return s
	}

	// Try array of content blocks.
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(msg.Content, &blocks); err == nil {
		for _, b := range blocks {
			if b.Type == "text" && b.Text != "" {
				return b.Text
			}
		}
	}
	return ""
}
