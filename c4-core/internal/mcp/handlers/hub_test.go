//go:build c5_hub

package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/changmin/c4-core/internal/hub"
	"github.com/changmin/c4-core/internal/mcp"
)

// newHubTestServer creates a mock Hub API server and returns a hub.Client + registry.
// APIPrefix is empty so paths like "/jobs/submit" are used directly.
func newHubTestServer(t *testing.T, mux *http.ServeMux) (*hub.Client, *mcp.Registry) {
	t.Helper()
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	client := hub.NewClient(hub.HubConfig{
		URL:    ts.URL,
		APIKey: "test-key",
		TeamID: "test-team",
	})

	reg := mcp.NewRegistry()
	RegisterHubHandlers(reg, client)
	return client, reg
}

func hubJSON(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

// =========================================================================
// Hub Job Tests (10 handlers)
// =========================================================================

func TestHubSubmit(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/jobs/submit", func(w http.ResponseWriter, r *http.Request) {
		hubJSON(w, map[string]any{"job_id": "job-123", "status": "QUEUED", "queue_position": 1})
	})
	_, reg := newHubTestServer(t, mux)

	result, err := reg.Call("c4_hub_submit", json.RawMessage(`{
		"name": "train-resnet", "workdir": "/workspace", "command": "python train.py"
	}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]any)
	if m["job_id"] != "job-123" {
		t.Errorf("job_id = %v, want job-123", m["job_id"])
	}
	if m["status"] != "QUEUED" {
		t.Errorf("status = %v, want QUEUED", m["status"])
	}
}

func TestHubSubmit_WithAllOptions(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/jobs/submit", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		hubJSON(w, map[string]any{"job_id": "job-opt", "status": "QUEUED", "queue_position": 0})
	})
	_, reg := newHubTestServer(t, mux)

	result, err := reg.Call("c4_hub_submit", json.RawMessage(`{
		"name": "full", "workdir": "/ws", "command": "run.sh",
		"env": {"CUDA": "1"}, "tags": ["gpu"], "requires_gpu": false,
		"priority": 10, "exp_id": "exp-1", "memo": "test", "timeout_sec": 600
	}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]any)
	if m["job_id"] != "job-opt" {
		t.Errorf("job_id = %v, want job-opt", m["job_id"])
	}
}

func TestHubSubmit_MissingRequired(t *testing.T) {
	mux := http.NewServeMux()
	_, reg := newHubTestServer(t, mux)
	_, err := reg.Call("c4_hub_submit", json.RawMessage(`{"name": "test"}`))
	if err == nil {
		t.Fatal("expected error for missing workdir/command")
	}
}

func TestHubSubmit_InvalidJSON(t *testing.T) {
	mux := http.NewServeMux()
	_, reg := newHubTestServer(t, mux)
	_, err := reg.Call("c4_hub_submit", json.RawMessage(`{invalid`))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestHubStatus(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/jobs/job-123", func(w http.ResponseWriter, r *http.Request) {
		hubJSON(w, map[string]any{"id": "job-123", "name": "train", "status": "RUNNING"})
	})
	_, reg := newHubTestServer(t, mux)

	result, err := reg.Call("c4_hub_status", json.RawMessage(`{"job_id": "job-123"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	job := result.(*hub.Job)
	if job.Status != "RUNNING" {
		t.Errorf("status = %q, want RUNNING", job.Status)
	}
}

func TestHubStatus_MissingJobID(t *testing.T) {
	mux := http.NewServeMux()
	_, reg := newHubTestServer(t, mux)
	_, err := reg.Call("c4_hub_status", json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error for missing job_id")
	}
}

func TestHubList(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/jobs", func(w http.ResponseWriter, r *http.Request) {
		hubJSON(w, []map[string]any{
			{"id": "j-1", "name": "train", "status": "QUEUED"},
			{"id": "j-2", "name": "eval", "status": "RUNNING"},
		})
	})
	_, reg := newHubTestServer(t, mux)

	result, err := reg.Call("c4_hub_list", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]any)
	jobs := m["jobs"].([]hub.Job)
	if len(jobs) != 2 {
		t.Errorf("count = %d, want 2", len(jobs))
	}
}

func TestHubList_WithFilter(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/jobs", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("status") != "RUNNING" {
			hubJSON(w, []map[string]any{})
			return
		}
		hubJSON(w, []map[string]any{{"id": "j-2", "status": "RUNNING"}})
	})
	_, reg := newHubTestServer(t, mux)

	result, err := reg.Call("c4_hub_list", json.RawMessage(`{"status": "RUNNING", "limit": 10}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]any)
	jobs := m["jobs"].([]hub.Job)
	if len(jobs) != 1 {
		t.Errorf("count = %d, want 1", len(jobs))
	}
}

