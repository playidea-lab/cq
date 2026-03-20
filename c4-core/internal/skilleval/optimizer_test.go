package skilleval

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/changmin/c4-core/internal/llm"
)

// newOptimizerMockGateway creates a Gateway with routes for both "sonnet" and "haiku".
func newOptimizerMockGateway(responses []string) (*llm.Gateway, *mockProvider) {
	p := &mockProvider{responses: responses}
	gw := llm.NewGateway(llm.RoutingTable{
		Default: "mock",
		Routes: map[string]llm.ModelRef{
			"sonnet": {Provider: "mock", Model: "mock-model"},
			"haiku":  {Provider: "mock", Model: "mock-model"},
		},
	})
	gw.Register(p)
	return gw, p
}

// writeSkillMD writes a SKILL.md file for testing.
func writeSkillMD(t *testing.T, projectRoot, skillName, content string) {
	t.Helper()
	dir := filepath.Join(projectRoot, ".claude", "skills", skillName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}
}

var optimizerTestEvals = []BinaryEval{
	{
		Name:     "has_greeting",
		Question: "Does the output contain a greeting?",
		Pass:     "Output contains 'hello' or 'hi'",
		Fail:     "Output does not contain a greeting",
	},
}

const testSkillContent = `# test-skill
> A test skill for optimization

## When to use
Use this skill when the user asks for help.

## Instructions
1. Greet the user
2. Do the thing`

