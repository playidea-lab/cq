package hub

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// =========================================================================
// CreateDAG
// =========================================================================

func TestCreateDAG(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/rest/v1/hub_dags", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("method = %s, want POST", r.Method)
		}
		var row map[string]any
		json.NewDecoder(r.Body).Decode(&row)
		if row["name"] != "train-pipeline" {
			t.Errorf("name = %v", row["name"])
		}
		if row["description"] != "End-to-end training" {
			t.Errorf("description = %v", row["description"])
		}
		jsonResponse(w, []map[string]any{
			{"id": "dag-1", "name": "train-pipeline", "status": "pending"},
		})
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
	mux.HandleFunc("/rest/v1/hub_dag_nodes", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("method = %s, want POST", r.Method)
		}
		var row map[string]any
		json.NewDecoder(r.Body).Decode(&row)
		if row["name"] != "preprocess" {
			t.Errorf("name = %v", row["name"])
		}
		if row["command"] != "python preprocess.py" {
			t.Errorf("command = %v", row["command"])
		}
		gpuCount, _ := row["gpu_count"].(float64)
		if int(gpuCount) != 1 {
			t.Errorf("gpu_count = %v", row["gpu_count"])
		}
		jsonResponse(w, []map[string]any{
			{"id": "node-1", "name": "preprocess"},
		})
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
	mux.HandleFunc("/rest/v1/hub_dag_dependencies", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("method = %s, want POST", r.Method)
		}
		var row map[string]any
		json.NewDecoder(r.Body).Decode(&row)
		if row["source_id"] != "node-1" {
			t.Errorf("source_id = %v", row["source_id"])
		}
		if row["target_id"] != "node-2" {
			t.Errorf("target_id = %v", row["target_id"])
		}
		if row["dep_type"] != "sequential" {
			t.Errorf("dep_type = %v", row["dep_type"])
		}
		w.WriteHeader(200)
		w.Write([]byte(`[]`))
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
	mux.HandleFunc("/rest/v1/hub_dags", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PATCH" {
			t.Errorf("method = %s, want PATCH", r.Method)
		}
		if !strings.Contains(r.URL.RawQuery, "id=eq.dag-1") {
			t.Errorf("query = %q, want id=eq.dag-1", r.URL.RawQuery)
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if body["status"] != "running" {
			t.Errorf("status = %v", body["status"])
		}
		w.WriteHeader(200)
		w.Write([]byte(`[]`))
	})
	client, _ := newTestServer(t, mux)

	resp, err := client.ExecuteDAG("dag-1", false)
	if err != nil {
		t.Fatalf("ExecuteDAG: %v", err)
	}
	if resp.Status != "running" {
		t.Errorf("Status = %q", resp.Status)
	}
}

func TestExecuteDAG_DryRun(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/rest/v1/hub_dags", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("method = %s, want GET", r.Method)
		}
		jsonResponse(w, []map[string]any{
			{"id": "dag-2", "name": "test", "status": "pending"},
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
	exitCode := 0

	mux.HandleFunc("/rest/v1/hub_dags", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("method = %s, want GET", r.Method)
		}
		jsonResponse(w, []map[string]any{
			{"id": "dag-1", "name": "train-pipeline", "status": "running"},
		})
	})
	mux.HandleFunc("/rest/v1/hub_dag_nodes", func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, []map[string]any{
			{"id": "node-1", "name": "preprocess", "status": "succeeded", "exit_code": exitCode},
			{"id": "node-2", "name": "train", "status": "running"},
			{"id": "node-3", "name": "eval", "status": "pending"},
		})
	})
	mux.HandleFunc("/rest/v1/hub_dag_dependencies", func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, []map[string]any{
			{"source_id": "node-1", "target_id": "node-2", "dep_type": "sequential"},
			{"source_id": "node-2", "target_id": "node-3", "dep_type": "sequential"},
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
	mux.HandleFunc("/rest/v1/hub_dags", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("method = %s, want GET", r.Method)
		}
		status := r.URL.Query().Get("status")
		if status != "eq.running" {
			t.Errorf("status filter = %q, want eq.running", status)
		}
		jsonResponse(w, []map[string]any{
			{"id": "dag-1", "name": "pipeline-1", "status": "running"},
			{"id": "dag-2", "name": "pipeline-2", "status": "running"},
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
	mux.HandleFunc("/rest/v1/hub_dags", func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, []map[string]any{})
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
	mux.HandleFunc("/rest/v1/hub_dags", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("method = %s, want POST", r.Method)
		}
		var row map[string]any
		json.NewDecoder(r.Body).Decode(&row)
		if row["name"] != "resnet-cifar10" {
			t.Errorf("name = %v", row["name"])
		}
		jsonResponse(w, []map[string]any{
			{"id": "dag-yaml-1", "name": "resnet-cifar10", "status": "pending"},
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
}

// =========================================================================
// Error cases
// =========================================================================

func TestCreateDAG_ServerError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/rest/v1/hub_dags", func(w http.ResponseWriter, r *http.Request) {
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
	mux.HandleFunc("/rest/v1/hub_dags", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		w.Write([]byte(`{"message":"cycle detected in DAG"}`))
	})
	client, _ := newTestServer(t, mux)

	_, err := client.ExecuteDAG("dag-bad", false)
	if err == nil {
		t.Error("expected error on 400")
	}
}

func TestGetDAGStatus_NotFound(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/rest/v1/hub_dags", func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, []map[string]any{}) // empty = not found
	})
	// Also need handlers for nodes/deps even though they won't be called
	client, _ := newTestServer(t, mux)

	_, err := client.GetDAGStatus("dag-missing")
	if err == nil {
		t.Error("expected error for missing DAG")
	}
}

// =========================================================================
// Full pipeline flow
// =========================================================================

func TestDAG_FullPipeline(t *testing.T) {
	mux := http.NewServeMux()

	mux.HandleFunc("/rest/v1/hub_dags", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "POST":
			var row map[string]any
			json.NewDecoder(r.Body).Decode(&row)
			jsonResponse(w, []map[string]any{
				{"id": "dag-full", "name": row["name"], "status": "pending"},
			})
		case "GET":
			if strings.Contains(r.URL.RawQuery, "dag-full") {
				jsonResponse(w, []map[string]any{
					{"id": "dag-full", "status": "completed"},
				})
			}
		case "PATCH":
			w.WriteHeader(200)
			w.Write([]byte(`[]`))
		}
	})

	mux.HandleFunc("/rest/v1/hub_dag_nodes", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "POST":
			var row map[string]any
			json.NewDecoder(r.Body).Decode(&row)
			name, _ := row["name"].(string)
			jsonResponse(w, []map[string]any{
				{"id": "n-" + name, "name": name},
			})
		case "GET":
			jsonResponse(w, []map[string]any{
				{"id": "n-preprocess", "name": "preprocess", "status": "succeeded"},
				{"id": "n-train", "name": "train", "status": "succeeded"},
				{"id": "n-eval", "name": "eval", "status": "succeeded"},
			})
		}
	})

	mux.HandleFunc("/rest/v1/hub_dag_dependencies", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "POST":
			w.WriteHeader(200)
			w.Write([]byte(`[]`))
		case "GET":
			jsonResponse(w, []map[string]any{})
		}
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

	// 3. Add dependencies
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
