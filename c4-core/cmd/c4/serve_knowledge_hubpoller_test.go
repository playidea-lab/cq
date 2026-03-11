package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// TestPollerWakeChannel verifies that sending on the wake channel triggers an
// immediate poll() without waiting for the next ticker tick.
// A subtest also verifies that a nil wake channel leaves ticker-only behavior intact.
func TestPollerWakeChannel(t *testing.T) {
	t.Run("wake triggers immediate poll", func(t *testing.T) {
		// Count actual HTTP requests made to the Hub during poll().
		var pollCount atomic.Int64
		fakeSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			pollCount.Add(1)
			// Return empty completed-jobs list so poll() succeeds quickly.
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte("[]")) //nolint:errcheck
		}))
		defer fakeSrv.Close()

		// Use a very long ticker interval so the ticker never fires during the test.
		p := newKnowledgeHubPoller(knowledgeHubPollerConfig{
			HubURL:       fakeSrv.URL,
			PollInterval: 24 * time.Hour,
			SeenPath:     t.TempDir() + "/seen.json",
		})

		wakeCh := make(chan struct{}, 1)
		p.SetWakeChannel(wakeCh)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		if err := p.Start(ctx); err != nil {
			t.Fatalf("Start: %v", err)
		}

		// Capture baseline count (ticker-driven poll before wake signal).
		time.Sleep(20 * time.Millisecond)
		before := pollCount.Load()

		// Send wake signal — should trigger an immediate poll.
		wakeCh <- struct{}{}

		// Give the loop time to process.
		time.Sleep(100 * time.Millisecond)

		after := pollCount.Load()
		if after <= before {
			t.Errorf("expected at least one wake-driven poll: before=%d after=%d", before, after)
		}

		stopCtx, stopCancel := context.WithTimeout(context.Background(), time.Second)
		defer stopCancel()
		if err := p.Stop(stopCtx); err != nil {
			t.Fatalf("Stop: %v", err)
		}
	})

	t.Run("nil wake channel uses ticker only", func(t *testing.T) {
		// A poller with nil wake channel should compile and run without panicking.
		p := newKnowledgeHubPoller(knowledgeHubPollerConfig{
			PollInterval: 24 * time.Hour,
		})
		// wake is nil — the select case on nil channel never fires (Go spec).

		ctx, cancel := context.WithCancel(context.Background())
		if err := p.Start(ctx); err != nil {
			cancel()
			t.Fatalf("Start: %v", err)
		}

		// Let it run briefly; it should not panic.
		time.Sleep(20 * time.Millisecond)

		cancel()

		stopCtx, stopCancel := context.WithTimeout(context.Background(), time.Second)
		defer stopCancel()
		if err := p.Stop(stopCtx); err != nil {
			t.Fatalf("Stop: %v", err)
		}
	})
}
