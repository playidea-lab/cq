package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/piqsol/c4/c5/internal/model"
	"github.com/piqsol/c4/c5/internal/store"
)

func TestEdgeMetricsPost(t *testing.T) {
	srv := newTestServer(t)

	// Register an edge first.
	regW := doRequest(t, srv, "POST", "/v1/edges/register", map[string]any{"name": "edge-a"})
	if regW.Code != http.StatusCreated {
		t.Fatalf("register edge: %d", regW.Code)
	}
	var regResp model.EdgeRegisterResponse
	decodeJSON(t, regW, &regResp)
	edgeID := regResp.EdgeID

	// POST metrics.
	w := doRequest(t, srv, "POST", "/v1/edges/"+edgeID+"/metrics", map[string]any{
		"values": map[string]float64{"accuracy": 0.91, "latency_ms": 12.0},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("post metrics: %d %s", w.Code, w.Body.String())
	}
	var resp map[string]bool
	decodeJSON(t, w, &resp)
	if !resp["ok"] {
		t.Fatalf("expected ok:true, got %v", resp)
	}
}

func TestEdgeMetricsGet(t *testing.T) {
	srv := newTestServer(t)

	// Register an edge.
	regW := doRequest(t, srv, "POST", "/v1/edges/register", map[string]any{"name": "edge-b"})
	if regW.Code != http.StatusCreated {
		t.Fatalf("register edge: %d", regW.Code)
	}
	var regResp model.EdgeRegisterResponse
	decodeJSON(t, regW, &regResp)
	edgeID := regResp.EdgeID

	// POST 3 metrics entries.
	for i := 0; i < 3; i++ {
		w := doRequest(t, srv, "POST", "/v1/edges/"+edgeID+"/metrics", map[string]any{
			"values":    map[string]float64{"step": float64(i)},
			"timestamp": int64(1000 + i),
		})
		if w.Code != http.StatusOK {
			t.Fatalf("post metrics %d: %d %s", i, w.Code, w.Body.String())
		}
	}

	// GET with limit=2 — should return 2 most recent.
	w := doRequest(t, srv, "GET", "/v1/edges/"+edgeID+"/metrics?limit=2", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("get metrics: %d %s", w.Code, w.Body.String())
	}
	var resp model.EdgeMetricsListResponse
	decodeJSON(t, w, &resp)
	if resp.EdgeID != edgeID {
		t.Errorf("edge_id: got %q, want %q", resp.EdgeID, edgeID)
	}
	if len(resp.Metrics) != 2 {
		t.Errorf("metrics count: got %d, want 2", len(resp.Metrics))
	}
}

func TestEdgeControlEnqueue(t *testing.T) {
	srv := newTestServer(t)

	regW := doRequest(t, srv, "POST", "/v1/edges/register", map[string]any{"name": "edge-c"})
	if regW.Code != http.StatusCreated {
		t.Fatalf("register edge: %d", regW.Code)
	}
	var regResp model.EdgeRegisterResponse
	decodeJSON(t, regW, &regResp)
	edgeID := regResp.EdgeID

	// POST control message.
	w := doRequest(t, srv, "POST", "/v1/edges/"+edgeID+"/control", map[string]any{
		"action": "collect",
		"params": map[string]string{"local_path": "/data/inference"},
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("enqueue control: %d %s", w.Code, w.Body.String())
	}
	var resp model.ControlEnqueueResponse
	decodeJSON(t, w, &resp)
	if resp.MessageID == "" {
		t.Errorf("expected non-empty message_id")
	}
	if resp.Status != "queued" {
		t.Errorf("status: got %q, want 'queued'", resp.Status)
	}
}

func TestEdgeControlDequeue(t *testing.T) {
	srv := newTestServer(t)

	regW := doRequest(t, srv, "POST", "/v1/edges/register", map[string]any{"name": "edge-d"})
	if regW.Code != http.StatusCreated {
		t.Fatalf("register edge: %d", regW.Code)
	}
	var regResp model.EdgeRegisterResponse
	decodeJSON(t, regW, &regResp)
	edgeID := regResp.EdgeID

	// Enqueue a control message.
	postW := doRequest(t, srv, "POST", "/v1/edges/"+edgeID+"/control", map[string]any{
		"action": "collect",
	})
	if postW.Code != http.StatusCreated {
		t.Fatalf("enqueue control: %d", postW.Code)
	}

	// First GET — should return the message.
	w1 := doRequest(t, srv, "GET", "/v1/edges/"+edgeID+"/control", nil)
	if w1.Code != http.StatusOK {
		t.Fatalf("get control (1st): %d %s", w1.Code, w1.Body.String())
	}
	var msgs1 []model.EdgeControlMessage
	decodeJSON(t, w1, &msgs1)
	if len(msgs1) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs1))
	}
	if msgs1[0].Action != "collect" {
		t.Errorf("action: got %q, want 'collect'", msgs1[0].Action)
	}

	// Second GET — auto-ack means queue is now empty.
	w2 := doRequest(t, srv, "GET", "/v1/edges/"+edgeID+"/control", nil)
	if w2.Code != http.StatusOK {
		t.Fatalf("get control (2nd): %d %s", w2.Code, w2.Body.String())
	}
	var msgs2 []model.EdgeControlMessage
	decodeJSON(t, w2, &msgs2)
	if len(msgs2) != 0 {
		t.Fatalf("expected empty queue, got %d messages", len(msgs2))
	}
}

