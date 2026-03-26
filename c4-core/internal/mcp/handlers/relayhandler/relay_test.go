package relayhandler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/changmin/c4-core/internal/mcp"
)

func TestHandleWorkers_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"status":       "ok",
			"workers":      2,
			"worker_names": []string{"pi-gpu", "mac-local"},
		})
	}))
	defer srv.Close()

	deps := &Deps{RelayURL: strings.Replace(srv.URL, "http://", "ws://", 1)}
	result, err := handleWorkers(deps)
	if err != nil {
		t.Fatalf("handleWorkers: %v", err)
	}

	m := result.(map[string]any)
	if m["count"].(int) != 2 {
		t.Errorf("count: got %v, want 2", m["count"])
	}
	workers := m["workers"].([]map[string]any)
	if len(workers) != 2 {
		t.Fatalf("workers: got %d, want 2", len(workers))
	}
	if workers[0]["id"] != "pi-gpu" {
		t.Errorf("worker[0].id: got %q, want %q", workers[0]["id"], "pi-gpu")
	}
}

func TestHandleWorkers_NotConfigured(t *testing.T) {
	deps := &Deps{RelayURL: ""}
	result, err := handleWorkers(deps)
	if err != nil {
		t.Fatalf("handleWorkers: %v", err)
	}
	m := result.(map[string]any)
	if m["error"] != "relay not configured" {
		t.Errorf("expected 'relay not configured', got %v", m["error"])
	}
}

func TestHandleRelayCall_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/w/pi-gpu/mcp" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != "POST" {
			t.Errorf("unexpected method: %s", r.Method)
		}
		// Verify auth header.
		if auth := r.Header.Get("Authorization"); auth != "Bearer test-token" {
			t.Errorf("auth: got %q, want %q", auth, "Bearer test-token")
		}

		// Parse incoming JSON-RPC.
		var rpc struct {
			Method string `json:"method"`
			Params struct {
				Name string          `json:"name"`
				Args json.RawMessage `json:"arguments"`
			} `json:"params"`
		}
		json.NewDecoder(r.Body).Decode(&rpc)
		if rpc.Params.Name != "cq_read_file" {
			t.Errorf("tool name: got %q, want %q", rpc.Params.Name, "cq_read_file")
		}

		// Return mock response.
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      1,
			"result": map[string]any{
				"content":    "hello world",
				"total_lines": 1,
			},
		})
	}))
	defer srv.Close()

	deps := &Deps{
		RelayURL:  strings.Replace(srv.URL, "http://", "ws://", 1),
		TokenFunc: func() string { return "test-token" },
	}

	args, _ := json.Marshal(map[string]any{
		"worker_id": "pi-gpu",
		"tool":      "cq_read_file",
		"args":      map[string]any{"path": "/data/test.txt"},
	})
	result, err := handleRelayCall(deps, args)
	if err != nil {
		t.Fatalf("handleRelayCall: %v", err)
	}

	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("result type: got %T, want map[string]any", result)
	}
	if m["content"] != "hello world" {
		t.Errorf("content: got %v, want %q", m["content"], "hello world")
	}
}

func TestHandleRelayCall_WorkerOffline(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"error": "worker offline"})
	}))
	defer srv.Close()

	deps := &Deps{
		RelayURL:  strings.Replace(srv.URL, "http://", "ws://", 1),
		TokenFunc: func() string { return "tok" },
	}

	args, _ := json.Marshal(map[string]any{
		"worker_id": "dead-worker",
		"tool":      "cq_read_file",
		"args":      map[string]any{"path": "/nope"},
	})
	result, err := handleRelayCall(deps, args)
	if err != nil {
		t.Fatalf("handleRelayCall: %v", err)
	}

	m := result.(map[string]any)
	errStr, _ := m["error"].(string)
	if !strings.Contains(errStr, "offline") {
		t.Errorf("expected offline error, got %q", errStr)
	}
}

func TestHandleRelayCall_GatewayTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusGatewayTimeout)
		json.NewEncoder(w).Encode(map[string]string{"error": "timeout"})
	}))
	defer srv.Close()

	deps := &Deps{
		RelayURL:  strings.Replace(srv.URL, "http://", "ws://", 1),
		TokenFunc: func() string { return "tok" },
	}

	args, _ := json.Marshal(map[string]any{
		"worker_id": "slow-worker",
		"tool":      "cq_execute",
		"args":      map[string]any{"command": "sleep 999"},
	})
	result, err := handleRelayCall(deps, args)
	if err != nil {
		t.Fatalf("handleRelayCall: %v", err)
	}

	m := result.(map[string]any)
	errStr, _ := m["error"].(string)
	if !strings.Contains(errStr, "timeout") {
		t.Errorf("expected timeout error, got %q", errStr)
	}
	hint, _ := m["hint"].(string)
	if !strings.Contains(hint, "hub submit") {
		t.Errorf("expected hub submit hint, got %q", hint)
	}
}

func TestHandleRelayCall_NotConfigured(t *testing.T) {
	deps := &Deps{RelayURL: ""}
	args, _ := json.Marshal(map[string]any{
		"worker_id": "x",
		"tool":      "y",
	})
	result, err := handleRelayCall(deps, args)
	if err != nil {
		t.Fatalf("handleRelayCall: %v", err)
	}
	m := result.(map[string]any)
	if m["error"] != "relay not configured" {
		t.Errorf("expected 'relay not configured', got %v", m["error"])
	}
}

func TestRegister_ToolCount(t *testing.T) {
	reg := mcp.NewRegistry()
	deps := &Deps{RelayURL: "wss://test.fly.dev"}
	Register(reg, deps)

	tools := reg.ListTools()
	if len(tools) != 2 {
		t.Errorf("tool count: got %d, want 2", len(tools))
	}

	names := make(map[string]bool)
	for _, tool := range tools {
		names[tool.Name] = true
	}
	if !names["cq_workers"] {
		t.Error("missing cq_workers tool")
	}
	if !names["cq_relay_call"] {
		t.Error("missing cq_relay_call tool")
	}
}
