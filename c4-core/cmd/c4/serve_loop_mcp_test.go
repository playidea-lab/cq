//go:build research

package main

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/changmin/c4-core/internal/knowledge"
)

func newTestLoopOrchestratorForMCP(t *testing.T) *LoopOrchestrator {
	t.Helper()
	return newLoopOrchestrator(LoopOrchestratorConfig{
		Store:        mustNewKnowledgeStore(t),
		Hub:          newMockHubClient(),
		PollInterval: 10 * time.Millisecond,
	})
}

// TestLoopMCPHandlers_StartWithHypothesisText verifies that loopStartHandler
// creates a TypeHypothesis document when hypothesis text is provided (no ID),
// then starts a session with status="running".
func TestLoopMCPHandlers_StartWithHypothesisText(t *testing.T) {
	ks := mustNewKnowledgeStore(t)
	lo := newLoopOrchestrator(LoopOrchestratorConfig{
		Store:        ks,
		Hub:          newMockHubClient(),
		PollInterval: 10 * time.Millisecond,
	})

	handler := loopStartHandler(lo, ks)
	args, _ := json.Marshal(map[string]any{
		"hypothesis":     "Test hypothesis text",
		"max_iterations": 5,
	})
	result, err := handler(args)
	if err != nil {
		t.Fatalf("loopStartHandler: %v", err)
	}
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map result, got %T", result)
	}
	hypID, _ := m["hypothesis_id"].(string)
	if hypID == "" {
		t.Fatal("expected hypothesis_id in result")
	}
	if m["status"] != "running" {
		t.Errorf("expected status=running, got %v", m["status"])
	}
	if m["max_iterations"] != 5 {
		t.Errorf("expected max_iterations=5, got %v", m["max_iterations"])
	}

	// Verify session is tracked in orchestrator
	sess := lo.GetLoop(hypID)
	if sess == nil {
		t.Fatal("expected session in orchestrator, got nil")
	}
	if sess.Status != "running" {
		t.Errorf("expected session status=running, got %s", sess.Status)
	}

	// Verify hypothesis document was created in knowledge store
	doc, err := ks.Get(hypID)
	if err != nil || doc == nil {
		t.Fatalf("expected hypothesis document in store, err=%v doc=%v", err, doc)
	}
}

// TestLoopMCPHandlers_Stop verifies that loopStopHandler marks the session as stopped.
func TestLoopMCPHandlers_Stop(t *testing.T) {
	ks := mustNewKnowledgeStore(t)
	lo := newLoopOrchestrator(LoopOrchestratorConfig{
		Store:        ks,
		Hub:          newMockHubClient(),
		PollInterval: 10 * time.Millisecond,
	})

	// First start a session
	hypID, err := ks.Create(knowledge.TypeHypothesis, map[string]any{"title": "stop-test"}, "## Hyp")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	startHandler := loopStartHandler(lo, ks)
	startArgs, _ := json.Marshal(map[string]any{"hypothesis_id": hypID})
	if _, err := startHandler(startArgs); err != nil {
		t.Fatalf("loopStartHandler: %v", err)
	}

	// Now stop it
	stopHandler := loopStopHandler(lo)
	stopArgs, _ := json.Marshal(map[string]any{"hypothesis_id": hypID})
	result, err := stopHandler(stopArgs)
	if err != nil {
		t.Fatalf("loopStopHandler: %v", err)
	}
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map result, got %T", result)
	}
	if m["status"] != "stopped" {
		t.Errorf("expected status=stopped, got %v", m["status"])
	}
	if m["hypothesis_id"] != hypID {
		t.Errorf("expected hypothesis_id=%s, got %v", hypID, m["hypothesis_id"])
	}

	sess := lo.GetLoop(hypID)
	if sess == nil || sess.Status != "stopped" {
		t.Errorf("expected session status=stopped, got %v", sess)
	}
}

// TestLoopMCPHandlers_Status verifies that loopStatusHandler returns accurate session state.
func TestLoopMCPHandlers_Status(t *testing.T) {
	ks := mustNewKnowledgeStore(t)
	lo := newLoopOrchestrator(LoopOrchestratorConfig{
		Store:        ks,
		Hub:          newMockHubClient(),
		PollInterval: 10 * time.Millisecond,
	})

	// Start a session first
	startHandler := loopStartHandler(lo, ks)
	startArgs, _ := json.Marshal(map[string]any{
		"hypothesis":     "Status check hypothesis",
		"max_iterations": 3,
	})
	startResult, err := startHandler(startArgs)
	if err != nil {
		t.Fatalf("loopStartHandler: %v", err)
	}
	hypID, _ := startResult.(map[string]any)["hypothesis_id"].(string)

	statusHandler := loopStatusHandler(lo)
	statusArgs, _ := json.Marshal(map[string]any{"hypothesis_id": hypID})
	result, err := statusHandler(statusArgs)
	if err != nil {
		t.Fatalf("loopStatusHandler: %v", err)
	}
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map result, got %T", result)
	}
	if m["hypothesis_id"] != hypID {
		t.Errorf("hypothesis_id mismatch: got %v", m["hypothesis_id"])
	}
	if m["status"] != "running" {
		t.Errorf("expected status=running, got %v", m["status"])
	}
	if m["max_iterations"] != 3 {
		t.Errorf("expected max_iterations=3, got %v", m["max_iterations"])
	}
}

// TestLoopMCPHandlers_NilOrchestrator verifies that registerLoopMCPHandlers
// returns nil (no panic) when loopOrchestrator is nil in initContext.
func TestLoopMCPHandlers_NilOrchestrator(t *testing.T) {
	ks := mustNewKnowledgeStore(t)
	ctx := &initContext{
		knowledgeStore:   ks,
		loopOrchestrator: nil, // nil — should be a no-op
	}
	if err := registerLoopMCPHandlers(ctx); err != nil {
		t.Fatalf("expected nil error with nil orchestrator, got %v", err)
	}
}
