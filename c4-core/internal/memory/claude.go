package memory

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"
	"time"
)

// ClaudeCodeParser parses Claude Code session JSONL files.
type ClaudeCodeParser struct{}

func (p *ClaudeCodeParser) Source() string { return "claude-code" }

// Parse reads a JSONL stream (one JSON object per line) and returns
// a single Session containing all turns found.
func (p *ClaudeCodeParser) Parse(r io.Reader) ([]Session, error) {
	var turns []Turn

	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 1<<20), 1<<20) // 1 MiB line buffer
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var entry claudeEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue // skip malformed lines
		}

		role := entry.Role
		var contentRaw any = entry.Content

		if entry.Message != nil {
			if entry.Message.Role != "" {
				role = entry.Message.Role
			}
			contentRaw = entry.Message.Content
		}

		if role == "" {
			continue
		}
		if role != "user" && role != "assistant" && role != "system" {
			continue
		}

		text := claudeExtractText(contentRaw)
		if text == "" {
			continue
		}

		turns = append(turns, Turn{Role: role, Content: text})
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	s := Session{
		Source:    "claude-code",
		StartedAt: time.Now().UTC(),
		Turns:    turns,
	}
	return []Session{s}, nil
}

// claudeEntry matches the JSONL format used by Claude Code sessions.
type claudeEntry struct {
	Type    string `json:"type"`
	Role    string `json:"role"`
	Message *struct {
		Role    string `json:"role"`
		Content any    `json:"content"` // string or []map[string]any
	} `json:"message"`
	Content any `json:"content"`
}

// claudeExtractText converts a content field (string or []block) to plain text.
func claudeExtractText(content any) string {
	switch v := content.(type) {
	case string:
		return strings.TrimSpace(v)
	case []any:
		var parts []string
		for _, block := range v {
			if m, ok := block.(map[string]any); ok {
				if t, ok := m["text"].(string); ok && t != "" {
					parts = append(parts, strings.TrimSpace(t))
				}
			}
		}
		return strings.Join(parts, " ")
	}
	return ""
}
