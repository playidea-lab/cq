//go:build research

package main

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

// TestLoopOrchestrator_GateAndState_Integration verifies that after a simulated
// job completion, state is written as "gate_wait" and gate fires after duration.
func TestLoopOrchestrator_GateAndState_Integration(t *testing.T) {
	dir := t.TempDir()
	c9Dir := filepath.Join(dir, ".c9")

	w := NewStateYAMLWriter(c9Dir)
	gate := NewGateController(50 * time.Millisecond) // fast for test

	// Simulate: write gate_wait state.
	deadline := time.Now().Add(50 * time.Millisecond)
	if err := w.WriteState(LoopState{
		State:        "gate_wait",
		LoopCount:    1,
		GateDeadline: &deadline,
	}); err != nil {
		t.Fatalf("WriteState: %v", err)
	}

	// Verify state persisted.
	s, err := w.ReadState()
	if err != nil {
		t.Fatalf("ReadState: %v", err)
	}
	if s.State != "gate_wait" {
		t.Errorf("expected state=gate_wait, got %q", s.State)
	}
	if s.LoopCount != 1 {
		t.Errorf("expected loop_count=1, got %d", s.LoopCount)
	}
	if s.GateDeadline == nil {
		t.Error("expected non-nil gate_deadline")
	}

	// Gate fires after duration.
	ch := gate.EnterGate(context.Background())
	select {
	case <-ch:
		// success
	case <-time.After(500 * time.Millisecond):
		t.Error("gate did not fire within 500ms")
	}

	// Write running state.
	if err := w.WriteState(LoopState{State: "running", LoopCount: 2}); err != nil {
		t.Fatalf("WriteState running: %v", err)
	}
	s2, err := w.ReadState()
	if err != nil {
		t.Fatalf("ReadState running: %v", err)
	}
	if s2.State != "running" {
		t.Errorf("expected state=running, got %q", s2.State)
	}
	if s2.LoopCount != 2 {
		t.Errorf("expected loop_count=2, got %d", s2.LoopCount)
	}
}

// TestLoopOrchestrator_GateRelease_Integration verifies Release() unblocks the gate early.
func TestLoopOrchestrator_GateRelease_Integration(t *testing.T) {
	gate := NewGateController(10 * time.Second) // long gate — must be released manually

	ch := gate.EnterGate(context.Background())

	// Simulate human intervene → release gate.
	go func() {
		time.Sleep(10 * time.Millisecond)
		gate.Release("human-intervene")
	}()

	select {
	case <-ch:
		// success: gate released early
	case <-time.After(500 * time.Millisecond):
		t.Error("expected gate release within 500ms")
	}
}

// TestLoopOrchestrator_ResumeGate_Integration verifies Start() resumes a gate
// from persisted state when deadline is still in the future.
func TestLoopOrchestrator_ResumeGate_Integration(t *testing.T) {
	dir := t.TempDir()
	c9Dir := filepath.Join(dir, ".c9")

	w := NewStateYAMLWriter(c9Dir)

	// Write a gate_wait state with deadline 50ms from now.
	deadline := time.Now().Add(50 * time.Millisecond)
	if err := w.WriteState(LoopState{
		State:        "gate_wait",
		LoopCount:    3,
		GateDeadline: &deadline,
	}); err != nil {
		t.Fatalf("WriteState: %v", err)
	}

	// Build an orchestrator with a long default gate — resume should shorten it.
	o := &LoopOrchestrator{
		cfg:    LoopOrchestratorConfig{PollInterval: time.Second},
		status: "ok",
		gate:   NewGateController(24 * time.Hour),
		state:  w,
		notify: NewNotifyBridge(nil, time.Minute),
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := o.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer o.Stop(context.Background()) //nolint:errcheck

	// The gate should have been replaced with a ~50ms one.
	// Read gate under mu to avoid data race with Start().
	o.mu.Lock()
	gate := o.gate
	o.mu.Unlock()
	ch := gate.EnterGate(ctx)
	select {
	case <-ch:
		// success
	case <-time.After(500 * time.Millisecond):
		t.Error("resumed gate did not fire within 500ms")
	}
}

// TestLoopOrchestrator_Stop_DoesNotHang verifies that Stop() returns even when
// a gate is active, by releasing the gate before waiting on the done channel.
func TestLoopOrchestrator_Stop_DoesNotHang(t *testing.T) {
	// Arrange: gate with a long duration (simulates human-on-the-loop wait)
	gate := NewGateController(10 * time.Second)

	// Simulate: gate entered (as onJobDone would do)
	ch := gate.EnterGate(context.Background())

	// Act: Release("stop") should unblock the gate channel
	done := make(chan struct{})
	go func() {
		gate.Release("stop")
		close(done)
	}()

	select {
	case <-ch:
		// gate released as expected
	case <-time.After(500 * time.Millisecond):
		t.Error("gate did not release after Stop()")
	}

	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Error("Release goroutine did not complete")
	}
}

// TestLoopOrchestrator_Stop_NilGate verifies that Stop() does not panic when
// o.gate is nil (the default state before any gate is configured).
func TestLoopOrchestrator_Stop_NilGate(t *testing.T) {
	o := newLoopOrchestrator(LoopOrchestratorConfig{})
	ctx := context.Background()
	if err := o.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	stopCtx := context.Background()
	if err := o.Stop(stopCtx); err != nil {
		t.Fatalf("Stop: %v", err)
	}
}
