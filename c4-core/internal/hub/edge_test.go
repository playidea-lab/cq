package hub

import (
	"encoding/json"
	"net/http"
	"testing"
)

// =========================================================================
// Edge Registration
// =========================================================================

func TestRegisterEdge(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/rest/v1/hub_edges", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("method = %s, want POST", r.Method)
		}
		var row map[string]any
		json.NewDecoder(r.Body).Decode(&row)
		if row["name"] != "jetson-1" {
			t.Errorf("name = %v", row["name"])
		}
		jsonResponse(w, []map[string]any{
			{"id": "edge-001", "name": "jetson-1"},
		})
	})
	client, _ := newTestServer(t, mux)

	edgeID, err := client.RegisterEdge("jetson-1", []string{"onnx", "arm64"}, map[string]any{
		"arch":       "arm64",
		"runtime":    "onnx",
		"storage_gb": 32.0,
	})
	if err != nil {
		t.Fatalf("RegisterEdge: %v", err)
	}
	if edgeID != "edge-001" {
		t.Errorf("edgeID = %q", edgeID)
	}
}

func TestListEdges(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/rest/v1/hub_edges", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("method = %s", r.Method)
		}
		jsonResponse(w, []Edge{
			{ID: "edge-001", Name: "jetson-1", Status: "online", Tags: []string{"onnx"}, Arch: "arm64"},
			{ID: "edge-002", Name: "rpi-fleet", Status: "offline", Tags: []string{"tflite"}, Arch: "arm64"},
		})
	})
	client, _ := newTestServer(t, mux)

	edges, err := client.ListEdges()
	if err != nil {
		t.Fatalf("ListEdges: %v", err)
	}
	if len(edges) != 2 {
		t.Fatalf("got %d edges, want 2", len(edges))
	}
	if edges[0].Name != "jetson-1" {
		t.Errorf("edges[0].Name = %q", edges[0].Name)
	}
	if edges[1].Status != "offline" {
		t.Errorf("edges[1].Status = %q", edges[1].Status)
	}
}

func TestEdgeHeartbeat(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/rest/v1/hub_edges", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PATCH" {
			t.Errorf("method = %s, want PATCH", r.Method)
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if body["status"] != "online" {
			t.Errorf("status = %v", body["status"])
		}
		w.WriteHeader(200)
		w.Write([]byte(`[]`))
	})
	client, _ := newTestServer(t, mux)

	if err := client.EdgeHeartbeat("edge-001", "online"); err != nil {
		t.Fatalf("EdgeHeartbeat: %v", err)
	}
}

func TestRemoveEdge(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/rest/v1/hub_edges", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "DELETE" {
			t.Errorf("method = %s, want DELETE", r.Method)
		}
		w.WriteHeader(200)
	})
	client, _ := newTestServer(t, mux)

	if err := client.RemoveEdge("edge-001"); err != nil {
		t.Fatalf("RemoveEdge: %v", err)
	}
}

// =========================================================================
// Deploy Rules
// =========================================================================

func TestCreateDeployRule(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/rest/v1/hub_deploy_rules", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("method = %s", r.Method)
		}
		var row map[string]any
		json.NewDecoder(r.Body).Decode(&row)
		if row["trigger_expr"] != "job_tag:production" {
			t.Errorf("trigger_expr = %v", row["trigger_expr"])
		}
		if row["edge_filter"] != "tag:onnx" {
			t.Errorf("edge_filter = %v", row["edge_filter"])
		}
		if row["artifact_pattern"] != "outputs/*.onnx" {
			t.Errorf("artifact_pattern = %v", row["artifact_pattern"])
		}
		jsonResponse(w, []map[string]any{{"id": "rule-1"}})
	})
	client, _ := newTestServer(t, mux)

	resp, err := client.CreateDeployRule(&DeployRuleCreateRequest{
		Trigger:         "job_tag:production",
		EdgeFilter:      "tag:onnx",
		ArtifactPattern: "outputs/*.onnx",
		PostCommand:     "systemctl restart inference",
	})
	if err != nil {
		t.Fatalf("CreateDeployRule: %v", err)
	}
	if resp.RuleID != "rule-1" {
		t.Errorf("RuleID = %q", resp.RuleID)
	}
}

