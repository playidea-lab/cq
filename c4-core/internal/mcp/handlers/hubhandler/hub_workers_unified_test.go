//go:build hub

package hubhandler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/changmin/c4-core/internal/hub"
	"github.com/changmin/c4-core/internal/mcp"
)

// =========================================================================
// Hub Workers Unified Tests — Relay-based status model
// Status: online (in relay /health) or offline (not in relay)
// =========================================================================

func newUnifiedTestServer(
	t *testing.T,
	hubMux *http.ServeMux,
	relayMux *http.ServeMux,
) (*hub.Client, string, *mcp.Registry) {
	t.Helper()

	hubTS := httptest.NewServer(hubMux)
	t.Cleanup(hubTS.Close)

	relayTS := httptest.NewServer(relayMux)
	t.Cleanup(relayTS.Close)

	client := hub.NewClient(hub.HubConfig{
		URL:    hubTS.URL,
		APIKey: "test-key",
		TeamID: "test-team",
	})

	reg := mcp.NewRegistry()
	RegisterHubHandlers(reg, client)
	return client, relayTS.URL, reg
}

func callUnified(t *testing.T, reg *mcp.Registry, relayURL string, includeOffline *bool) map[string]any {
	t.Helper()
	args := map[string]any{"relay_url": relayURL}
	if includeOffline != nil {
		args["include_offline"] = *includeOffline
	}
	raw, _ := json.Marshal(args)
	result, err := reg.Call("cq_hub_workers_unified", raw)
	if err != nil {
		t.Fatalf("cq_hub_workers_unified: %v", err)
	}
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map, got %T", result)
	}
	return m
}

func boolPtr(b bool) *bool { return &b }

func toInt(t *testing.T, v any) int {
	t.Helper()
	switch n := v.(type) {
	case int:
		return n
	case float64:
		return int(n)
	default:
		t.Fatalf("toInt: unexpected type %T for value %v", v, v)
		return 0
	}
}

// =========================================================================
// Test: Hub offline → empty list + error field
// =========================================================================

func TestHubWorkersUnified_HubOffline(t *testing.T) {
	hubMux := http.NewServeMux()
	hubMux.HandleFunc("/rest/v1/hub_workers", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	})

	relayMux := http.NewServeMux()
	relayMux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"status":       "ok",
			"workers":      0,
			"worker_names": []string{},
		})
	})

	_, relayURL, reg := newUnifiedTestServer(t, hubMux, relayMux)
	result := callUnified(t, reg, relayURL, nil)

	if _, hasErr := result["error"]; !hasErr {
		t.Error("expected error field when hub is offline")
	}
	workers, ok := result["workers"].([]any)
	if !ok || len(workers) != 0 {
		t.Errorf("expected empty workers list, got %v", result["workers"])
	}
}

// =========================================================================
// Test: Relay offline → all workers are offline
// =========================================================================

func TestHubWorkersUnified_RelayOffline(t *testing.T) {
	now := time.Now().UTC()

	hubMux := http.NewServeMux()
	hubMux.HandleFunc("/rest/v1/hub_workers", func(w http.ResponseWriter, r *http.Request) {
		workers := []map[string]any{
			{
				"id": "worker-1", "name": "gpu-box", "hostname": "gpu-box",
				"status": "idle", "gpu_count": 2, "gpu_model": "RTX 4090",
				"total_vram_gb": 48.0, "free_vram_gb": 40.0,
				"last_heartbeat": now.Add(-10 * time.Second).Format(time.RFC3339),
				"registered_at":  now.Add(-1 * time.Hour).Format(time.RFC3339),
			},
		}
		json.NewEncoder(w).Encode(workers)
	})
	hubMux.HandleFunc("/rest/v1/hub_capabilities", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]map[string]any{})
	})

	// Relay completely down.
	relayURL := "http://127.0.0.1:1"

	_, _, reg := newUnifiedTestServer(t, hubMux, http.NewServeMux())
	args := map[string]any{"relay_url": relayURL}
	raw, _ := json.Marshal(args)
	result, err := reg.Call("cq_hub_workers_unified", raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]any)

	workers := m["workers"].([]any)
	if len(workers) != 1 {
		t.Fatalf("expected 1 worker, got %d", len(workers))
	}
	w := workers[0].(map[string]any)
	// Relay down → not in relay → offline
	if w["status"] != "offline" {
		t.Errorf("status = %v, want offline (relay is down)", w["status"])
	}
}

// =========================================================================
// Test: Workers in relay = online, not in relay = offline
// =========================================================================

func TestHubWorkersUnified_RelayBasedStatus(t *testing.T) {
	now := time.Now().UTC()

	hubMux := http.NewServeMux()
	hubMux.HandleFunc("/rest/v1/hub_workers", func(w http.ResponseWriter, r *http.Request) {
		workers := []map[string]any{
			{
				"id": "worker-1", "name": "connected-box", "hostname": "connected-box",
				"status": "idle", "gpu_count": 1, "gpu_model": "RTX 3090",
				"total_vram_gb": 24.0, "free_vram_gb": 20.0,
				"last_heartbeat": now.Add(-5 * time.Second).Format(time.RFC3339),
				"registered_at":  now.Add(-2 * time.Hour).Format(time.RFC3339),
			},
			{
				"id": "worker-2", "name": "disconnected-box", "hostname": "disconnected-box",
				"status": "idle", "gpu_count": 2, "gpu_model": "A100",
				"total_vram_gb": 80.0, "free_vram_gb": 70.0,
				"last_heartbeat": now.Add(-20 * time.Second).Format(time.RFC3339),
				"registered_at":  now.Add(-3 * time.Hour).Format(time.RFC3339),
			},
		}
		json.NewEncoder(w).Encode(workers)
	})
	hubMux.HandleFunc("/rest/v1/hub_capabilities", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]map[string]any{})
	})

	relayMux := http.NewServeMux()
	relayMux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"status":       "ok",
			"workers":      1,
			"worker_names": []string{"connected-box"}, // Only one in relay
		})
	})

	_, relayURL, reg := newUnifiedTestServer(t, hubMux, relayMux)
	result := callUnified(t, reg, relayURL, nil)

	workers := result["workers"].([]any)
	if len(workers) != 2 {
		t.Fatalf("expected 2 workers, got %d", len(workers))
	}

	byName := make(map[string]map[string]any)
	for _, wRaw := range workers {
		w := wRaw.(map[string]any)
		byName[w["name"].(string)] = w
	}

	connected := byName["connected-box"]
	if connected["status"] != "online" {
		t.Errorf("connected-box status = %v, want online", connected["status"])
	}

	disconnected := byName["disconnected-box"]
	if disconnected["status"] != "offline" {
		t.Errorf("disconnected-box status = %v, want offline", disconnected["status"])
	}
}

