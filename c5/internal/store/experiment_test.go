package store

import (
	"context"
	"errors"
	"testing"
)

func TestSQLiteExperimentStore_StartRun(t *testing.T) {
	s := newTestStore(t)
	runID, err := s.StartRun(context.Background(), "pose-exp", "pose-estimation")
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	if runID == "" {
		t.Fatal("run_id must not be empty")
	}
}

func TestSQLiteExperimentStore_RecordCheckpoint_FirstIsBest(t *testing.T) {
	s := newTestStore(t)
	runID, _ := s.StartRun(context.Background(), "exp1", "cap1")

	isBest, err := s.RecordCheckpoint(context.Background(), runID, 50.0, "")
	if err != nil {
		t.Fatalf("RecordCheckpoint: %v", err)
	}
	if !isBest {
		t.Fatal("first checkpoint must always be best")
	}
}

func TestSQLiteExperimentStore_RecordCheckpoint_ImprovesRecord(t *testing.T) {
	s := newTestStore(t)
	runID, _ := s.StartRun(context.Background(), "exp1", "cap1")
	s.RecordCheckpoint(context.Background(), runID, 50.0, "") //nolint:errcheck

	isBest, err := s.RecordCheckpoint(context.Background(), runID, 45.0, "ckpt2")
	if err != nil {
		t.Fatalf("RecordCheckpoint: %v", err)
	}
	if !isBest {
		t.Fatal("lower metric should be best")
	}
}

func TestSQLiteExperimentStore_RecordCheckpoint_NoImprovement(t *testing.T) {
	s := newTestStore(t)
	runID, _ := s.StartRun(context.Background(), "exp1", "cap1")
	s.RecordCheckpoint(context.Background(), runID, 45.0, "") //nolint:errcheck

	isBest, err := s.RecordCheckpoint(context.Background(), runID, 50.0, "worse")
	if err != nil {
		t.Fatalf("RecordCheckpoint: %v", err)
	}
	if isBest {
		t.Fatal("higher metric should not be best (lower is better)")
	}
}

func TestSQLiteExperimentStore_ShouldContinue_Running(t *testing.T) {
	s := newTestStore(t)
	runID, _ := s.StartRun(context.Background(), "exp1", "cap1")

	ok, err := s.ShouldContinue(context.Background(), runID)
	if err != nil {
		t.Fatalf("ShouldContinue: %v", err)
	}
	if !ok {
		t.Fatal("running run should return true")
	}
}

func TestSQLiteExperimentStore_ShouldContinue_Completed(t *testing.T) {
	s := newTestStore(t)
	runID, _ := s.StartRun(context.Background(), "exp1", "cap1")
	s.CompleteRun(context.Background(), runID, "success", 42.0, "") //nolint:errcheck

	ok, err := s.ShouldContinue(context.Background(), runID)
	if err != nil {
		t.Fatalf("ShouldContinue: %v", err)
	}
	if ok {
		t.Fatal("completed run should return false")
	}
}

func TestSQLiteExperimentStore_CompleteRun(t *testing.T) {
	s := newTestStore(t)
	runID, _ := s.StartRun(context.Background(), "exp1", "cap1")

	if err := s.CompleteRun(context.Background(), runID, "success", 42.0, "done"); err != nil {
		t.Fatalf("CompleteRun: %v", err)
	}
}

func TestSQLiteExperimentStore_RecordCheckpoint_RunNotFound(t *testing.T) {
	s := newTestStore(t)
	_, err := s.RecordCheckpoint(context.Background(), "no-such-run", 1.0, "")
	if !errors.Is(err, ErrRunNotFound) {
		t.Fatalf("expected ErrRunNotFound, got %v", err)
	}
}

func TestSQLiteExperimentStore_SearchRuns(t *testing.T) {
	s := newTestStore(t)
	s.StartRun(context.Background(), "pose-exp", "pose-estimation")   //nolint:errcheck
	s.StartRun(context.Background(), "depth-exp", "depth-estimation") //nolint:errcheck

	runs, err := s.SearchRuns(context.Background(), "pose", 10)
	if err != nil {
		t.Fatalf("SearchRuns: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected 1 result, got %d", len(runs))
	}
	if runs[0].Name != "pose-exp" {
		t.Fatalf("expected pose-exp, got %s", runs[0].Name)
	}
}
