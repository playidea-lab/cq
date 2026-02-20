//go:build research


package handlers

import (
	"encoding/json"
	"testing"

	"github.com/changmin/c4-core/internal/research"
)

func newTestResearchStore(t *testing.T) *research.Store {
	t.Helper()
	s, err := research.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestResearchStart_Success(t *testing.T) {
	store := newTestResearchStore(t)
	handler := researchStartHandler(store)

	args, _ := json.Marshal(map[string]any{"name": "My Paper"})
	result, err := handler(args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	if m["success"] != true {
		t.Errorf("success = %v, want true", m["success"])
	}
	if m["project_id"] == nil || m["project_id"] == "" {
		t.Error("expected non-empty project_id")
	}
	if m["iteration_id"] == nil || m["iteration_id"] == "" {
		t.Error("expected non-empty iteration_id")
	}
}

func TestResearchStart_NoName(t *testing.T) {
	store := newTestResearchStore(t)
	handler := researchStartHandler(store)

	args, _ := json.Marshal(map[string]any{})
	result, _ := handler(args)
	m := result.(map[string]any)
	if m["error"] != "name is required" {
		t.Errorf("error = %v, want 'name is required'", m["error"])
	}
}

func TestResearchStatus_Success(t *testing.T) {
	store := newTestResearchStore(t)
	startHandler := researchStartHandler(store)
	statusHandler := researchStatusHandler(store)

	// Create a project
	args, _ := json.Marshal(map[string]any{"name": "Test"})
	result, _ := startHandler(args)
	pid := result.(map[string]any)["project_id"].(string)

	// Get status
	args, _ = json.Marshal(map[string]any{"project_id": pid})
	result, err := statusHandler(args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	project, ok := m["project"].(map[string]any)
	if !ok {
		t.Fatal("expected project map")
	}
	if project["name"] != "Test" {
		t.Errorf("name = %v, want Test", project["name"])
	}

	iters, ok := m["iterations"].([]any)
	if !ok {
		t.Fatal("expected iterations list")
	}
	if len(iters) != 1 {
		t.Errorf("iterations = %d, want 1", len(iters))
	}
}

func TestResearchStatus_NotFound(t *testing.T) {
	store := newTestResearchStore(t)
	handler := researchStatusHandler(store)

	args, _ := json.Marshal(map[string]any{"project_id": "nonexistent"})
	result, _ := handler(args)
	m := result.(map[string]any)
	if _, ok := m["error"]; !ok {
		t.Error("expected error for nonexistent project")
	}
}

func TestResearchRecord_Success(t *testing.T) {
	store := newTestResearchStore(t)
	startHandler := researchStartHandler(store)
	recordHandler := researchRecordHandler(store)

	args, _ := json.Marshal(map[string]any{"name": "Test"})
	result, _ := startHandler(args)
	pid := result.(map[string]any)["project_id"].(string)

	args, _ = json.Marshal(map[string]any{
		"project_id":   pid,
		"review_score": 7.5,
		"status":       "planning",
	})
	result, err := recordHandler(args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	if m["success"] != true {
		t.Errorf("success = %v, want true", m["success"])
	}
}

func TestResearchApprove_Continue(t *testing.T) {
	store := newTestResearchStore(t)
	startHandler := researchStartHandler(store)
	approveHandler := researchApproveHandler(store)

	args, _ := json.Marshal(map[string]any{"name": "Test"})
	result, _ := startHandler(args)
	pid := result.(map[string]any)["project_id"].(string)

	args, _ = json.Marshal(map[string]any{"project_id": pid, "action": "continue"})
	result, err := approveHandler(args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	if m["success"] != true {
		t.Errorf("success = %v, want true", m["success"])
	}
	if m["iteration_id"] == nil {
		t.Error("expected iteration_id for continue action")
	}
}

func TestResearchApprove_Pause(t *testing.T) {
	store := newTestResearchStore(t)
	startHandler := researchStartHandler(store)
	approveHandler := researchApproveHandler(store)

	args, _ := json.Marshal(map[string]any{"name": "Test"})
	result, _ := startHandler(args)
	pid := result.(map[string]any)["project_id"].(string)

	args, _ = json.Marshal(map[string]any{"project_id": pid, "action": "pause"})
	result, _ = approveHandler(args)
	m := result.(map[string]any)
	if m["success"] != true {
		t.Errorf("success = %v", m["success"])
	}
}

func TestResearchApprove_Complete(t *testing.T) {
	store := newTestResearchStore(t)
	startHandler := researchStartHandler(store)
	approveHandler := researchApproveHandler(store)

	args, _ := json.Marshal(map[string]any{"name": "Test"})
	result, _ := startHandler(args)
	pid := result.(map[string]any)["project_id"].(string)

	args, _ = json.Marshal(map[string]any{"project_id": pid, "action": "complete"})
	result, _ = approveHandler(args)
	m := result.(map[string]any)
	if m["success"] != true {
		t.Errorf("success = %v", m["success"])
	}
}

func TestResearchApprove_InvalidAction(t *testing.T) {
	store := newTestResearchStore(t)
	handler := researchApproveHandler(store)

	args, _ := json.Marshal(map[string]any{"project_id": "x", "action": "invalid"})
	result, _ := handler(args)
	m := result.(map[string]any)
	if _, ok := m["error"]; !ok {
		t.Error("expected error for invalid action")
	}
}

func TestResearchNext_Success(t *testing.T) {
	store := newTestResearchStore(t)
	startHandler := researchStartHandler(store)
	nextHandler := researchNextHandler(store)

	args, _ := json.Marshal(map[string]any{"name": "Test"})
	result, _ := startHandler(args)
	pid := result.(map[string]any)["project_id"].(string)

	args, _ = json.Marshal(map[string]any{"project_id": pid})
	result, err := nextHandler(args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	if m["action"] != "review" {
		t.Errorf("action = %v, want review", m["action"])
	}
}

func TestResearchE2E_FullFlow(t *testing.T) {
	store := newTestResearchStore(t)

	// 1. Start
	startH := researchStartHandler(store)
	args, _ := json.Marshal(map[string]any{"name": "E2E Test", "target_score": 7.0})
	result, _ := startH(args)
	pid := result.(map[string]any)["project_id"].(string)

	// 2. Record review score
	recordH := researchRecordHandler(store)
	args, _ = json.Marshal(map[string]any{"project_id": pid, "review_score": 5.0, "status": "planning"})
	recordH(args)

	// 3. Check next → should suggest plan_experiments
	nextH := researchNextHandler(store)
	args, _ = json.Marshal(map[string]any{"project_id": pid})
	result, _ = nextH(args)
	if result.(map[string]any)["action"] != "plan_experiments" {
		t.Errorf("expected plan_experiments, got %v", result.(map[string]any)["action"])
	}

	// 4. Approve continue → new iteration
	approveH := researchApproveHandler(store)
	args, _ = json.Marshal(map[string]any{"project_id": pid, "action": "continue"})
	result, _ = approveH(args)
	if result.(map[string]any)["success"] != true {
		t.Error("continue should succeed")
	}

	// 5. Record high score
	args, _ = json.Marshal(map[string]any{"project_id": pid, "review_score": 8.0, "status": "planning"})
	recordH(args)

	// 6. Next → should suggest complete
	args, _ = json.Marshal(map[string]any{"project_id": pid})
	result, _ = nextH(args)
	if result.(map[string]any)["action"] != "complete" {
		t.Errorf("expected complete, got %v", result.(map[string]any)["action"])
	}

	// 7. Complete
	args, _ = json.Marshal(map[string]any{"project_id": pid, "action": "complete"})
	result, _ = approveH(args)
	if result.(map[string]any)["success"] != true {
		t.Error("complete should succeed")
	}

	// 8. Status check
	statusH := researchStatusHandler(store)
	args, _ = json.Marshal(map[string]any{"project_id": pid})
	result, _ = statusH(args)
	project := result.(map[string]any)["project"].(map[string]any)
	if project["status"] != "completed" {
		t.Errorf("status = %v, want completed", project["status"])
	}
}
