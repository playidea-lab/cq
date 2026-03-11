//go:build c5_hub && c3_eventbus

package serve

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// capturingPublisher records PublishAsync calls.
type capturingPublisher struct {
	calls atomic.Int64
}

func (p *capturingPublisher) PublishAsync(evType, source string, data json.RawMessage, projectID string) {
	p.calls.Add(1)
}

// sseTestBroker manages SSE subscriber channels for the test server.
// It is fully thread-safe.
type sseTestBroker struct {
	mu   sync.Mutex
	subs []chan string
}

func (b *sseTestBroker) add(ch chan string) {
	b.mu.Lock()
	b.subs = append(b.subs, ch)
	b.mu.Unlock()
}

func (b *sseTestBroker) broadcast(msg string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, ch := range b.subs {
		select {
		case ch <- msg:
		default:
		}
	}
}

// newSSETestServer returns a test HTTP server that streams SSE events.
// The returned sendFn pushes a "data: <msg>" line to all connected clients.
// closeServer stops the test server.
func newSSETestServer(t *testing.T) (url string, sendFn func(msg string), closeServer func()) {
	t.Helper()

	broker := &sseTestBroker{}

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/events/stream", func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "no flush", http.StatusInternalServerError)
			return
		}
		ch := make(chan string, 16)
		broker.add(ch)

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher.Flush()

		for {
			select {
			case <-r.Context().Done():
				return
			case msg, ok := <-ch:
				if !ok {
					return
				}
				fmt.Fprintf(w, "data: %s\n\n", msg)
				flusher.Flush()
			}
		}
	})

	ts := httptest.NewServer(mux)

	sendFn = func(msg string) {
		broker.broadcast(msg)
	}

	closeServer = func() {
		// CloseClientConnections forces active SSE connections to drop,
		// then Close waits for the server to finish.
		ts.CloseClientConnections()
		ts.Close()
	}

	return ts.URL, sendFn, closeServer
}

// TestSSESubscriberComponent_Start verifies that a healthy connection sets Health=ok.
func TestSSESubscriberComponent_Start(t *testing.T) {
	tsURL, _, closeSrv := newSSETestServer(t)
	defer closeSrv()

	pub := &capturingPublisher{}
	comp := NewSSESubscriberComponent(SSESubscriberConfig{URL: tsURL}, pub)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := comp.Start(ctx); err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	defer comp.Stop(context.Background())

	// Give goroutine time to connect.
	time.Sleep(100 * time.Millisecond)

	h := comp.Health()
	if h.Status != "ok" {
		t.Errorf("Health().Status = %q, want %q; detail: %s", h.Status, "ok", h.Detail)
	}
}

