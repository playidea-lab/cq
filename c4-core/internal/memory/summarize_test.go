package memory

import (
	"strings"
	"testing"
)

func Test_formatConversation(t *testing.T) {
	sess := Session{
		Turns: []Turn{
			{Role: "user", Content: "hello"},
			{Role: "assistant", Content: "hi there"},
			{Role: "user", Content: "  "},  // empty after trim - should be skipped
			{Role: "assistant", Content: "bye"},
		},
	}
	got := formatConversation(sess)
	if !strings.Contains(got, "User: hello") {
		t.Errorf("missing user turn: %s", got)
	}
	if !strings.Contains(got, "Assistant: hi there") {
		t.Errorf("missing assistant turn: %s", got)
	}
	if !strings.Contains(got, "Assistant: bye") {
		t.Errorf("missing last assistant turn: %s", got)
	}
	// Empty turn should be skipped.
	lines := strings.Split(got, "\n\n")
	nonEmpty := 0
	for _, l := range lines {
		if strings.TrimSpace(l) != "" {
			nonEmpty++
		}
	}
	if nonEmpty != 3 {
		t.Errorf("expected 3 non-empty blocks, got %d", nonEmpty)
	}
}

func Test_truncateToTokens_Short(t *testing.T) {
	text := "short text"
	got := truncateToTokens(text, 1000)
	if got != text {
		t.Errorf("short text should not be truncated: got %q", got)
	}
}

func Test_truncateToTokens_Long(t *testing.T) {
	// Build a string longer than maxInputTokens * approxCharsPerToken
	text := strings.Repeat("a", 50000)
	got := truncateToTokens(text, 8000)
	maxChars := 8000 * approxCharsPerToken
	// Should be truncated and have the prefix marker.
	if len(got) > maxChars+50 { // allow for the prefix
		t.Errorf("truncated too long: %d chars", len(got))
	}
	if !strings.HasPrefix(got, "[이전 대화 생략]") {
		t.Errorf("missing truncation marker")
	}
}

func Test_buildImportPrompt(t *testing.T) {
	prompt := buildImportPrompt("myproject", "claude-code", "2026-03-27", "User: hi\n\nAssistant: hello\n\n")
	if !strings.Contains(prompt, "myproject") {
		t.Error("prompt missing project name")
	}
	if !strings.Contains(prompt, "claude-code") {
		t.Error("prompt missing source")
	}
	if !strings.Contains(prompt, "2026-03-27") {
		t.Error("prompt missing date")
	}
	if !strings.Contains(prompt, "## 주제") {
		t.Error("prompt missing topic heading")
	}
}

func Test_Summarize_NilLLM_Fallback(t *testing.T) {
	s := &Summarizer{LLM: nil}
	sess := Session{
		ID:      "test-001",
		Source:  "claude-code",
		Project: "test-project",
		Turns: []Turn{
			{Role: "user", Content: "hello"},
			{Role: "assistant", Content: "world"},
		},
	}
	got, err := s.Summarize(nil, sess)
	if err != nil {
		t.Fatalf("Summarize with nil LLM should fallback: %v", err)
	}
	if !strings.Contains(got, "test-project") {
		t.Errorf("fallback should contain project: %s", got)
	}
}

func Test_Summarize_EmptyConversation(t *testing.T) {
	s := &Summarizer{LLM: nil}
	sess := Session{
		ID:    "test-empty",
		Turns: nil,
	}
	_, err := s.Summarize(nil, sess)
	if err == nil {
		t.Error("expected error for empty conversation")
	}
}
