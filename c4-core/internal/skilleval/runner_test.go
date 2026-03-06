package skilleval

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/changmin/c4-core/internal/llm"
)

// mockProvider is a fake LLM provider for testing.
type mockProvider struct {
	responses []string
	callIdx   int
}

func (m *mockProvider) Name() string { return "mock" }
func (m *mockProvider) Models() []llm.ModelInfo {
	return []llm.ModelInfo{{Name: "mock-model"}}
}
func (m *mockProvider) IsAvailable() bool { return true }
func (m *mockProvider) Chat(_ context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
	if m.callIdx >= len(m.responses) {
		m.callIdx++
		return &llm.ChatResponse{Content: `{"should_trigger": false, "confidence": 0.5}`}, nil
	}
	resp := m.responses[m.callIdx]
	m.callIdx++
	return &llm.ChatResponse{Content: resp}, nil
}

// newMockGateway creates a Gateway wired to the mock provider.
func newMockGateway(responses []string) (*llm.Gateway, *mockProvider) {
	p := &mockProvider{responses: responses}
	gw := llm.NewGateway(llm.RoutingTable{
		Default: "mock",
		Routes: map[string]llm.ModelRef{
			"scout": {Provider: "mock", Model: "mock-model"},
		},
	})
	gw.Register(p)
	return gw, p
}

// writeEvalMD writes a minimal EVAL.md to a temp skill directory.
func writeEvalMD(t *testing.T, projectRoot, skillName, content string) {
	t.Helper()
	dir := filepath.Join(projectRoot, ".claude", "skills", skillName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "EVAL.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write EVAL.md: %v", err)
	}
}

const evalMD5Cases = `# test-skill
> Helps you finish tasks quickly

## trigger_tests
- [x] 구현 완료했으니 커밋해줘
- [x] finish the implementation
- [x] 빌드하고 커밋해
- [x] please run tests and commit
- [ ] 코드 설명해줘
`

func TestRunEval_TriggerAccuracy(t *testing.T) {
	projectRoot := t.TempDir()
	writeEvalMD(t, projectRoot, "test-skill", evalMD5Cases)

	// 5 test cases, k=1 each
	// 4 correct responses, 1 wrong
	// Cases: [x][x][x][x][ ] → expected: true true true true false
	// Responses: true true true true true (last [ ] expected false, got true → wrong)
	responses := []string{
		`{"should_trigger": true, "confidence": 0.9}`,
		`{"should_trigger": true, "confidence": 0.9}`,
		`{"should_trigger": true, "confidence": 0.9}`,
		`{"should_trigger": true, "confidence": 0.9}`,
		`{"should_trigger": true, "confidence": 0.9}`, // wrong: expected false
	}

	gw, _ := newMockGateway(responses)
	result, err := RunEval(context.Background(), gw, projectRoot, "test-skill", 1)
	if err != nil {
		t.Fatalf("RunEval: %v", err)
	}

	// 4 correct out of 5 → accuracy ≈ 0.8
	want := 4.0 / 5.0
	if result.TriggerAccuracy < want-0.01 || result.TriggerAccuracy > want+0.01 {
		t.Errorf("TriggerAccuracy = %.4f, want %.4f", result.TriggerAccuracy, want)
	}
	if result.TestCount != 5 {
		t.Errorf("TestCount = %d, want 5", result.TestCount)
	}
}

func TestRunEval_PassAtK(t *testing.T) {
	projectRoot := t.TempDir()
	// single test case, should trigger = true
	evalContent := "# test-skill\n> desc\n\n## trigger_tests\n- [x] 구현 완료해줘\n"
	writeEvalMD(t, projectRoot, "test-skill", evalContent)

	// k=3: first call returns false, second returns true → pass@k = true
	responses := []string{
		`{"should_trigger": false, "confidence": 0.4}`,
		`{"should_trigger": true, "confidence": 0.9}`,
		`{"should_trigger": false, "confidence": 0.4}`,
	}

	gw, _ := newMockGateway(responses)
	result, err := RunEval(context.Background(), gw, projectRoot, "test-skill", 3)
	if err != nil {
		t.Fatalf("RunEval: %v", err)
	}

	// At least 1 correct in 3 trials → pass@k fraction = 1.0
	if result.PassAtK < 1.0-0.01 {
		t.Errorf("PassAtK = %.4f, want 1.0", result.PassAtK)
	}
	// Not all correct → pass^k fraction = 0.0
	if result.PassK > 0.01 {
		t.Errorf("PassK = %.4f, want 0.0", result.PassK)
	}
}

func TestRunEval_PassK(t *testing.T) {
	projectRoot := t.TempDir()
	evalContent := "# test-skill\n> desc\n\n## trigger_tests\n- [x] 구현 완료해줘\n"
	writeEvalMD(t, projectRoot, "test-skill", evalContent)

	// k=3: all calls return true → pass^k = true
	responses := []string{
		`{"should_trigger": true, "confidence": 0.9}`,
		`{"should_trigger": true, "confidence": 0.9}`,
		`{"should_trigger": true, "confidence": 0.9}`,
	}

	gw, _ := newMockGateway(responses)
	result, err := RunEval(context.Background(), gw, projectRoot, "test-skill", 3)
	if err != nil {
		t.Fatalf("RunEval: %v", err)
	}

	// All correct → pass@k = 1.0, pass^k = 1.0
	if result.PassAtK < 1.0-0.01 {
		t.Errorf("PassAtK = %.4f, want 1.0", result.PassAtK)
	}
	if result.PassK < 1.0-0.01 {
		t.Errorf("PassK = %.4f, want 1.0", result.PassK)
	}
}

func TestRunEval_ExpIDFormat(t *testing.T) {
	projectRoot := t.TempDir()
	evalContent := "# test-skill\n> desc\n\n## trigger_tests\n- [x] trigger me\n"
	writeEvalMD(t, projectRoot, "test-skill", evalContent)

	gw, _ := newMockGateway([]string{`{"should_trigger": true, "confidence": 0.9}`})
	result, err := RunEval(context.Background(), gw, projectRoot, "test-skill", 1)
	if err != nil {
		t.Fatalf("RunEval: %v", err)
	}

	if result.ExpID == "" {
		t.Error("ExpID should not be empty")
	}
	// ExpID should start with "skill-eval-test-skill-"
	prefix := "skill-eval-test-skill-"
	if len(result.ExpID) < len(prefix) || result.ExpID[:len(prefix)] != prefix {
		t.Errorf("ExpID = %q, want prefix %q", result.ExpID, prefix)
	}
}

func TestRunEval_MissingEvalMD(t *testing.T) {
	projectRoot := t.TempDir()
	// No EVAL.md and no SKILL.md → should fail because auto-generate needs SKILL.md
	gw, _ := newMockGateway(nil)

	// Auto-generate will fail because SKILL.md doesn't exist
	_, err := RunEval(context.Background(), gw, projectRoot, "nonexistent-skill", 1)
	if err == nil {
		t.Fatal("expected error for missing EVAL.md and SKILL.md")
	}
}
