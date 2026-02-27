package hub

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// newLongPollTestServer creates a Client with an explicit Transport (required by
// ClaimJobWithWait which calls c.httpClient.Transport.RoundTrip directly).
func newLongPollTestServer(t *testing.T, handler http.Handler) (*Client, *httptest.Server) {
	t.Helper()
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)
	client := &Client{
		baseURL:   ts.URL,
		apiPrefix: "/v1",
		apiKey:    "test-key",
		teamID:    "test-team",
		httpClient: &http.Client{
			Transport: http.DefaultTransport,
		},
	}
	return client, ts
}

// =========================================================================
// ClaimJobWithWait
// =========================================================================

func TestClaimJobWithWait_ZeroWait_DelegatesToClaimJob(t *testing.T) {
	// waitSecs=0 delegates to ClaimJob which uses httpClient.Do (no Transport needed).
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/leases/acquire", func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, ClaimResponse{
			JobID:   "job-w1",
			LeaseID: "lease-w1",
			Job:     Job{ID: "job-w1", Name: "train", Status: "RUNNING"},
		})
	})
	client, _ := newTestServer(t, mux)

	job, leaseID, err := client.ClaimJobWithWait(context.Background(), 16.0, 0)
	if err != nil {
		t.Fatalf("ClaimJobWithWait(waitSecs=0): %v", err)
	}
	if job == nil {
		t.Fatal("expected non-nil job")
	}
	if job.ID != "job-w1" {
		t.Errorf("job.ID = %q, want job-w1", job.ID)
	}
	if leaseID != "lease-w1" {
		t.Errorf("leaseID = %q, want lease-w1", leaseID)
	}
}

func TestClaimJobWithWait_NoJob(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/leases/acquire", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if _, ok := body["wait_seconds"]; !ok {
			t.Error("expected wait_seconds in body")
		}
		jsonResponse(w, ClaimResponse{}) // no job available
	})
	client, _ := newLongPollTestServer(t, mux)

	job, leaseID, err := client.ClaimJobWithWait(context.Background(), 8.0, 5)
	if err != nil {
		t.Fatalf("ClaimJobWithWait: %v", err)
	}
	if job != nil {
		t.Errorf("expected nil job when no job available, got %+v", job)
	}
	if leaseID != "" {
		t.Errorf("expected empty leaseID, got %q", leaseID)
	}
}

func TestClaimJobWithWait_WithJob(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/leases/acquire", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if ws, ok := body["wait_seconds"].(float64); !ok || ws != 30 {
			t.Errorf("expected wait_seconds=30, got %v", body["wait_seconds"])
		}
		jsonResponse(w, ClaimResponse{
			JobID:   "job-long-poll",
			LeaseID: "lease-lp",
			Job:     Job{ID: "job-long-poll", Name: "inference", Status: "RUNNING"},
		})
	})
	client, _ := newLongPollTestServer(t, mux)

	job, leaseID, err := client.ClaimJobWithWait(context.Background(), 0.0, 30)
	if err != nil {
		t.Fatalf("ClaimJobWithWait: %v", err)
	}
	if job == nil || job.ID != "job-long-poll" {
		t.Errorf("unexpected job: %+v", job)
	}
	if leaseID != "lease-lp" {
		t.Errorf("leaseID = %q, want lease-lp", leaseID)
	}
}

func TestClaimJobWithWait_ContextCancelled(t *testing.T) {
	// Server delays response longer than context timeout.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
		jsonResponse(w, ClaimResponse{JobID: "job-late"})
	}))
	defer server.Close()

	client := &Client{
		baseURL:   server.URL,
		apiPrefix: "/v1",
		apiKey:    "test-key",
		teamID:    "test-team",
		httpClient: &http.Client{
			Transport: http.DefaultTransport,
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// When context is cancelled before response, implementation returns nil, nil, nil.
	job, leaseID, err := client.ClaimJobWithWait(ctx, 0.0, 60)
	if err != nil {
		// Implementation returns nil error when reqCtx is cancelled — log if different
		t.Logf("ClaimJobWithWait context cancel: got err=%v (acceptable)", err)
	}
	if job != nil {
		t.Errorf("expected nil job on context cancel, got %+v", job)
	}
	if leaseID != "" {
		t.Errorf("expected empty leaseID on context cancel, got %q", leaseID)
	}
}

func TestClaimJobWithWait_ServerError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/leases/acquire", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	})
	client, _ := newLongPollTestServer(t, mux)

	_, _, err := client.ClaimJobWithWait(context.Background(), 0.0, 5)
	if err == nil {
		t.Fatal("expected error on server error response")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error should mention status code, got: %v", err)
	}
}

// =========================================================================
// GetID (models.go)
// =========================================================================

func TestGetID_PrefersID(t *testing.T) {
	j := &Job{ID: "hub-id", JobID: "daemon-job-id"}
	if got := j.GetID(); got != "hub-id" {
		t.Errorf("GetID() = %q, want hub-id", got)
	}
}

func TestGetID_FallsBackToJobID(t *testing.T) {
	j := &Job{ID: "", JobID: "daemon-job-id"}
	if got := j.GetID(); got != "daemon-job-id" {
		t.Errorf("GetID() = %q, want daemon-job-id", got)
	}
}

func TestGetID_BothEmpty(t *testing.T) {
	j := &Job{}
	if got := j.GetID(); got != "" {
		t.Errorf("GetID() = %q, want empty string", got)
	}
}
