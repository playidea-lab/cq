package hub

import (
	"encoding/json"
	"net/http"
	"testing"
)

// =========================================================================
// CreateDAG
// =========================================================================

func TestCreateDAG(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/dags", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("method = %s, want POST", r.Method)
		}
		var req DAGCreateRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.Name != "train-pipeline" {
			t.Errorf("name = %q", req.Name)
		}
		if req.Description != "End-to-end training" {
			t.Errorf("description = %q", req.Description)
		}
		jsonResponse(w, DAGCreateResponse{DAGID: "dag-1", Status: "pending"})
	})
	client, _ := newTestServer(t, mux)

	resp, err := client.CreateDAG(&DAGCreateRequest{
		Name:        "train-pipeline",
		Description: "End-to-end training",
		Tags:        []string{"ml", "train"},
	})
	if err != nil {
		t.Fatalf("CreateDAG: %v", err)
	}
	if resp.DAGID != "dag-1" {
		t.Errorf("DAGID = %q", resp.DAGID)
	}
	if resp.Status != "pending" {
		t.Errorf("Status = %q", resp.Status)
	}
}

// =========================================================================
// AddDAGNode
// =========================================================================

func TestAddDAGNode(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/dags/dag-1/nodes", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("method = %s, want POST", r.Method)
		}
		var req DAGAddNodeRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.Name != "preprocess" {
			t.Errorf("name = %q", req.Name)
		}
		if req.Command != "python preprocess.py" {
			t.Errorf("command = %q", req.Command)
		}
		if req.GPUCount != 1 {
			t.Errorf("gpu_count = %d", req.GPUCount)
		}
		jsonResponse(w, DAGAddNodeResponse{NodeID: "node-1", Name: "preprocess"})
	})
	client, _ := newTestServer(t, mux)

	resp, err := client.AddDAGNode("dag-1", &DAGAddNodeRequest{
		Name:     "preprocess",
		Command:  "python preprocess.py",
		GPUCount: 1,
	})
	if err != nil {
		t.Fatalf("AddDAGNode: %v", err)
	}
	if resp.NodeID != "node-1" {
		t.Errorf("NodeID = %q", resp.NodeID)
	}
}

// =========================================================================
// AddDAGDependency
// =========================================================================

func TestAddDAGDependency(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/dags/dag-1/dependencies", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("method = %s, want POST", r.Method)
		}
		var req DAGAddDependencyRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.SourceID != "node-1" {
			t.Errorf("source_id = %q", req.SourceID)
		}
		if req.TargetID != "node-2" {
			t.Errorf("target_id = %q", req.TargetID)
		}
		if req.Type != "sequential" {
			t.Errorf("type = %q", req.Type)
		}
		w.WriteHeader(200)
		w.Write([]byte(`{"ok":true}`))
	})
	client, _ := newTestServer(t, mux)

	err := client.AddDAGDependency("dag-1", &DAGAddDependencyRequest{
		SourceID: "node-1",
		TargetID: "node-2",
		Type:     "sequential",
	})
	if err != nil {
		t.Fatalf("AddDAGDependency: %v", err)
	}
}

// =========================================================================
// ExecuteDAG
// =========================================================================

func TestExecuteDAG(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/dags/dag-1/execute", func(w http.ResponseWriter, r *http.Request) {
		var req DAGExecuteRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.DryRun {
			t.Error("expected dry_run=false")
		}
		jsonResponse(w, DAGExecuteResponse{
			DAGID:     "dag-1",
			Status:    "running",
			NodeOrder: []string{"preprocess", "train", "eval"},
		})
	})
	client, _ := newTestServer(t, mux)

	resp, err := client.ExecuteDAG("dag-1", false)
	if err != nil {
		t.Fatalf("ExecuteDAG: %v", err)
	}
	if resp.Status != "running" {
		t.Errorf("Status = %q", resp.Status)
	}
	if len(resp.NodeOrder) != 3 {
		t.Errorf("NodeOrder len = %d", len(resp.NodeOrder))
	}
}

func TestExecuteDAG_DryRun(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/dags/dag-2/execute", func(w http.ResponseWriter, r *http.Request) {
		var req DAGExecuteRequest
		json.NewDecoder(r.Body).Decode(&req)
		if !req.DryRun {
			t.Error("expected dry_run=true")
		}
		jsonResponse(w, DAGExecuteResponse{
			DAGID:      "dag-2",
			Status:     "pending",
			Validation: "valid",
			NodeOrder:  []string{"a", "b"},
		})
	})
	client, _ := newTestServer(t, mux)

	resp, err := client.ExecuteDAG("dag-2", true)
	if err != nil {
		t.Fatalf("ExecuteDAG dry_run: %v", err)
	}
	if resp.Validation != "valid" {
		t.Errorf("Validation = %q", resp.Validation)
	}
}

