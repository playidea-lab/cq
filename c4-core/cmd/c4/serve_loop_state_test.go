//go:build research

package main

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestStateYAMLWriter_WriteRead(t *testing.T) {
	dir := t.TempDir()
	w := NewStateYAMLWriter(dir)

	deadline := time.Now().Add(1 * time.Hour).UTC().Truncate(time.Second)
	in := LoopState{
		State:               "running",
		LoopCount:           3,
		CurrentHypothesisID: "hyp-42",
		LastJobID:           "job-99",
		GateDeadline:        &deadline,
	}
	if err := w.WriteState(in); err != nil {
		t.Fatalf("WriteState: %v", err)
	}

	got, err := w.ReadState()
	if err != nil {
		t.Fatalf("ReadState: %v", err)
	}
	if got.State != in.State {
		t.Errorf("State: want %q got %q", in.State, got.State)
	}
	if got.LoopCount != in.LoopCount {
		t.Errorf("LoopCount: want %d got %d", in.LoopCount, got.LoopCount)
	}
	if got.CurrentHypothesisID != in.CurrentHypothesisID {
		t.Errorf("CurrentHypothesisID: want %q got %q", in.CurrentHypothesisID, got.CurrentHypothesisID)
	}
	if got.LastJobID != in.LastJobID {
		t.Errorf("LastJobID: want %q got %q", in.LastJobID, got.LastJobID)
	}
	if got.GateDeadline == nil {
		t.Fatal("GateDeadline: want non-nil got nil")
	}
	if !got.GateDeadline.Equal(deadline) {
		t.Errorf("GateDeadline: want %v got %v", deadline, *got.GateDeadline)
	}
	if got.LastUpdated.IsZero() {
		t.Error("LastUpdated should be set")
	}
}

func TestStateYAMLWriter_ConcurrentWriteNoTear(t *testing.T) {
	dir := t.TempDir()
	w := NewStateYAMLWriter(dir)

	// prime file
	if err := w.WriteState(LoopState{State: "running", LoopCount: 0}); err != nil {
		t.Fatalf("initial write: %v", err)
	}

	var wg sync.WaitGroup
	errCh := make(chan error, 30)

	// 2 writer goroutines
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				s := LoopState{State: "running", LoopCount: n*10 + j}
				if err := w.WriteState(s); err != nil {
					errCh <- err
				}
			}
		}(i)
	}

	// 1 reader goroutine — reads must always be valid YAML (no torn writes)
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 20; i++ {
			_, err := w.ReadState()
			if err != nil {
				errCh <- err
			}
		}
	}()

	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Errorf("concurrent error: %v", err)
	}
}

func TestStateYAMLWriter_MkdirOnMissing(t *testing.T) {
	base := t.TempDir()
	c9Dir := filepath.Join(base, "nonexistent", ".c9")
	w := NewStateYAMLWriter(c9Dir)

	if err := w.WriteState(LoopState{State: "stopped"}); err != nil {
		t.Fatalf("WriteState: %v", err)
	}
	if _, err := os.Stat(c9Dir); err != nil {
		t.Errorf("directory not created: %v", err)
	}
	if _, err := os.Stat(filepath.Join(c9Dir, "state.yaml")); err != nil {
		t.Errorf("state.yaml not created: %v", err)
	}
}

func TestStateYAMLWriter_Resume_GateWait(t *testing.T) {
	dir := t.TempDir()
	w := NewStateYAMLWriter(dir)

	if err := w.WriteState(LoopState{State: "gate_wait", LoopCount: 5}); err != nil {
		t.Fatalf("WriteState: %v", err)
	}
	got, err := w.ReadState()
	if err != nil {
		t.Fatalf("ReadState: %v", err)
	}
	if got.State != "gate_wait" {
		t.Errorf("State: want %q got %q", "gate_wait", got.State)
	}
	if got.LoopCount != 5 {
		t.Errorf("LoopCount: want 5 got %d", got.LoopCount)
	}
}

func TestStateYAMLWriter_Resume_Running(t *testing.T) {
	dir := t.TempDir()
	w := NewStateYAMLWriter(dir)

	in := LoopState{
		State:               "running",
		LoopCount:           12,
		CurrentHypothesisID: "hyp-7",
		LastJobID:           "job-55",
	}
	if err := w.WriteState(in); err != nil {
		t.Fatalf("WriteState: %v", err)
	}
	got, err := w.ReadState()
	if err != nil {
		t.Fatalf("ReadState: %v", err)
	}
	if got.State != "running" {
		t.Errorf("State: want running got %q", got.State)
	}
	if got.CurrentHypothesisID != "hyp-7" {
		t.Errorf("CurrentHypothesisID: want hyp-7 got %q", got.CurrentHypothesisID)
	}
}

func TestStateYAMLWriter_Resume_ExpiredDeadline(t *testing.T) {
	dir := t.TempDir()
	w := NewStateYAMLWriter(dir)

	past := time.Now().Add(-2 * time.Hour).UTC().Truncate(time.Second)
	in := LoopState{
		State:        "gate_wait",
		LoopCount:    2,
		GateDeadline: &past,
	}
	if err := w.WriteState(in); err != nil {
		t.Fatalf("WriteState: %v", err)
	}
	got, err := w.ReadState()
	if err != nil {
		t.Fatalf("ReadState: %v", err)
	}
	if got.GateDeadline == nil {
		t.Fatal("GateDeadline should be non-nil")
	}
	if !got.GateDeadline.Equal(past) {
		t.Errorf("GateDeadline: want %v got %v", past, *got.GateDeadline)
	}
	// Caller is responsible for detecting expiry; ReadState just restores.
	if got.GateDeadline.After(time.Now()) {
		t.Error("expected deadline to be in the past")
	}
}