func TestHubCancel(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/jobs/job-c/cancel", func(w http.ResponseWriter, r *http.Request) {
		hubJSON(w, map[string]any{"cancelled": true})
	})
	_, reg := newHubTestServer(t, mux)

	result, err := reg.Call("c4_hub_cancel", json.RawMessage(`{"job_id": "job-c"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]any)
	if m["cancelled"] != true {
		t.Errorf("cancelled = %v, want true", m["cancelled"])
	}
	if m["job_id"] != "job-c" {
		t.Errorf("job_id = %v, want job-c", m["job_id"])
	}
}

func TestHubCancel_MissingJobID(t *testing.T) {
	mux := http.NewServeMux()
	_, reg := newHubTestServer(t, mux)
	_, err := reg.Call("c4_hub_cancel", json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error for missing job_id")
	}
}

func TestHubMetrics(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/metrics/job-m", func(w http.ResponseWriter, r *http.Request) {
		hubJSON(w, hub.MetricsResponse{JobID: "job-m", TotalSteps: 10})
	})
	_, reg := newHubTestServer(t, mux)

	result, err := reg.Call("c4_hub_metrics", json.RawMessage(`{"job_id": "job-m"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]any)
	if m["job_id"] != "job-m" {
		t.Errorf("job_id = %v, want job-m", m["job_id"])
	}
	if m["total_steps"] != 10 {
		t.Errorf("total_steps = %v, want 10", m["total_steps"])
	}
}

func TestHubMetrics_MissingJobID(t *testing.T) {
	mux := http.NewServeMux()
	_, reg := newHubTestServer(t, mux)
	_, err := reg.Call("c4_hub_metrics", json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error for missing job_id")
	}
}

func TestHubLogMetrics(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/metrics/job-lm", func(w http.ResponseWriter, r *http.Request) {
		hubJSON(w, map[string]any{"logged": true})
	})
	_, reg := newHubTestServer(t, mux)

	result, err := reg.Call("c4_hub_log_metrics", json.RawMessage(`{
		"job_id": "job-lm", "step": 5, "metrics": {"loss": 0.3}
	}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]any)
	if m["logged"] != true {
		t.Errorf("logged = %v, want true", m["logged"])
	}
	if m["step"] != 5 {
		t.Errorf("step = %v, want 5", m["step"])
	}
}

func TestHubLogMetrics_MissingMetrics(t *testing.T) {
	mux := http.NewServeMux()
	_, reg := newHubTestServer(t, mux)
	_, err := reg.Call("c4_hub_log_metrics", json.RawMessage(`{"job_id": "j", "step": 0}`))
	if err == nil {
		t.Fatal("expected error for missing metrics")
	}
}

func TestHubWatch(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/jobs/job-w/logs", func(w http.ResponseWriter, r *http.Request) {
		hubJSON(w, hub.JobLogsResponse{
			JobID: "job-w", Lines: []string{"epoch 1", "epoch 2"},
			TotalLines: 2, Offset: 0, HasMore: false,
		})
	})
	_, reg := newHubTestServer(t, mux)

	result, err := reg.Call("c4_hub_watch", json.RawMessage(`{"job_id": "job-w"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]any)
	lines := m["lines"].([]string)
	if len(lines) != 2 {
		t.Errorf("lines count = %d, want 2", len(lines))
	}
	if m["has_more"] != false {
		t.Errorf("has_more = %v, want false", m["has_more"])
	}
}

func TestHubWatch_MissingJobID(t *testing.T) {
	mux := http.NewServeMux()
	_, reg := newHubTestServer(t, mux)
	_, err := reg.Call("c4_hub_watch", json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error for missing job_id")
	}
}