// =========================================================================
// Test: include_offline=false filters offline workers
// =========================================================================

func TestHubWorkersUnified_IncludeOffline_False(t *testing.T) {
	now := time.Now().UTC()

	hubMux := http.NewServeMux()
	hubMux.HandleFunc("/rest/v1/hub_workers", func(w http.ResponseWriter, r *http.Request) {
		workers := []map[string]any{
			{
				"id": "w-online", "name": "online-box", "hostname": "online-box",
				"status": "idle", "gpu_count": 1, "gpu_model": "RTX 3080",
				"total_vram_gb": 10.0, "free_vram_gb": 8.0,
				"last_heartbeat": now.Add(-10 * time.Second).Format(time.RFC3339),
				"registered_at":  now.Add(-1 * time.Hour).Format(time.RFC3339),
			},
			{
				"id": "w-offline", "name": "dead-box", "hostname": "dead-box",
				"status": "offline", "gpu_count": 0, "gpu_model": "",
				"total_vram_gb": 0.0, "free_vram_gb": 0.0,
				"last_heartbeat": now.Add(-10 * time.Minute).Format(time.RFC3339),
				"registered_at":  now.Add(-5 * time.Hour).Format(time.RFC3339),
			},
		}
		json.NewEncoder(w).Encode(workers)
	})
	hubMux.HandleFunc("/rest/v1/hub_capabilities", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]map[string]any{})
	})

	relayMux := http.NewServeMux()
	relayMux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"status":       "ok",
			"workers":      1,
			"worker_names": []string{"online-box"}, // Only online-box in relay
		})
	})

	_, relayURL, reg := newUnifiedTestServer(t, hubMux, relayMux)
	result := callUnified(t, reg, relayURL, boolPtr(false))

	workers := result["workers"].([]any)
	if len(workers) != 1 {
		t.Errorf("expected 1 online worker, got %d", len(workers))
	}
	for _, wRaw := range workers {
		w := wRaw.(map[string]any)
		if w["status"] == "offline" {
			t.Errorf("offline worker %v leaked through include_offline=false", w["name"])
		}
	}
}

// =========================================================================
// Test: Summary counts are correct (relay-based)
// =========================================================================

func TestHubWorkersUnified_Summary(t *testing.T) {
	now := time.Now().UTC()

	hubMux := http.NewServeMux()
	hubMux.HandleFunc("/rest/v1/hub_workers", func(w http.ResponseWriter, r *http.Request) {
		workers := []map[string]any{
			{
				"id": "w1", "name": "a", "hostname": "a", "status": "idle",
				"gpu_count": 1, "gpu_model": "RTX", "total_vram_gb": 10.0, "free_vram_gb": 8.0,
				"last_heartbeat": now.Add(-5 * time.Second).Format(time.RFC3339),
				"registered_at":  now.Add(-1 * time.Hour).Format(time.RFC3339),
			},
			{
				"id": "w2", "name": "b", "hostname": "b", "status": "idle",
				"gpu_count": 1, "gpu_model": "RTX", "total_vram_gb": 10.0, "free_vram_gb": 8.0,
				"last_heartbeat": now.Add(-2 * time.Minute).Format(time.RFC3339),
				"registered_at":  now.Add(-1 * time.Hour).Format(time.RFC3339),
			},
			{
				"id": "w3", "name": "c", "hostname": "c", "status": "offline",
				"gpu_count": 0, "gpu_model": "", "total_vram_gb": 0.0, "free_vram_gb": 0.0,
				"last_heartbeat": now.Add(-10 * time.Minute).Format(time.RFC3339),
				"registered_at":  now.Add(-5 * time.Hour).Format(time.RFC3339),
			},
		}
		json.NewEncoder(w).Encode(workers)
	})
	hubMux.HandleFunc("/rest/v1/hub_capabilities", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]map[string]any{})
	})

	relayMux := http.NewServeMux()
	relayMux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"status":       "ok",
			"workers":      2,
			"worker_names": []string{"a", "b"}, // a and b in relay, c not
		})
	})

	_, relayURL, reg := newUnifiedTestServer(t, hubMux, relayMux)
	result := callUnified(t, reg, relayURL, boolPtr(true))

	summary := result["summary"].(map[string]any)

	total := toInt(t, summary["total"])
	online := toInt(t, summary["online"])
	offline := toInt(t, summary["offline"])

	if total != 3 {
		t.Errorf("total = %d, want 3", total)
	}
	if online != 2 {
		t.Errorf("online = %d, want 2 (a and b in relay)", online)
	}
	if offline != 1 {
		t.Errorf("offline = %d, want 1 (c not in relay)", offline)
	}
}
