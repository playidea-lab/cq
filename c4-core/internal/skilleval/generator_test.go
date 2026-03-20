package skilleval

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateEvalMD_OutputQualitySection(t *testing.T) {
	projectRoot := t.TempDir()

	// Create a minimal SKILL.md for the test skill.
	skillDir := filepath.Join(projectRoot, ".claude", "skills", "test-gen-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	skillContent := "# test-gen-skill\n\nThis skill finalizes and commits work.\n\n## Triggers\n- user says 'commit'\n"
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillContent), 0o644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}

	// Mock LLM response that includes both trigger_tests and Output Quality Evals sections.
	mockResponse := `# test-gen-skill
> Finalizes and commits implementation work.

## trigger_tests
- [x] commit the changes
- [x] finalize and commit
- [x] push my work
- [x] wrap up and commit
- [x] finish the implementation
- [ ] debug this error
- [ ] explain this code
- [ ] add a new feature
- [ ] show git log
- [ ] what is the status

## Output Quality Evals
EVAL 1: has_trigger_tests / Question: Does the EVAL.md contain a trigger_tests section with at least 5 entries? / Pass: Section exists with 5+ entries / Fail: Section missing or fewer than 5 entries
EVAL 2: has_output_quality_section / Question: Does the EVAL.md contain an Output Quality Evals section? / Pass: Section exists with at least 1 EVAL entry / Fail: Section missing
EVAL 3: binary_questions / Question: Are all output quality evals phrased as yes/no questions? / Pass: Every EVAL question ends with '?' and is answerable yes or no / Fail: Any eval uses a scale or open-ended question`

	gw, _ := newMockGateway([]string{mockResponse})

	path, err := GenerateEvalMD(context.Background(), gw, projectRoot, "test-gen-skill")
	if err != nil {
		t.Fatalf("GenerateEvalMD: %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read EVAL.md: %v", err)
	}

	body := string(content)

	// Verify trigger_tests section is present.
	if !strings.Contains(body, "## trigger_tests") {
		t.Error("EVAL.md missing ## trigger_tests section")
	}

	// Verify Output Quality Evals section is present.
	if !strings.Contains(body, "## Output Quality Evals") {
		t.Error("EVAL.md missing ## Output Quality Evals section")
	}

	// Verify at least one EVAL entry is present.
	if !strings.Contains(body, "EVAL 1:") {
		t.Error("EVAL.md missing EVAL 1: entry in Output Quality Evals")
	}
}

func TestGenerateEvalMD_MissingSkillMD(t *testing.T) {
	projectRoot := t.TempDir()
	gw, _ := newMockGateway(nil)

	_, err := GenerateEvalMD(context.Background(), gw, projectRoot, "no-such-skill")
	if err == nil {
		t.Fatal("expected error when SKILL.md is missing")
	}
}

func TestGenerateEvalMD_PromptContainsEvalGuide(t *testing.T) {
	// Verify the eval guide constant contains the expected guide concepts.
	if !strings.Contains(evalGuide, "Binary Eval Writing Guide") {
		t.Error("evalGuide missing Binary Eval Writing Guide heading")
	}
	// Verify the prompt template references Output Quality Evals section.
	if !strings.Contains(generateEvalPrompt, "Output Quality Evals") {
		t.Error("generateEvalPrompt missing Output Quality Evals section template")
	}
	// Verify the prompt template includes EVAL N: format.
	if !strings.Contains(generateEvalPrompt, "EVAL 1:") {
		t.Error("generateEvalPrompt missing EVAL N: format template")
	}
}
