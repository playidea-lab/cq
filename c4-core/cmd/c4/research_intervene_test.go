//go:build research

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/changmin/c4-core/internal/knowledge"
	"github.com/changmin/c4-core/internal/research"
)

func newTestInterveneStores(t *testing.T) (*research.Store, *knowledge.Store) {
	t.Helper()
	dir := t.TempDir()

	rs, err := research.NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore (research): %v", err)
	}
	t.Cleanup(func() { rs.Close() })

	ksDir := filepath.Join(dir, "knowledge")
	os.MkdirAll(ksDir, 0755)
	ks, err := knowledge.NewStore(ksDir)
	if err != nil {
		t.Fatalf("NewStore (knowledge): %v", err)
	}
	t.Cleanup(func() { ks.Close() })

	return rs, ks
}

func createTestProject(t *testing.T, rs *research.Store, name string) string {
	t.Helper()
	pid, err := rs.CreateProject(name, nil, nil, 7.0)
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if _, err := rs.CreateIteration(pid); err != nil {
		t.Fatalf("CreateIteration: %v", err)
	}
	return pid
}

func TestResearchIntervene_MissingLoopID(t *testing.T) {
	rs, ks := newTestInterveneStores(t)
	handler := researchInterveneHandler(rs, ks)

	args, _ := json.Marshal(map[string]any{"type": "steering", "context": "ctx"})
	_, err := handler(args)
	if err == nil || !strings.Contains(err.Error(), "loop_id") {
		t.Errorf("expected loop_id error, got: %v", err)
	}
}

func TestResearchIntervene_MissingType(t *testing.T) {
	rs, ks := newTestInterveneStores(t)
	handler := researchInterveneHandler(rs, ks)

	args, _ := json.Marshal(map[string]any{"loop_id": "x"})
	_, err := handler(args)
	if err == nil || !strings.Contains(err.Error(), "type") {
		t.Errorf("expected type error, got: %v", err)
	}
}

func TestResearchIntervene_LoopNotFound(t *testing.T) {
	rs, ks := newTestInterveneStores(t)
	handler := researchInterveneHandler(rs, ks)

	args, _ := json.Marshal(map[string]any{"loop_id": "nonexistent", "type": "abort", "abort_reason": "test"})
	_, err := handler(args)
	if err == nil || !strings.Contains(err.Error(), "loop not found") {
		t.Errorf("expected loop not found error, got: %v", err)
	}
}

