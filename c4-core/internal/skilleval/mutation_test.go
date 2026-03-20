package skilleval

import (
	"context"
	"strings"
	"testing"

	"github.com/changmin/c4-core/internal/llm"
)

// newSonnetMockGateway creates a Gateway wired to "sonnet" route.
func newSonnetMockGateway(responses []string) (*llm.Gateway, *mockProvider) {
	p := &mockProvider{responses: responses}
	gw := llm.NewGateway(llm.RoutingTable{
		Default: "mock",
		Routes: map[string]llm.ModelRef{
			"sonnet": {Provider: "mock", Model: "mock-model"},
		},
	})
	gw.Register(p)
	return gw, p
}

// TestMutation_AnalyzeFailures_NoResults verifies behavior with an empty result set.
func TestMutation_AnalyzeFailures_NoResults(t *testing.T) {
	got := AnalyzeFailures(nil)
	if got == "" {
		t.Error("AnalyzeFailures(nil) returned empty string")
	}
}

// TestMutation_AnalyzeFailures_AllPass verifies that all-pass results report no failures.
func TestMutation_AnalyzeFailures_AllPass(t *testing.T) {
	results := []EvalResult{
		{Name: "eval_a", Passed: true, Reasoning: "YES"},
		{Name: "eval_b", Passed: true, Reasoning: "YES"},
	}
	got := AnalyzeFailures(results)
	if strings.Contains(got, "[FAIL]") {
		t.Errorf("expected no FAIL markers, got: %s", got)
	}
	if !strings.Contains(got, "passed") {
		t.Errorf("expected 'passed' in output, got: %s", got)
	}
}

// TestMutation_AnalyzeFailures_PartialFailure verifies that failures are listed by name and reasoning.
func TestMutation_AnalyzeFailures_PartialFailure(t *testing.T) {
	results := []EvalResult{
		{Name: "has_greeting", Passed: true, Reasoning: "YES"},
		{Name: "is_concise", Passed: false, Reasoning: "NO - output is too long"},
		{Name: "no_errors", Passed: false, Reasoning: "NO - error message found"},
	}
	got := AnalyzeFailures(results)

	if !strings.Contains(got, "is_concise") {
		t.Errorf("expected 'is_concise' in failure analysis, got: %s", got)
	}
	if !strings.Contains(got, "no_errors") {
		t.Errorf("expected 'no_errors' in failure analysis, got: %s", got)
	}
	if !strings.Contains(got, "2 of 3 evals failed") {
		t.Errorf("expected failure count '2 of 3 evals failed', got: %s", got)
	}
	// Passing eval should not appear as FAIL
	if strings.Count(got, "[FAIL]") != 2 {
		t.Errorf("expected exactly 2 [FAIL] markers, got: %s", got)
	}
}

// TestMutation_ProposeMutation_ReturnsDifferentContent verifies that the mutated skill differs from original.
func TestMutation_ProposeMutation_ReturnsDifferentContent(t *testing.T) {
	original := `# my-skill
> Helps you do things

## When to use
Use this skill when the user asks for help.

## Instructions
1. Do the thing
2. Return the result`

	mutated := `# my-skill
> Helps you do things

## When to use
Use this skill when the user asks for help.

## Instructions
1. Do the thing clearly and concisely
2. Return the result`

	gw, _ := newSonnetMockGateway([]string{mutated})

	failureAnalysis := "1 of 2 evals failed:\n- [FAIL] is_concise: NO - output was too verbose"

	gotMutated, desc, err := ProposeMutation(context.Background(), gw, original, failureAnalysis)
	if err != nil {
		t.Fatalf("ProposeMutation: %v", err)
	}

	if gotMutated == original {
		t.Error("mutated content is identical to original — expected a change")
	}
	if gotMutated != mutated {
		t.Errorf("mutated content mismatch\ngot:  %s\nwant: %s", gotMutated, mutated)
	}
	if desc == "" {
		t.Error("description should not be empty")
	}
}

// TestMutation_ProposeMutation_DescriptionSummary verifies that description is a non-empty one-liner.
func TestMutation_ProposeMutation_DescriptionSummary(t *testing.T) {
	original := "line one\nline two\nline three\n"
	mutated := "line one\nline two modified\nline three\n"

	gw, _ := newSonnetMockGateway([]string{mutated})

	_, desc, err := ProposeMutation(context.Background(), gw, original, "some failure analysis")
	if err != nil {
		t.Fatalf("ProposeMutation: %v", err)
	}

	if desc == "" {
		t.Error("description is empty")
	}
	// Should not contain newlines (one-liner)
	if strings.Contains(desc, "\n") {
		t.Errorf("description should be a single line, got: %q", desc)
	}
}

// TestMutation_ProposeMutation_LLMError verifies error propagation on LLM failure.
func TestMutation_ProposeMutation_LLMError(t *testing.T) {
	p := &mockProvider{err: errTest}
	gw := llm.NewGateway(llm.RoutingTable{
		Default: "mock",
		Routes:  map[string]llm.ModelRef{"sonnet": {Provider: "mock", Model: "mock-model"}},
	})
	gw.Register(p)

	_, _, err := ProposeMutation(context.Background(), gw, "some skill", "some failures")
	if err == nil {
		t.Error("expected error when LLM fails, got nil")
	}
}

// errTest is a sentinel error for mock failures.
var errTest = errSentinel("mock LLM error")

type errSentinel string

func (e errSentinel) Error() string { return string(e) }
