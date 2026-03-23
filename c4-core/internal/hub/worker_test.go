package hub

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// newLongPollTestServer creates a Client suitable for ClaimJobWithWait tests.
// Uses http.DefaultClient (Supabase path uses httpClient.Do, not Transport.RoundTrip).
func newLongPollTestServer(t *testing.T, handler http.Handler) (*Client, *httptest.Server) {
	t.Helper()
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)
	client := &Client{
		baseURL:    ts.URL,
		apiPrefix:  "/v1",
		apiKey:     "test-key",
		teamID:     "test-team",
		httpClient: http.DefaultClient,
	}
	return client, ts
}

// =========================================================================
// ClaimJobWithWait
// =========================================================================

func TestClaimJobWithWait_ZeroWait_DelegatesToClaimJob(t *testing.T) {
	// waitSecs=0 delegates to ClaimJob which uses Supabase RPC.
	mux := http.NewServeMux()
	mux.HandleFunc("/rest/v1/rpc/claim_job", func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, map[string]any{
			"lease_id": "lease-w1",
			"job":      map[string]any{"id": "job-w1", "name": "train", "status": "RUNNING"},
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
	// RPC returns null when no job available.
	mux.HandleFunc("/rest/v1/rpc/claim_job", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`null`))
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
	mux.HandleFunc("/rest/v1/rpc/claim_job", func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, map[string]any{
			"lease_id": "lease-lp",
			"job":      map[string]any{"id": "job-long-poll", "name": "inference", "status": "RUNNING"},
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
		jsonResponse(w, map[string]any{"lease_id": "late", "job": map[string]any{"id": "job-late"}})
	}))
	defer server.Close()

	client := &Client{
		baseURL:    server.URL,
		apiPrefix:  "/v1",
		apiKey:     "test-key",
		teamID:     "test-team",
		httpClient: http.DefaultClient,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// When context is cancelled before response, implementation returns nil, nil, nil.
	job, leaseID, err := client.ClaimJobWithWait(ctx, 0.0, 60)
	if err != nil {
		// Implementation returns nil error when ctx is cancelled — log if different
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
	mux.HandleFunc("/rest/v1/rpc/claim_job", func(w http.ResponseWriter, r *http.Request) {
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

// =========================================================================
// RegisterWorker
// =========================================================================

func TestRegisterWorker_StoresCapabilities(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/rest/v1/rpc/register_worker", func(w http.ResponseWriter, r *http.Request) {
		// Parse the request body to verify capabilities were sent.
		body, _ := io.ReadAll(r.Body)
		var req map[string]any
		json.Unmarshal(body, &req)

		caps, _ := req["p_capabilities"].([]any)
		if len(caps) != 2 {
			http.Error(w, "expected 2 capabilities", http.StatusBadRequest)
			return
		}
		jsonResponse(w, map[string]any{"id": "worker-abc"})
	})
	client, _ := newTestServer(t, mux)

	id, err := client.RegisterWorker(map[string]any{"gpu": true, "python": true})
	if err != nil {
		t.Fatalf("RegisterWorker: %v", err)
	}
	if id != "worker-abc" {
		t.Errorf("worker ID = %q, want worker-abc", id)
	}
	// Capabilities must be stored on client after registration.
	if len(client.capabilities) != 2 {
		t.Errorf("client.capabilities length = %d, want 2", len(client.capabilities))
	}
}

func TestRegisterWorker_EmptyCapabilities(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/rest/v1/rpc/register_worker", func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, map[string]any{"id": "worker-empty"})
	})
	client, _ := newTestServer(t, mux)

	_, err := client.RegisterWorker(map[string]any{})
	if err != nil {
		t.Fatalf("RegisterWorker: %v", err)
	}
	if len(client.capabilities) != 0 {
		t.Errorf("client.capabilities = %v, want empty slice", client.capabilities)
	}
}

// =========================================================================
// ClaimJob
// =========================================================================

func TestClaimJob_SendsRegisteredCapabilities(t *testing.T) {
	var capturedCaps []any
	mux := http.NewServeMux()
	mux.HandleFunc("/rest/v1/rpc/claim_job", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req map[string]any
		json.Unmarshal(body, &req)
		capturedCaps, _ = req["p_capabilities"].([]any)
		jsonResponse(w, map[string]any{
			"lease_id": "lease-cap-1",
			"job":      map[string]any{"id": "job-cap-1", "name": "train", "status": "RUNNING"},
		})
	})
	client, _ := newTestServer(t, mux)
	client.capabilities = []string{"gpu", "python"}

	job, leaseID, err := client.ClaimJob(16.0)
	if err != nil {
		t.Fatalf("ClaimJob: %v", err)
	}
	if job == nil || job.ID != "job-cap-1" {
		t.Errorf("unexpected job: %+v", job)
	}
	if leaseID != "lease-cap-1" {
		t.Errorf("leaseID = %q, want lease-cap-1", leaseID)
	}
	// Verify capabilities were forwarded to the RPC.
	if len(capturedCaps) != 2 {
		t.Errorf("p_capabilities sent = %v, want [gpu python]", capturedCaps)
	}
}

func TestClaimJob_EmptyCapabilitiesWhenNotRegistered(t *testing.T) {
	var capturedCaps []any
	mux := http.NewServeMux()
	mux.HandleFunc("/rest/v1/rpc/claim_job", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req map[string]any
		json.Unmarshal(body, &req)
		capturedCaps, _ = req["p_capabilities"].([]any)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`null`))
	})
	client, _ := newTestServer(t, mux)
	// No capabilities set (nil) — should send empty array, not nil.

	job, leaseID, err := client.ClaimJob(0.0)
	if err != nil {
		t.Fatalf("ClaimJob: %v", err)
	}
	if job != nil {
		t.Errorf("expected nil job, got %+v", job)
	}
	if leaseID != "" {
		t.Errorf("expected empty leaseID, got %q", leaseID)
	}
	// Empty capabilities must be sent as an empty array (not null/nil).
	if capturedCaps == nil {
		t.Errorf("p_capabilities was nil, want empty array []")
	}
}