func TestListDeployRules(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/rest/v1/hub_deploy_rules", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("method = %s", r.Method)
		}
		jsonResponse(w, []map[string]any{
			{"id": "rule-1", "trigger_expr": "job_tag:production", "edge_filter": "tag:onnx", "artifact_pattern": "outputs/*.onnx", "enabled": true},
			{"id": "rule-2", "trigger_expr": "dag_complete:*", "edge_filter": "name:jetson-*", "artifact_pattern": "*.trt", "enabled": false},
		})
	})
	client, _ := newTestServer(t, mux)

	rules, err := client.ListDeployRules()
	if err != nil {
		t.Fatalf("ListDeployRules: %v", err)
	}
	if len(rules) != 2 {
		t.Fatalf("got %d rules, want 2", len(rules))
	}
	if !rules[0].Enabled {
		t.Error("rules[0] should be enabled")
	}
}

func TestDeleteDeployRule(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/rest/v1/hub_deploy_rules", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "DELETE" {
			t.Errorf("method = %s, want DELETE", r.Method)
		}
		w.WriteHeader(200)
	})
	client, _ := newTestServer(t, mux)

	if err := client.DeleteDeployRule("rule-1"); err != nil {
		t.Fatalf("DeleteDeployRule: %v", err)
	}
}

// =========================================================================
// Deploy Trigger + Status
// =========================================================================

func TestTriggerDeploy(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/rest/v1/hub_deployments", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("method = %s, want POST", r.Method)
		}
		var row map[string]any
		json.NewDecoder(r.Body).Decode(&row)
		if row["job_id"] != "job-123" {
			t.Errorf("job_id = %v", row["job_id"])
		}
		jsonResponse(w, []map[string]any{{"id": "deploy-1"}})
	})
	client, _ := newTestServer(t, mux)

	resp, err := client.TriggerDeploy(&DeployTriggerRequest{
		JobID:           "job-123",
		ArtifactPattern: "outputs/model.onnx",
		EdgeFilter:      "tag:onnx",
	})
	if err != nil {
		t.Fatalf("TriggerDeploy: %v", err)
	}
	if resp.DeployID != "deploy-1" {
		t.Errorf("DeployID = %q", resp.DeployID)
	}
}

func TestTriggerDeploy_ExplicitEdges(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/rest/v1/hub_deployments", func(w http.ResponseWriter, r *http.Request) {
		var row map[string]any
		json.NewDecoder(r.Body).Decode(&row)
		jsonResponse(w, []map[string]any{{"id": "deploy-2"}})
	})
	client, _ := newTestServer(t, mux)

	resp, err := client.TriggerDeploy(&DeployTriggerRequest{
		JobID:   "job-456",
		EdgeIDs: []string{"edge-001", "edge-002"},
	})
	if err != nil {
		t.Fatalf("TriggerDeploy: %v", err)
	}
	if resp.TargetCount != 2 {
		t.Errorf("TargetCount = %d, want 2", resp.TargetCount)
	}
}

func TestGetDeployStatus(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/rest/v1/hub_deployments", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("method = %s", r.Method)
		}
		jsonResponse(w, []Deployment{
			{ID: "deploy-1", JobID: "job-123", Status: "completed"},
		})
	})
	mux.HandleFunc("/rest/v1/hub_deploy_targets", func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, []DeployTarget{
			{EdgeID: "edge-001", EdgeName: "jetson-1", Status: "succeeded"},
			{EdgeID: "edge-002", EdgeName: "jetson-2", Status: "succeeded"},
			{EdgeID: "edge-003", EdgeName: "rpi-1", Status: "failed", Error: "disk full"},
		})
	})
	client, _ := newTestServer(t, mux)

	deploy, err := client.GetDeployStatus("deploy-1")
	if err != nil {
		t.Fatalf("GetDeployStatus: %v", err)
	}
	if deploy.Status != "completed" {
		t.Errorf("Status = %q", deploy.Status)
	}
	if len(deploy.Targets) != 3 {
		t.Fatalf("Targets = %d", len(deploy.Targets))
	}
	if deploy.Targets[2].Status != "failed" {
		t.Errorf("target[2].Status = %q", deploy.Targets[2].Status)
	}
	if deploy.Targets[2].Error != "disk full" {
		t.Errorf("target[2].Error = %q", deploy.Targets[2].Error)
	}
}

