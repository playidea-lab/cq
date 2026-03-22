package pop

import (
	"context"
	"errors"
	"testing"
)

func TestOntologyExtractor_LLMSuccess(t *testing.T) {
	llm := &mockLLMClient{
		response: `[{"label":"Input Validation","description":"Always validates user input.","tags":["reliability","security"]}]`,
	}
	ext := NewOntologyExtractor(llm)

	nodes, err := ext.Extract(context.Background(), "Added input validation to API handlers.")
	if err != nil {
		t.Fatalf("Extract: unexpected error: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(nodes))
	}
	if nodes[0].Label != "Input Validation" {
		t.Errorf("label: got %q, want %q", nodes[0].Label, "Input Validation")
	}
	if len(nodes[0].Tags) == 0 {
		t.Error("expected tags to be set")
	}
}

func TestOntologyExtractor_LLMFails_FallsBackToRuleBased(t *testing.T) {
	llm := &mockLLMClient{err: errors.New("llm unavailable")}
	ext := NewOntologyExtractor(llm)

	// summarizeDiff-style input — rule-based persona.AnalyzeEdits will handle it
	nodes, err := ext.Extract(context.Background(), "refactored the function to be shorter")
	if err != nil {
		t.Fatalf("Extract: should not return error on LLM failure, got: %v", err)
	}
	// Fallback must return at least one node
	if len(nodes) == 0 {
		t.Fatal("expected at least 1 fallback node, got 0")
	}
}

func TestOntologyExtractor_LLMReturnsNoNodes_FallsBack(t *testing.T) {
	llm := &mockLLMClient{response: "[]"}
	ext := NewOntologyExtractor(llm)

	nodes, err := ext.Extract(context.Background(), "Added error handling")
	if err != nil {
		t.Fatalf("Extract: unexpected error: %v", err)
	}
	// Fallback must return at least one node
	if len(nodes) == 0 {
		t.Fatal("expected at least 1 fallback node when LLM returns empty, got 0")
	}
}

func TestOntologyExtractor_EmptySummary(t *testing.T) {
	llm := &mockLLMClient{response: "[]"}
	ext := NewOntologyExtractor(llm)

	nodes, err := ext.Extract(context.Background(), "")
	if err != nil {
		t.Fatalf("Extract: unexpected error: %v", err)
	}
	if len(nodes) != 0 {
		t.Fatalf("expected 0 nodes for empty summary, got %d", len(nodes))
	}
}

func TestOntologyExtractor_LLMReturnsInvalidJSON_FallsBack(t *testing.T) {
	llm := &mockLLMClient{response: "not valid json"}
	ext := NewOntologyExtractor(llm)

	nodes, err := ext.Extract(context.Background(), "Some behavior summary")
	if err != nil {
		t.Fatalf("Extract: unexpected error: %v", err)
	}
	if len(nodes) == 0 {
		t.Fatal("expected fallback nodes on invalid JSON, got 0")
	}
}

func TestParseOntologyNodes_ValidJSON(t *testing.T) {
	raw := `preamble [{"label":"T1","description":"D1","tags":["a","b"]}] trailing`
	nodes := parseOntologyNodes(raw)
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(nodes))
	}
	if nodes[0].Label != "T1" {
		t.Errorf("label: got %q, want %q", nodes[0].Label, "T1")
	}
	if nodes[0].Description != "D1" {
		t.Errorf("description: got %q, want %q", nodes[0].Description, "D1")
	}
}

func TestParseOntologyNodes_InvalidJSON(t *testing.T) {
	nodes := parseOntologyNodes("not json")
	if nodes != nil {
		t.Fatalf("expected nil for invalid JSON, got %v", nodes)
	}
}

func TestParseOntologyNodes_SkipsEmptyLabel(t *testing.T) {
	raw := `[{"label":"","description":"no label"},{"label":"Valid","description":"has label"}]`
	nodes := parseOntologyNodes(raw)
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node (empty label skipped), got %d", len(nodes))
	}
	if nodes[0].Label != "Valid" {
		t.Errorf("label: got %q, want %q", nodes[0].Label, "Valid")
	}
}

func TestRuleBasedNodes_WithSummary(t *testing.T) {
	nodes := ruleBasedNodes("Added feature X to the system")
	if len(nodes) == 0 {
		t.Fatal("expected at least 1 rule-based node")
	}
}

func TestRuleBasedNodes_EmptySummary(t *testing.T) {
	nodes := ruleBasedNodes("")
	if len(nodes) != 0 {
		t.Fatalf("expected 0 nodes for empty summary, got %d", len(nodes))
	}
}
