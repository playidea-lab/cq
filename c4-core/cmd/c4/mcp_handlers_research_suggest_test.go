//go:build research

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/changmin/c4-core/internal/knowledge"
)

// fakeSuggestLLMCaller is a test double for suggestGatewayLLMCaller.
type fakeSuggestLLMCaller struct {
	response string
	err      error
}

func (f *fakeSuggestLLMCaller) call(_ context.Context, _, _ string) (string, error) {
	return f.response, f.err
}

// suggestHandlerWithCaller is a variant of researchSuggestHandler that accepts the fake caller.
func suggestHandlerWithCaller(ks *knowledge.Store, caller interface {
	call(context.Context, string, string) (string, error)
}) func(json.RawMessage) (any, error) {
	return func(rawArgs json.RawMessage) (any, error) {
		var params struct {
			Tag   string `json:"tag"`
			Limit int    `json:"limit"`
		}
		if len(rawArgs) > 0 {
			if err := json.Unmarshal(rawArgs, &params); err != nil {
				return nil, fmt.Errorf("parsing arguments: %w", err)
			}
		}
		if params.Limit <= 0 {
			params.Limit = 10
		}

		experiments, err := ks.List(string(knowledge.TypeExperiment), params.Tag, params.Limit)
		if err != nil {
			return nil, fmt.Errorf("listing experiments: %w", err)
		}

		llmOut, err := caller.call(context.Background(), "", "")
		if err != nil {
			return nil, fmt.Errorf("LLM call failed: %w", err)
		}

		insight, yamlDraft := parseResearchSuggestOutput(llmOut)

		hypothesisID, err := ks.Create(knowledge.TypeHypothesis, map[string]any{
			"title":             truncateRunes(insight, 80),
			"domain":            "research",
			"hypothesis":        insight,
			"hypothesis_status": "pending",
		}, llmOut)
		if err != nil {
			return nil, fmt.Errorf("storing hypothesis: %w", err)
		}

		return map[string]any{
			"hypothesis_id":       hypothesisID,
			"insight":             truncateRunes(insight, 200),
			"yaml_draft_preview":  truncateRunes(yamlDraft, 200),
			"experiment_count":    len(experiments),
		}, nil
	}
}

func TestResearchSuggestHandler_NoTag(t *testing.T) {
	dir := t.TempDir()
	ks, err := knowledge.NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer ks.Close()

	// Pre-populate one experiment
	_, err = ks.Create(knowledge.TypeExperiment, map[string]any{
		"title":  "Exp A",
		"domain": "ml",
	}, "experiment body")
	if err != nil {
		t.Fatalf("Create experiment: %v", err)
	}

	caller := &fakeSuggestLLMCaller{
		response: "INSIGHT: Test hypothesis\nYAML_DRAFT:\nexperiment: test",
	}
	handler := suggestHandlerWithCaller(ks, caller)

	rawArgs := json.RawMessage(`{}`)
	result, err := handler(rawArgs)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}

	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map, got %T", result)
	}
	if m["hypothesis_id"] == "" {
		t.Error("expected non-empty hypothesis_id")
	}
	if m["insight"] != "Test hypothesis" {
		t.Errorf("insight = %q, want %q", m["insight"], "Test hypothesis")
	}
	if m["experiment_count"] != 1 {
		t.Errorf("experiment_count = %v, want 1", m["experiment_count"])
	}
}

func TestResearchSuggestHandler_WithTag(t *testing.T) {
	dir := t.TempDir()
	ks, err := knowledge.NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer ks.Close()

	caller := &fakeSuggestLLMCaller{
		response: "INSIGHT: Tagged hypothesis\nYAML_DRAFT:\nrun: python train.py",
	}
	handler := suggestHandlerWithCaller(ks, caller)

	rawArgs := json.RawMessage(`{"tag":"ml","limit":5}`)
	result, err := handler(rawArgs)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}

	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map, got %T", result)
	}
	if m["insight"] != "Tagged hypothesis" {
		t.Errorf("insight = %q, want %q", m["insight"], "Tagged hypothesis")
	}
	if m["yaml_draft_preview"] != "run: python train.py" {
		t.Errorf("yaml_draft_preview = %q, want %q", m["yaml_draft_preview"], "run: python train.py")
	}
	// No experiments seeded with tag "ml", so count should be 0
	if m["experiment_count"] != 0 {
		t.Errorf("experiment_count = %v, want 0", m["experiment_count"])
	}
}

func TestResearchSuggestHandler_LLMError(t *testing.T) {
	dir := t.TempDir()
	ks, err := knowledge.NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer ks.Close()

	caller := &fakeSuggestLLMCaller{
		err: fmt.Errorf("LLM unavailable"),
	}
	handler := suggestHandlerWithCaller(ks, caller)

	rawArgs := json.RawMessage(`{}`)
	_, err = handler(rawArgs)
	if err == nil {
		t.Fatal("expected error from LLM failure, got nil")
	}
}

func TestTruncateRunes_Korean(t *testing.T) {
	// Each Korean char is 3 bytes in UTF-8, so byte truncation at 200 would cut CJK text.
	korean := "가나다라마바사아자차카타파하" // 14 runes
	result := truncateRunes(korean, 10)
	runes := []rune(result)
	if len(runes) != 10 {
		t.Errorf("len(runes) = %d, want 10", len(runes))
	}
	// Verify no garbled bytes
	for _, r := range result {
		if r == '?' {
			t.Error("unexpected ? rune, possible byte corruption")
		}
	}
}

func TestTruncateRunes_Short(t *testing.T) {
	s := "hello"
	if truncateRunes(s, 200) != s {
		t.Errorf("short string should not be truncated")
	}
}