func TestHubSummary(t *testing.T) {
	dur := 120.5
	code := 0
	mux := http.NewServeMux()
	mux.HandleFunc("/jobs/job-s/summary", func(w http.ResponseWriter, r *http.Request) {
		hubJSON(w, hub.JobSummaryResponse{
			JobID: "job-s", Name: "train", Status: "SUCCEEDED",
			DurationSec: &dur, ExitCode: &code,
			Metrics: map[string]any{"loss": 0.1}, LogTail: []string{"done"},
		})
	})
	_, reg := newHubTestServer(t, mux)

	result, err := reg.Call("c4_hub_summary", json.RawMessage(`{"job_id": "job-s"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]any)
	if m["status"] != "SUCCEEDED" {
		t.Errorf("status = %v, want SUCCEEDED", m["status"])
	}
	if m["duration_seconds"] != dur {
		t.Errorf("duration = %v, want %v", m["duration_seconds"], dur)
	}
	if m["exit_code"] != code {
		t.Errorf("exit_code = %v, want %d", m["exit_code"], code)
	}
}

func TestHubSummary_MinimalResponse(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/jobs/job-min/summary", func(w http.ResponseWriter, r *http.Request) {
		hubJSON(w, hub.JobSummaryResponse{JobID: "job-min", Name: "run", Status: "RUNNING"})
	})
	_, reg := newHubTestServer(t, mux)

	result, err := reg.Call("c4_hub_summary", json.RawMessage(`{"job_id": "job-min"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]any)
	if _, ok := m["duration_seconds"]; ok {
		t.Error("running job should not have duration_seconds")
	}
	if _, ok := m["exit_code"]; ok {
		t.Error("running job should not have exit_code")
	}
}

func TestHubSummary_MissingJobID(t *testing.T) {
	mux := http.NewServeMux()
	_, reg := newHubTestServer(t, mux)
	_, err := reg.Call("c4_hub_summary", json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error for missing job_id")
	}
}

func TestHubRetry(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/jobs/job-r/retry", func(w http.ResponseWriter, r *http.Request) {
		hubJSON(w, hub.JobRetryResponse{NewJobID: "job-r2", Status: "QUEUED", OriginalJobID: "job-r"})
	})
	_, reg := newHubTestServer(t, mux)

	result, err := reg.Call("c4_hub_retry", json.RawMessage(`{"job_id": "job-r"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]any)
	if m["new_job_id"] != "job-r2" {
		t.Errorf("new_job_id = %v, want job-r2", m["new_job_id"])
	}
	if m["original_job_id"] != "job-r" {
		t.Errorf("original_job_id = %v, want job-r", m["original_job_id"])
	}
}

func TestHubRetry_MissingJobID(t *testing.T) {
	mux := http.NewServeMux()
	_, reg := newHubTestServer(t, mux)
	_, err := reg.Call("c4_hub_retry", json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error for missing job_id")
	}
}

func TestHubEstimate(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/jobs/job-e/estimate", func(w http.ResponseWriter, r *http.Request) {
		hubJSON(w, hub.JobEstimateResponse{
			EstimatedDurationSec: 300, Confidence: "high", Method: "historical",
			QueueWaitSec: 60, EstimatedStartTime: "2026-01-01T00:01:00Z",
			EstimatedEndTime: "2026-01-01T00:06:00Z",
		})
	})
	_, reg := newHubTestServer(t, mux)

	result, err := reg.Call("c4_hub_estimate", json.RawMessage(`{"job_id": "job-e"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]any)
	if m["method"] != "historical" {
		t.Errorf("method = %v, want historical", m["method"])
	}
	if m["queue_wait_sec"] != 60.0 {
		t.Errorf("queue_wait_sec = %v, want 60", m["queue_wait_sec"])
	}
	if _, ok := m["estimated_start_time"]; !ok {
		t.Error("expected estimated_start_time")
	}
}

func TestHubEstimate_MissingJobID(t *testing.T) {
	mux := http.NewServeMux()
	_, reg := newHubTestServer(t, mux)
	_, err := reg.Call("c4_hub_estimate", json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error for missing job_id")
	}
}

// =========================================================================
// Hub Infra Tests (4 handlers)
// =========================================================================

func TestHubWorkers(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/workers", func(w http.ResponseWriter, r *http.Request) {
		hubJSON(w, []map[string]any{{"id": "w-1", "hostname": "gpu-01", "status": "idle"}})
	})
	_, reg := newHubTestServer(t, mux)

	result, err := reg.Call("c4_hub_workers", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]any)
	workers := m["workers"].([]hub.Worker)
	if len(workers) != 1 {
		t.Errorf("workers count = %d, want 1", len(workers))
	}
}

func TestHubStats(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/stats/queue", func(w http.ResponseWriter, r *http.Request) {
		hubJSON(w, hub.QueueStats{Queued: 5, Running: 2, Succeeded: 10, Failed: 1, Cancelled: 0})
	})
	_, reg := newHubTestServer(t, mux)

	result, err := reg.Call("c4_hub_stats", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]any)
	if m["queued"] != 5 {
		t.Errorf("queued = %v, want 5", m["queued"])
	}
	total := m["total"].(int)
	if total != 18 {
		t.Errorf("total = %d, want 18 (5+2+10+1+0)", total)
	}
}

func TestHubUpload_MissingRequired(t *testing.T) {
	mux := http.NewServeMux()
	_, reg := newHubTestServer(t, mux)
	_, err := reg.Call("c4_hub_upload", json.RawMessage(`{"job_id": "j"}`))
	if err == nil {
		t.Fatal("expected error for missing local_path/storage_path")
	}
}

func TestHubDownload_MissingRequired(t *testing.T) {
	mux := http.NewServeMux()
	_, reg := newHubTestServer(t, mux)
	_, err := reg.Call("c4_hub_download", json.RawMessage(`{"job_id": "j"}`))
	if err == nil {
		t.Fatal("expected error for missing name/local_path")
	}
}

