//go:build research

package main

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/changmin/c4-core/internal/hub"
	"github.com/changmin/c4-core/internal/knowledge"
	"github.com/changmin/c4-core/internal/serve/orchestrator"
)

// mockHubClientMCP implements orchestrator.HubClient for MCP tests.
type mockHubClientMCP struct{}

func (m *mockHubClientMCP) GetJob(_ string) (*hub.Job, error)                         { return nil, nil }
func (m *mockHubClientMCP) SubmitJob(_ *hub.JobSubmitRequest) (*hub.JobSubmitResponse, error) {
	return nil, nil
}

// TestLoopMCPHandlers_StartWithHypothesisText verifies that loopStartHandler
// creates a TypeHypothesis document when hypothesis text is provided (no ID),
// then starts a session with status="running".
func TestLoopMCPHandlers_StartWithHypothesisText(t *testing.T) {
	ks := mustNewKnowledgeStore(t)
	lo := orchestrator.New(orchestrator.Config{
		Store:        ks,
		Hub:          &mockHubClientMCP{},
		PollInterval: 10 * time.Millisecond,
	})

	handler := loopStartHandler(lo, ks, loopConvergenceDefaults{})
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

	sess := lo.GetLoop(hypID)
	if sess == nil {
		t.Fatal("expected session in orchestrator, got nil")
	}
	if sess.Status != "running" {
		t.Errorf("expected session status=running, got %s", sess.Status)
	}

	doc, err := ks.Get(hypID)
	if err != nil || doc == nil {
		t.Fatalf("expected hypothesis document in store, err=%v doc=%v", err, doc)
	}
}

// TestLoopMCPHandlers_Stop verifies that loopStopHandler marks the session as stopped.
func TestLoopMCPHandlers_Stop(t *testing.T) {
	ks := mustNewKnowledgeStore(t)
	lo := orchestrator.New(orchestrator.Config{
		Store:        ks,
		Hub:          &mockHubClientMCP{},
		PollInterval: 10 * time.Millisecond,
	})

	hypID, err := ks.Create(knowledge.TypeHypothesis, map[string]any{"title": "stop-test"}, "## Hyp")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	startHandler := loopStartHandler(lo, ks, loopConvergenceDefaults{})
	startArgs, _ := json.Marshal(map[string]any{"hypothesis_id": hypID})
	if _, err := startHandler(startArgs); err != nil {
		t.Fatalf("loopStartHandler: %v", err)
	}

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

	sess := lo.GetLoop(hypID)
	if sess == nil || sess.Status != "stopped" {
		t.Errorf("expected session status=stopped, got %v", sess)
	}
}

// TestLoopMCPHandlers_Status verifies that loopStatusHandler returns accurate session state.
func TestLoopMCPHandlers_Status(t *testing.T) {
	ks := mustNewKnowledgeStore(t)
	lo := orchestrator.New(orchestrator.Config{
		Store:        ks,
		Hub:          &mockHubClientMCP{},
		PollInterval: 10 * time.Millisecond,
	})

	startHandler := loopStartHandler(lo, ks, loopConvergenceDefaults{})
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
		loopOrchestrator: nil,
	}
	if err := registerLoopMCPHandlers(ctx); err != nil {
		t.Fatalf("expected nil error with nil orchestrator, got %v", err)
	}
}

// TestLoopMCP_ConvergenceParamsOverride verifies that max_patience and
// convergence_threshold caller-supplied values override config defaults.
func TestLoopMCP_ConvergenceParamsOverride(t *testing.T) {
	ks := mustNewKnowledgeStore(t)
	lo := orchestrator.New(orchestrator.Config{
		Store:        ks,
		Hub:          &mockHubClientMCP{},
		PollInterval: 10 * time.Millisecond,
	})

	// Config defaults: patience=3, threshold=0.5
	defaults := loopConvergenceDefaults{
		MaxPatience:          3,
		ConvergenceThreshold: 0.5,
		MetricLowerIsBetter:  true,
	}
	handler := loopStartHandler(lo, ks, defaults)

	// Caller overrides patience=5, threshold=0.1
	patience := 5
	threshold := 0.1
	args, _ := json.Marshal(map[string]any{
		"hypothesis":            "Convergence override test",
		"max_patience":          patience,
		"convergence_threshold": threshold,
	})
	result, err := handler(args)
	if err != nil {
		t.Fatalf("loopStartHandler: %v", err)
	}
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map result, got %T", result)
	}
	if m["max_patience"] != patience {
		t.Errorf("expected max_patience=%d, got %v", patience, m["max_patience"])
	}
	if m["convergence_threshold"] != threshold {
		t.Errorf("expected convergence_threshold=%v, got %v", threshold, m["convergence_threshold"])
	}

	hypID, _ := m["hypothesis_id"].(string)
	sess := lo.GetLoop(hypID)
	if sess == nil {
		t.Fatal("expected session in orchestrator")
	}
	if sess.MaxPatience != patience {
		t.Errorf("session MaxPatience: got %d, want %d", sess.MaxPatience, patience)
	}
	if sess.ConvergenceThreshold != threshold {
		t.Errorf("session ConvergenceThreshold: got %v, want %v", sess.ConvergenceThreshold, threshold)
	}
	if !sess.MetricLowerIsBetter {
		t.Error("expected MetricLowerIsBetter=true from defaults")
	}
}

