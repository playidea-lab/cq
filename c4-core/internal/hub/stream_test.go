package hub

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
)

// =========================================================================
// wsURL (deterministic unit tests)
// =========================================================================

func TestWsURL_HTTP(t *testing.T) {
	c := &Client{baseURL: "http://hub.example.com:8000", apiPrefix: "/v1"}
	got := c.wsURL("job-1", false)
	want := "ws://hub.example.com:8000/v1/ws/metrics/job-1"
	if got != want {
		t.Errorf("wsURL = %q, want %q", got, want)
	}
}

func TestWsURL_HTTPS(t *testing.T) {
	c := &Client{baseURL: "https://hub.example.com", apiPrefix: "/v1"}
	got := c.wsURL("job-2", true)
	want := "wss://hub.example.com/v1/ws/metrics/job-2?include_history=true"
	if got != want {
		t.Errorf("wsURL = %q, want %q", got, want)
	}
}

// =========================================================================
// IsTerminal (deterministic)
// =========================================================================

func TestIsTerminal(t *testing.T) {
	tests := []struct {
		status string
		want   bool
	}{
		{"SUCCEEDED", true},
		{"FAILED", true},
		{"CANCELLED", true},
		{"RUNNING", false},
		{"QUEUED", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := IsTerminal(tt.status); got != tt.want {
			t.Errorf("IsTerminal(%q) = %v, want %v", tt.status, got, tt.want)
		}
	}
}

// =========================================================================
// StreamMetrics WebSocket integration tests
// Skipped by default. Run with: C4_HUB_INTEGRATION=1 go test ./internal/hub/...
// =========================================================================

func skipUnlessIntegration(t *testing.T) {
	t.Helper()
	if os.Getenv("C4_HUB_INTEGRATION") == "" {
		t.Skip("skipping WebSocket integration test (set C4_HUB_INTEGRATION=1)")
	}
}

func TestStreamMetrics_ReceivesAndStopsOnTerminal(t *testing.T) {
	skipUnlessIntegration(t)

	// Create a WebSocket server that sends metrics then terminal status
	var mu sync.Mutex
	serverDone := make(chan struct{})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/v1/ws/metrics/job-test") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		conn, _, _, err := ws.UpgradeHTTP(r, w)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		defer conn.Close()

		messages := []MetricMessage{
			{Type: "metric", JobID: "job-test", Step: 0, Metrics: map[string]any{"loss": 1.0}},
			{Type: "metric", JobID: "job-test", Step: 1, Metrics: map[string]any{"loss": 0.5}},
			{Type: "status", JobID: "job-test", Status: "SUCCEEDED"},
		}

		for _, msg := range messages {
			mu.Lock()
			data, _ := json.Marshal(msg)
			err := wsutil.WriteServerText(conn, data)
			mu.Unlock()
			if err != nil {
				return
			}
			time.Sleep(50 * time.Millisecond) // stable timing
		}
		close(serverDone)
	}))
	defer server.Close()

	client := &Client{
		baseURL:    server.URL,
		apiKey:     "test",
		teamID:     "test",
		httpClient: http.DefaultClient,
	}

	var received []MetricMessage
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := client.StreamMetrics(ctx, "job-test", false, func(msg MetricMessage) {
		received = append(received, msg)
	})
	if err != nil {
		t.Fatalf("StreamMetrics: %v", err)
	}

	if len(received) < 2 {
		t.Fatalf("received %d messages, want at least 2", len(received))
	}

	// Last message should be terminal status
	last := received[len(received)-1]
	if last.Type != "status" || last.Status != "SUCCEEDED" {
		t.Errorf("last message = %+v, want status=SUCCEEDED", last)
	}
}

// =========================================================================
// isTerminalStatus + isTimeout (non-integration, deterministic)
// =========================================================================

func TestIsTerminalStatus(t *testing.T) {
	tests := []struct {
		status string
		want   bool
	}{
		{"SUCCEEDED", true},
		{"FAILED", true},
		{"CANCELLED", true},
		{"RUNNING", false},
		{"QUEUED", false},
		{"", false},
		{"succeeded", false}, // case-sensitive
	}
	for _, tt := range tests {
		if got := isTerminalStatus(tt.status); got != tt.want {
			t.Errorf("isTerminalStatus(%q) = %v, want %v", tt.status, got, tt.want)
		}
	}
}

func TestIsTimeout_NetError(t *testing.T) {
	// Create a fake net.Error that times out
	ne := &fakeNetError{timeout: true}
	if !isTimeout(ne) {
		t.Error("isTimeout(net.Error{timeout:true}) should be true")
	}
}

func TestIsTimeout_NetErrorNotTimeout(t *testing.T) {
	ne := &fakeNetError{timeout: false}
	if isTimeout(ne) {
		t.Error("isTimeout(net.Error{timeout:false}) should be false")
	}
}

func TestIsTimeout_NonNetError(t *testing.T) {
	err := fmt.Errorf("some generic error")
	if isTimeout(err) {
		t.Error("isTimeout(generic error) should be false")
	}
}

func TestIsTimeout_EOF(t *testing.T) {
	if isTimeout(io.EOF) {
		t.Error("isTimeout(io.EOF) should be false")
	}
}

// fakeNetError implements net.Error for testing.
type fakeNetError struct {
	timeout bool
}

func (e *fakeNetError) Error() string   { return "fake net error" }
func (e *fakeNetError) Timeout() bool   { return e.timeout }
func (e *fakeNetError) Temporary() bool { return false }