// TestOptimizer_Baseline verifies that the optimizer correctly measures a baseline score.
func TestOptimizer_Baseline(t *testing.T) {
	projectRoot := t.TempDir()
	writeSkillMD(t, projectRoot, "test-skill", testSkillContent)

	// Responses: simulate run output (1), then score YES (1), then mutation proposal,
	// then simulate again (1), score YES (1) — but we only need baseline + 1 experiment.
	// RunsPerExperiment=1, BudgetCap=0 effectively means: just baseline, no mutations.
	// But BudgetCap minimum is 1 after clamp, so we set BudgetCap=1 and provide enough responses.
	//
	// Flow for RunsPerExperiment=1, BudgetCap=1:
	//   Baseline: simulateRun(sonnet) → "hello world" → ScoreOutput(haiku) → "YES"
	//   Exp 1: AnalyzeFailures → ProposeMutation(sonnet) → mutated skill
	//          → simulateRun(sonnet) → "hello again" → ScoreOutput(haiku) → "YES"
	responses := []string{
		"hello world",     // baseline simulate
		"YES",             // baseline score (has_greeting)
		testSkillContent,  // mutation proposal (returns same content)
		"hello again",     // exp 1 simulate
		"YES",             // exp 1 score
	}

	gw, _ := newOptimizerMockGateway(responses)

	opt := &SkillOptimizer{
		ProjectDir: projectRoot,
		LLM:        gw,
		KStore:     nil, // no knowledge store for this test
	}

	result, err := opt.Run(context.Background(), "test-skill", optimizerTestEvals, OptimizeOpts{
		RunsPerExperiment: 1,
		BudgetCap:         1,
		TargetPassRate:    0.95,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Baseline should be 1.0 (1 passed / 1 total)
	if result.Baseline < 0.99 {
		t.Errorf("Baseline = %.4f, want ~1.0", result.Baseline)
	}

	// Verify backup was created
	backupPath := filepath.Join(projectRoot, ".claude", "skills", "test-skill", "SKILL.md.baseline")
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		t.Error("SKILL.md.baseline was not created")
	}
}

// TestOptimizer_KeepDecision verifies that a mutation with better pass rate is kept.
func TestOptimizer_KeepDecision(t *testing.T) {
	projectRoot := t.TempDir()
	writeSkillMD(t, projectRoot, "test-skill", testSkillContent)

	// Flow for RunsPerExperiment=1, BudgetCap=1:
	//   Baseline: simulateRun → "no greeting" → ScoreOutput → "NO" (passRate=0)
	//   Exp 1: AnalyzeFailures (has failures) → ProposeMutation → improved skill
	//          → simulateRun → "hello!" → ScoreOutput → "YES" (passRate=1.0)
	//   Result: mutation kept (1.0 > 0.0)
	improvedSkill := `# test-skill
> A test skill with better greeting

## When to use
Use this skill when the user asks for help.

## Instructions
1. Always say hello first
2. Do the thing`

	responses := []string{
		"no greeting here", // baseline simulate — bad output
		"NO",               // baseline score — fails
		improvedSkill,      // mutation proposal
		"hello user!",      // exp 1 simulate — good output
		"YES",              // exp 1 score — passes
	}

	gw, _ := newOptimizerMockGateway(responses)

	opt := &SkillOptimizer{
		ProjectDir: projectRoot,
		LLM:        gw,
		KStore:     nil,
	}

	result, err := opt.Run(context.Background(), "test-skill", optimizerTestEvals, OptimizeOpts{
		RunsPerExperiment: 1,
		BudgetCap:         1,
		TargetPassRate:    0.95,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if result.Baseline > 0.01 {
		t.Errorf("Baseline = %.4f, want ~0.0", result.Baseline)
	}
	if result.FinalScore < 0.99 {
		t.Errorf("FinalScore = %.4f, want ~1.0", result.FinalScore)
	}
	if result.KeptCount != 1 {
		t.Errorf("KeptCount = %d, want 1", result.KeptCount)
	}
	if result.Experiments != 1 {
		t.Errorf("Experiments = %d, want 1", result.Experiments)
	}

	// Verify SKILL.md was updated with the improved version
	finalContent, err := os.ReadFile(filepath.Join(projectRoot, ".claude", "skills", "test-skill", "SKILL.md"))
	if err != nil {
		t.Fatalf("read final SKILL.md: %v", err)
	}
	if string(finalContent) != improvedSkill {
		t.Error("SKILL.md was not updated to the improved version")
	}
}

// TestOptimizer_DiscardDecision verifies that a mutation with worse pass rate is discarded.
func TestOptimizer_DiscardDecision(t *testing.T) {
	projectRoot := t.TempDir()
	writeSkillMD(t, projectRoot, "test-skill", testSkillContent)

	// Flow for RunsPerExperiment=1, BudgetCap=1:
	//   Baseline: simulateRun → "hello!" → ScoreOutput → "YES" (passRate=1.0)
	//   Exp 1: AnalyzeFailures (no failures) → ProposeMutation → bad skill
	//          → simulateRun → "no greeting" → ScoreOutput → "NO" (passRate=0)
	//   Result: mutation discarded (0 < 1.0), restore original
	badSkill := `# test-skill
> A broken skill`

	responses := []string{
		"hello there!",  // baseline simulate — good
		"YES",           // baseline score — passes
		badSkill,        // mutation proposal (worse)
		"broken output", // exp 1 simulate — bad
		"NO",            // exp 1 score — fails
	}

	gw, _ := newOptimizerMockGateway(responses)

	opt := &SkillOptimizer{
		ProjectDir: projectRoot,
		LLM:        gw,
		KStore:     nil,
	}

	result, err := opt.Run(context.Background(), "test-skill", optimizerTestEvals, OptimizeOpts{
		RunsPerExperiment: 1,
		BudgetCap:         1,
		TargetPassRate:    0.95,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if result.Baseline < 0.99 {
		t.Errorf("Baseline = %.4f, want ~1.0", result.Baseline)
	}
	if result.FinalScore < 0.99 {
		t.Errorf("FinalScore = %.4f, want ~1.0 (should keep original)", result.FinalScore)
	}
	if result.KeptCount != 0 {
		t.Errorf("KeptCount = %d, want 0 (mutation should be discarded)", result.KeptCount)
	}

	// Verify SKILL.md was restored to original
	finalContent, err := os.ReadFile(filepath.Join(projectRoot, ".claude", "skills", "test-skill", "SKILL.md"))
	if err != nil {
		t.Fatalf("read final SKILL.md: %v", err)
	}
	if string(finalContent) != testSkillContent {
		t.Error("SKILL.md was not restored to original after discard")
	}
}

// TestOptimizer_MissingSkillMD verifies error when SKILL.md doesn't exist.
func TestOptimizer_MissingSkillMD(t *testing.T) {
	projectRoot := t.TempDir()
	gw, _ := newOptimizerMockGateway(nil)

	opt := &SkillOptimizer{
		ProjectDir: projectRoot,
		LLM:        gw,
		KStore:     nil,
	}

	_, err := opt.Run(context.Background(), "nonexistent", optimizerTestEvals, OptimizeOpts{
		RunsPerExperiment: 1,
		BudgetCap:         1,
	})
	if err == nil {
		t.Error("expected error for missing SKILL.md, got nil")
	}
}

// TestOptimizer_DefaultOpts verifies that zero-value opts get reasonable defaults.
func TestOptimizer_DefaultOpts(t *testing.T) {
	projectRoot := t.TempDir()
	writeSkillMD(t, projectRoot, "test-skill", testSkillContent)

	// Provide enough responses for default RunsPerExperiment=3, BudgetCap up to 10.
	// We'll provide responses for baseline (3 runs * 1 eval = 6 calls) + 1 experiment.
	// Each run = 1 simulate + 1 score = 2 calls. 3 runs = 6 calls.
	// Then mutation = 1 call, then 3 runs again = 6 calls.
	// But the mock fallback handles exhausted responses gracefully.
	responses := make([]string, 0, 50)
	// Baseline: 3 simulate + 3 score
	for i := 0; i < 3; i++ {
		responses = append(responses, "hello output")
		responses = append(responses, "YES")
	}
	// Exp 1: mutation + 3 simulate + 3 score (all YES = target met)
	responses = append(responses, testSkillContent) // mutation returns same
	for i := 0; i < 3; i++ {
		responses = append(responses, "hello output")
		responses = append(responses, "YES")
	}
	// Exp 2-3: same pattern to hit 3 consecutive target
	for exp := 0; exp < 2; exp++ {
		// re-measure for fresh details: 3 simulate + 3 score
		for i := 0; i < 3; i++ {
			responses = append(responses, "hello output")
			responses = append(responses, "YES")
		}
		responses = append(responses, testSkillContent) // mutation
		for i := 0; i < 3; i++ {
			responses = append(responses, "hello output")
			responses = append(responses, "YES")
		}
	}

	gw, _ := newOptimizerMockGateway(responses)

	opt := &SkillOptimizer{
		ProjectDir: projectRoot,
		LLM:        gw,
		KStore:     nil,
	}

	result, err := opt.Run(context.Background(), "test-skill", optimizerTestEvals, OptimizeOpts{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Should have run with default values, baseline should be measurable
	if result.Baseline < 0 || result.Baseline > 1 {
		t.Errorf("Baseline = %.4f, want in [0, 1]", result.Baseline)
	}
}
