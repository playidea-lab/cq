package store

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	_ "modernc.org/sqlite"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open in-memory sqlite: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestSQLiteExperimentStore_StartRun(t *testing.T) {
	s, err := NewSQLiteExperimentStore(openTestDB(t))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	ctx := context.Background()

	runID, err := s.StartRun(ctx, "test-run", `{"lr":0.01}`)
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	if runID == "" {
		t.Error("expected non-empty run_id")
	}
}

func TestSQLiteExperimentStore_RecordCheckpoint_Atomic(t *testing.T) {
	s, err := NewSQLiteExperimentStore(openTestDB(t))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	ctx := context.Background()

	runID, _ := s.StartRun(ctx, "chk-run", "")

	// First checkpoint — should be best.
	isBest, err := s.RecordCheckpoint(ctx, runID, 1.5, "/ckpt/1")
	if err != nil {
		t.Fatalf("first checkpoint: %v", err)
	}
	if !isBest {
		t.Error("first checkpoint should be best")
	}

	// Better (lower) metric — should be best.
	isBest, err = s.RecordCheckpoint(ctx, runID, 0.8, "/ckpt/2")
	if err != nil {
		t.Fatalf("second checkpoint: %v", err)
	}
	if !isBest {
		t.Error("lower metric should be best")
	}

	// Worse (higher) metric — should NOT be best.
	isBest, err = s.RecordCheckpoint(ctx, runID, 2.0, "/ckpt/3")
	if err != nil {
		t.Fatalf("third checkpoint: %v", err)
	}
	if isBest {
		t.Error("higher metric should not be best")
	}

	// Verify all 3 checkpoint history rows were recorded.
	var count int
	s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM exp_checkpoints WHERE run_id=?`, runID).Scan(&count)
	if count != 3 {
		t.Errorf("expected 3 checkpoint rows, got %d", count)
	}
}

func TestSQLiteExperimentStore_ShouldContinue(t *testing.T) {
	s, err := NewSQLiteExperimentStore(openTestDB(t))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	ctx := context.Background()

	runID, _ := s.StartRun(ctx, "cont-run", "")

	ok, err := s.ShouldContinue(ctx, runID)
	if err != nil || !ok {
		t.Errorf("new run should continue: ok=%v err=%v", ok, err)
	}

	s.CompleteRun(ctx, runID, "success", 0.9)

	ok, err = s.ShouldContinue(ctx, runID)
	if err != nil || ok {
		t.Errorf("completed run should not continue: ok=%v err=%v", ok, err)
	}
}

func TestSQLiteExperimentStore_ShouldContinue_UnknownRun(t *testing.T) {
	s, err := NewSQLiteExperimentStore(openTestDB(t))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	ok, err := s.ShouldContinue(context.Background(), "non-existent-run")
	if !errors.Is(err, ErrRunNotFound) {
		t.Fatalf("expected ErrRunNotFound, got: %v", err)
	}
	if ok {
		t.Error("unknown run should return should_continue=false")
	}
}

func TestSQLiteExperimentStore_CompleteRun(t *testing.T) {
	s, err := NewSQLiteExperimentStore(openTestDB(t))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	ctx := context.Background()
	runID, _ := s.StartRun(ctx, "complete-run", "")

	if err := s.CompleteRun(ctx, runID, "success", 0.95); err != nil {
		t.Fatalf("CompleteRun: %v", err)
	}

	// Completing a non-existent run should return an error.
	if err := s.CompleteRun(ctx, "bad-id", "success", 0); err == nil {
		t.Error("expected error for non-existent run_id")
	}
}
