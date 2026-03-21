package harness

import (
	"encoding/json"
	"strings"

	"github.com/changmin/c4-core/internal/channelpush"
)

// LLMUsageInfo holds model and token usage extracted from a Claude Code assistant line.
type LLMUsageInfo struct {
	Model      string
	Provider   string
	InputTok   int
	OutputTok  int
	CacheRead  int
	CacheWrite int
}

// ExtractUsage parses a single JSONL line and returns usage info if it is an assistant
// message with a usage field. Returns nil, nil when the line is not applicable.
func ExtractUsage(data []byte) (*LLMUsageInfo, error) {
	var line claudeCodeLine
	if err := json.Unmarshal(data, &line); err != nil {
		return nil, err
	}

	if line.Type != "assistant" {
		return nil, nil
	}

	var msg struct {
		Model string `json:"model"`
		Usage *struct {
			InputTokens              int `json:"input_tokens"`
			OutputTokens             int `json:"output_tokens"`
			CacheReadInputTokens     int `json:"cache_read_input_tokens"`
			CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(line.Message, &msg); err != nil {
		return nil, err
	}

	if msg.Usage == nil {
		return nil, nil
	}

	provider := inferProvider(msg.Model)

	return &LLMUsageInfo{
		Model:      msg.Model,
		Provider:   provider,
		InputTok:   msg.Usage.InputTokens,
		OutputTok:  msg.Usage.OutputTokens,
		CacheRead:  msg.Usage.CacheReadInputTokens,
		CacheWrite: msg.Usage.CacheCreationInputTokens,
	}, nil
}

// inferProvider maps a model name prefix to a provider string.
func inferProvider(model string) string {
	switch {
	case strings.HasPrefix(model, "claude-"):
		return "anthropic"
	case strings.HasPrefix(model, "gpt-"):
		return "openai"
	case strings.HasPrefix(model, "gemini-"):
		return "google"
	default:
		return "unknown"
	}
}

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
func ParseClaudeCodeLine(data []byte) (*channelpush.PushMessage, error) {
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

	return &channelpush.PushMessage{
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