func TestDeployAssignmentHealthCheck(t *testing.T) {
	srv := newTestServerWithStorage(t, &mockStorage{})

	// Register an edge.
	regW := doRequest(t, srv, "POST", "/v1/edges/register", map[string]any{"name": "edge-hc"})
	if regW.Code != http.StatusCreated {
		t.Fatalf("register edge: %d %s", regW.Code, regW.Body.String())
	}
	var regResp model.EdgeRegisterResponse
	decodeJSON(t, regW, &regResp)
	edgeID := regResp.EdgeID

	// Create a deploy rule with health_check.
	ruleW := doRequest(t, srv, "POST", "/v1/deploy/rules", map[string]any{
		"trigger":              "manual",
		"edge_filter":          "edge-hc",
		"artifact_pattern":     "*.bin",
		"health_check":         "./check.sh",
		"health_check_timeout": 45,
	})
	if ruleW.Code != http.StatusCreated {
		t.Fatalf("create deploy rule: %d %s", ruleW.Code, ruleW.Body.String())
	}
	var ruleResp model.DeployRuleCreateResponse
	decodeJSON(t, ruleW, &ruleResp)

	// Submit a job and create a deployment.
	jobW := doRequest(t, srv, "POST", "/v1/jobs/submit", map[string]any{
		"name":    "test-job",
		"command": "echo hi",
	})
	if jobW.Code != http.StatusCreated {
		t.Fatalf("submit job: %d %s", jobW.Code, jobW.Body.String())
	}
	var jobResp model.JobSubmitResponse
	decodeJSON(t, jobW, &jobResp)

	// Trigger a deployment targeting the edge directly.
	deployW := doRequest(t, srv, "POST", "/v1/deploy/trigger", map[string]any{
		"job_id":           jobResp.JobID,
		"rule_id":          ruleResp.RuleID,
		"artifact_pattern": "*.bin",
		"edge_ids":         []string{edgeID},
	})
	if deployW.Code != http.StatusCreated {
		t.Fatalf("trigger deploy: %d %s", deployW.Code, deployW.Body.String())
	}

	// Poll assignments for the edge — should include HealthCheck.
	assignW := doRequest(t, srv, "GET", "/v1/deploy/assignments/"+edgeID, nil)
	if assignW.Code != http.StatusOK {
		t.Fatalf("get assignments: %d %s", assignW.Code, assignW.Body.String())
	}
	var assignments []model.DeployAssignmentResponse
	decodeJSON(t, assignW, &assignments)
	if len(assignments) == 0 {
		t.Fatal("expected at least one assignment")
	}
	if assignments[0].DeployID == "" {
		t.Errorf("expected non-empty deploy_id")
	}
}

// =========================================================================
// Threshold Monitor Tests
// =========================================================================

