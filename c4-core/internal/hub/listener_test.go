//go:build hub

package hub

import (
	"context"
	"testing"
	"time"
)

// TestNewChannelJobListener_NoURL verifies that a no-op listener is created when
// directURL is empty: C is nil and Close does not block.
func TestNewChannelJobListener_NoURL(t *testing.T) {
	l := NewChannelJobListener("")
	if l.C != nil {
		t.Fatal("expected C to be nil for empty directURL")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	l.Start(ctx, "")
	l.Close() // must not block
}

// TestChannelJobListener_NilChannelBlocksForever verifies that a nil channel
// returned by a no-op listener would block in a select (Go semantic guarantee).
// We test this indirectly: a select with only a nil case and a default must hit default.
func TestChannelJobListener_NilChannelBlocksForever(t *testing.T) {
	var ch <-chan string // nil
	select {
	case <-ch:
		t.Fatal("nil channel should block forever")
	default:
		// expected
	}
}

// TestNewChannelJobListener_WithURL verifies that a non-empty directURL creates
// a listener with a non-nil C channel. We do not actually connect to Postgres.
func TestNewChannelJobListener_WithURL(t *testing.T) {
	l := NewChannelJobListener("postgres://localhost:5432/testdb")
	if l.C == nil {
		t.Fatal("expected non-nil C for non-empty directURL")
	}
	if l.ch == nil {
		t.Fatal("expected non-nil ch for non-empty directURL")
	}
}

// TestChannelJobListener_StartCancelClose verifies that Start and Close work when
// the context is cancelled before any notifications arrive.
// We use a URL that will fail to connect (guaranteed) so the goroutine just retries
// with backoff; cancelling ctx must cause it to exit cleanly.
func TestChannelJobListener_StartCancelClose(t *testing.T) {
	l := NewChannelJobListener("postgres://127.0.0.1:1/nonexistent")

	ctx, cancel := context.WithCancel(context.Background())
	l.Start(ctx, "postgres://127.0.0.1:1/nonexistent")

	// Give the goroutine a moment to try its first connect attempt.
	time.Sleep(20 * time.Millisecond)
	cancel()

	done := make(chan struct{})
	go func() {
		l.Close()
		close(done)
	}()

	select {
	case <-done:
		// success
	case <-time.After(5 * time.Second):
		t.Fatal("Close() timed out — goroutine did not exit after ctx cancel")
	}
}