// =========================================================================
// Error cases
// =========================================================================

func TestRegisterEdge_ServerError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/rest/v1/hub_edges", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		w.Write([]byte("internal error"))
	})
	client, _ := newTestServer(t, mux)

	_, err := client.RegisterEdge("test", nil, nil)
	if err == nil {
		t.Error("expected error on 500")
	}
}

func TestRemoveEdge_NotFound(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/rest/v1/hub_edges", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
	})
	client, _ := newTestServer(t, mux)

	err := client.RemoveEdge("edge-missing")
	if err == nil {
		t.Error("expected error on 404")
	}
}

// =========================================================================
// Full E2E flow: register edge → create rule → deploy → status
// =========================================================================

func TestEdgeDeploy_FullFlow(t *testing.T) {
	mux := http.NewServeMux()

	mux.HandleFunc("/rest/v1/hub_edges", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "POST":
			jsonResponse(w, []map[string]any{{"id": "edge-e2e", "name": "jetson-factory"}})
		case "GET":
			jsonResponse(w, []Edge{
				{ID: "edge-e2e", Name: "jetson-factory", Status: "online", Tags: []string{"onnx", "arm64"}},
			})
		}
	})

	mux.HandleFunc("/rest/v1/hub_deploy_rules", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "POST":
			jsonResponse(w, []map[string]any{{"id": "rule-e2e"}})
		case "GET":
			jsonResponse(w, []map[string]any{
				{"id": "rule-e2e", "trigger_expr": "job_tag:production", "edge_filter": "tag:onnx", "artifact_pattern": "*.onnx", "enabled": true},
			})
		}
	})

	mux.HandleFunc("/rest/v1/hub_deployments", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "POST":
			jsonResponse(w, []map[string]any{{"id": "deploy-e2e"}})
		case "GET":
			jsonResponse(w, []Deployment{
				{ID: "deploy-e2e", Status: "completed"},
			})
		}
	})

	mux.HandleFunc("/rest/v1/hub_deploy_targets", func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, []DeployTarget{
			{EdgeID: "edge-e2e", EdgeName: "jetson-factory", Status: "succeeded"},
		})
	})

	client, _ := newTestServer(t, mux)

	// 1. Register edge
	edgeID, err := client.RegisterEdge("jetson-factory", []string{"onnx", "arm64"}, map[string]any{"arch": "arm64"})
	if err != nil {
		t.Fatalf("register edge: %v", err)
	}
	if edgeID != "edge-e2e" {
		t.Errorf("edgeID = %q", edgeID)
	}

	// 2. Verify edge visible
	edges, err := client.ListEdges()
	if err != nil {
		t.Fatalf("list edges: %v", err)
	}
	if len(edges) != 1 || edges[0].Status != "online" {
		t.Errorf("unexpected edges: %+v", edges)
	}

	// 3. Create deploy rule
	rule, err := client.CreateDeployRule(&DeployRuleCreateRequest{
		Trigger:         "job_tag:production",
		EdgeFilter:      "tag:onnx",
		ArtifactPattern: "*.onnx",
	})
	if err != nil {
		t.Fatalf("create rule: %v", err)
	}
	if rule.RuleID != "rule-e2e" {
		t.Errorf("ruleID = %q", rule.RuleID)
	}

	// 4. Manual deploy trigger
	deploy, err := client.TriggerDeploy(&DeployTriggerRequest{
		JobID:      "job-trained",
		EdgeFilter: "tag:onnx",
	})
	if err != nil {
		t.Fatalf("trigger: %v", err)
	}
	if deploy.DeployID != "deploy-e2e" {
		t.Errorf("deployID = %q", deploy.DeployID)
	}

	// 5. Check deploy status
	status, err := client.GetDeployStatus(deploy.DeployID)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if status.Status != "completed" {
		t.Errorf("deploy status = %q", status.Status)
	}
	if status.Targets[0].Status != "succeeded" {
		t.Errorf("target status = %q", status.Targets[0].Status)
	}
}