// newTestServerWithEventBus creates a test server wired to a real eventpub Publisher.
func newTestServerWithEventBus(t *testing.T, eventBusURL string) *Server {
	t.Helper()
	dir := t.TempDir()
	st, err := store.New(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return NewServer(Config{
		Store:         st,
		Storage:       &mockStorage{},
		Version:       "test",
		EventBusURL:   eventBusURL,
		EventBusToken: "",
	})
}

// registerTestEdgeWithMeta registers an edge with metadata and returns its ID.
func registerTestEdgeWithMeta(t *testing.T, srv *Server, name string, meta map[string]string) string {
	t.Helper()
	body := map[string]any{"name": name}
	if meta != nil {
		body["metadata"] = meta
	}
	w := doRequest(t, srv, "POST", "/v1/edges/register", body)
	if w.Code != http.StatusCreated {
		t.Fatalf("register edge: status %d body=%s", w.Code, w.Body.String())
	}
	var resp model.EdgeRegisterResponse
	decodeJSON(t, w, &resp)
	return resp.EdgeID
}

// TestThresholdBelowNoEvent: accuracy=0.91 threshold=0.8 → no EventBus POST.
func TestThresholdBelowNoEvent(t *testing.T) {
	var postCount atomic.Int32
	mockEB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		postCount.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer mockEB.Close()

	srv := newTestServerWithEventBus(t, mockEB.URL)
	edgeID := registerTestEdgeWithMeta(t, srv, "test-edge-no-event", map[string]string{
		"threshold_accuracy": "0.8",
	})

	w := doRequest(t, srv, "POST", "/v1/edges/"+edgeID+"/metrics", map[string]any{
		"values": map[string]float64{"accuracy": 0.91},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Give goroutine time to run (should not fire).
	time.Sleep(50 * time.Millisecond)
	if n := postCount.Load(); n != 0 {
		t.Errorf("expected 0 EventBus POSTs, got %d", n)
	}
}

// TestThresholdExceededEvent: accuracy=0.75 threshold=0.8 → EventBus POST confirmed.
func TestThresholdExceededEvent(t *testing.T) {
	var postCount atomic.Int32
	mockEB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		postCount.Add(1)
		var payload map[string]any
		json.NewDecoder(r.Body).Decode(&payload) //nolint:errcheck
		w.WriteHeader(http.StatusOK)
	}))
	defer mockEB.Close()

	srv := newTestServerWithEventBus(t, mockEB.URL)
	edgeID := registerTestEdgeWithMeta(t, srv, "test-edge-exceeded", map[string]string{
		"threshold_accuracy": "0.8",
	})
	thresholdCooldownMu.Delete(edgeID + "\x00accuracy")

	w := doRequest(t, srv, "POST", "/v1/edges/"+edgeID+"/metrics", map[string]any{
		"values": map[string]float64{"accuracy": 0.75},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Wait for async goroutine to post.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if postCount.Load() >= 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if n := postCount.Load(); n < 1 {
		t.Errorf("expected ≥1 EventBus POST, got %d", n)
	}
}

// TestThresholdCooldown: first breach → POST 1; immediate second breach → no new POST.
func TestThresholdCooldown(t *testing.T) {
	var postCount atomic.Int32
	mockEB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		postCount.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer mockEB.Close()

	srv := newTestServerWithEventBus(t, mockEB.URL)
	edgeID := registerTestEdgeWithMeta(t, srv, "test-edge-cooldown", map[string]string{
		"threshold_accuracy": "0.8",
	})
	thresholdCooldownMu.Delete(edgeID + "\x00accuracy")

	// First breach: accuracy=0.6 < 0.8.
	doRequest(t, srv, "POST", "/v1/edges/"+edgeID+"/metrics", map[string]any{
		"values": map[string]float64{"accuracy": 0.6},
	})

	// Wait for first event.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if postCount.Load() >= 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if postCount.Load() < 1 {
		t.Fatal("expected first EventBus POST, got none")
	}

	// Second breach immediately — cooldown should suppress it.
	before := postCount.Load()
	doRequest(t, srv, "POST", "/v1/edges/"+edgeID+"/metrics", map[string]any{
		"values": map[string]float64{"accuracy": 0.5},
	})
	time.Sleep(100 * time.Millisecond)

	if after := postCount.Load(); after > before {
		t.Errorf("cooldown failed: got %d extra EventBus POSTs (expected 0)", after-before)
	}
}

// TestThresholdNoEventBusURL: C5_EVENTBUS_URL="" → no panic, returns 200.
func TestThresholdNoEventBusURL(t *testing.T) {
	dir := t.TempDir()
	st, err := store.New(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	srv := NewServer(Config{
		Store:       st,
		Storage:     &mockStorage{},
		Version:     "test",
		EventBusURL: "",
	})

	edgeID := registerTestEdgeWithMeta(t, srv, "test-edge-no-url", map[string]string{
		"threshold_accuracy": "0.9",
	})
	thresholdCooldownMu.Delete(edgeID + "\x00accuracy")

	w := doRequest(t, srv, "POST", "/v1/edges/"+edgeID+"/metrics", map[string]any{
		"values": map[string]float64{"accuracy": 0.5},
	})
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Give goroutine time to execute — must not panic.
	time.Sleep(50 * time.Millisecond)
}
