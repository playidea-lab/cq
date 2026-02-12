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
	mux.HandleFunc("/v1/edges/register", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("method = %s, want POST", r.Method)
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if body["name"] != "jetson-1" {
			t.Errorf("name = %v", body["name"])
		}
		jsonResponse(w, EdgeRegisterResponse{EdgeID: "edge-001"})
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
	mux.HandleFunc("/v1/edges", func(w http.ResponseWriter, r *http.Request) {
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
	mux.HandleFunc("/v1/edges/heartbeat", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if body["edge_id"] != "edge-001" {
			t.Errorf("edge_id = %v", body["edge_id"])
		}
		jsonResponse(w, HeartbeatResponse{Acknowledged: true})
	})
	client, _ := newTestServer(t, mux)

	if err := client.EdgeHeartbeat("edge-001", "online"); err != nil {
		t.Fatalf("EdgeHeartbeat: %v", err)
	}
}

func TestRemoveEdge(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/edges/edge-001", func(w http.ResponseWriter, r *http.Request) {
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
	mux.HandleFunc("/v1/deploy/rules", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("method = %s", r.Method)
		}
		var req DeployRuleCreateRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.Trigger != "job_tag:production" {
			t.Errorf("trigger = %q", req.Trigger)
		}
		if req.EdgeFilter != "tag:onnx" {
			t.Errorf("edge_filter = %q", req.EdgeFilter)
		}
		if req.ArtifactPattern != "outputs/*.onnx" {
			t.Errorf("artifact_pattern = %q", req.ArtifactPattern)
		}
		jsonResponse(w, DeployRuleCreateResponse{RuleID: "rule-1"})
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
	mux.HandleFunc("/v1/deploy/rules", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("method = %s", r.Method)
		}
		jsonResponse(w, []DeployRule{
			{ID: "rule-1", Trigger: "job_tag:production", EdgeFilter: "tag:onnx", ArtifactPattern: "outputs/*.onnx", Enabled: true},
			{ID: "rule-2", Trigger: "dag_complete:*", EdgeFilter: "name:jetson-*", ArtifactPattern: "*.trt", Enabled: false},
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
	mux.HandleFunc("/v1/deploy/rules/rule-1", func(w http.ResponseWriter, r *http.Request) {
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
	mux.HandleFunc("/v1/deploy/trigger", func(w http.ResponseWriter, r *http.Request) {
		var req DeployTriggerRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.JobID != "job-123" {
			t.Errorf("job_id = %q", req.JobID)
		}
		if req.ArtifactPattern != "outputs/model.onnx" {
			t.Errorf("artifact_pattern = %q", req.ArtifactPattern)
		}
		jsonResponse(w, DeployTriggerResponse{
			DeployID:    "deploy-1",
			Status:      "deploying",
			TargetCount: 3,
		})
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
	if resp.TargetCount != 3 {
		t.Errorf("TargetCount = %d", resp.TargetCount)
	}
}

func TestTriggerDeploy_ExplicitEdges(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/deploy/trigger", func(w http.ResponseWriter, r *http.Request) {
		var req DeployTriggerRequest
		json.NewDecoder(r.Body).Decode(&req)
		if len(req.EdgeIDs) != 2 {
			t.Errorf("edge_ids len = %d", len(req.EdgeIDs))
		}
		jsonResponse(w, DeployTriggerResponse{
			DeployID:    "deploy-2",
			Status:      "deploying",
			TargetCount: 2,
		})
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
		t.Errorf("TargetCount = %d", resp.TargetCount)
	}
}

func TestGetDeployStatus(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/deploy/deploy-1/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("method = %s", r.Method)
		}
		jsonResponse(w, Deployment{
			ID:     "deploy-1",
			JobID:  "job-123",
			Status: "completed",
			Targets: []DeployTarget{
				{EdgeID: "edge-001", EdgeName: "jetson-1", Status: "succeeded"},
				{EdgeID: "edge-002", EdgeName: "jetson-2", Status: "succeeded"},
				{EdgeID: "edge-003", EdgeName: "rpi-1", Status: "failed", Error: "disk full"},
			},
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
	mux.HandleFunc("/v1/edges/register", func(w http.ResponseWriter, r *http.Request) {
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
	mux.HandleFunc("/v1/edges/edge-missing", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
	})
	client, _ := newTestServer(t, mux)

	err := client.RemoveEdge("edge-missing")
	if err == nil {
		t.Error("expected error on 404")
	}
}

// =========================================================================
// Full E2E flow: register edge → create rule → train → deploy → status
// =========================================================================

func TestEdgeDeploy_FullFlow(t *testing.T) {
	mux := http.NewServeMux()

	// Edge register
	mux.HandleFunc("/v1/edges/register", func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, EdgeRegisterResponse{EdgeID: "edge-e2e"})
	})

	// Edge list
	mux.HandleFunc("/v1/edges", func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, []Edge{
			{ID: "edge-e2e", Name: "jetson-factory", Status: "online", Tags: []string{"onnx", "arm64"}},
		})
	})

	// Deploy rule
	mux.HandleFunc("/v1/deploy/rules", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			jsonResponse(w, DeployRuleCreateResponse{RuleID: "rule-e2e"})
		} else {
			jsonResponse(w, []DeployRule{
				{ID: "rule-e2e", Trigger: "job_tag:production", EdgeFilter: "tag:onnx", ArtifactPattern: "*.onnx", Enabled: true},
			})
		}
	})

	// Manual deploy
	mux.HandleFunc("/v1/deploy/trigger", func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, DeployTriggerResponse{DeployID: "deploy-e2e", Status: "deploying", TargetCount: 1})
	})

	// Deploy status
	mux.HandleFunc("/v1/deploy/deploy-e2e/status", func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, Deployment{
			ID:     "deploy-e2e",
			Status: "completed",
			Targets: []DeployTarget{
				{EdgeID: "edge-e2e", EdgeName: "jetson-factory", Status: "succeeded"},
			},
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
	if deploy.TargetCount != 1 {
		t.Errorf("targets = %d", deploy.TargetCount)
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