// =========================================================================
// GetDAGStatus
// =========================================================================

func TestGetDAGStatus(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/dags/dag-1/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("method = %s, want GET", r.Method)
		}
		exitCode := 0
		jsonResponse(w, DAG{
			ID:     "dag-1",
			Name:   "train-pipeline",
			Status: "running",
			Nodes: []DAGNode{
				{ID: "node-1", Name: "preprocess", Status: "succeeded", ExitCode: &exitCode},
				{ID: "node-2", Name: "train", Status: "running"},
				{ID: "node-3", Name: "eval", Status: "pending"},
			},
			Dependencies: []DAGDependency{
				{SourceID: "node-1", TargetID: "node-2", Type: "sequential"},
				{SourceID: "node-2", TargetID: "node-3", Type: "sequential"},
			},
		})
	})
	client, _ := newTestServer(t, mux)

	dag, err := client.GetDAGStatus("dag-1")
	if err != nil {
		t.Fatalf("GetDAGStatus: %v", err)
	}
	if dag.Status != "running" {
		t.Errorf("Status = %q", dag.Status)
	}
	if len(dag.Nodes) != 3 {
		t.Fatalf("Nodes = %d, want 3", len(dag.Nodes))
	}
	if dag.Nodes[0].Status != "succeeded" {
		t.Errorf("node[0].Status = %q", dag.Nodes[0].Status)
	}
	if dag.Nodes[1].Status != "running" {
		t.Errorf("node[1].Status = %q", dag.Nodes[1].Status)
	}
	if len(dag.Dependencies) != 2 {
		t.Errorf("Dependencies = %d", len(dag.Dependencies))
	}
}

// =========================================================================
// ListDAGs
// =========================================================================

func TestListDAGs(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/dags", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("method = %s, want GET", r.Method)
		}
		status := r.URL.Query().Get("status")
		if status != "running" {
			t.Errorf("status = %q, want running", status)
		}
		limit := r.URL.Query().Get("limit")
		if limit != "10" {
			t.Errorf("limit = %q, want 10", limit)
		}
		jsonResponse(w, []DAG{
			{ID: "dag-1", Name: "pipeline-1", Status: "running"},
			{ID: "dag-2", Name: "pipeline-2", Status: "running"},
		})
	})
	client, _ := newTestServer(t, mux)

	dags, err := client.ListDAGs("running", 10)
	if err != nil {
		t.Fatalf("ListDAGs: %v", err)
	}
	if len(dags) != 2 {
		t.Fatalf("got %d dags, want 2", len(dags))
	}
	if dags[0].ID != "dag-1" {
		t.Errorf("dags[0].ID = %q", dags[0].ID)
	}
}

func TestListDAGs_NoFilter(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/dags", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery != "" {
			t.Errorf("expected no query params, got %q", r.URL.RawQuery)
		}
		jsonResponse(w, []DAG{})
	})
	client, _ := newTestServer(t, mux)

	dags, err := client.ListDAGs("", 0)
	if err != nil {
		t.Fatalf("ListDAGs: %v", err)
	}
	if len(dags) != 0 {
		t.Errorf("got %d dags, want 0", len(dags))
	}
}

// =========================================================================
// CreateDAGFromYAML
// =========================================================================

func TestCreateDAGFromYAML(t *testing.T) {
	yaml := `
name: resnet-cifar10
description: ResNet training pipeline
nodes:
  - name: preprocess
    command: python preprocess.py
  - name: train
    command: python train.py
    gpu_count: 1
dependencies:
  - source: preprocess
    target: train
`

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/dags/from-yaml", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("method = %s, want POST", r.Method)
		}
		var req DAGFromYAMLRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.YAMLContent == "" {
			t.Error("empty yaml_content")
		}
		jsonResponse(w, DAG{
			ID:     "dag-yaml-1",
			Name:   "resnet-cifar10",
			Status: "pending",
			Nodes: []DAGNode{
				{ID: "n1", Name: "preprocess", Command: "python preprocess.py"},
				{ID: "n2", Name: "train", Command: "python train.py", GPUCount: 1},
			},
			Dependencies: []DAGDependency{
				{SourceID: "n1", TargetID: "n2", Type: "sequential"},
			},
		})
	})
	client, _ := newTestServer(t, mux)

	dag, err := client.CreateDAGFromYAML(yaml)
	if err != nil {
		t.Fatalf("CreateDAGFromYAML: %v", err)
	}
	if dag.ID != "dag-yaml-1" {
		t.Errorf("ID = %q", dag.ID)
	}
	if len(dag.Nodes) != 2 {
		t.Errorf("Nodes = %d", len(dag.Nodes))
	}
	if len(dag.Dependencies) != 1 {
		t.Errorf("Dependencies = %d", len(dag.Dependencies))
	}
}

