//go:build hub

package hub

import (
	"context"
	"log"
)

// ChannelJobListener wraps the callback-based JobListener with a channel API
// for use in a worker standby select loop.
//
// When directURL is empty the listener is a no-op: C is nil and the caller
// falls back to polling only.
//
// On every (re)connect an empty string is sent on C so the caller can poll for
// jobs that arrived while the connection was down. Real NOTIFY payloads carry
// the job_id string.
type ChannelJobListener struct {
	// C delivers job_id strings from pg_notify payloads.
	// An empty string signals a reconnect poll (caller should ClaimJob).
	// C is nil when directURL was empty (no LISTEN configured).
	C    <-chan string
	ch   chan string
	done chan struct{}
}

// NewChannelJobListener creates a ChannelJobListener for directURL.
// If directURL is empty a no-op listener is returned (C is nil).
// Call Start(ctx, directURL) to begin listening.
func NewChannelJobListener(directURL string) *ChannelJobListener {
	if directURL == "" {
		return &ChannelJobListener{done: make(chan struct{})}
	}
	ch := make(chan string, 8) // buffered to absorb bursts
	return &ChannelJobListener{
		C:    ch,
		ch:   ch,
		done: make(chan struct{}),
	}
}

// Start launches the LISTEN loop in a background goroutine.
// Calling Start on a no-op listener (empty directURL) is safe and closes done immediately.
// The loop runs until ctx is cancelled.
func (l *ChannelJobListener) Start(ctx context.Context, directURL string) {
	if l.ch == nil {
		// no-op: signal done so Close does not block
		close(l.done)
		return
	}

	go func() {
		defer close(l.done)
		defer close(l.ch) // signal receiver that listener has stopped

		inner := NewJobListener(directURL)
		err := inner.Listen(ctx, func(n JobNotification) error {
			select {
			case l.ch <- n.Payload:
			case <-ctx.Done():
				return ctx.Err()
			}
			return nil
		})
		if err != nil && ctx.Err() == nil {
			log.Printf("hub/listener: LISTEN loop stopped: %v", err)
		}
	}()
}

// Close waits for the listener goroutine to finish.
// The caller is responsible for cancelling the context passed to Start.
// Safe to call on a no-op listener.
func (l *ChannelJobListener) Close() {
	<-l.done
}