// =========================================================================
// Hub Edge Tests (5 handlers)
// =========================================================================

func TestHubEdgeRegister(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/edges/register", func(w http.ResponseWriter, r *http.Request) {
		hubJSON(w, map[string]any{"edge_id": "edge-abc"})
	})
	_, reg := newHubTestServer(t, mux)

	result, err := reg.Call("c4_hub_edge_register", json.RawMessage(`{
		"name": "jetson-1", "tags": ["onnx"], "arch": "arm64", "runtime": "tensorrt", "storage_gb": 32.5
	}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]any)
	if m["edge_id"] != "edge-abc" {
		t.Errorf("edge_id = %v, want edge-abc", m["edge_id"])
	}
	if m["name"] != "jetson-1" {
		t.Errorf("name = %v, want jetson-1", m["name"])
	}
}

func TestHubEdgeRegister_MissingName(t *testing.T) {
	mux := http.NewServeMux()
	_, reg := newHubTestServer(t, mux)
	_, err := reg.Call("c4_hub_edge_register", json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestHubEdgeList(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/edges", func(w http.ResponseWriter, r *http.Request) {
		hubJSON(w, []hub.Edge{{ID: "e-1", Name: "jetson-1", Status: "online"}})
	})
	_, reg := newHubTestServer(t, mux)

	result, err := reg.Call("c4_hub_edge_list", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]any)
	if m["count"] != 1 {
		t.Errorf("count = %v, want 1", m["count"])
	}
}

func TestHubDeployRule(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/deploy/rules", func(w http.ResponseWriter, r *http.Request) {
		hubJSON(w, hub.DeployRuleCreateResponse{RuleID: "rule-123"})
	})
	_, reg := newHubTestServer(t, mux)

	result, err := reg.Call("c4_hub_deploy_rule", json.RawMessage(`{
		"name": "auto-prod",
		"trigger": "job_tag:production", "edge_filter": "tag:onnx",
		"artifact_pattern": "*.onnx", "post_command": "systemctl restart inference"
	}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]any)
	if m["rule_id"] != "rule-123" {
		t.Errorf("rule_id = %v, want rule-123", m["rule_id"])
	}
}

func TestHubDeployRule_MissingRequired(t *testing.T) {
	mux := http.NewServeMux()
	_, reg := newHubTestServer(t, mux)
	_, err := reg.Call("c4_hub_deploy_rule", json.RawMessage(`{"trigger": "x"}`))
	if err == nil {
		t.Fatal("expected error for missing edge_filter/artifact_pattern")
	}
}

func TestHubDeploy(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/deploy/trigger", func(w http.ResponseWriter, r *http.Request) {
		hubJSON(w, hub.DeployTriggerResponse{DeployID: "dep-1", Status: "pending", TargetCount: 3})
	})
	_, reg := newHubTestServer(t, mux)

	result, err := reg.Call("c4_hub_deploy", json.RawMessage(`{
		"job_id": "job-deploy", "edge_ids": ["e-1", "e-2", "e-3"]
	}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]any)
	if m["deploy_id"] != "dep-1" {
		t.Errorf("deploy_id = %v, want dep-1", m["deploy_id"])
	}
	if m["target_count"] != 3 {
		t.Errorf("target_count = %v, want 3", m["target_count"])
	}
}

func TestHubDeploy_MissingJobID(t *testing.T) {
	mux := http.NewServeMux()
	_, reg := newHubTestServer(t, mux)
	_, err := reg.Call("c4_hub_deploy", json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error for missing job_id")
	}
}

func TestHubDeployStatus(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/deploy/dep-s/status", func(w http.ResponseWriter, r *http.Request) {
		hubJSON(w, hub.Deployment{
			ID: "dep-s", Status: "completed",
			Targets: []hub.DeployTarget{
				{EdgeID: "e-1", EdgeName: "jetson-1", Status: "succeeded"},
				{EdgeID: "e-2", Status: "failed", Error: "disk full"},
			},
		})
	})
	_, reg := newHubTestServer(t, mux)

	result, err := reg.Call("c4_hub_deploy_status", json.RawMessage(`{"deploy_id": "dep-s"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]any)
	if m["status"] != "completed" {
		t.Errorf("status = %v, want completed", m["status"])
	}
	targets := m["targets"].([]map[string]any)
	if len(targets) != 2 {
		t.Errorf("targets count = %d, want 2", len(targets))
	}
	if targets[0]["edge_name"] != "jetson-1" {
		t.Errorf("target[0].edge_name = %v, want jetson-1", targets[0]["edge_name"])
	}
	if targets[1]["error"] != "disk full" {
		t.Errorf("target[1].error = %v, want 'disk full'", targets[1]["error"])
	}
}

func TestHubDeployStatus_MissingDeployID(t *testing.T) {
	mux := http.NewServeMux()
	_, reg := newHubTestServer(t, mux)
	_, err := reg.Call("c4_hub_deploy_status", json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error for missing deploy_id")
	}
}

