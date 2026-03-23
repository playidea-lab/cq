package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestHealthEndpoint(t *testing.T) {
	srv := newServer()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", srv.handleHealth)

	ts := httptest.NewServer(mux)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/health")
	if err != nil {
		t.Fatalf("GET /health: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("expected status=ok, got %v", body["status"])
	}
	if _, ok := body["workers"]; !ok {
		t.Error("missing workers field")
	}
}

func TestWorkerNotFound503(t *testing.T) {
	srv := newServer()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /w/{id}/mcp", srv.handleMCP)

	ts := httptest.NewServer(mux)
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/w/nonexistent/mcp", "application/json",
		strings.NewReader(`{"method":"test"}`))
	if err != nil {
		t.Fatalf("POST /w/nonexistent/mcp: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", resp.StatusCode)
	}

	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["error"] != "worker offline" {
		t.Errorf("expected error=worker offline, got %v", body["error"])
	}
}

func TestConnectAndRelay(t *testing.T) {
	srv := newServer()
	// dev mode: SUPABASE_JWT_SECRET is empty → skip validation
	mux := http.NewServeMux()
	mux.HandleFunc("GET /connect", srv.handleConnect)
	mux.HandleFunc("POST /w/{id}/mcp", srv.handleMCP)
	mux.HandleFunc("GET /w/{id}/health", srv.handleWorkerHealth)

	ts := httptest.NewServer(mux)
	defer ts.Close()

	workerID := "test-worker-1"

	// Connect worker via WebSocket.
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/connect?worker_id=" + workerID
	dialer := websocket.Dialer{}
	wconn, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial worker ws: %v", err)
	}
	defer wconn.Close()

	// Give server a moment to register the worker.
	time.Sleep(50 * time.Millisecond)

	// Verify worker health endpoint reports 200.
	healthResp, err := http.Get(ts.URL + "/w/" + workerID + "/health")
	if err != nil {
		t.Fatalf("GET worker health: %v", err)
	}
	healthResp.Body.Close()
	if healthResp.StatusCode != http.StatusOK {
		t.Fatalf("expected worker health 200, got %d", healthResp.StatusCode)
	}

	// Start a goroutine that acts as the worker: read relay message, echo back response.
	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		_, data, err := wconn.ReadMessage()
		if err != nil {
			t.Logf("worker read: %v", err)
			return
		}
		var msg relayMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			t.Logf("worker unmarshal: %v", err)
			return
		}
		// Echo back the body as the response.
		resp := relayMessage{RelayID: msg.RelayID, Body: msg.Body}
		out, _ := json.Marshal(resp)
		wconn.WriteMessage(websocket.TextMessage, out)
	}()

	// Send HTTP request to relay.
	payload := `{"jsonrpc":"2.0","method":"ping","id":1}`
	httpResp, err := http.Post(ts.URL+"/w/"+workerID+"/mcp", "application/json",
		strings.NewReader(payload))
	if err != nil {
		t.Fatalf("POST /w/%s/mcp: %v", workerID, err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from relay, got %d", httpResp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(httpResp.Body).Decode(&result); err != nil {
		t.Fatalf("decode relay response: %v", err)
	}
	if result["method"] != "ping" {
		t.Errorf("expected echoed method=ping, got %v", result["method"])
	}

	<-workerDone
}
