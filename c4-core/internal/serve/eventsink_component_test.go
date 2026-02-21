//go:build c3_eventbus

package serve

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"
)

// testPublisher implements eventbus.Publisher for testing.
type testPublisher struct{}

func (p testPublisher) PublishAsync(evType, source string, data json.RawMessage, projectID string) {}

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("freePort: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return port
}

func TestEventSinkComponent_StartStop(t *testing.T) {
	port := freePort(t)
	comp := NewEventSinkComponent(port, "", testPublisher{})

	if comp.Name() != "eventsink" {
		t.Errorf("Name() = %q, want %q", comp.Name(), "eventsink")
	}

	// Before start, health should report error.
	h := comp.Health()
	if h.Status != "error" {
		t.Errorf("Health before start = %q, want %q", h.Status, "error")
	}

	// Start
	ctx := context.Background()
	if err := comp.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Wait for server to be ready.
	time.Sleep(50 * time.Millisecond)

	// Health should be ok (GET returns 405 which means server is alive).
	h = comp.Health()
	if h.Status != "ok" {
		t.Errorf("Health after start = %q (%s), want %q", h.Status, h.Detail, "ok")
	}

	// Stop
	if err := comp.Stop(ctx); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	// After stop, health should report error.
	h = comp.Health()
	if h.Status != "error" {
		t.Errorf("Health after stop = %q, want %q", h.Status, "error")
	}
}

func TestEventSinkComponent_StopIdempotent(t *testing.T) {
	comp := NewEventSinkComponent(0, "", testPublisher{})

	// Stop without start should not error.
	if err := comp.Stop(context.Background()); err != nil {
		t.Errorf("Stop without start: %v", err)
	}
}

func TestEventSinkComponent_PortConflict(t *testing.T) {
	port := freePort(t)

	// Occupy the port.
	l, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer l.Close()

	comp := NewEventSinkComponent(port, "", testPublisher{})

	// Start should succeed (StartEventSinkServer launches in goroutine),
	// but health should report error since the port is occupied.
	ctx := context.Background()
	_ = comp.Start(ctx)

	time.Sleep(50 * time.Millisecond)

	h := comp.Health()
	if h.Status == "ok" {
		t.Errorf("Health with conflicted port should not be ok")
	}

	_ = comp.Stop(ctx)
}

func TestEventSinkComponent_HealthWithToken(t *testing.T) {
	port := freePort(t)
	comp := NewEventSinkComponent(port, "secret-token", testPublisher{})

	ctx := context.Background()
	if err := comp.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer comp.Stop(ctx)

	time.Sleep(50 * time.Millisecond)

	// Health uses GET (no auth header), handler returns 405 for wrong method.
	// Token check only applies to POST, so GET still returns 405 = alive.
	h := comp.Health()
	if h.Status != "ok" {
		t.Errorf("Health = %q (%s), want %q", h.Status, h.Detail, "ok")
	}
}

func TestEventSinkComponent_IntegrationPost(t *testing.T) {
	port := freePort(t)
	pub := testPublisher{}
	comp := NewEventSinkComponent(port, "", pub)

	ctx := context.Background()
	if err := comp.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer comp.Stop(ctx)

	time.Sleep(50 * time.Millisecond)

	// POST a valid event.
	url := fmt.Sprintf("http://localhost:%d/v1/events/publish", port)
	body, _ := json.Marshal(map[string]any{"event_type": "test.event", "source": "test"})
	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("POST status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}
