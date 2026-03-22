package ontology

import (
	"context"
	"errors"
	"testing"
)

// mockCrossLLM is a test stub for CrossLLMClient.
type mockCrossLLM struct {
	response string
	err      error
}

func (m *mockCrossLLM) Complete(_ context.Context, _ string) (string, error) {
	return m.response, m.err
}

func TestCrossPositionDetector_DetectsRoles(t *testing.T) {
	llm := &mockCrossLLM{
		response: `{"source_role": "frontend", "target_role": "backend"}`,
	}
	d := NewCrossPositionDetector(llm)

	result, err := d.Detect(context.Background(), "Frontend dev suggested backend should validate inputs more strictly.")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Detected {
		t.Fatal("expected Detected=true")
	}
	if result.SourceRole != "frontend" {
		t.Errorf("expected source_role=frontend, got %q", result.SourceRole)
	}
	if result.TargetRole != "backend" {
		t.Errorf("expected target_role=backend, got %q", result.TargetRole)
	}
	if result.Scope != "cross:frontend→backend" {
		t.Errorf("expected scope=cross:frontend→backend, got %q", result.Scope)
	}
}

func TestCrossPositionDetector_NoCrossPosition(t *testing.T) {
	llm := &mockCrossLLM{
		response: `{"source_role": "", "target_role": ""}`,
	}
	d := NewCrossPositionDetector(llm)

	result, err := d.Detect(context.Background(), "Developer refactored the sorting algorithm.")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Detected {
		t.Error("expected Detected=false when no roles extracted")
	}
	if result.Scope != "" {
		t.Errorf("expected empty scope, got %q", result.Scope)
	}
}

func TestCrossPositionDetector_LLMError_ReturnsNoDetection(t *testing.T) {
	llm := &mockCrossLLM{err: errors.New("llm unavailable")}
	d := NewCrossPositionDetector(llm)

	result, err := d.Detect(context.Background(), "Some summary text.")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Detected {
		t.Error("expected Detected=false on LLM error")
	}
}

func TestCrossPositionDetector_InvalidJSON_ReturnsNoDetection(t *testing.T) {
	llm := &mockCrossLLM{response: "not valid json at all"}
	d := NewCrossPositionDetector(llm)

	result, err := d.Detect(context.Background(), "Some summary text.")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Detected {
		t.Error("expected Detected=false on invalid JSON")
	}
}

func TestCrossPositionDetector_EmptySummary_ReturnsNoDetection(t *testing.T) {
	llm := &mockCrossLLM{response: `{"source_role": "researcher", "target_role": "engineer"}`}
	d := NewCrossPositionDetector(llm)

	result, err := d.Detect(context.Background(), "   ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Detected {
		t.Error("expected Detected=false for empty summary")
	}
}

func TestCrossPositionDetector_PartialRoles_NoScope(t *testing.T) {
	// Only source role, no target → should not detect
	llm := &mockCrossLLM{response: `{"source_role": "researcher", "target_role": ""}`}
	d := NewCrossPositionDetector(llm)

	result, err := d.Detect(context.Background(), "Researcher noted that the API is slow.")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Detected {
		t.Error("expected Detected=false when target_role is empty")
	}
}

func TestCrossScope_FormatsCorrectly(t *testing.T) {
	cases := []struct {
		src, tgt string
		want     string
	}{
		{"frontend", "backend", "cross:frontend→backend"},
		{"researcher", "engineer", "cross:researcher→engineer"},
		{"", "backend", ""},
		{"frontend", "", ""},
		{"", "", ""},
		{" ", " ", ""},
	}
	for _, tc := range cases {
		got := crossScope(tc.src, tc.tgt)
		if got != tc.want {
			t.Errorf("crossScope(%q, %q) = %q, want %q", tc.src, tc.tgt, got, tc.want)
		}
	}
}

func TestTagNode_AppliesCrossPosition(t *testing.T) {
	node := Node{Label: "API Contract", NodeConfidence: ConfidenceHigh}
	result := DetectResult{
		SourceRole: "frontend",
		TargetRole: "backend",
		Scope:      "cross:frontend→backend",
		Detected:   true,
	}
	tagged := TagNode(node, result)

	if tagged.Scope != "cross:frontend→backend" {
		t.Errorf("expected scope=cross:frontend→backend, got %q", tagged.Scope)
	}
	if tagged.SourceRole != "frontend" {
		t.Errorf("expected source_role=frontend, got %q", tagged.SourceRole)
	}
	if tagged.Properties["target_role"] != "backend" {
		t.Errorf("expected target_role=backend in properties, got %q", tagged.Properties["target_role"])
	}
}

func TestTagNode_NoDetection_NodeUnchanged(t *testing.T) {
	node := Node{Label: "Caching", Scope: "project", SourceRole: "backend"}
	result := DetectResult{Detected: false}
	tagged := TagNode(node, result)

	if tagged.Scope != "project" {
		t.Errorf("expected scope unchanged, got %q", tagged.Scope)
	}
	if tagged.SourceRole != "backend" {
		t.Errorf("expected source_role unchanged, got %q", tagged.SourceRole)
	}
}

func TestParseCrossRoles_WithSurroundingText(t *testing.T) {
	raw := `Here is the result: {"source_role": "designer", "target_role": "frontend"} as requested.`
	roles := parseCrossRoles(raw)
	if roles.SourceRole != "designer" {
		t.Errorf("expected source_role=designer, got %q", roles.SourceRole)
	}
	if roles.TargetRole != "frontend" {
		t.Errorf("expected target_role=frontend, got %q", roles.TargetRole)
	}
}
