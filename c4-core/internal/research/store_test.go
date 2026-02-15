package research

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	s, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestCreateAndGetProject(t *testing.T) {
	s := newTestStore(t)
	paper := "paper.tex"
	repo := "/code"

	id, err := s.CreateProject("Test Paper", &paper, &repo, 7.5)
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if id == "" {
		t.Fatal("expected non-empty project ID")
	}

	p, err := s.GetProject(id)
	if err != nil {
		t.Fatalf("GetProject: %v", err)
	}
	if p == nil {
		t.Fatal("expected project, got nil")
	}
	if p.Name != "Test Paper" {
		t.Errorf("Name = %q, want %q", p.Name, "Test Paper")
	}
	if p.TargetScore != 7.5 {
		t.Errorf("TargetScore = %f, want 7.5", p.TargetScore)
	}
	if p.Status != StatusActive {
		t.Errorf("Status = %q, want %q", p.Status, StatusActive)
	}
	if p.PaperPath == nil || *p.PaperPath != "paper.tex" {
		t.Errorf("PaperPath = %v, want paper.tex", p.PaperPath)
	}
}

func TestGetProjectNotFound(t *testing.T) {
	s := newTestStore(t)
	p, err := s.GetProject("nonexistent")
	if err != nil {
		t.Fatalf("GetProject: %v", err)
	}
	if p != nil {
		t.Fatal("expected nil for nonexistent project")
	}
}

func TestListProjects(t *testing.T) {
	s := newTestStore(t)
	s.CreateProject("P1", nil, nil, 7.0)
	s.CreateProject("P2", nil, nil, 7.0)

	projects, err := s.ListProjects("")
	if err != nil {
		t.Fatalf("ListProjects: %v", err)
	}
	if len(projects) != 2 {
		t.Errorf("len = %d, want 2", len(projects))
	}
}

func TestListProjectsByStatus(t *testing.T) {
	s := newTestStore(t)
	id1, _ := s.CreateProject("Active", nil, nil, 7.0)
	id2, _ := s.CreateProject("Paused", nil, nil, 7.0)
	s.UpdateProject(id2, map[string]any{"status": "paused"})
	_ = id1

	active, err := s.ListProjects("active")
	if err != nil {
		t.Fatalf("ListProjects(active): %v", err)
	}
	if len(active) != 1 {
		t.Errorf("active count = %d, want 1", len(active))
	}
}

func TestUpdateProject(t *testing.T) {
	s := newTestStore(t)
	id, _ := s.CreateProject("Original", nil, nil, 7.0)

	err := s.UpdateProject(id, map[string]any{"name": "Updated", "status": "paused"})
	if err != nil {
		t.Fatalf("UpdateProject: %v", err)
	}

	p, _ := s.GetProject(id)
	if p.Name != "Updated" {
		t.Errorf("Name = %q, want Updated", p.Name)
	}
	if p.Status != StatusPaused {
		t.Errorf("Status = %q, want paused", p.Status)
	}
}

func TestCreateAndGetIteration(t *testing.T) {
	s := newTestStore(t)
	pid, _ := s.CreateProject("P", nil, nil, 7.0)

	iid, err := s.CreateIteration(pid)
	if err != nil {
		t.Fatalf("CreateIteration: %v", err)
	}
	if iid == "" {
		t.Fatal("expected non-empty iteration ID")
	}

	iter, err := s.GetIteration(iid)
	if err != nil {
		t.Fatalf("GetIteration: %v", err)
	}
	if iter.IterationNum != 1 {
		t.Errorf("IterationNum = %d, want 1", iter.IterationNum)
	}
	if iter.Status != IterReviewing {
		t.Errorf("Status = %q, want reviewing", iter.Status)
	}
}

func TestIterationAutoIncrement(t *testing.T) {
	s := newTestStore(t)
	pid, _ := s.CreateProject("P", nil, nil, 7.0)

	s.CreateIteration(pid)
	iid2, _ := s.CreateIteration(pid)

	iter, _ := s.GetIteration(iid2)
	if iter.IterationNum != 2 {
		t.Errorf("IterationNum = %d, want 2", iter.IterationNum)
	}

	// Project current_iteration should be updated
	p, _ := s.GetProject(pid)
	if p.CurrentIteration != 2 {
		t.Errorf("CurrentIteration = %d, want 2", p.CurrentIteration)
	}
}

func TestGetCurrentIteration(t *testing.T) {
	s := newTestStore(t)
	pid, _ := s.CreateProject("P", nil, nil, 7.0)

	// No iterations yet
	current, err := s.GetCurrentIteration(pid)
	if err != nil {
		t.Fatalf("GetCurrentIteration: %v", err)
	}
	if current != nil {
		t.Fatal("expected nil for no iterations")
	}

	s.CreateIteration(pid)
	s.CreateIteration(pid)

	current, err = s.GetCurrentIteration(pid)
	if err != nil {
		t.Fatalf("GetCurrentIteration: %v", err)
	}
	if current.IterationNum != 2 {
		t.Errorf("IterationNum = %d, want 2", current.IterationNum)
	}
}

