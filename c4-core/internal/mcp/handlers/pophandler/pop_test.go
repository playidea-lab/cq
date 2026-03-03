package pophandler

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/changmin/c4-core/internal/knowledge"
	"github.com/changmin/c4-core/internal/mcp"
)

// newTestStore creates a temporary knowledge.Store for testing.
func newTestStore(t *testing.T) (*knowledge.Store, string) {
	t.Helper()
	dir := t.TempDir()
	store, err := knowledge.NewStore(dir)
	if err != nil {
		t.Fatalf("knowledge.NewStore: %v", err)
	}
	return store, dir
}

func TestPopHandler_Status(t *testing.T) {
	store, projectDir := newTestStore(t)

	opts := &Opts{ProjectDir: projectDir, Store: store}
	handler := statusHandler(opts)

	result, err := handler(json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("statusHandler error: %v", err)
	}
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map result, got %T", result)
	}
	if _, hasGauges := m["gauges"]; !hasGauges {
		t.Error("result missing 'gauges' key")
	}
	if _, hasKS := m["knowledge_stats"]; !hasKS {
		t.Error("result missing 'knowledge_stats' key")
	}
}

func TestPopHandler_Extract(t *testing.T) {
	store, projectDir := newTestStore(t)

	opts := &Opts{ProjectDir: projectDir, Store: store, LLM: nil}
	handler := extractHandler(opts)

	result, err := handler(json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("extractHandler error: %v", err)
	}
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map result, got %T", result)
	}
	// With noopLLMClient, no messages → success with no proposals.
	if errVal, hasErr := m["error"]; hasErr {
		t.Errorf("unexpected error from extractHandler: %v", errVal)
	}
	if success, _ := m["success"].(bool); !success {
		t.Errorf("expected success=true, got %v", m["success"])
	}
}

func TestPopHandler_Reflect(t *testing.T) {
	store, projectDir := newTestStore(t)

	opts := &Opts{ProjectDir: projectDir, Store: store}
	handler := reflectHandler(opts)

	result, err := handler(json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("reflectHandler error: %v", err)
	}
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map result, got %T", result)
	}
	if _, hasProposals := m["proposals"]; !hasProposals {
		t.Error("result missing 'proposals' key")
	}
}

func TestPopHandler_Register_NilOpts(t *testing.T) {
	reg := mcp.NewRegistry()
	// Should not panic with nil opts.
	Register(reg, nil)
	tools := reg.ListTools()
	for _, s := range tools {
		if s.Name == "c4_pop_status" || s.Name == "c4_pop_extract" || s.Name == "c4_pop_reflect" {
			t.Errorf("expected no POP tools registered with nil opts, found %s", s.Name)
		}
	}
}

func TestPopHandler_Register_NilStore(t *testing.T) {
	reg := mcp.NewRegistry()
	Register(reg, &Opts{ProjectDir: "/tmp", Store: nil})
	tools := reg.ListTools()
	for _, s := range tools {
		if s.Name == "c4_pop_status" || s.Name == "c4_pop_extract" || s.Name == "c4_pop_reflect" {
			t.Errorf("expected no POP tools registered with nil store, found %s", s.Name)
		}
	}
}

func TestSoulWriterAdapter_PathTraversalGuard(t *testing.T) {
	dir := t.TempDir()
	adapter := &soulWriterAdapter{projectDir: dir}

	// A malicious userID with path traversal.
	err := adapter.AppendInsight(nil, "../../etc/passwd", "evil insight") //nolint:staticcheck
	if err == nil {
		t.Error("expected path traversal error, got nil")
	}

	// A safe userID.
	err = adapter.AppendInsight(nil, "testuser", "safe insight") //nolint:staticcheck
	if err != nil {
		t.Errorf("unexpected error for safe userID: %v", err)
	}
	soulPath := filepath.Join(dir, ".c4", "souls", "testuser", "soul-developer.md")
	if _, statErr := os.Stat(soulPath); statErr != nil {
		t.Errorf("expected soul file at %s, stat error: %v", soulPath, statErr)
	}
}
