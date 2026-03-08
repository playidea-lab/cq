//go:build research

package main

import (
	"context"
	"testing"
	"time"

	"github.com/changmin/c4-core/internal/hub"
	"github.com/changmin/c4-core/internal/llm"
	"github.com/changmin/c4-core/internal/serve"
)

// compile-time interface assertion
var _ serve.Component = (*LoopOrchestrator)(nil)

// mockHubClient implements HubClient for tests.
type mockHubClient struct {
	jobs map[string]*hub.Job
}

func newMockHubClient() *mockHubClient {
	return &mockHubClient{jobs: make(map[string]*hub.Job)}
}

func (m *mockHubClient) GetJob(jobID string) (*hub.Job, error) {
	if j, ok := m.jobs[jobID]; ok {
		return j, nil
	}
	return &hub.Job{ID: jobID, Status: "RUNNING"}, nil
}

func (m *mockHubClient) SubmitJob(req *hub.JobSubmitRequest) (*hub.JobSubmitResponse, error) {
	id := "job-new-001"
	m.jobs[id] = &hub.Job{ID: id, Status: "QUEUED"}
	return &hub.JobSubmitResponse{JobID: id, Status: "QUEUED"}, nil
}

func newTestLoopOrchestrator(t *testing.T) *LoopOrchestrator {
	t.Helper()
	return newLoopOrchestrator(LoopOrchestratorConfig{
		Store:        mustNewKnowledgeStore(t),
		Hub:          newMockHubClient(),
		PollInterval: 10 * time.Millisecond,
	})
}

// TestLoopOrchestrator_StartStop verifies that Start launches the loop goroutine
// and Stop cancels it cleanly (done channel is closed).
func TestLoopOrchestrator_StartStop(t *testing.T) {
	o := newTestLoopOrchestrator(t)

	ctx := context.Background()
	if err := o.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Give the loop goroutine a moment to start.
	time.Sleep(20 * time.Millisecond)

	if err := o.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	// Verify done channel is closed (Stop should have waited).
	o.mu.Lock()
	done := o.done
	o.mu.Unlock()
	select {
	case <-done:
		// ok — loop exited
	default:
		t.Error("done channel not closed after Stop")
	}
}

// TestLoopOrchestrator_StartLoop_GetLoop verifies that StartLoop stores the session
// and GetLoop retrieves it with Status="running".
func TestLoopOrchestrator_StartLoop_GetLoop(t *testing.T) {
	o := newTestLoopOrchestrator(t)

	session := &LoopSession{
		HypothesisID:  "hyp-001",
		JobID:         "job-001",
		Round:         1,
		MaxIterations: 5,
	}

	ctx := context.Background()
	if err := o.StartLoop(ctx, session); err != nil {
		t.Fatalf("StartLoop: %v", err)
	}

	got := o.GetLoop("hyp-001")
	if got == nil {
		t.Fatal("GetLoop returned nil")
	}
	if got.Status != "running" {
		t.Errorf("Status = %q, want %q", got.Status, "running")
	}
	if got.HypothesisID != "hyp-001" {
		t.Errorf("HypothesisID = %q, want %q", got.HypothesisID, "hyp-001")
	}
}

// TestLoopOrchestrator_StopLoop verifies that StopLoop sets session.Status="stopped".
func TestLoopOrchestrator_StopLoop(t *testing.T) {
	o := newTestLoopOrchestrator(t)
	ctx := context.Background()

	session := &LoopSession{HypothesisID: "hyp-002", JobID: "job-002"}
	if err := o.StartLoop(ctx, session); err != nil {
		t.Fatalf("StartLoop: %v", err)
	}

	if err := o.StopLoop(ctx, "hyp-002"); err != nil {
		t.Fatalf("StopLoop: %v", err)
	}

	got := o.GetLoop("hyp-002")
	if got == nil {
		t.Fatal("GetLoop returned nil after StopLoop")
	}
	if got.Status != "stopped" {
		t.Errorf("Status = %q, want %q", got.Status, "stopped")
	}
}

// TestLoopOrchestrator_Steer verifies that Steer sets SteeringGuidance on the session.
func TestLoopOrchestrator_Steer(t *testing.T) {
	o := newTestLoopOrchestrator(t)
	ctx := context.Background()

	session := &LoopSession{HypothesisID: "hyp-003", JobID: "job-003"}
	if err := o.StartLoop(ctx, session); err != nil {
		t.Fatalf("StartLoop: %v", err)
	}

	const guidance = "focus on lower learning rate"
	if err := o.Steer(ctx, "hyp-003", guidance); err != nil {
		t.Fatalf("Steer: %v", err)
	}

	got := o.GetLoop("hyp-003")
	if got == nil {
		t.Fatal("GetLoop returned nil after Steer")
	}
	if got.SteeringGuidance != guidance {
		t.Errorf("SteeringGuidance = %q, want %q", got.SteeringGuidance, guidance)
	}
}

// TestRegisterLoopOrchestratorComponent_RegistersWithManager verifies that
// registerLoopOrchestratorComponent adds loop_orchestrator to the serve manager.
func TestRegisterLoopOrchestratorComponent_RegistersWithManager(t *testing.T) {
	gw := llm.NewGateway(llm.RoutingTable{})
	gw.Register(&stubLLMProvider{response: "ok"})

	ictx := &initContext{
		knowledgeStore: mustNewKnowledgeStore(t),
		llmGateway:     gw,
	}
	mgr := serve.NewManager()
	registerLoopOrchestratorComponent(mgr, ictx)

	health := mgr.HealthMap()
	if _, ok := health["loop_orchestrator"]; !ok {
		t.Errorf("loop_orchestrator not found in manager health map; got %v", health)
	}
}
