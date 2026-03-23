package hub

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// =========================================================================
// topologicalSort (pure function — no HTTP)
// =========================================================================

func TestTopologicalSort_Linear(t *testing.T) {
	// A → B → C
	nodes := []string{"A", "B", "C"}
	deps := map[string][]string{
		"B": {"A"},
		"C": {"B"},
	}
	order, err := topologicalSort(nodes, deps)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(order) != 3 {
		t.Fatalf("len = %d, want 3", len(order))
	}
	// A must come before B, B before C
	pos := map[string]int{}
	for i, n := range order {
		pos[n] = i
	}
	if pos["A"] >= pos["B"] || pos["B"] >= pos["C"] {
		t.Errorf("order = %v, want A before B before C", order)
	}
}

func TestTopologicalSort_Diamond(t *testing.T) {
	// A → B, A → C, B → D, C → D
	nodes := []string{"A", "B", "C", "D"}
	deps := map[string][]string{
		"B": {"A"},
		"C": {"A"},
		"D": {"B", "C"},
	}
	order, err := topologicalSort(nodes, deps)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(order) != 4 {
		t.Fatalf("len = %d, want 4", len(order))
	}
	pos := map[string]int{}
	for i, n := range order {
		pos[n] = i
	}
	if pos["A"] >= pos["B"] || pos["A"] >= pos["C"] {
		t.Errorf("A must precede B and C: %v", order)
	}
	if pos["B"] >= pos["D"] || pos["C"] >= pos["D"] {
		t.Errorf("B and C must precede D: %v", order)
	}
}

func TestTopologicalSort_NoEdges(t *testing.T) {
	nodes := []string{"X", "Y", "Z"}
	order, err := topologicalSort(nodes, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(order) != 3 {
		t.Fatalf("len = %d, want 3", len(order))
	}
}

func TestTopologicalSort_Cycle(t *testing.T) {
	// A → B → C → A  (cycle)
	nodes := []string{"A", "B", "C"}
	deps := map[string][]string{
		"B": {"A"},
		"C": {"B"},
		"A": {"C"},
	}
	_, err := topologicalSort(nodes, deps)
	if err == nil {
		t.Fatal("expected cycle error, got nil")
	}
}

func TestTopologicalSort_SingleNode(t *testing.T) {
	order, err := topologicalSort([]string{"solo"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(order) != 1 || order[0] != "solo" {
		t.Errorf("order = %v", order)
	}
}

// =========================================================================
// ExecuteDAG with topological sort + root node submission
// =========================================================================

func TestExecuteDAG_WithRootNodeSubmission(t *testing.T) {
	submittedJobs := []map[string]any{}
	patchedNodes := []string{}

	mux := http.NewServeMux()

	// GET hub_dag_nodes
	mux.HandleFunc("/rest/v1/hub_dag_nodes", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			jsonResponse(w, []map[string]any{
				{"id": "node-A", "name": "preprocess", "command": "python preprocess.py", "gpu_count": 0},
				{"id": "node-B", "name": "train", "command": "python train.py", "gpu_count": 1},
			})
			return
		}
		// PATCH node job_id
		if r.Method == "PATCH" {
			patchedNodes = append(patchedNodes, r.URL.RawQuery)
			w.WriteHeader(200)
			w.Write([]byte(`[]`))
			return
		}
		w.WriteHeader(405)
	})

	// GET hub_dag_dependencies
	mux.HandleFunc("/rest/v1/hub_dag_dependencies", func(w http.ResponseWriter, r *http.Request) {
		// node-B depends on node-A → only node-A is root
		jsonResponse(w, []map[string]any{
			{"source_id": "node-A", "target_id": "node-B"},
		})
	})

	// POST hub_jobs (SubmitJob)
	mux.HandleFunc("/rest/v1/hub_jobs", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("hub_jobs method = %s, want POST", r.Method)
		}
		var row map[string]any
		json.NewDecoder(r.Body).Decode(&row)
		submittedJobs = append(submittedJobs, row)
		jsonResponse(w, []map[string]any{
			{"id": "job-root-1", "status": "QUEUED"},
		})
	})

	// PATCH hub_dag_nodes and PATCH hub_dags
	mux.HandleFunc("/rest/v1/hub_dags", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PATCH" {
			t.Errorf("hub_dags method = %s, want PATCH", r.Method)
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if body["status"] != "running" {
			t.Errorf("dag status = %v, want running", body["status"])
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
		t.Errorf("Status = %q, want running", resp.Status)
	}
	if len(resp.NodeOrder) != 2 {
		t.Errorf("NodeOrder len = %d, want 2", len(resp.NodeOrder))
	}
	// Only root node (node-A) should be submitted
	if len(submittedJobs) != 1 {
		t.Errorf("submitted jobs = %d, want 1 (root only)", len(submittedJobs))
	}
	if submittedJobs[0]["name"] != "preprocess" {
		t.Errorf("submitted job name = %v, want preprocess", submittedJobs[0]["name"])
	}
	// node-A patch should have happened
	if len(patchedNodes) != 1 || !strings.Contains(patchedNodes[0], "node-A") {
		t.Errorf("patched nodes = %v, want [id=eq.node-A]", patchedNodes)
	}
}

