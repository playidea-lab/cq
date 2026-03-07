package api

import (
	"context"
	"testing"
	"time"
)

// TestJobCompletionChannel_HappyPath verifies that a job submitted and then
// immediately completed via handleWorkerComplete unblocks waitForCompletion
// within 50 ms (well below the 2 s ticker baseline).
func TestJobCompletionChannel_HappyPath(t *testing.T) {
	s := newTestServer(t)
	jobID := "test-job-happy"

	ch := make(chan struct{}, 1)
	s.completionHub.Store(jobID, ch)

	go func() {
		time.Sleep(5 * time.Millisecond)
		s.handleWorkerComplete(jobID)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	if err := s.waitForCompletion(ctx, jobID); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	elapsed := time.Since(start)
	if elapsed >= 50*time.Millisecond {
		t.Fatalf("waitForCompletion took %v, expected < 50ms", elapsed)
	}
}

// TestJobCompletionChannel_Timeout verifies that waitForCompletion returns an
// error when the context deadline is exceeded before the job completes.
func TestJobCompletionChannel_Timeout(t *testing.T) {
	s := newTestServer(t)
	jobID := "test-job-timeout"

	ch := make(chan struct{}, 1)
	s.completionHub.Store(jobID, ch)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	err := s.waitForCompletion(ctx, jobID)
	if err == nil {
		t.Fatal("expected non-nil error on timeout, got nil")
	}
	if ctx.Err() == nil {
		t.Fatal("expected context to be cancelled")
	}
}

// TestJobCompletionChannel_MockNotifyPath verifies that completionHub.Load +
// close actually unblocks the waiter (no mock shortcut — real channel close).
func TestJobCompletionChannel_MockNotifyPath(t *testing.T) {
	s := newTestServer(t)
	jobID := "test-job-notify"

	ch := make(chan struct{}, 1)
	s.completionHub.Store(jobID, ch)

	// Confirm the channel is stored.
	v, ok := s.completionHub.Load(jobID)
	if !ok {
		t.Fatal("completionHub.Load: key not found")
	}
	stored := v.(chan struct{})

	// Close the channel directly (simulates handleWorkerComplete internals).
	close(stored)

	// After close, LoadAndDelete should no longer find the key (already loaded once).
	// Re-store so we can LoadAndDelete.
	ch2 := make(chan struct{}, 1)
	s.completionHub.Store(jobID+"2", ch2)
	if _, loaded := s.completionHub.LoadAndDelete(jobID + "2"); !loaded {
		t.Fatal("LoadAndDelete: expected to find stored key")
	}
	if _, loaded := s.completionHub.LoadAndDelete(jobID + "2"); loaded {
		t.Fatal("LoadAndDelete: second call should not find deleted key (idempotency)")
	}

	// Ensure the closed channel unblocks a select immediately.
	ctx := context.Background()
	done := make(chan error, 1)
	go func() {
		// Re-register the already-closed channel to exercise waitForCompletion.
		s.completionHub.Store(jobID, stored)
		done <- s.waitForCompletion(ctx, jobID)
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	case <-time.After(50 * time.Millisecond):
		t.Fatal("waitForCompletion did not unblock within 50ms after channel close")
	}
}

// TestJobCompletionChannel_CtxCancel verifies that cancelling the context
// causes waitForCompletion to return immediately.
func TestJobCompletionChannel_CtxCancel(t *testing.T) {
	s := newTestServer(t)
	jobID := "test-job-cancel"

	ch := make(chan struct{}, 1)
	s.completionHub.Store(jobID, ch)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- s.waitForCompletion(ctx, jobID)
	}()

	// Cancel after a short delay.
	time.Sleep(5 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected non-nil error on ctx cancel, got nil")
		}
	case <-time.After(50 * time.Millisecond):
		t.Fatal("waitForCompletion did not return within 50ms after ctx cancel")
	}
}

// TestJobCompletionChannel_DuplicateComplete verifies that calling
// handleWorkerComplete twice for the same job does not panic.
func TestJobCompletionChannel_DuplicateComplete(t *testing.T) {
	s := newTestServer(t)
	jobID := "test-job-duplicate"

	ch := make(chan struct{}, 1)
	s.completionHub.Store(jobID, ch)

	// First call should close and delete the channel.
	s.handleWorkerComplete(jobID)

	// Second call must not panic (LoadAndDelete returns loaded=false).
	s.handleWorkerComplete(jobID)
}
