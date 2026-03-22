package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// TestMetricWriter_ParseAndForward verifies that @key=value lines are parsed
// and forwarded to the collector, and that the underlying writer receives all bytes.
func TestMetricWriter_ParseAndForward(t *testing.T) {
	var mu sync.Mutex
	var received []checkpointBody

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body checkpointBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		mu.Lock()
		received = append(received, body)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	var underlying bytes.Buffer
	mw := NewMetricWriter(&underlying, "exp123", ts.URL, "anon-key", "jwt-token")

	lines := []string{
		"Epoch 1 training\n",
		"@loss=0.5 @acc=0.9\n",
		"@mpjpe=42.1\n",
		"no metrics here\n",
	}
	for _, line := range lines {
		if _, err := io.WriteString(mw, line); err != nil {
			t.Fatalf("Write: %v", err)
		}
	}
	mw.Close()

	// All bytes must pass through to the underlying writer.
	want := ""
	for _, l := range lines {
		want += l
	}
	if underlying.String() != want {
		t.Errorf("underlying writer got %q; want %q", underlying.String(), want)
	}

	// We expect 3 metrics: loss, acc, mpjpe.
	mu.Lock()
	defer mu.Unlock()
	if len(received) != 3 {
		t.Errorf("got %d checkpoints; want 3 (got: %+v)", len(received), received)
	}
	for _, cp := range received {
		if cp.RunID != "exp123" {
			t.Errorf("run_id = %q; want %q", cp.RunID, "exp123")
		}
	}
}

// TestMetricWriter_NoMetrics verifies that lines without @key=value produce no RPC calls.
func TestMetricWriter_NoMetrics(t *testing.T) {
	called := false
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	var underlying bytes.Buffer
	mw := NewMetricWriter(&underlying, "exp456", ts.URL, "", "")
	io.WriteString(mw, "no metrics in this line\n")
	io.WriteString(mw, "training complete\n")
	mw.Close()

	if called {
		t.Error("RPC should not be called when no @key=value patterns found")
	}
}

// TestMetricWriter_CircuitBreaker verifies that after 3 consecutive failures
// the MetricWriter stops sending requests.
func TestMetricWriter_CircuitBreaker(t *testing.T) {
	var callCount int
	var mu sync.Mutex

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		callCount++
		mu.Unlock()
		// Always return error to trigger circuit breaker.
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer ts.Close()

	var underlying bytes.Buffer
	mw := NewMetricWriter(&underlying, "exp789", ts.URL, "", "")

	// Send 6 metrics — circuit breaker should trip after 3 failures.
	for i := range 6 {
		io.WriteString(mw, "@loss="+string(rune('0'+i))+".0\n")
	}
	mw.Close()

	mu.Lock()
	got := callCount
	mu.Unlock()

	// Exactly 3 calls should have been made before circuit breaker disabled further calls.
	if got != 3 {
		t.Errorf("call count = %d; want 3 (circuit breaker should stop at 3 failures)", got)
	}
}

// TestMetricWriter_Close_Idempotent ensures Close() can be called multiple times safely.
func TestMetricWriter_Close_Idempotent(t *testing.T) {
	var underlying bytes.Buffer
	mw := NewMetricWriter(&underlying, "exp-close", "http://localhost:9999", "", "")
	mw.Close()
	mw.Close() // second call must not panic
}

// TestMetricWriter_UnderlyingReceivesPartialWrite verifies bytes written without
// a trailing newline are still passed to the underlying writer.
func TestMetricWriter_UnderlyingReceivesPartialWrite(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	var underlying bytes.Buffer
	mw := NewMetricWriter(&underlying, "exp-partial", ts.URL, "", "")
	io.WriteString(mw, "partial line without newline")
	mw.Close()

	if underlying.String() != "partial line without newline" {
		t.Errorf("underlying = %q; want %q", underlying.String(), "partial line without newline")
	}
}

// TestPostCheckpoint_Headers verifies that apikey and Authorization headers are set.
func TestPostCheckpoint_Headers(t *testing.T) {
	var gotAPIKey, gotAuth string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAPIKey = r.Header.Get("apikey")
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	client := &http.Client{Timeout: 2 * time.Second}
	body := checkpointBody{RunID: "r1", Metric: 1.0, Path: "loss"}
	if err := postCheckpoint(client, ts.URL, "anon-key", "user-jwt", body); err != nil {
		t.Fatalf("postCheckpoint: %v", err)
	}

	if gotAPIKey != "anon-key" {
		t.Errorf("apikey header = %q; want %q", gotAPIKey, "anon-key")
	}
	if gotAuth != "Bearer user-jwt" {
		t.Errorf("Authorization header = %q; want %q", gotAuth, "Bearer user-jwt")
	}
}