func TestExecuteDAG_CycleReturnsError(t *testing.T) {
	mux := http.NewServeMux()

	// Return nodes forming a cycle
	mux.HandleFunc("/rest/v1/hub_dag_nodes", func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, []map[string]any{
			{"id": "n1", "name": "a", "command": "echo a"},
			{"id": "n2", "name": "b", "command": "echo b"},
		})
	})
	mux.HandleFunc("/rest/v1/hub_dag_dependencies", func(w http.ResponseWriter, r *http.Request) {
		// n1→n2 and n2→n1 = cycle
		jsonResponse(w, []map[string]any{
			{"source_id": "n1", "target_id": "n2"},
			{"source_id": "n2", "target_id": "n1"},
		})
	})

	client, _ := newTestServer(t, mux)

	_, err := client.ExecuteDAG("dag-cycle", false)
	if err == nil {
		t.Fatal("expected cycle error, got nil")
	}
	if !strings.Contains(err.Error(), "cycle") {
		t.Errorf("error = %q, want cycle mention", err.Error())
	}
}

func TestExecuteDAG_DryRunReturnsNodeOrder(t *testing.T) {
	mux := http.NewServeMux()

	mux.HandleFunc("/rest/v1/hub_dag_nodes", func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, []map[string]any{
			{"id": "n1", "name": "step1", "command": "echo 1"},
			{"id": "n2", "name": "step2", "command": "echo 2"},
		})
	})
	mux.HandleFunc("/rest/v1/hub_dag_dependencies", func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, []map[string]any{
			{"source_id": "n1", "target_id": "n2"},
		})
	})
	mux.HandleFunc("/rest/v1/hub_dags", func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, []map[string]any{
			{"id": "dag-dr", "status": "pending"},
		})
	})

	client, _ := newTestServer(t, mux)

	resp, err := client.ExecuteDAG("dag-dr", true)
	if err != nil {
		t.Fatalf("ExecuteDAG dry run: %v", err)
	}
	if resp.Validation != "valid" {
		t.Errorf("Validation = %q", resp.Validation)
	}
	if len(resp.NodeOrder) != 2 {
		t.Errorf("NodeOrder len = %d, want 2", len(resp.NodeOrder))
	}
}

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
		if r.Method == "PATCH" {
			w.WriteHeader(200)
			w.Write([]byte(`[]`))
			return
		}
		// GET for validation
		jsonResponse(w, []map[string]any{
			{"id": "dag-1", "name": "test", "status": "pending"},
		})
	})
	mux.HandleFunc("/rest/v1/hub_dag_nodes", func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, []map[string]any{
			{"id": "node-1", "dag_id": "dag-1", "name": "root", "job_template": `{"command":"echo hi"}`, "max_retries": 3},
		})
	})
	mux.HandleFunc("/rest/v1/hub_dag_dependencies", func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, []map[string]any{})
	})
	mux.HandleFunc("/rest/v1/hub_jobs", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			jsonResponse(w, []map[string]any{{"id": "job-dag-1", "status": "QUEUED"}})
			return
		}
		jsonResponse(w, []map[string]any{})
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
		jsonResponse(w, []map[string]any{
			{"id": "dag-2", "name": "test", "status": "pending"},
		})
	})
	mux.HandleFunc("/rest/v1/hub_dag_nodes", func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, []map[string]any{
			{"id": "node-1", "dag_id": "dag-2", "name": "root", "job_template": `{}`, "max_retries": 3},
		})
	})
	mux.HandleFunc("/rest/v1/hub_dag_dependencies", func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, []map[string]any{})
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

	mux.HandleFunc("/rest/v1/hub_jobs", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			jsonResponse(w, []map[string]any{{"id": "job-full-1", "status": "QUEUED"}})
			return
		}
		jsonResponse(w, []map[string]any{})
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