// =========================================================================
// Hub DAG Tests (7 handlers)
// =========================================================================

func TestHubDAGCreate(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/dags", func(w http.ResponseWriter, r *http.Request) {
		hubJSON(w, hub.DAGCreateResponse{DAGID: "dag-1", Status: "pending"})
	})
	_, reg := newHubTestServer(t, mux)

	result, err := reg.Call("c4_hub_dag_create", json.RawMessage(`{
		"name": "ml-pipeline", "description": "Train and evaluate", "tags": ["ml"]
	}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]any)
	if m["dag_id"] != "dag-1" {
		t.Errorf("dag_id = %v, want dag-1", m["dag_id"])
	}
}

func TestHubDAGCreate_MissingName(t *testing.T) {
	mux := http.NewServeMux()
	_, reg := newHubTestServer(t, mux)
	_, err := reg.Call("c4_hub_dag_create", json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestHubDAGAddNode(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/dags/dag-1/nodes", func(w http.ResponseWriter, r *http.Request) {
		hubJSON(w, hub.DAGAddNodeResponse{NodeID: "node-prep", Name: "preprocess"})
	})
	_, reg := newHubTestServer(t, mux)

	result, err := reg.Call("c4_hub_dag_add_node", json.RawMessage(`{
		"dag_id": "dag-1", "name": "preprocess", "command": "python preprocess.py",
		"gpu_count": 1, "max_retries": 2
	}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]any)
	if m["node_id"] != "node-prep" {
		t.Errorf("node_id = %v, want node-prep", m["node_id"])
	}
	if m["dag_id"] != "dag-1" {
		t.Errorf("dag_id = %v, want dag-1", m["dag_id"])
	}
}

func TestHubDAGAddNode_MissingRequired(t *testing.T) {
	mux := http.NewServeMux()
	_, reg := newHubTestServer(t, mux)
	_, err := reg.Call("c4_hub_dag_add_node", json.RawMessage(`{"dag_id": "d"}`))
	if err == nil {
		t.Fatal("expected error for missing name/command")
	}
}

func TestHubDAGAddDep(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/dags/dag-1/dependencies", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	_, reg := newHubTestServer(t, mux)

	result, err := reg.Call("c4_hub_dag_add_dep", json.RawMessage(`{
		"dag_id": "dag-1", "source_id": "n-prep", "target_id": "n-train"
	}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]any)
	if m["added"] != true {
		t.Errorf("added = %v, want true", m["added"])
	}
	if m["type"] != "sequential" {
		t.Errorf("type = %v, want sequential (default)", m["type"])
	}
}

func TestHubDAGAddDep_WithType(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/dags/dag-1/dependencies", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	_, reg := newHubTestServer(t, mux)

	result, err := reg.Call("c4_hub_dag_add_dep", json.RawMessage(`{
		"dag_id": "dag-1", "source_id": "s", "target_id": "t", "dependency_type": "data_dependency"
	}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]any)
	if m["type"] != "data_dependency" {
		t.Errorf("type = %v, want data_dependency", m["type"])
	}
}

func TestHubDAGAddDep_MissingRequired(t *testing.T) {
	mux := http.NewServeMux()
	_, reg := newHubTestServer(t, mux)
	_, err := reg.Call("c4_hub_dag_add_dep", json.RawMessage(`{"dag_id": "d", "source_id": "s"}`))
	if err == nil {
		t.Fatal("expected error for missing target_id")
	}
}

func TestHubDAGExecute(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/dags/dag-1/execute", func(w http.ResponseWriter, r *http.Request) {
		hubJSON(w, hub.DAGExecuteResponse{
			DAGID: "dag-1", Status: "running", NodeOrder: []string{"prep", "train", "eval"},
		})
	})
	_, reg := newHubTestServer(t, mux)

	result, err := reg.Call("c4_hub_dag_execute", json.RawMessage(`{"dag_id": "dag-1"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]any)
	if m["status"] != "running" {
		t.Errorf("status = %v, want running", m["status"])
	}
	order := m["node_order"].([]string)
	if len(order) != 3 {
		t.Errorf("node_order count = %d, want 3", len(order))
	}
}

func TestHubDAGExecute_DryRun(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/dags/dag-1/execute", func(w http.ResponseWriter, r *http.Request) {
		hubJSON(w, hub.DAGExecuteResponse{
			DAGID: "dag-1", Status: "validated", Validation: "valid",
		})
	})
	_, reg := newHubTestServer(t, mux)

	result, err := reg.Call("c4_hub_dag_execute", json.RawMessage(`{"dag_id": "dag-1", "dry_run": true}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]any)
	if m["validation"] != "valid" {
		t.Errorf("validation = %v, want valid", m["validation"])
	}
}

