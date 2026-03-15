//go:build research

package orchestrator

import (
	"context"
	"testing"
	"time"
)

func TestGateController_AutoContinue(t *testing.T) {
	gc := NewGateController(20 * time.Millisecond)
	ch := gc.EnterGate(context.Background())
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	select {
	case <-ch:
		// success: timer fired
	case <-ctx.Done():
		t.Error("expected gate to auto-close after duration, but timed out")
	}
}

func TestGateController_ReleasedByIntervene(t *testing.T) {
	gc := NewGateController(10 * time.Second) // long duration — must not expire naturally
	ch := gc.EnterGate(context.Background())

	go gc.Release("test-intervene")

	select {
	case <-ch:
		// success
	case <-time.After(200 * time.Millisecond):
		t.Error("expected gate to close after Release(), but timed out")
	}
}

func TestGateController_ZeroDuration(t *testing.T) {
	gc := NewGateController(0)
	ch := gc.EnterGate(context.Background())
	select {
	case <-ch:
		// immediate pass
	default:
		t.Error("expected gate with zero duration to close immediately")
	}
}

func TestGateController_CtxCancelDoesNotClose(t *testing.T) {
	gc := NewGateController(200 * time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	ch := gc.EnterGate(ctx)
	cancel() // cancel context immediately

	// channel should NOT be closed yet; give a brief window to detect a false close
	select {
	case <-ch:
		t.Error("gate channel closed on ctx cancel; expected it to remain open")
	case <-time.After(30 * time.Millisecond):
		// good: still open after cancel
	}

	// clean up: Release so the goroutine exits
	gc.Release("cleanup")
}

func TestGateController_ReentrantCall(t *testing.T) {
	gc := NewGateController(10 * time.Second)
	ch1 := gc.EnterGate(context.Background())
	ch2 := gc.EnterGate(context.Background()) // second call overrides releaseCh

	// Release should close ch2 (the latest gate)
	gc.Release("test")

	select {
	case <-ch2:
		// success
	case <-time.After(200 * time.Millisecond):
		t.Error("second gate channel not closed after Release()")
	}

	// ch1 goroutine is still waiting on its own timer; Release it too
	gc.Release("cleanup-ch1")
	_ = ch1 // silence unused warning
}
