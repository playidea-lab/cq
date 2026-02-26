package api

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestSSEHandler_BasicStream verifies a client can connect and receive an event.
func TestSSEHandler_BasicStream(t *testing.T) {
	srv := newTestServer(t)

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/v1/events/stream", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("Content-Type = %q, want text/event-stream", ct)
	}

	// Trigger an event.
	go srv.notifyJobAvailable()

	// Read until we see a "data:" line.
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data:") {
			return // success
		}
	}
	t.Fatal("did not receive data line from SSE stream")
}

// TestSSEHandler_Disconnect verifies that goroutines are cleaned up after disconnect.
func TestSSEHandler_Disconnect(t *testing.T) {
	srv := newTestServer(t)

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	ctx, cancel := context.WithCancel(context.Background())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/v1/events/stream", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}

	// Verify subscriber is registered.
	time.Sleep(20 * time.Millisecond)
	count := 0
	srv.sseSubs.Range(func(_, _ any) bool { count++; return true })
	if count != 1 {
		t.Fatalf("expected 1 subscriber, got %d", count)
	}

	// Cancel client context → disconnect.
	cancel()
	resp.Body.Close()

	// Wait for cleanup.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		count = 0
		srv.sseSubs.Range(func(_, _ any) bool { count++; return true })
		if count == 0 {
			return // success
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("subscriber not cleaned up after disconnect; count=%d", count)
}

// TestSSEHandler_SlowConsumer verifies that a slow consumer causes drops, not deadlocks.
func TestSSEHandler_SlowConsumer(t *testing.T) {
	srv := newTestServer(t)

	// Manually register a zero-buffer (well, buffer=1) channel to simulate slow consumer.
	// We fill the channel so the next broadcastSSEEvent call must drop.
	slowCh := make(chan string, 1)
	slowCh <- "existing" // pre-fill
	srv.sseSubs.Store(slowCh, struct{}{})
	defer srv.sseSubs.Delete(slowCh)

	// broadcastSSEEvent should not block even though the channel is full.
	done := make(chan struct{})
	go func() {
		srv.broadcastSSEEvent("test", nil)
		close(done)
	}()

	select {
	case <-done:
		// OK — no deadlock
	case <-time.After(2 * time.Second):
		t.Fatal("broadcastSSEEvent blocked on slow consumer")
	}
}

// TestSSEHandler_MultipleSubscribers verifies that all concurrent subscribers receive events.
func TestSSEHandler_MultipleSubscribers(t *testing.T) {
	srv := newTestServer(t)

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	const numSubs = 2
	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		results []string
	)

	for i := 0; i < numSubs; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			req, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/v1/events/stream", nil)
			if err != nil {
				t.Errorf("NewRequest: %v", err)
				return
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Errorf("Do: %v", err)
				return
			}
			defer resp.Body.Close()

			scanner := bufio.NewScanner(resp.Body)
			for scanner.Scan() {
				line := scanner.Text()
				if strings.HasPrefix(line, "data:") {
					mu.Lock()
					results = append(results, line)
					mu.Unlock()
					return
				}
			}
		}()
	}

	// Wait for all subscribers to connect.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		count := 0
		srv.sseSubs.Range(func(_, _ any) bool { count++; return true })
		if count >= numSubs {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	// Trigger notification.
	srv.notifyJobAvailable()

	// Wait for all goroutines.
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for subscribers")
	}

	mu.Lock()
	got := len(results)
	mu.Unlock()

	if got < numSubs {
		t.Fatalf("got %d data lines, want %d", got, numSubs)
	}
}

// TestSSEHandler_MethodNotAllowed verifies non-GET methods are rejected.
func TestSSEHandler_MethodNotAllowed(t *testing.T) {
	srv := newTestServer(t)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/events/stream", nil)
	srv.handleSSEStream(w, r)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", w.Code)
	}
}

// TestSSE_Keepalive verifies that the SSE handler sends keepalive comments
// at a regular interval even when no events are being broadcast.
func TestSSE_Keepalive(t *testing.T) {
	srv := newTestServer(t)

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/v1/events/stream", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	// Read lines until we see a keepalive comment (": keepalive").
	// The ticker fires every 15s, so we should see one within ~16s.
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == ": keepalive" {
			return // success
		}
	}
	if err := scanner.Err(); err != nil && ctx.Err() != nil {
		t.Fatalf("timed out waiting for keepalive comment")
	}
	t.Fatal("stream ended without receiving keepalive comment")
}