func TestHubDAGExecute_MissingDAGID(t *testing.T) {
	mux := http.NewServeMux()
	_, reg := newHubTestServer(t, mux)
	_, err := reg.Call("c4_hub_dag_execute", json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error for missing dag_id")
	}
}

func TestHubDAGStatus(t *testing.T) {
	ec := 0
	mux := http.NewServeMux()
	mux.HandleFunc("/dags/dag-1/status", func(w http.ResponseWriter, r *http.Request) {
		hubJSON(w, hub.DAG{
			ID: "dag-1", Name: "pipeline", Status: "completed",
			Nodes: []hub.DAGNode{
				{ID: "n1", Name: "prep", Status: "succeeded", ExitCode: &ec},
				{ID: "n2", Name: "train", Status: "succeeded", JobID: "job-x", ExitCode: &ec},
			},
			Dependencies: []hub.DAGDependency{{SourceID: "n1", TargetID: "n2", Type: "sequential"}},
		})
	})
	_, reg := newHubTestServer(t, mux)

	result, err := reg.Call("c4_hub_dag_status", json.RawMessage(`{"dag_id": "dag-1"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]any)
	if m["status"] != "completed" {
		t.Errorf("status = %v, want completed", m["status"])
	}
	nodes := m["nodes"].([]map[string]any)
	if len(nodes) != 2 {
		t.Errorf("nodes = %d, want 2", len(nodes))
	}
	if nodes[1]["job_id"] != "job-x" {
		t.Errorf("nodes[1].job_id = %v, want job-x", nodes[1]["job_id"])
	}
}

func TestHubDAGStatus_MissingDAGID(t *testing.T) {
	mux := http.NewServeMux()
	_, reg := newHubTestServer(t, mux)
	_, err := reg.Call("c4_hub_dag_status", json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error for missing dag_id")
	}
}

func TestHubDAGList(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/dags", func(w http.ResponseWriter, r *http.Request) {
		hubJSON(w, []hub.DAG{{ID: "dag-1", Name: "p1", Status: "completed"}})
	})
	_, reg := newHubTestServer(t, mux)

	result, err := reg.Call("c4_hub_dag_list", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]any)
	if m["count"] != 1 {
		t.Errorf("count = %v, want 1", m["count"])
	}
}

func TestHubDAGFromYAML(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/dags/from-yaml", func(w http.ResponseWriter, r *http.Request) {
		hubJSON(w, hub.DAG{
			ID: "dag-y", Name: "yaml-dag", Status: "pending",
			Nodes:        []hub.DAGNode{{ID: "n1"}, {ID: "n2"}},
			Dependencies: []hub.DAGDependency{{SourceID: "n1", TargetID: "n2"}},
		})
	})
	_, reg := newHubTestServer(t, mux)

	result, err := reg.Call("c4_hub_dag_from_yaml", json.RawMessage(`{"yaml_content": "name: test"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]any)
	if m["dag_id"] != "dag-y" {
		t.Errorf("dag_id = %v, want dag-y", m["dag_id"])
	}
	if m["nodes"] != 2 {
		t.Errorf("nodes = %v, want 2", m["nodes"])
	}
}

func TestHubDAGFromYAML_MissingContent(t *testing.T) {
	mux := http.NewServeMux()
	_, reg := newHubTestServer(t, mux)
	_, err := reg.Call("c4_hub_dag_from_yaml", json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error for missing yaml_content")
	}
}

// =========================================================================
// Error response tests (one per handler category)
// =========================================================================

func TestHubSubmit_ServerError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/jobs/submit", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
	})
	_, reg := newHubTestServer(t, mux)

	_, err := reg.Call("c4_hub_submit", json.RawMessage(`{
		"name": "test", "workdir": "/ws", "command": "echo"
	}`))
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestHubWorkers_ServerError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/workers", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	})
	_, reg := newHubTestServer(t, mux)

	_, err := reg.Call("c4_hub_workers", json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error for 503 response")
	}
}

func TestHubEdgeRegister_ServerError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/edges/register", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "forbidden", http.StatusForbidden)
	})
	_, reg := newHubTestServer(t, mux)

	_, err := reg.Call("c4_hub_edge_register", json.RawMessage(`{"name": "e1"}`))
	if err == nil {
		t.Fatal("expected error for 403 response")
	}
}

func TestHubDAGCreate_ServerError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/dags", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad gateway", http.StatusBadGateway)
	})
	_, reg := newHubTestServer(t, mux)

	_, err := reg.Call("c4_hub_dag_create", json.RawMessage(`{"name": "fail-dag"}`))
	if err == nil {
		t.Fatal("expected error for 502 response")
	}
}

// =========================================================================
// Hub Lease Renew Tests (1 handler)
// =========================================================================