// TestSSESubscriberComponent_Reconnect verifies that the component reconnects after server restart.
func TestSSESubscriberComponent_Reconnect(t *testing.T) {
	// Start first server.
	tsURL, _, closeSrv := newSSETestServer(t)

	pub := &capturingPublisher{}
	comp := NewSSESubscriberComponent(SSESubscriberConfig{URL: tsURL}, pub)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := comp.Start(ctx); err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	defer comp.Stop(context.Background())

	// Let it connect, then kill server.
	time.Sleep(100 * time.Millisecond)
	closeSrv()

	// failCount should increase after disconnect.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		comp.mu.Lock()
		fc := comp.failCount
		comp.mu.Unlock()
		if fc > 0 {
			return // reconnect attempt observed
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("expected failCount > 0 after server shutdown")
}

// TestSSESubscriberComponent_Stop verifies that Stop() terminates the background goroutine.
func TestSSESubscriberComponent_Stop(t *testing.T) {
	tsURL, _, closeSrv := newSSETestServer(t)
	defer closeSrv()

	pub := &capturingPublisher{}
	comp := NewSSESubscriberComponent(SSESubscriberConfig{URL: tsURL}, pub)

	ctx := context.Background()
	if err := comp.Start(ctx); err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	done := make(chan struct{})
	go func() {
		comp.Stop(ctx)
		close(done)
	}()

	select {
	case <-done:
		// OK
	case <-time.After(3 * time.Second):
		t.Fatal("Stop() did not return in time")
	}

	h := comp.Health()
	if h.Status != "stopped" {
		t.Errorf("Health().Status after Stop = %q, want %q", h.Status, "stopped")
	}
}

// TestSSESubscriberComponent_DegradedAfter3Failures verifies degraded health after 3 failures.
func TestSSESubscriberComponent_DegradedAfter3Failures(t *testing.T) {
	// Use a URL that will never connect.
	comp := NewSSESubscriberComponent(SSESubscriberConfig{URL: "http://127.0.0.1:1"}, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := comp.Start(ctx); err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	defer comp.Stop(context.Background())

	// Manually set failCount to trigger degraded.
	comp.mu.Lock()
	comp.failCount = sseDegradedAfter
	comp.mu.Unlock()

	h := comp.Health()
	if h.Status != "degraded" {
		t.Errorf("Health().Status = %q, want %q; detail: %s", h.Status, "degraded", h.Detail)
	}
	if !strings.Contains(h.Detail, "reconnect failed") {
		t.Errorf("Health().Detail = %q, want 'reconnect failed...'", h.Detail)
	}
}

// TestSSESubscriberComponent_ImplementsComponent verifies interface compliance.
func TestSSESubscriberComponent_ImplementsComponent(t *testing.T) {
	var _ Component = (*SSESubscriberComponent)(nil)
}

// TestSSESubscriberComponent_StartNoURL verifies error when URL is empty.
func TestSSESubscriberComponent_StartNoURL(t *testing.T) {
	comp := NewSSESubscriberComponent(SSESubscriberConfig{}, nil)
	err := comp.Start(context.Background())
	if err == nil {
		t.Fatal("expected error when URL is empty")
	}
}

// TestSSESubscriberComponent_PublishEvent verifies events are forwarded to the publisher.
func TestSSESubscriberComponent_PublishEvent(t *testing.T) {
	pub := &capturingPublisher{}
	comp := NewSSESubscriberComponent(SSESubscriberConfig{URL: "http://x", ProjectID: "proj"}, pub)

	// Call publishEvent directly.
	comp.publishEvent(`{"type":"job.available"}`)

	if pub.calls.Load() != 1 {
		t.Errorf("expected 1 publish call, got %d", pub.calls.Load())
	}
}

// TestSSESubscriberComponent_PublishEventNilPublisher verifies nil publisher doesn't panic.
func TestSSESubscriberComponent_PublishEventNilPublisher(t *testing.T) {
	comp := NewSSESubscriberComponent(SSESubscriberConfig{URL: "http://x"}, nil)
	// Should not panic.
	comp.publishEvent(`{"type":"test"}`)
}

// TestSSESubscriberWake verifies that a hub.job.completed payload sends a signal
// on the wake channel, and that hub.job.failed also triggers a wake signal.
// Non-job events must NOT send a wake signal.
func TestSSESubscriberWake(t *testing.T) {
	pub := &capturingPublisher{}
	comp := NewSSESubscriberComponent(SSESubscriberConfig{URL: "http://x", ProjectID: "p"}, pub)

	wakeCh := make(chan struct{}, 1)
	comp.SetWakeChannel(wakeCh)

	// hub.job.completed should wake.
	comp.publishEvent(`{"type":"hub.job.completed","job_id":"j1"}`)
	select {
	case <-wakeCh:
		// OK
	case <-time.After(100 * time.Millisecond):
		t.Error("expected wake signal for hub.job.completed")
	}

	// hub.job.failed should wake.
	comp.publishEvent(`{"type":"hub.job.failed","job_id":"j2"}`)
	select {
	case <-wakeCh:
		// OK
	case <-time.After(100 * time.Millisecond):
		t.Error("expected wake signal for hub.job.failed")
	}

	// A non-job event must NOT send a wake signal.
	comp.publishEvent(`{"type":"hub.job.started","job_id":"j3"}`)
	select {
	case <-wakeCh:
		t.Error("unexpected wake signal for hub.job.started")
	case <-time.After(50 * time.Millisecond):
		// OK: no signal
	}
}

// TestSSESubscriberComponent_EventForwarding verifies end-to-end: server sends event → publisher called.
func TestSSESubscriberComponent_EventForwarding(t *testing.T) {
	tsURL, sendFn, closeSrv := newSSETestServer(t)
	defer closeSrv()

	pub := &capturingPublisher{}
	comp := NewSSESubscriberComponent(SSESubscriberConfig{URL: tsURL}, pub)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := comp.Start(ctx); err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	defer comp.Stop(context.Background())

	// Wait for connection.
	time.Sleep(100 * time.Millisecond)

	// Send an event from the server.
	sendFn(`{"type":"job.available","data":null}`)

	// Wait for the event to be forwarded.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if pub.calls.Load() > 0 {
			return // success
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("publisher was not called within timeout")
}