// TestLoopMCP_ConvergenceParamsDefault verifies that config defaults are used
// when caller does not supply max_patience or convergence_threshold.
func TestLoopMCP_ConvergenceParamsDefault(t *testing.T) {
	ks := mustNewKnowledgeStore(t)
	lo := orchestrator.New(orchestrator.Config{
		Store:        ks,
		Hub:          &mockHubClientMCP{},
		PollInterval: 10 * time.Millisecond,
	})

	defaults := loopConvergenceDefaults{
		MaxPatience:          3,
		ConvergenceThreshold: 0.5,
		MetricLowerIsBetter:  false,
	}
	handler := loopStartHandler(lo, ks, defaults)

	args, _ := json.Marshal(map[string]any{
		"hypothesis": "Default convergence test",
	})
	result, err := handler(args)
	if err != nil {
		t.Fatalf("loopStartHandler: %v", err)
	}
	m := result.(map[string]any)
	hypID, _ := m["hypothesis_id"].(string)

	sess := lo.GetLoop(hypID)
	if sess == nil {
		t.Fatal("expected session")
	}
	if sess.MaxPatience != 3 {
		t.Errorf("expected MaxPatience=3, got %d", sess.MaxPatience)
	}
	if sess.ConvergenceThreshold != 0.5 {
		t.Errorf("expected ConvergenceThreshold=0.5, got %v", sess.ConvergenceThreshold)
	}
	if sess.MetricLowerIsBetter {
		t.Error("expected MetricLowerIsBetter=false from defaults")
	}
}

// TestLoopMCP_StatusConvergenceBlock verifies that loopStatusHandler response
// includes the convergence block with patience_count, best_metric, converged,
// threshold, and max_patience.
func TestLoopMCP_StatusConvergenceBlock(t *testing.T) {
	ks := mustNewKnowledgeStore(t)
	lo := orchestrator.New(orchestrator.Config{
		Store:        ks,
		Hub:          &mockHubClientMCP{},
		PollInterval: 10 * time.Millisecond,
	})

	defaults := loopConvergenceDefaults{
		MaxPatience:          2,
		ConvergenceThreshold: 0.3,
		MetricLowerIsBetter:  true,
	}
	startHandler := loopStartHandler(lo, ks, defaults)
	startArgs, _ := json.Marshal(map[string]any{"hypothesis": "Status convergence test"})
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

	conv, ok := m["convergence"].(map[string]any)
	if !ok {
		t.Fatalf("expected convergence block in status response, got %T", m["convergence"])
	}
	if conv["max_patience"] != 2 {
		t.Errorf("convergence.max_patience: got %v, want 2", conv["max_patience"])
	}
	if conv["threshold"] != 0.3 {
		t.Errorf("convergence.threshold: got %v, want 0.3", conv["threshold"])
	}
	if conv["converged"] != false {
		t.Errorf("convergence.converged: expected false for new session, got %v", conv["converged"])
	}
	if _, ok := conv["patience_count"]; !ok {
		t.Error("expected convergence.patience_count in response")
	}
	if _, ok := conv["best_metric"]; !ok {
		t.Error("expected convergence.best_metric in response")
	}
}

// TestLoopMCP_StatusConvergedFlag verifies that converged=true when PatienceCount >= MaxPatience.
func TestLoopMCP_StatusConvergedFlag(t *testing.T) {
	ks := mustNewKnowledgeStore(t)
	lo := orchestrator.New(orchestrator.Config{
		Store:        ks,
		Hub:          &mockHubClientMCP{},
		PollInterval: 10 * time.Millisecond,
	})

	// Start session with max_patience=2.
	defaults := loopConvergenceDefaults{MaxPatience: 2, ConvergenceThreshold: 0.5, MetricLowerIsBetter: true}
	startHandler := loopStartHandler(lo, ks, defaults)
	startArgs, _ := json.Marshal(map[string]any{"hypothesis": "Converged test"})
	startResult, err := startHandler(startArgs)
	if err != nil {
		t.Fatalf("loopStartHandler: %v", err)
	}
	hypID, _ := startResult.(map[string]any)["hypothesis_id"].(string)

	// Manually set PatienceCount = MaxPatience to simulate convergence.
	sess := lo.GetLoop(hypID)
	if sess == nil {
		t.Fatal("expected session")
	}
	updated := *sess
	updated.PatienceCount = 2 // equals MaxPatience
	lo.Sessions.Store(hypID, &updated)

	statusHandler := loopStatusHandler(lo)
	statusArgs, _ := json.Marshal(map[string]any{"hypothesis_id": hypID})
	result, err := statusHandler(statusArgs)
	if err != nil {
		t.Fatalf("loopStatusHandler: %v", err)
	}
	m := result.(map[string]any)
	conv := m["convergence"].(map[string]any)
	if conv["converged"] != true {
		t.Errorf("expected converged=true when PatienceCount >= MaxPatience, got %v", conv["converged"])
	}
}