func TestHubLeaseRenew_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/leases/renew", func(w http.ResponseWriter, r *http.Request) {
		hubJSON(w, map[string]any{"renewed": true, "new_expires_at": "2026-02-19T12:05:00Z"})
	})
	_, reg := newHubTestServer(t, mux)

	result, err := reg.Call("c4_hub_lease_renew", json.RawMessage(`{"lease_id": "lease-abc"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]any)
	if m["renewed"] != true {
		t.Errorf("renewed = %v, want true", m["renewed"])
	}
	if m["lease_id"] != "lease-abc" {
		t.Errorf("lease_id = %v, want lease-abc", m["lease_id"])
	}
	if m["new_expires_at"] != "2026-02-19T12:05:00Z" {
		t.Errorf("new_expires_at = %v, want 2026-02-19T12:05:00Z", m["new_expires_at"])
	}
}

func TestHubLeaseRenew_Failure(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/leases/renew", func(w http.ResponseWriter, r *http.Request) {
		// Hub returns renewed=false when lease has already expired
		hubJSON(w, map[string]any{"renewed": false})
	})
	_, reg := newHubTestServer(t, mux)

	_, err := reg.Call("c4_hub_lease_renew", json.RawMessage(`{"lease_id": "lease-expired"}`))
	if err == nil {
		t.Fatal("expected error when lease renewal fails")
	}
}

func TestHubLeaseRenew_MissingLeaseID(t *testing.T) {
	mux := http.NewServeMux()
	_, reg := newHubTestServer(t, mux)
	_, err := reg.Call("c4_hub_lease_renew", json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error for missing lease_id")
	}
}

func TestHubLeaseRenew_ServerError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/leases/renew", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	})
	_, reg := newHubTestServer(t, mux)

	_, err := reg.Call("c4_hub_lease_renew", json.RawMessage(`{"lease_id": "lease-abc"}`))
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

// =========================================================================
// Event publishing tests
// =========================================================================

func TestHubWatch_PublishesCompletedEvent(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/jobs/job-done/logs", func(w http.ResponseWriter, r *http.Request) {
		hubJSON(w, hub.JobLogsResponse{
			JobID: "job-done", Lines: []string{"done"}, TotalLines: 1,
		})
	})
	mux.HandleFunc("/jobs/job-done", func(w http.ResponseWriter, r *http.Request) {
		hubJSON(w, hub.Job{ID: "job-done", Name: "train", Status: "SUCCEEDED"})
	})
	_, reg := newHubTestServer(t, mux)

	pub := &mockPublisher{}
	origPub := hubEventPub
	hubEventPub = pub
	t.Cleanup(func() { hubEventPub = origPub })

	_, err := reg.Call("c4_hub_watch", json.RawMessage(`{"job_id": "job-done"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	events := pub.getEvents()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].evType != "hub.job.completed" {
		t.Errorf("event = %q, want hub.job.completed", events[0].evType)
	}
}

func TestHubWatch_PublishesFailedEvent(t *testing.T) {
	exitCode := 1
	mux := http.NewServeMux()
	mux.HandleFunc("/jobs/job-fail/logs", func(w http.ResponseWriter, r *http.Request) {
		hubJSON(w, hub.JobLogsResponse{JobID: "job-fail", Lines: []string{"error"}})
	})
	mux.HandleFunc("/jobs/job-fail", func(w http.ResponseWriter, r *http.Request) {
		hubJSON(w, hub.Job{ID: "job-fail", Name: "train", Status: "FAILED", ExitCode: &exitCode})
	})
	_, reg := newHubTestServer(t, mux)

	pub := &mockPublisher{}
	origPub := hubEventPub
	hubEventPub = pub
	t.Cleanup(func() { hubEventPub = origPub })

	_, err := reg.Call("c4_hub_watch", json.RawMessage(`{"job_id": "job-fail"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	events := pub.getEvents()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].evType != "hub.job.failed" {
		t.Errorf("event = %q, want hub.job.failed", events[0].evType)
	}
}

func TestHubWatch_NilPublisher_NoPanic(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/jobs/job-np/logs", func(w http.ResponseWriter, r *http.Request) {
		hubJSON(w, hub.JobLogsResponse{JobID: "job-np", Lines: []string{"ok"}})
	})
	_, reg := newHubTestServer(t, mux)

	origPub := hubEventPub
	hubEventPub = nil
	t.Cleanup(func() { hubEventPub = origPub })

	// Should not panic even if hubEventPub is nil.
	_, err := reg.Call("c4_hub_watch", json.RawMessage(`{"job_id": "job-np"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHubRetry_PublishesRetriedEvent(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/jobs/job-ev-r/retry", func(w http.ResponseWriter, r *http.Request) {
		hubJSON(w, hub.JobRetryResponse{NewJobID: "job-ev-r2", Status: "QUEUED", OriginalJobID: "job-ev-r"})
	})
	_, reg := newHubTestServer(t, mux)

	pub := &mockPublisher{}
	origPub := hubEventPub
	hubEventPub = pub
	t.Cleanup(func() { hubEventPub = origPub })

	_, err := reg.Call("c4_hub_retry", json.RawMessage(`{"job_id": "job-ev-r"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	events := pub.getEvents()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].evType != "hub.job.retried" {
		t.Errorf("event = %q, want hub.job.retried", events[0].evType)
	}
}

func TestHubDAGExecute_PublishesDagExecutedEvent(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/dags/dag-ev/execute", func(w http.ResponseWriter, r *http.Request) {
		hubJSON(w, hub.DAGExecuteResponse{
			DAGID: "dag-ev", Status: "running", NodeOrder: []string{"a", "b", "c"},
		})
	})
	_, reg := newHubTestServer(t, mux)

	pub := &mockPublisher{}
	origPub := hubEventPub
	hubEventPub = pub
	t.Cleanup(func() { hubEventPub = origPub })

	_, err := reg.Call("c4_hub_dag_execute", json.RawMessage(`{"dag_id": "dag-ev"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	events := pub.getEvents()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].evType != "hub.dag.executed" {
		t.Errorf("event = %q, want hub.dag.executed", events[0].evType)
	}
}

func TestHubDAGExecute_DryRun_NoEvent(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/dags/dag-dr/execute", func(w http.ResponseWriter, r *http.Request) {
		hubJSON(w, hub.DAGExecuteResponse{DAGID: "dag-dr", Status: "validated", Validation: "valid"})
	})
	_, reg := newHubTestServer(t, mux)

	pub := &mockPublisher{}
	origPub := hubEventPub
	hubEventPub = pub
	t.Cleanup(func() { hubEventPub = origPub })

	_, err := reg.Call("c4_hub_dag_execute", json.RawMessage(`{"dag_id": "dag-dr", "dry_run": true}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// dry_run should not emit events
	events := pub.getEvents()
	if len(events) != 0 {
		t.Errorf("expected 0 events for dry_run, got %d", len(events))
	}
}

// =========================================================================
// Artifact param tests (T-838-0)
// =========================================================================

func TestHubSubmit_WithArtifacts(t *testing.T) {
	var receivedBody map[string]any
	mux := http.NewServeMux()
	mux.HandleFunc("/jobs/submit", func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedBody)
		hubJSON(w, map[string]any{"job_id": "job-art", "status": "QUEUED", "queue_position": 0})
	})
	_, reg := newHubTestServer(t, mux)

	result, err := reg.Call("c4_hub_submit", json.RawMessage(`{
		"name": "train", "workdir": "/ws", "command": "run.sh",
		"input_artifacts":  [{"path": "datasets/cifar.tar.gz", "local_path": "/data/cifar.tar.gz", "required": true}],
		"output_artifacts": [{"path": "models/resnet.pt", "local_path": "/out/resnet.pt"}]
	}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]any)
	if m["job_id"] != "job-art" {
		t.Errorf("job_id = %v, want job-art", m["job_id"])
	}

	// Verify artifact fields were forwarded to C5.
	if _, ok := receivedBody["input_artifacts"]; !ok {
		t.Error("expected input_artifacts in request body sent to Hub")
	}
	if _, ok := receivedBody["output_artifacts"]; !ok {
		t.Error("expected output_artifacts in request body sent to Hub")
	}
}

func TestHubSubmit_WithoutArtifacts_Omitted(t *testing.T) {
	var receivedBody map[string]any
	mux := http.NewServeMux()
	mux.HandleFunc("/jobs/submit", func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedBody)
		hubJSON(w, map[string]any{"job_id": "job-noart", "status": "QUEUED", "queue_position": 0})
	})
	_, reg := newHubTestServer(t, mux)

	_, err := reg.Call("c4_hub_submit", json.RawMessage(`{
		"name": "train", "workdir": "/ws", "command": "run.sh"
	}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify artifact fields are absent when not provided (omitempty).
	if _, ok := receivedBody["input_artifacts"]; ok {
		t.Error("input_artifacts should be omitted from request body when not provided")
	}
	if _, ok := receivedBody["output_artifacts"]; ok {
		t.Error("output_artifacts should be omitted from request body when not provided")
	}
}

// =========================================================================
// Registration count test
// =========================================================================

func TestRegisterHubHandlersToolCount(t *testing.T) {
	mux := http.NewServeMux()
	_, reg := newHubTestServer(t, mux)
	tools := reg.ListTools()
	if len(tools) != 27 {
		names := make([]string, 0, len(tools))
		for _, tool := range tools {
			names = append(names, tool.Name)
		}
		t.Errorf("registered %d hub tools, want 27: %v", len(tools), names)
	}
}
