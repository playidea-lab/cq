package memory

import (
	"strings"
	"testing"
)

func TestClaudeCodeParser_Source(t *testing.T) {
	p := &ClaudeCodeParser{}
	if got := p.Source(); got != "claude-code" {
		t.Errorf("Source() = %q, want %q", got, "claude-code")
	}
}

func TestClaudeCodeParser_Parse_ThreeTurns(t *testing.T) {
	// JSONL with 3 turns: user (string content), assistant (array content), user (string content).
	input := `{"type":"summary","role":"user","message":{"role":"user","content":"Hello from user"}}
{"type":"summary","role":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Hello from assistant"}]}}
{"type":"summary","role":"user","message":{"role":"user","content":"Follow up question"}}
`

	p := &ClaudeCodeParser{}
	sessions, err := p.Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("got %d sessions, want 1", len(sessions))
	}

	s := sessions[0]
	if s.Source != "claude-code" {
		t.Errorf("Source = %q, want %q", s.Source, "claude-code")
	}
	if len(s.Turns) != 3 {
		t.Fatalf("got %d turns, want 3", len(s.Turns))
	}

	tests := []struct {
		role    string
		content string
	}{
		{"user", "Hello from user"},
		{"assistant", "Hello from assistant"},
		{"user", "Follow up question"},
	}
	for i, tt := range tests {
		if s.Turns[i].Role != tt.role {
			t.Errorf("turn[%d].Role = %q, want %q", i, s.Turns[i].Role, tt.role)
		}
		if s.Turns[i].Content != tt.content {
			t.Errorf("turn[%d].Content = %q, want %q", i, s.Turns[i].Content, tt.content)
		}
	}
}

func TestClaudeCodeParser_Parse_EmptyInput(t *testing.T) {
	p := &ClaudeCodeParser{}
	sessions, err := p.Parse(strings.NewReader(""))
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("got %d sessions, want 1", len(sessions))
	}
	if len(sessions[0].Turns) != 0 {
		t.Errorf("got %d turns, want 0", len(sessions[0].Turns))
	}
}

func TestClaudeCodeParser_Parse_ContentAsString(t *testing.T) {
	// message.content as plain string (not array).
	input := `{"type":"summary","role":"assistant","message":{"role":"assistant","content":"plain string response"}}
`
	p := &ClaudeCodeParser{}
	sessions, err := p.Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if len(sessions[0].Turns) != 1 {
		t.Fatalf("got %d turns, want 1", len(sessions[0].Turns))
	}
	if sessions[0].Turns[0].Content != "plain string response" {
		t.Errorf("content = %q, want %q", sessions[0].Turns[0].Content, "plain string response")
	}
}

func TestClaudeCodeParser_Parse_SkipsBlankLines(t *testing.T) {
	input := `
{"type":"summary","role":"user","message":{"role":"user","content":"msg"}}

`
	p := &ClaudeCodeParser{}
	sessions, err := p.Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if len(sessions[0].Turns) != 1 {
		t.Fatalf("got %d turns, want 1", len(sessions[0].Turns))
	}
}

func TestClaudeCodeParser_Parse_SkipsMalformedLines(t *testing.T) {
	// Malformed JSON lines should be skipped, not cause a hard error.
	input := `not json
{"type":"summary","role":"user","message":{"role":"user","content":"valid"}}
also not json
`
	p := &ClaudeCodeParser{}
	sessions, err := p.Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if len(sessions[0].Turns) != 1 {
		t.Fatalf("got %d turns, want 1", len(sessions[0].Turns))
	}
	if sessions[0].Turns[0].Content != "valid" {
		t.Errorf("content = %q, want %q", sessions[0].Turns[0].Content, "valid")
	}
}

func TestClaudeCodeParser_Parse_TopLevelContent(t *testing.T) {
	// Some JSONL entries have top-level role+content without nested message.
	input := `{"type":"summary","role":"user","content":"top level content"}
`
	p := &ClaudeCodeParser{}
	sessions, err := p.Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if len(sessions[0].Turns) != 1 {
		t.Fatalf("got %d turns, want 1", len(sessions[0].Turns))
	}
	if sessions[0].Turns[0].Content != "top level content" {
		t.Errorf("content = %q, want %q", sessions[0].Turns[0].Content, "top level content")
	}
}

func TestClaudeCodeParser_Parse_MultipleTextBlocks(t *testing.T) {
	// Assistant content with multiple text blocks should be joined.
	input := `{"type":"summary","role":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"first"},{"type":"text","text":"second"}]}}
`
	p := &ClaudeCodeParser{}
	sessions, err := p.Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if sessions[0].Turns[0].Content != "first second" {
		t.Errorf("content = %q, want %q", sessions[0].Turns[0].Content, "first second")
	}
}
