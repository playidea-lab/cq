package api

import (
	"net/http"
	"testing"

	"github.com/piqsol/c4/c5/internal/model"
)

// =========================================================================
// Integration tests — verify hub.Client-compatible API contracts
// =========================================================================

// TestIntegrationJobLifecycle exercises the full job lifecycle:
// submit → acquire → logs → metrics → complete → summary → retry
func TestIntegrationJobLifecycle(t *testing.T) {
	srv := newTestServer(t)

	// 1. Submit job with env and timeout
	w := doRequest(t, srv, "POST", "/v1/jobs/submit", model.JobSubmitRequest{
		Name:       "integ-job",
		Command:    "echo hello",
		Workdir:    "/tmp",
		TimeoutSec: 60,
		Env:        map[string]string{"FOO": "bar"},
		Tags:       []string{"test"},
		Priority:   5,
		ExpID:      "exp001",
		Memo:       "integration test",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("submit: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var submitResp model.JobSubmitResponse
	decodeJSON(t, w, &submitResp)
	jobID := submitResp.JobID

	if submitResp.Status != "QUEUED" {
		t.Fatalf("expected QUEUED, got %s", submitResp.Status)
	}

	// 2. Get job — verify all fields persisted
	w2 := doRequest(t, srv, "GET", "/v1/jobs/"+jobID, nil)
	if w2.Code != http.StatusOK {
		t.Fatalf("get job: expected 200, got %d", w2.Code)
	}
	var job model.Job
	decodeJSON(t, w2, &job)
	if job.Name != "integ-job" {
		t.Fatalf("name mismatch: %s", job.Name)
	}
	if job.TimeoutSec != 60 {
		t.Fatalf("timeout_sec mismatch: %d", job.TimeoutSec)
	}
	if job.Env["FOO"] != "bar" {
		t.Fatalf("env not persisted: %v", job.Env)
	}

	// 3. Estimate — should include estimated_start_time for QUEUED
	w3 := doRequest(t, srv, "GET", "/v1/jobs/"+jobID+"/estimate", nil)
	if w3.Code != http.StatusOK {
		t.Fatalf("estimate: expected 200, got %d", w3.Code)
	}
	var est model.EstimateResponse
	decodeJSON(t, w3, &est)
	if est.EstimatedStartTime == "" {
		t.Fatal("estimated_start_time should not be empty for queued job")
	}
	if est.Method == "" {
		t.Fatal("method should not be empty")
	}

	// 4. Register worker
	ww := doRequest(t, srv, "POST", "/v1/workers/register", model.WorkerRegisterRequest{
		Hostname:  "integ-worker",
		GPUCount:  1,
		GPUModel:  "RTX4090",
		TotalVRAM: 24,
		FreeVRAM:  24,
	})
	if ww.Code != http.StatusCreated {
		t.Fatalf("register: expected 201, got %d", ww.Code)
	}
	var regResp model.WorkerRegisterResponse
	decodeJSON(t, ww, &regResp)

	// 5. Acquire lease
	wl := doRequest(t, srv, "POST", "/v1/leases/acquire", model.LeaseAcquireRequest{
		WorkerID: regResp.WorkerID,
	})
	if wl.Code != http.StatusOK {
		t.Fatalf("acquire: expected 200, got %d", wl.Code)
	}
	var acqResp model.LeaseAcquireResponse
	decodeJSON(t, wl, &acqResp)
	if acqResp.Job.ID != jobID {
		t.Fatalf("acquired wrong job: %s", acqResp.Job.ID)
	}

	// 6. Renew lease
	wr := doRequest(t, srv, "POST", "/v1/leases/renew", model.LeaseRenewRequest{
		LeaseID:  acqResp.LeaseID,
		WorkerID: regResp.WorkerID,
	})
	if wr.Code != http.StatusOK {
		t.Fatalf("renew: expected 200, got %d", wr.Code)
	}

	// 7. Append logs
	doRequest(t, srv, "POST", "/v1/jobs/"+jobID+"/logs", map[string]string{
		"line": "starting training", "stream": "stdout",
	})
	doRequest(t, srv, "POST", "/v1/jobs/"+jobID+"/logs", map[string]string{
		"line": "epoch 1/10", "stream": "stdout",
	})

	// 8. Log metrics
	doRequest(t, srv, "POST", "/v1/metrics/"+jobID, model.MetricsLogRequest{
		Step:    0,
		Metrics: map[string]any{"loss": 0.9, "lr": 0.001},
	})
	doRequest(t, srv, "POST", "/v1/metrics/"+jobID, model.MetricsLogRequest{
		Step:    1,
		Metrics: map[string]any{"loss": 0.5, "lr": 0.001},
	})

	// 9. Get metrics
	wm := doRequest(t, srv, "GET", "/v1/metrics/"+jobID, nil)
	if wm.Code != http.StatusOK {
		t.Fatalf("get metrics: expected 200, got %d", wm.Code)
	}
	var metricsResp model.MetricsResponse
	decodeJSON(t, wm, &metricsResp)
	if metricsResp.TotalSteps != 2 {
		t.Fatalf("expected 2 steps, got %d", metricsResp.TotalSteps)
	}

	// 10. Get logs
	wlogs := doRequest(t, srv, "GET", "/v1/jobs/"+jobID+"/logs", nil)
	if wlogs.Code != http.StatusOK {
		t.Fatalf("get logs: expected 200, got %d", wlogs.Code)
	}
	var logsResp model.JobLogsResponse
	decodeJSON(t, wlogs, &logsResp)
	if logsResp.TotalLines != 2 {
		t.Fatalf("expected 2 log lines, got %d", logsResp.TotalLines)
	}

	// 11. Complete job
	wc := doRequest(t, srv, "POST", "/v1/jobs/"+jobID+"/complete", model.JobCompleteRequest{
		Status:   "SUCCEEDED",
		ExitCode: 0,
	})
	if wc.Code != http.StatusOK {
		t.Fatalf("complete: expected 200, got %d: %s", wc.Code, wc.Body.String())
	}

	// 12. Summary
	ws := doRequest(t, srv, "GET", "/v1/jobs/"+jobID+"/summary", nil)
	if ws.Code != http.StatusOK {
		t.Fatalf("summary: expected 200, got %d", ws.Code)
	}
	var summary model.JobSummaryResponse
	decodeJSON(t, ws, &summary)
	if summary.Status != "SUCCEEDED" {
		t.Fatalf("summary status: %s", summary.Status)
	}
	if summary.Name != "integ-job" {
		t.Fatalf("summary name: %s", summary.Name)
	}

	// 13. Retry — should succeed for terminal jobs
	wrt := doRequest(t, srv, "POST", "/v1/jobs/"+jobID+"/retry", nil)
	if wrt.Code != http.StatusOK {
		t.Fatalf("retry: expected 200, got %d: %s", wrt.Code, wrt.Body.String())
	}
	var retryResp model.JobRetryResponse
	decodeJSON(t, wrt, &retryResp)
	if retryResp.OriginalJobID != jobID {
		t.Fatalf("retry original_job_id mismatch")
	}
	if retryResp.NewJobID == "" {
		t.Fatal("retry should return new_job_id")
	}
}

// TestIntegrationWorkerLifecycle exercises: register → heartbeat → list → stale
func TestIntegrationWorkerLifecycle(t *testing.T) {
	srv := newTestServer(t)

	// Register
	w := doRequest(t, srv, "POST", "/v1/workers/register", model.WorkerRegisterRequest{
		Hostname:  "gpu-node-1",
		GPUCount:  4,
		GPUModel:  "A100",
		TotalVRAM: 320,
		FreeVRAM:  320,
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("register: expected 201, got %d", w.Code)
	}
	var regResp model.WorkerRegisterResponse
	decodeJSON(t, w, &regResp)

	// Heartbeat
	wh := doRequest(t, srv, "POST", "/v1/workers/heartbeat", model.HeartbeatRequest{
		WorkerID: regResp.WorkerID,
		FreeVRAM: 200,
	})
	if wh.Code != http.StatusOK {
		t.Fatalf("heartbeat: expected 200, got %d", wh.Code)
	}

	// List workers
	wl := doRequest(t, srv, "GET", "/v1/workers", nil)
	if wl.Code != http.StatusOK {
		t.Fatalf("list: expected 200, got %d", wl.Code)
	}
	var listResp map[string]any
	decodeJSON(t, wl, &listResp)
	workers := listResp["workers"].([]any)
	if len(workers) != 1 {
		t.Fatalf("expected 1 worker, got %d", len(workers))
	}
}

// TestIntegrationDAGLifecycle exercises: create → add nodes → add dep → execute → status
func TestIntegrationDAGLifecycle(t *testing.T) {
	srv := newTestServer(t)

	// Create DAG
	w := doRequest(t, srv, "POST", "/v1/dags", model.DAGCreateRequest{
		Name:        "train-pipeline",
		Description: "preprocess → train → eval",
		Tags:        []string{"ml"},
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create DAG: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var dagResp model.DAGCreateResponse
	decodeJSON(t, w, &dagResp)
	dagID := dagResp.DAGID

	// Add nodes
	w1 := doRequest(t, srv, "POST", "/v1/dags/"+dagID+"/nodes", model.DAGAddNodeRequest{
		Name: "preprocess", Command: "echo preprocess",
	})
	if w1.Code != http.StatusCreated {
		t.Fatalf("add node: expected 201, got %d: %s", w1.Code, w1.Body.String())
	}
	var n1 model.DAGAddNodeResponse
	decodeJSON(t, w1, &n1)

	w2 := doRequest(t, srv, "POST", "/v1/dags/"+dagID+"/nodes", model.DAGAddNodeRequest{
		Name: "train", Command: "echo train",
	})
	if w2.Code != http.StatusCreated {
		t.Fatalf("add node: expected 201, got %d", w2.Code)
	}
	var n2 model.DAGAddNodeResponse
	decodeJSON(t, w2, &n2)

	// Add dependency: preprocess → train
	wd := doRequest(t, srv, "POST", "/v1/dags/"+dagID+"/dependencies", model.DAGAddDependencyRequest{
		SourceID: n1.NodeID,
		TargetID: n2.NodeID,
		Type:     "sequential",
	})
	if wd.Code != http.StatusCreated {
		t.Fatalf("add dep: expected 201, got %d: %s", wd.Code, wd.Body.String())
	}

	// Get DAG
	wg := doRequest(t, srv, "GET", "/v1/dags/"+dagID, nil)
	if wg.Code != http.StatusOK {
		t.Fatalf("get DAG: expected 200, got %d", wg.Code)
	}
	var dag model.DAG
	decodeJSON(t, wg, &dag)
	if len(dag.Nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(dag.Nodes))
	}
	if len(dag.Dependencies) != 1 {
		t.Fatalf("expected 1 dep, got %d", len(dag.Dependencies))
	}

	// Execute (dry run)
	we := doRequest(t, srv, "POST", "/v1/dags/"+dagID+"/execute", model.DAGExecuteRequest{
		DryRun: true,
	})
	if we.Code != http.StatusOK {
		t.Fatalf("execute: expected 200, got %d: %s", we.Code, we.Body.String())
	}
	var execResp model.DAGExecuteResponse
	decodeJSON(t, we, &execResp)
	if len(execResp.NodeOrder) != 2 {
		t.Fatalf("expected 2 nodes in order, got %d", len(execResp.NodeOrder))
	}

	// List DAGs
	wdl := doRequest(t, srv, "GET", "/v1/dags", nil)
	if wdl.Code != http.StatusOK {
		t.Fatalf("list DAGs: expected 200, got %d", wdl.Code)
	}
}

// TestIntegrationEdgeDeployLifecycle exercises:
// edge register → deploy rule → deploy trigger → deploy status (both paths)
func TestIntegrationEdgeDeployLifecycle(t *testing.T) {
	srv := newTestServer(t)

	// 1. Register edge
	we := doRequest(t, srv, "POST", "/v1/edges/register", model.EdgeRegisterRequest{
		Name:    "jetson-1",
		Tags:    []string{"onnx", "arm64"},
		Arch:    "arm64",
		Runtime: "onnx",
		Storage: 32,
	})
	if we.Code != http.StatusCreated {
		t.Fatalf("register edge: expected 201, got %d", we.Code)
	}
	var edgeResp model.EdgeRegisterResponse
	decodeJSON(t, we, &edgeResp)

	// 2. Edge heartbeat
	wh := doRequest(t, srv, "POST", "/v1/edges/heartbeat", model.EdgeHeartbeatRequest{
		EdgeID: edgeResp.EdgeID,
	})
	if wh.Code != http.StatusOK {
		t.Fatalf("edge heartbeat: expected 200, got %d", wh.Code)
	}

	// 3. List edges
	wl := doRequest(t, srv, "GET", "/v1/edges", nil)
	if wl.Code != http.StatusOK {
		t.Fatalf("list edges: expected 200, got %d", wl.Code)
	}

	// 4. Get edge by ID
	wg := doRequest(t, srv, "GET", "/v1/edges/"+edgeResp.EdgeID, nil)
	if wg.Code != http.StatusOK {
		t.Fatalf("get edge: expected 200, got %d", wg.Code)
	}

	// 5. Create deploy rule
	wr := doRequest(t, srv, "POST", "/v1/deploy/rules", model.DeployRuleCreateRequest{
		Name:            "auto-deploy-onnx",
		Trigger:         "job_tag:production",
		EdgeFilter:      "tag:onnx",
		ArtifactPattern: "outputs/*.onnx",
		PostCommand:     "systemctl restart inference",
	})
	if wr.Code != http.StatusCreated {
		t.Fatalf("create rule: expected 201, got %d: %s", wr.Code, wr.Body.String())
	}
	var ruleResp model.DeployRuleCreateResponse
	decodeJSON(t, wr, &ruleResp)

	// 6. List deploy rules
	wrl := doRequest(t, srv, "GET", "/v1/deploy/rules", nil)
	if wrl.Code != http.StatusOK {
		t.Fatalf("list rules: expected 200, got %d", wrl.Code)
	}

	// 7. Submit a job for deployment
	wj := doRequest(t, srv, "POST", "/v1/jobs/submit", model.JobSubmitRequest{
		Name: "deploy-source", Command: "echo train",
	})
	var jobResp model.JobSubmitResponse
	decodeJSON(t, wj, &jobResp)

	// 8. Trigger deployment
	wd := doRequest(t, srv, "POST", "/v1/deploy/trigger", model.DeployTriggerRequest{
		JobID:   jobResp.JobID,
		EdgeIDs: []string{edgeResp.EdgeID},
	})
	if wd.Code != http.StatusCreated {
		t.Fatalf("trigger deploy: expected 201, got %d: %s", wd.Code, wd.Body.String())
	}
	var deployResp model.DeployTriggerResponse
	decodeJSON(t, wd, &deployResp)
	if deployResp.TargetCount != 1 {
		t.Fatalf("expected 1 target, got %d", deployResp.TargetCount)
	}

	// 9. Get deploy status via original path
	ws1 := doRequest(t, srv, "GET", "/v1/deploy/"+deployResp.DeployID, nil)
	if ws1.Code != http.StatusOK {
		t.Fatalf("deploy status (original): expected 200, got %d", ws1.Code)
	}
	var d1 model.Deployment
	decodeJSON(t, ws1, &d1)

	// 10. Get deploy status via hub.Client path (/status suffix)
	ws2 := doRequest(t, srv, "GET", "/v1/deploy/"+deployResp.DeployID+"/status", nil)
	if ws2.Code != http.StatusOK {
		t.Fatalf("deploy status (hub.Client): expected 200, got %d: %s", ws2.Code, ws2.Body.String())
	}
	var d2 model.Deployment
	decodeJSON(t, ws2, &d2)

	if d1.ID != d2.ID || d1.Status != d2.Status {
		t.Fatalf("deploy status mismatch between paths")
	}

	// 11. Delete deploy rule
	wdr := doRequest(t, srv, "DELETE", "/v1/deploy/rules/"+ruleResp.RuleID, nil)
	if wdr.Code != http.StatusOK {
		t.Fatalf("delete rule: expected 200, got %d", wdr.Code)
	}

	// 12. Delete edge
	wde := doRequest(t, srv, "DELETE", "/v1/edges/"+edgeResp.EdgeID, nil)
	if wde.Code != http.StatusOK {
		t.Fatalf("delete edge: expected 200, got %d", wde.Code)
	}
}

// TestIntegrationQueueStats verifies stats across multiple job states.
func TestIntegrationQueueStats(t *testing.T) {
	srv := newTestServer(t)

	// Submit 3 jobs
	for i := 0; i < 3; i++ {
		doRequest(t, srv, "POST", "/v1/jobs/submit", model.JobSubmitRequest{
			Name: "stats-job", Command: "echo",
		})
	}

	// Cancel 1
	wl := doRequest(t, srv, "GET", "/v1/jobs?limit=1", nil)
	var listResp struct {
		Jobs []model.Job `json:"jobs"`
	}
	decodeJSON(t, wl, &listResp)
	doRequest(t, srv, "POST", "/v1/jobs/"+listResp.Jobs[0].ID+"/cancel", nil)

	// Check stats
	ws := doRequest(t, srv, "GET", "/v1/stats/queue", nil)
	if ws.Code != http.StatusOK {
		t.Fatalf("stats: expected 200, got %d", ws.Code)
	}
	var stats model.QueueStats
	decodeJSON(t, ws, &stats)
	if stats.Queued != 2 {
		t.Fatalf("expected 2 queued, got %d", stats.Queued)
	}
	if stats.Cancelled != 1 {
		t.Fatalf("expected 1 cancelled, got %d", stats.Cancelled)
	}
}

// TestIntegrationDAGFromYAML exercises YAML-based DAG creation.
func TestIntegrationDAGFromYAML(t *testing.T) {
	srv := newTestServer(t)

	yamlContent := `
name: yaml-pipeline
description: YAML-defined pipeline
tags: [test]
nodes:
  - name: step1
    command: echo step1
  - name: step2
    command: echo step2
dependencies:
  - source: step1
    target: step2
    type: sequential
`
	w := doRequest(t, srv, "POST", "/v1/dags/from-yaml", model.DAGFromYAMLRequest{
		YAMLContent: yamlContent,
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("from-yaml: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var resp model.DAGCreateResponse
	decodeJSON(t, w, &resp)
	if resp.DAGID == "" {
		t.Fatal("dag_id should not be empty")
	}

	// Verify DAG was created properly
	wg := doRequest(t, srv, "GET", "/v1/dags/"+resp.DAGID, nil)
	if wg.Code != http.StatusOK {
		t.Fatalf("get DAG: expected 200, got %d", wg.Code)
	}
	var dag model.DAG
	decodeJSON(t, wg, &dag)
	if dag.Name != "yaml-pipeline" {
		t.Fatalf("name mismatch: %s", dag.Name)
	}
	if len(dag.Nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(dag.Nodes))
	}
}

// TestIntegrationJobCancelFlow exercises: submit → acquire → cancel → verify
func TestIntegrationJobCancelFlow(t *testing.T) {
	srv := newTestServer(t)

	// Submit
	w := doRequest(t, srv, "POST", "/v1/jobs/submit", model.JobSubmitRequest{
		Name: "cancel-flow", Command: "sleep 1000",
	})
	var submitResp model.JobSubmitResponse
	decodeJSON(t, w, &submitResp)

	// Register worker + acquire
	wr := doRequest(t, srv, "POST", "/v1/workers/register", model.WorkerRegisterRequest{Hostname: "w1"})
	var regResp model.WorkerRegisterResponse
	decodeJSON(t, wr, &regResp)

	doRequest(t, srv, "POST", "/v1/leases/acquire", model.LeaseAcquireRequest{
		WorkerID: regResp.WorkerID,
	})

	// Verify RUNNING
	wg := doRequest(t, srv, "GET", "/v1/jobs/"+submitResp.JobID, nil)
	var job model.Job
	decodeJSON(t, wg, &job)
	if job.Status != model.StatusRunning {
		t.Fatalf("expected RUNNING, got %s", job.Status)
	}

	// Cancel
	wc := doRequest(t, srv, "POST", "/v1/jobs/"+submitResp.JobID+"/cancel", nil)
	if wc.Code != http.StatusOK {
		t.Fatalf("cancel: expected 200, got %d", wc.Code)
	}

	// Verify CANCELLED
	wg2 := doRequest(t, srv, "GET", "/v1/jobs/"+submitResp.JobID, nil)
	var job2 model.Job
	decodeJSON(t, wg2, &job2)
	if job2.Status != model.StatusCancelled {
		t.Fatalf("expected CANCELLED, got %s", job2.Status)
	}
}
