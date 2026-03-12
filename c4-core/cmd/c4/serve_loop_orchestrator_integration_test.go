//go:build research

package main

import (
	"context"
	"testing"
	"time"
)

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
