package handlers

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

// fakeExperimentStore is a minimal in-memory ExperimentStore for tests.
type fakeExperimentStore struct {
	runs       map[string]string // run_id -> status
	checkpoints []checkpointRecord
	completeErr error
	registerErr error
}

type checkpointRecord struct {
	runID  string
	metric float64
	path   string
}

func newFakeStore() *fakeExperimentStore {
	return &fakeExperimentStore{runs: make(map[string]string)}
}

func (f *fakeExperimentStore) StartRun(_ context.Context, name, _ string) (string, error) {
	if f.registerErr != nil {
		return "", f.registerErr
	}
	id := "run-" + name
	f.runs[id] = "running"
	return id, nil
}

func (f *fakeExperimentStore) RecordCheckpoint(_ context.Context, runID string, metric float64, path string) (bool, error) {
	f.checkpoints = append(f.checkpoints, checkpointRecord{runID, metric, path})
	return true, nil
}

func (f *fakeExperimentStore) ShouldContinue(_ context.Context, runID string) (bool, error) {
	status, ok := f.runs[runID]
	if !ok {
		return false, nil
	}
	return status == "running", nil
}

func (f *fakeExperimentStore) CompleteRun(_ context.Context, runID, status string, _ float64) error {
	if f.completeErr != nil {
		return f.completeErr
	}
	f.runs[runID] = status
	return nil
}

func TestExperimentHandler_Register(t *testing.T) {
	h := ExperimentHandlers{Store: newFakeStore()}
	fn := registerRunHandler(h)

	args, _ := json.Marshal(map[string]any{"name": "exp-1"})
	result, err := fn(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map result")
	}
	if m["success"] != true {
		t.Errorf("expected success=true, got %v", m)
	}
	if m["run_id"] == "" {
		t.Error("expected non-empty run_id")
	}
}

func TestExperimentHandler_Register_MissingName(t *testing.T) {
	h := ExperimentHandlers{Store: newFakeStore()}
	fn := registerRunHandler(h)

	args, _ := json.Marshal(map[string]any{})
	result, err := fn(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]any)
	if _, hasErr := m["error"]; !hasErr {
		t.Error("expected error field for missing name")
	}
}

func TestExperimentHandler_ShouldContinue_Running(t *testing.T) {
	store := newFakeStore()
	store.runs["run-abc"] = "running"
	h := ExperimentHandlers{Store: store}
	fn := shouldContinueHandler(h)

	args, _ := json.Marshal(map[string]any{"run_id": "run-abc"})
	result, err := fn(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]any)
	if m["should_continue"] != true {
		t.Errorf("expected should_continue=true, got %v", m["should_continue"])
	}
}

func TestExperimentHandler_ShouldContinue_Completed(t *testing.T) {
	store := newFakeStore()
	store.runs["run-done"] = "success"
	h := ExperimentHandlers{Store: store}
	fn := shouldContinueHandler(h)

	args, _ := json.Marshal(map[string]any{"run_id": "run-done"})
	result, _ := fn(context.Background(), args)
	m := result.(map[string]any)
	if m["should_continue"] != false {
		t.Errorf("expected should_continue=false for completed run, got %v", m["should_continue"])
	}
}

func TestExperimentHandler_Complete_AutoBridge(t *testing.T) {
	bridgeCalled := make(chan struct{}, 1)
	h := ExperimentHandlers{
		Store: newFakeStore(),
		KnowledgeRecord: func(ctx context.Context, title, content, domain string) error {
			bridgeCalled <- struct{}{}
			return nil
		},
	}

	// Register a run first so CompleteRun finds it.
	store := h.Store.(*fakeExperimentStore)
	store.runs["run-xyz"] = "running"

	fn := completeRunHandler(h)
	args, _ := json.Marshal(map[string]any{
		"run_id":       "run-xyz",
		"status":       "success",
		"final_metric": 0.95,
		"summary":      "great run",
	})
	result, err := fn(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]any)
	if m["success"] != true {
		t.Errorf("expected success=true, got %v", m)
	}

	// Verify auto-bridge KnowledgeRecord was called (with timeout).
	select {
	case <-bridgeCalled:
		// success
	case <-time.After(200 * time.Millisecond):
		t.Error("auto-bridge KnowledgeRecord was not called within 200ms")
	}
}

func TestExperimentHandler_Complete_NilKnowledgeRecord(t *testing.T) {
	// KnowledgeRecord=nil should not panic; auto-bridge is simply skipped.
	store := newFakeStore()
	store.runs["run-nok"] = "running"
	h := ExperimentHandlers{Store: store, KnowledgeRecord: nil}
	fn := completeRunHandler(h)

	args, _ := json.Marshal(map[string]any{"run_id": "run-nok", "status": "success"})
	result, err := fn(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected panic or error: %v", err)
	}
	m := result.(map[string]any)
	if m["success"] != true {
		t.Errorf("expected success=true, got %v", m)
	}
}

func TestExperimentHandler_AutoBridge_UsesWithoutCancel(t *testing.T) {
	// Verify that the bridge goroutine still runs after the parent ctx is cancelled.
	bridgeCalled := make(chan struct{}, 1)
	h := ExperimentHandlers{
		Store: newFakeStore(),
		KnowledgeRecord: func(ctx context.Context, title, content, domain string) error {
			// Should still be called even after parent ctx cancelled.
			bridgeCalled <- struct{}{}
			return nil
		},
	}
	store := h.Store.(*fakeExperimentStore)
	store.runs["run-ctx"] = "running"

	parentCtx, cancel := context.WithCancel(context.Background())
	fn := completeRunHandler(h)
	args, _ := json.Marshal(map[string]any{"run_id": "run-ctx", "status": "success"})
	fn(parentCtx, args) //nolint:errcheck
	// Cancel parent immediately after handler returns.
	cancel()

	select {
	case <-bridgeCalled:
		// success: context.WithoutCancel ensured goroutine ran despite parent cancellation
	case <-time.After(200 * time.Millisecond):
		t.Error("auto-bridge was not called after parent ctx cancelled (WithoutCancel not used)")
	}
}
