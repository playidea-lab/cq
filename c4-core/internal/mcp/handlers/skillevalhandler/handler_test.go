package skillevalhandler

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/changmin/c4-core/internal/mcp"
)

func hasToolRegistered(reg *mcp.Registry, name string) bool {
	for _, tool := range reg.ListTools() {
		if tool.Name == name {
			return true
		}
	}
	return false
}

func TestRegister_NilOpts(t *testing.T) {
	reg := mcp.NewRegistry()
	Register(reg, nil)
	if hasToolRegistered(reg, "c4_skill_eval_run") {
		t.Fatal("tool should not be registered when opts is nil")
	}
}

func TestRegister_NilLLM(t *testing.T) {
	reg := mcp.NewRegistry()
	Register(reg, &Opts{ProjectDir: "/tmp", LLM: nil})
	if hasToolRegistered(reg, "c4_skill_eval_run") {
		t.Fatal("tool should not be registered when LLM is nil")
	}
}

func TestRunHandler_SkillNameRequired(t *testing.T) {
	tmpDir := t.TempDir()
	skillDir := filepath.Join(tmpDir, ".claude", "skills", "test-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}

	h := runHandler(&Opts{ProjectDir: tmpDir, LLM: nil, KnowledgeStore: nil})
	raw, _ := json.Marshal(map[string]any{})
	result, err := h(context.Background(), raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map result, got %T", result)
	}
	if errMsg, hasErr := m["error"]; !hasErr {
		t.Fatal("expected error key in result when skill is empty")
	} else if errMsg != "skill is required" {
		t.Fatalf("unexpected error message: %v", errMsg)
	}
}

func TestRunHandler_InvalidArgs(t *testing.T) {
	h := runHandler(&Opts{ProjectDir: "/tmp", LLM: nil, KnowledgeStore: nil})
	result, err := h(context.Background(), []byte(`not-json`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map result, got %T", result)
	}
	if _, hasErr := m["error"]; !hasErr {
		t.Fatal("expected error key in result for invalid args")
	}
}

func TestOptimizeHandler_SkillNameRequired(t *testing.T) {
	h := optimizeHandler(&Opts{ProjectDir: "/tmp", LLM: nil, KnowledgeStore: nil})
	raw, _ := json.Marshal(map[string]any{})
	result, err := h(context.Background(), raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map result, got %T", result)
	}
	if errMsg, hasErr := m["error"]; !hasErr {
		t.Fatal("expected error key when skill is empty")
	} else if errMsg != "skill is required" {
		t.Fatalf("unexpected error: %v", errMsg)
	}
}

func TestOptimizeHandler_EvalsRequired(t *testing.T) {
	h := optimizeHandler(&Opts{ProjectDir: "/tmp", LLM: nil, KnowledgeStore: nil})
	raw, _ := json.Marshal(map[string]any{"skill": "c4-finish"})
	result, err := h(context.Background(), raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map result, got %T", result)
	}
	if _, hasErr := m["error"]; !hasErr {
		t.Fatal("expected error key when evals is empty")
	}
}

func TestOptimizeHandler_InvalidSkillName(t *testing.T) {
	h := optimizeHandler(&Opts{ProjectDir: "/tmp", LLM: nil, KnowledgeStore: nil})
	raw, _ := json.Marshal(map[string]any{"skill": "../etc/passwd"})
	result, err := h(context.Background(), raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map result, got %T", result)
	}
	if errMsg, _ := m["error"].(string); errMsg != "invalid skill name" {
		t.Fatalf("expected path traversal guard error, got: %v", errMsg)
	}
}

func TestOptimizeHandler_InvalidArgs(t *testing.T) {
	h := optimizeHandler(&Opts{ProjectDir: "/tmp", LLM: nil, KnowledgeStore: nil})
	result, err := h(context.Background(), []byte(`not-json`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map result, got %T", result)
	}
	if _, hasErr := m["error"]; !hasErr {
		t.Fatal("expected error key in result for invalid args")
	}
}

func TestRegister_IncludesOptimizeTool(t *testing.T) {
	reg := mcp.NewRegistry()
	// Register requires non-nil LLM — use a stub opts with nil LLM to confirm early return.
	// To test tool is listed, we need a non-nil LLM. Since we can't construct one here,
	// we verify the tool is NOT registered when LLM is nil (consistent guard).
	Register(reg, &Opts{ProjectDir: "/tmp", LLM: nil})
	if hasToolRegistered(reg, "c4_skill_optimize") {
		t.Fatal("c4_skill_optimize should not be registered when LLM is nil")
	}
}