func TestResearchIntervene_Steering_Success(t *testing.T) {
	rs, ks := newTestInterveneStores(t)
	pid := createTestProject(t, rs, "Test Loop")
	handler := researchInterveneHandler(rs, ks)

	args, _ := json.Marshal(map[string]any{
		"loop_id": pid,
		"type":    "steering",
		"context": "Focus on low-rank adaptation experiments",
	})
	result, err := handler(args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	if m["type"] != "steering" {
		t.Errorf("type = %v, want steering", m["type"])
	}
	if m["applied_at"] != "next_debate" {
		t.Errorf("applied_at = %v, want next_debate", m["applied_at"])
	}
	if m["loop_status"] != "running" {
		t.Errorf("loop_status = %v, want running", m["loop_status"])
	}
	if id, _ := m["intervention_id"].(string); !strings.HasPrefix(id, "iv-") {
		t.Errorf("intervention_id = %v, expected iv- prefix", id)
	}
}

func TestResearchIntervene_Steering_MissingContext(t *testing.T) {
	rs, ks := newTestInterveneStores(t)
	pid := createTestProject(t, rs, "Test Loop")
	handler := researchInterveneHandler(rs, ks)

	args, _ := json.Marshal(map[string]any{"loop_id": pid, "type": "steering"})
	_, err := handler(args)
	if err == nil || !strings.Contains(err.Error(), "context") {
		t.Errorf("expected context error, got: %v", err)
	}
}

func TestResearchIntervene_Injection_Success(t *testing.T) {
	rs, ks := newTestInterveneStores(t)
	pid := createTestProject(t, rs, "Test Loop")
	handler := researchInterveneHandler(rs, ks)

	args, _ := json.Marshal(map[string]any{
		"loop_id":          pid,
		"type":             "injection",
		"hypothesis_draft": "Quantization-aware training reduces inference cost by 40%",
	})
	result, err := handler(args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	if m["type"] != "injection" {
		t.Errorf("type = %v, want injection", m["type"])
	}
	if m["loop_status"] != "running" {
		t.Errorf("loop_status = %v, want running", m["loop_status"])
	}
	hypID, _ := m["hypothesis_id"].(string)
	if hypID == "" {
		t.Error("expected hypothesis_id in result")
	}
	// Verify hypothesis was stored in knowledge store
	doc, err := ks.Get(hypID)
	if err != nil || doc == nil {
		t.Errorf("hypothesis not found in knowledge store: %v", err)
	}
	if doc != nil && doc.Type != knowledge.TypeHypothesis {
		t.Errorf("doc type = %v, want hypothesis", doc.Type)
	}
}

func TestResearchIntervene_Injection_MissingDraft(t *testing.T) {
	rs, ks := newTestInterveneStores(t)
	pid := createTestProject(t, rs, "Test Loop")
	handler := researchInterveneHandler(rs, ks)

	args, _ := json.Marshal(map[string]any{"loop_id": pid, "type": "injection"})
	_, err := handler(args)
	if err == nil || !strings.Contains(err.Error(), "hypothesis_draft") {
		t.Errorf("expected hypothesis_draft error, got: %v", err)
	}
}

func TestResearchIntervene_Abort_Success(t *testing.T) {
	rs, ks := newTestInterveneStores(t)
	pid := createTestProject(t, rs, "Test Loop")
	handler := researchInterveneHandler(rs, ks)

	args, _ := json.Marshal(map[string]any{
		"loop_id":      pid,
		"type":         "abort",
		"abort_reason": "Experiment direction invalidated by new baseline",
	})
	result, err := handler(args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	if m["type"] != "abort" {
		t.Errorf("type = %v, want abort", m["type"])
	}
	if m["loop_status"] != "aborted" {
		t.Errorf("loop_status = %v, want aborted", m["loop_status"])
	}
	debateID, _ := m["debate_doc_id"].(string)
	if debateID == "" {
		t.Error("expected debate_doc_id in result")
	}

	// Verify project is paused
	project, err := rs.GetProject(pid)
	if err != nil {
		t.Fatalf("GetProject: %v", err)
	}
	if project.Status != research.StatusPaused {
		t.Errorf("project status = %v, want paused", project.Status)
	}

	// Verify debate doc was recorded
	doc, err := ks.Get(debateID)
	if err != nil || doc == nil {
		t.Errorf("debate doc not found: %v", err)
	}
	if doc != nil && doc.Type != knowledge.TypeDebate {
		t.Errorf("doc type = %v, want debate", doc.Type)
	}
}

func TestResearchIntervene_Abort_MissingReason(t *testing.T) {
	rs, ks := newTestInterveneStores(t)
	pid := createTestProject(t, rs, "Test Loop")
	handler := researchInterveneHandler(rs, ks)

	args, _ := json.Marshal(map[string]any{"loop_id": pid, "type": "abort"})
	_, err := handler(args)
	if err == nil || !strings.Contains(err.Error(), "abort_reason") {
		t.Errorf("expected abort_reason error, got: %v", err)
	}
}

func TestResearchIntervene_InvalidType(t *testing.T) {
	rs, ks := newTestInterveneStores(t)
	pid := createTestProject(t, rs, "Test Loop")
	handler := researchInterveneHandler(rs, ks)

	args, _ := json.Marshal(map[string]any{"loop_id": pid, "type": "unknown"})
	_, err := handler(args)
	if err == nil || !strings.Contains(err.Error(), "steering") {
		t.Errorf("expected type error, got: %v", err)
	}
}