// =========================================================================
// StreamMetrics + streamOnce — local WebSocket server tests (no env var)
// =========================================================================

// newWSTestClient creates a Client pointing at ts.URL (http://) with
// apiPrefix="/v1". The Client uses the default http transport.
func newWSTestClient(ts *httptest.Server) *Client {
	return &Client{
		baseURL:    ts.URL,
		apiPrefix:  "/v1",
		apiKey:     "test",
		teamID:     "test",
		httpClient: &http.Client{Transport: http.DefaultTransport},
	}
}

func TestStreamMetrics_StopsOnTerminalStatus(t *testing.T) {
	skipUnlessIntegration(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, _, _, err := ws.UpgradeHTTP(r, w)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		defer conn.Close()

		messages := []MetricMessage{
			{Type: "metric", JobID: "j1", Step: 0, Metrics: map[string]any{"loss": 1.0}},
			{Type: "status", JobID: "j1", Status: "SUCCEEDED"},
		}
		for _, msg := range messages {
			data, _ := json.Marshal(msg)
			if err := wsutil.WriteServerText(conn, data); err != nil {
				return
			}
		}
	}))
	defer server.Close()

	client := newWSTestClient(server)

	var received []MetricMessage
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := client.StreamMetrics(ctx, "j1", false, func(msg MetricMessage) {
		received = append(received, msg)
	})
	if err != nil {
		t.Fatalf("StreamMetrics: %v", err)
	}
	if len(received) < 2 {
		t.Fatalf("received %d messages, want >= 2", len(received))
	}
	last := received[len(received)-1]
	if last.Type != "status" || last.Status != "SUCCEEDED" {
		t.Errorf("last = %+v, want status=SUCCEEDED", last)
	}
}

func TestStreamMetrics_ContextCancelStops(t *testing.T) {
	skipUnlessIntegration(t)
	var mu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, _, _, err := ws.UpgradeHTTP(r, w)
		if err != nil {
			return
		}
		defer conn.Close()

		for i := 0; i < 100; i++ {
			mu.Lock()
			data, _ := json.Marshal(MetricMessage{Type: "metric", Step: i})
			writeErr := wsutil.WriteServerText(conn, data)
			mu.Unlock()
			if writeErr != nil {
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
	}))
	defer server.Close()

	client := newWSTestClient(server)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	count := 0
	err := client.StreamMetrics(ctx, "j-cancel", false, func(msg MetricMessage) {
		count++
	})
	if err == nil {
		t.Error("expected error on context cancel")
	}
	if count == 0 {
		t.Error("expected at least one message before cancel")
	}
}

func TestStreamMetrics_InvalidJSONSkipped(t *testing.T) {
	skipUnlessIntegration(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, _, _, err := ws.UpgradeHTTP(r, w)
		if err != nil {
			return
		}
		defer conn.Close()

		wsutil.WriteServerText(conn, []byte("not-json"))
		data, _ := json.Marshal(MetricMessage{Type: "status", Status: "SUCCEEDED"})
		wsutil.WriteServerText(conn, data)
	}))
	defer server.Close()

	client := newWSTestClient(server)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var received []MetricMessage
	err := client.StreamMetrics(ctx, "j-invalid-json", false, func(msg MetricMessage) {
		received = append(received, msg)
	})
	if err != nil {
		t.Fatalf("StreamMetrics with invalid JSON: %v", err)
	}
	if len(received) != 1 || received[0].Status != "SUCCEEDED" {
		t.Errorf("received = %+v, want [{status:SUCCEEDED}]", received)
	}
}

func TestStreamMetrics_ReconnectsOnDisconnect(t *testing.T) {
	skipUnlessIntegration(t)
	var connectCount int
	var mu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		connectCount++
		count := connectCount
		mu.Unlock()

		conn, _, _, err := ws.UpgradeHTTP(r, w)
		if err != nil {
			return
		}
		defer conn.Close()

		if count == 1 {
			return
		}
		data, _ := json.Marshal(MetricMessage{Type: "status", Status: "SUCCEEDED"})
		wsutil.WriteServerText(conn, data)
	}))
	defer server.Close()

	client := newWSTestClient(server)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	err := client.StreamMetrics(ctx, "j-reconnect", false, func(msg MetricMessage) {})
	if err != nil {
		t.Fatalf("StreamMetrics reconnect: %v", err)
	}

	mu.Lock()
	count := connectCount
	mu.Unlock()
	if count < 2 {
		t.Errorf("expected at least 2 connections (reconnect), got %d", count)
	}
}

func TestStreamMetrics_ContextCancel(t *testing.T) {
	skipUnlessIntegration(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, _, _, err := ws.UpgradeHTTP(r, w)
		if err != nil {
			return
		}
		defer conn.Close()

		for i := 0; i < 100; i++ {
			msg := MetricMessage{Type: "metric", Step: i, Metrics: map[string]any{"loss": float64(100 - i)}}
			data, _ := json.Marshal(msg)
			if err := wsutil.WriteServerText(conn, data); err != nil {
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
	}))
	defer server.Close()

	client := &Client{
		baseURL:    server.URL,
		apiKey:     "test",
		teamID:     "test",
		httpClient: http.DefaultClient,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	received := 0
	err := client.StreamMetrics(ctx, "job-slow", false, func(msg MetricMessage) {
		received++
	})

	if err == nil {
		t.Error("expected error on context cancellation")
	}
	if received < 1 {
		t.Errorf("received %d, expected at least 1 message before cancel", received)
	}
}
