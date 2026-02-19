package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/changmin/c4-core/internal/hub"
)

// newPollerTestServer creates a mock Hub server and HubPoller for testing.
func newPollerTestServer(t *testing.T, mux *http.ServeMux) (*hub.Client, *capturePublisher) {
	t.Helper()
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	client := hub.NewClient(hub.HubConfig{URL: ts.URL, APIKey: "test-key"})
	pub := &capturePublisher{}
	return client, pub
}

func TestHubPoller_SucceededTransition(t *testing.T) {
	// First call: job-1 is RUNNING
	// Second call: job-1 is gone from RUNNING list → fetch → SUCCEEDED
	var callCount atomic.Int32

	mux := http.NewServeMux()
	mux.HandleFunc("/jobs", func(w http.ResponseWriter, r *http.Request) {
		n := callCount.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if n == 1 {
			// First poll: job is RUNNING
			json.NewEncoder(w).Encode([]hub.Job{{ID: "job-1", Name: "train", Status: "RUNNING"}})
		} else {
			// Second poll: job no longer running
			json.NewEncoder(w).Encode([]hub.Job{})
		}
	})
	mux.HandleFunc("/jobs/job-1", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(hub.Job{ID: "job-1", Name: "train", Status: "SUCCEEDED"})
	})

	client, pub := newPollerTestServer(t, mux)
	poller := NewHubPoller(client, pub, 10*time.Millisecond)
	poller.SetProjectID("proj-test")

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	poller.Start(ctx)

	// Wait for at least 2 polls
	deadline := time.Now().Add(400 * time.Millisecond)
	for time.Now().Before(deadline) {
		if len(pub.calls) >= 1 {
			break
		}
		time.Sleep(15 * time.Millisecond)
	}

	if len(pub.calls) == 0 {
		t.Fatal("expected at least 1 event published")
	}
	found := false
	for _, c := range pub.calls {
		if c.evType == "hub.job.completed" {
			found = true
			if c.projectID != "proj-test" {
				t.Errorf("projectID = %q, want proj-test", c.projectID)
			}
		}
	}
	if !found {
		t.Errorf("expected hub.job.completed event, got: %v", pub.calls)
	}
}

func TestHubPoller_FailedTransition(t *testing.T) {
	var callCount atomic.Int32

	mux := http.NewServeMux()
	mux.HandleFunc("/jobs", func(w http.ResponseWriter, r *http.Request) {
		n := callCount.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if n == 1 {
			json.NewEncoder(w).Encode([]hub.Job{{ID: "job-f", Name: "fail-job", Status: "RUNNING"}})
		} else {
			json.NewEncoder(w).Encode([]hub.Job{})
		}
	})
	mux.HandleFunc("/jobs/job-f", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(hub.Job{ID: "job-f", Name: "fail-job", Status: "FAILED"})
	})

	client, pub := newPollerTestServer(t, mux)
	poller := NewHubPoller(client, pub, 10*time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	poller.Start(ctx)

	deadline := time.Now().Add(400 * time.Millisecond)
	for time.Now().Before(deadline) {
		if len(pub.calls) >= 1 {
			break
		}
		time.Sleep(15 * time.Millisecond)
	}

	if len(pub.calls) == 0 {
		t.Fatal("expected at least 1 event published")
	}
	found := false
	for _, c := range pub.calls {
		if c.evType == "hub.job.failed" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected hub.job.failed event, got: %v", pub.calls)
	}
}

func TestHubPoller_NoTransitionWhileRunning(t *testing.T) {
	// Job stays RUNNING — no events should be published
	mux := http.NewServeMux()
	mux.HandleFunc("/jobs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]hub.Job{{ID: "job-r", Name: "still-running", Status: "RUNNING"}})
	})

	client, pub := newPollerTestServer(t, mux)
	poller := NewHubPoller(client, pub, 10*time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	poller.Start(ctx)

	<-ctx.Done()
	time.Sleep(20 * time.Millisecond) // let goroutine wind down

	if len(pub.calls) != 0 {
		t.Errorf("expected 0 events for running job, got %d", len(pub.calls))
	}
}

func TestHubPoller_ContextCancel(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/jobs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]hub.Job{})
	})

	client, pub := newPollerTestServer(t, mux)
	poller := NewHubPoller(client, pub, 50*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	poller.Start(ctx)

	time.Sleep(20 * time.Millisecond)
	cancel() // Cancel should stop the goroutine cleanly
	time.Sleep(20 * time.Millisecond)
	// No assertion needed — test passes if it doesn't hang
}

func TestNewHubPoller_DefaultInterval(t *testing.T) {
	pub := &capturePublisher{}
	client := hub.NewClient(hub.HubConfig{URL: "http://localhost:9999"})
	poller := NewHubPoller(client, pub, 0) // 0 → should default to 30s
	if poller.interval != 30*time.Second {
		t.Errorf("interval = %v, want 30s", poller.interval)
	}
}

func TestHubPoller_LastSeenCleanedAfterTerminal(t *testing.T) {
	// Verify that lastSeen entries are removed after a terminal transition
	// so the map does not grow unbounded in long-running scenarios.
	var callCount atomic.Int32

	mux := http.NewServeMux()
	mux.HandleFunc("/jobs", func(w http.ResponseWriter, r *http.Request) {
		n := callCount.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if n == 1 {
			json.NewEncoder(w).Encode([]hub.Job{{ID: "job-cleanup", Name: "train", Status: "RUNNING"}})
		} else {
			json.NewEncoder(w).Encode([]hub.Job{})
		}
	})
	mux.HandleFunc("/jobs/job-cleanup", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(hub.Job{ID: "job-cleanup", Name: "train", Status: "SUCCEEDED"})
	})

	client, pub := newPollerTestServer(t, mux)
	poller := NewHubPoller(client, pub, 10*time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	poller.Start(ctx)

	// Wait until the terminal event is published
	deadline := time.Now().Add(400 * time.Millisecond)
	for time.Now().Before(deadline) {
		if len(pub.calls) >= 1 {
			break
		}
		time.Sleep(15 * time.Millisecond)
	}

	if len(pub.calls) == 0 {
		t.Fatal("expected hub.job.completed event")
	}

	// After terminal event, lastSeen should no longer contain the job entry.
	poller.mu.Lock()
	_, stillPresent := poller.lastSeen["job-cleanup"]
	poller.mu.Unlock()
	if stillPresent {
		t.Error("lastSeen entry not removed after terminal transition (memory leak)")
	}
}

func TestHubPoller_WithMaxJobs(t *testing.T) {
	pub := &capturePublisher{}
	client := hub.NewClient(hub.HubConfig{URL: "http://localhost:9999"})
	poller := NewHubPoller(client, pub, 30*time.Second, WithMaxJobs(50))
	if poller.maxJobs != 50 {
		t.Errorf("maxJobs = %d, want 50", poller.maxJobs)
	}
}

func TestHubPoller_DefaultMaxJobs(t *testing.T) {
	pub := &capturePublisher{}
	client := hub.NewClient(hub.HubConfig{URL: "http://localhost:9999"})
	poller := NewHubPoller(client, pub, 30*time.Second)
	if poller.maxJobs != 200 {
		t.Errorf("maxJobs = %d, want 200", poller.maxJobs)
	}
}