// =========================================================================
// Error cases
// =========================================================================

func TestCreateDAG_ServerError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/dags", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		w.Write([]byte("internal error"))
	})
	client, _ := newTestServer(t, mux)

	_, err := client.CreateDAG(&DAGCreateRequest{Name: "test"})
	if err == nil {
		t.Error("expected error on 500")
	}
}

func TestExecuteDAG_ValidationError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/dags/dag-bad/execute", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		w.Write([]byte(`{"error":"cycle detected in DAG"}`))
	})
	client, _ := newTestServer(t, mux)

	_, err := client.ExecuteDAG("dag-bad", false)
	if err == nil {
		t.Error("expected error on 400")
	}
}

func TestGetDAGStatus_NotFound(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/dags/dag-missing/status", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		w.Write([]byte(`{"error":"DAG not found"}`))
	})
	client, _ := newTestServer(t, mux)

	_, err := client.GetDAGStatus("dag-missing")
	if err == nil {
		t.Error("expected error on 404")
	}
}

// =========================================================================
// Full pipeline flow
// =========================================================================

func TestDAG_FullPipeline(t *testing.T) {
	mux := http.NewServeMux()

	// Create
	mux.HandleFunc("/v1/dags", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			jsonResponse(w, DAGCreateResponse{DAGID: "dag-full", Status: "pending"})
		}
	})

	// Add nodes
	mux.HandleFunc("/v1/dags/dag-full/nodes", func(w http.ResponseWriter, r *http.Request) {
		var req DAGAddNodeRequest
		json.NewDecoder(r.Body).Decode(&req)
		jsonResponse(w, DAGAddNodeResponse{NodeID: "n-" + req.Name, Name: req.Name})
	})

	// Add dependency
	mux.HandleFunc("/v1/dags/dag-full/dependencies", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(`{"ok":true}`))
	})

	// Execute
	mux.HandleFunc("/v1/dags/dag-full/execute", func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, DAGExecuteResponse{
			DAGID:     "dag-full",
			Status:    "running",
			NodeOrder: []string{"preprocess", "train", "eval"},
		})
	})

	// Status
	mux.HandleFunc("/v1/dags/dag-full/status", func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, DAG{
			ID:     "dag-full",
			Status: "completed",
			Nodes: []DAGNode{
				{ID: "n-preprocess", Name: "preprocess", Status: "succeeded"},
				{ID: "n-train", Name: "train", Status: "succeeded"},
				{ID: "n-eval", Name: "eval", Status: "succeeded"},
			},
		})
	})

	client, _ := newTestServer(t, mux)

	// 1. Create DAG
	dag, err := client.CreateDAG(&DAGCreateRequest{Name: "full-pipeline"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// 2. Add nodes
	n1, err := client.AddDAGNode(dag.DAGID, &DAGAddNodeRequest{
		Name: "preprocess", Command: "python preprocess.py",
	})
	if err != nil {
		t.Fatalf("add node preprocess: %v", err)
	}

	n2, err := client.AddDAGNode(dag.DAGID, &DAGAddNodeRequest{
		Name: "train", Command: "python train.py", GPUCount: 1,
	})
	if err != nil {
		t.Fatalf("add node train: %v", err)
	}

	n3, err := client.AddDAGNode(dag.DAGID, &DAGAddNodeRequest{
		Name: "eval", Command: "python eval.py",
	})
	if err != nil {
		t.Fatalf("add node eval: %v", err)
	}

	// 3. Add dependencies: preprocess → train → eval
	if err := client.AddDAGDependency(dag.DAGID, &DAGAddDependencyRequest{
		SourceID: n1.NodeID, TargetID: n2.NodeID, Type: "sequential",
	}); err != nil {
		t.Fatalf("dep 1→2: %v", err)
	}
	if err := client.AddDAGDependency(dag.DAGID, &DAGAddDependencyRequest{
		SourceID: n2.NodeID, TargetID: n3.NodeID, Type: "sequential",
	}); err != nil {
		t.Fatalf("dep 2→3: %v", err)
	}

	// 4. Execute
	exec, err := client.ExecuteDAG(dag.DAGID, false)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if exec.Status != "running" {
		t.Errorf("exec status = %q", exec.Status)
	}

	// 5. Check status
	status, err := client.GetDAGStatus(dag.DAGID)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if status.Status != "completed" {
		t.Errorf("final status = %q", status.Status)
	}
	for _, node := range status.Nodes {
		if node.Status != "succeeded" {
			t.Errorf("node %s status = %q", node.Name, node.Status)
		}
	}
}