func TestListIterations(t *testing.T) {
	s := newTestStore(t)
	pid, _ := s.CreateProject("P", nil, nil, 7.0)
	s.CreateIteration(pid)
	s.CreateIteration(pid)
	s.CreateIteration(pid)

	iters, err := s.ListIterations(pid)
	if err != nil {
		t.Fatalf("ListIterations: %v", err)
	}
	if len(iters) != 3 {
		t.Errorf("len = %d, want 3", len(iters))
	}
	// Should be ordered by iteration_num
	for i, iter := range iters {
		if iter.IterationNum != i+1 {
			t.Errorf("iters[%d].IterationNum = %d, want %d", i, iter.IterationNum, i+1)
		}
	}
}

func TestUpdateIteration(t *testing.T) {
	s := newTestStore(t)
	pid, _ := s.CreateProject("P", nil, nil, 7.0)
	iid, _ := s.CreateIteration(pid)

	err := s.UpdateIteration(iid, map[string]any{
		"review_score": 7.5,
		"axis_scores":  map[string]any{"quality": 7, "novelty": 8},
		"gaps":         []map[string]any{{"type": "experiment", "desc": "ablation"}},
		"status":       "planning",
	})
	if err != nil {
		t.Fatalf("UpdateIteration: %v", err)
	}

	iter, _ := s.GetIteration(iid)
	if iter.ReviewScore == nil || *iter.ReviewScore != 7.5 {
		t.Errorf("ReviewScore = %v, want 7.5", iter.ReviewScore)
	}
	if iter.Status != IterPlanning {
		t.Errorf("Status = %q, want planning", iter.Status)
	}

	// Verify axis_scores JSON
	var axis map[string]any
	json.Unmarshal(iter.AxisScores, &axis)
	if axis["quality"] != float64(7) {
		t.Errorf("axis_scores.quality = %v, want 7", axis["quality"])
	}
}

func TestSuggestNextNoIterations(t *testing.T) {
	s := newTestStore(t)
	pid, _ := s.CreateProject("P", nil, nil, 7.0)

	result := s.SuggestNext(pid)
	if result["action"] != "review" {
		t.Errorf("action = %v, want review", result["action"])
	}
}

func TestSuggestNextReviewing(t *testing.T) {
	s := newTestStore(t)
	pid, _ := s.CreateProject("P", nil, nil, 7.0)
	s.CreateIteration(pid) // status = reviewing

	result := s.SuggestNext(pid)
	if result["action"] != "review" {
		t.Errorf("action = %v, want review", result["action"])
	}
}

func TestSuggestNextTargetReached(t *testing.T) {
	s := newTestStore(t)
	pid, _ := s.CreateProject("P", nil, nil, 7.0)
	iid, _ := s.CreateIteration(pid)
	s.UpdateIteration(iid, map[string]any{"review_score": 8.0, "status": "planning"})

	result := s.SuggestNext(pid)
	if result["action"] != "complete" {
		t.Errorf("action = %v, want complete", result["action"])
	}
}

func TestSuggestNextDoneIteration(t *testing.T) {
	s := newTestStore(t)
	pid, _ := s.CreateProject("P", nil, nil, 7.0)
	iid, _ := s.CreateIteration(pid)
	s.UpdateIteration(iid, map[string]any{"status": "done"})

	result := s.SuggestNext(pid)
	if result["action"] != "review" {
		t.Errorf("action = %v, want review", result["action"])
	}
}

func TestSuggestNextPendingExperiments(t *testing.T) {
	s := newTestStore(t)
	pid, _ := s.CreateProject("P", nil, nil, 7.0)
	iid, _ := s.CreateIteration(pid)
	s.UpdateIteration(iid, map[string]any{
		"review_score": 5.0,
		"status":       "experimenting",
		"gaps":         []map[string]any{{"type": "experiment", "desc": "ablation", "status": "pending"}},
	})

	result := s.SuggestNext(pid)
	if result["action"] != "run_experiments" {
		t.Errorf("action = %v, want run_experiments", result["action"])
	}
}

func TestSuggestNextPlanExperiments(t *testing.T) {
	s := newTestStore(t)
	pid, _ := s.CreateProject("P", nil, nil, 7.0)
	iid, _ := s.CreateIteration(pid)
	s.UpdateIteration(iid, map[string]any{
		"review_score": 5.0,
		"status":       "planning",
	})

	result := s.SuggestNext(pid)
	if result["action"] != "plan_experiments" {
		t.Errorf("action = %v, want plan_experiments", result["action"])
	}
}

func TestSuggestNextInactiveProject(t *testing.T) {
	s := newTestStore(t)
	pid, _ := s.CreateProject("P", nil, nil, 7.0)
	s.UpdateProject(pid, map[string]any{"status": "paused"})

	result := s.SuggestNext(pid)
	if result["action"] != "none" {
		t.Errorf("action = %v, want none", result["action"])
	}
}

func TestSuggestNextNotFound(t *testing.T) {
	s := newTestStore(t)
	result := s.SuggestNext("nonexistent")
	if result["action"] != "none" {
		t.Errorf("action = %v, want none", result["action"])
	}
}

func TestDBFileCreated(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	s.Close()

	dbPath := filepath.Join(dir, ".c4", "research", "research.db")
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Error("database file not created")
	}
}
