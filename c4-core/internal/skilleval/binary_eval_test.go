package skilleval

import (
	"context"
	"testing"

	"github.com/changmin/c4-core/internal/llm"
)

// newHaikuMockGateway creates a Gateway with a mock provider routed to "haiku".
func newHaikuMockGateway(responses []string) (*llm.Gateway, *mockProvider) {
	p := &mockProvider{responses: responses}
	gw := llm.NewGateway(llm.RoutingTable{
		Default: "mock",
		Routes: map[string]llm.ModelRef{
			"haiku": {Provider: "mock", Model: "mock-model"},
		},
	})
	gw.Register(p)
	return gw, p
}

var testEvals = []BinaryEval{
	{
		Name:     "has_greeting",
		Question: "Does the output contain a greeting?",
		Pass:     "Output contains 'hello' or 'hi'",
		Fail:     "Output does not contain a greeting",
	},
	{
		Name:     "is_concise",
		Question: "Is the output concise (under 50 words)?",
		Pass:     "Output is concise",
		Fail:     "Output is too long",
	},
	{
		Name:     "no_errors",
		Question: "Is the output free of error messages?",
		Pass:     "No error messages found",
		Fail:     "Error messages found",
	},
}

// TestBinaryEval_AllPass verifies that all YES responses result in passed == total.
func TestBinaryEval_AllPass(t *testing.T) {
	responses := []string{"YES", "YES", "YES"}
	gw, _ := newHaikuMockGateway(responses)

	passed, total, details, err := ScoreOutput(context.Background(), gw, "hello world", testEvals)
	if err != nil {
		t.Fatalf("ScoreOutput: %v", err)
	}

	if total != 3 {
		t.Errorf("total = %d, want 3", total)
	}
	if passed != 3 {
		t.Errorf("passed = %d, want 3 (all YES)", passed)
	}
	if len(details) != 3 {
		t.Errorf("len(details) = %d, want 3", len(details))
	}
	for i, d := range details {
		if !d.Passed {
			t.Errorf("details[%d].Passed = false, want true", i)
		}
	}
}

// TestBinaryEval_PartialPass verifies that mixed YES/NO responses produce correct counts.
func TestBinaryEval_PartialPass(t *testing.T) {
	// 2 YES, 1 NO
	responses := []string{"YES", "NO", "YES"}
	gw, _ := newHaikuMockGateway(responses)

	passed, total, details, err := ScoreOutput(context.Background(), gw, "hello world", testEvals)
	if err != nil {
		t.Fatalf("ScoreOutput: %v", err)
	}

	if total != 3 {
		t.Errorf("total = %d, want 3", total)
	}
	if passed != 2 {
		t.Errorf("passed = %d, want 2", passed)
	}
	if len(details) != 3 {
		t.Errorf("len(details) = %d, want 3", len(details))
	}

	// Verify individual results
	if !details[0].Passed {
		t.Errorf("details[0].Passed = false, want true")
	}
	if details[1].Passed {
		t.Errorf("details[1].Passed = true, want false")
	}
	if !details[2].Passed {
		t.Errorf("details[2].Passed = false, want true")
	}
}

// TestBinaryEval_AllFail verifies that all NO responses result in passed == 0.
func TestBinaryEval_AllFail(t *testing.T) {
	responses := []string{"NO", "NO", "NO"}
	gw, _ := newHaikuMockGateway(responses)

	passed, total, details, err := ScoreOutput(context.Background(), gw, "error: something went wrong", testEvals)
	if err != nil {
		t.Fatalf("ScoreOutput: %v", err)
	}

	if total != 3 {
		t.Errorf("total = %d, want 3", total)
	}
	if passed != 0 {
		t.Errorf("passed = %d, want 0 (all NO)", passed)
	}
	if len(details) != 3 {
		t.Errorf("len(details) = %d, want 3", len(details))
	}
	for i, d := range details {
		if d.Passed {
			t.Errorf("details[%d].Passed = true, want false", i)
		}
	}
}

// TestBinaryEval_EmptyEvals verifies that an empty eval list returns zero counts.
func TestBinaryEval_EmptyEvals(t *testing.T) {
	gw, _ := newHaikuMockGateway(nil)

	passed, total, details, err := ScoreOutput(context.Background(), gw, "some output", nil)
	if err != nil {
		t.Fatalf("ScoreOutput: %v", err)
	}

	if total != 0 {
		t.Errorf("total = %d, want 0", total)
	}
	if passed != 0 {
		t.Errorf("passed = %d, want 0", passed)
	}
	if len(details) != 0 {
		t.Errorf("len(details) = %d, want 0", len(details))
	}
}

// TestBinaryEval_EvalResultNames verifies that EvalResult.Name matches BinaryEval.Name.
func TestBinaryEval_EvalResultNames(t *testing.T) {
	responses := []string{"YES", "NO", "YES"}
	gw, _ := newHaikuMockGateway(responses)

	_, _, details, err := ScoreOutput(context.Background(), gw, "hello world", testEvals)
	if err != nil {
		t.Fatalf("ScoreOutput: %v", err)
	}

	for i, d := range details {
		if d.Name != testEvals[i].Name {
			t.Errorf("details[%d].Name = %q, want %q", i, d.Name, testEvals[i].Name)
		}
	}
}
