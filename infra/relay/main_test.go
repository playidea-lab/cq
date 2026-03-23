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

func TestTunnelRoundtrip(t *testing.T) {
	srv := newServer()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /tunnel", srv.handleCreateTunnel)
	mux.HandleFunc("GET /tunnel/{id}", srv.handleTunnelConnect)

	ts := httptest.NewServer(mux)
	defer ts.Close()

	// 1. Create tunnel via POST /tunnel
	resp, err := http.Post(ts.URL+"/tunnel", "application/json", nil)
	if err != nil {
		t.Fatalf("POST /tunnel: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	var created map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decode tunnel response: %v", err)
	}
	tunnelID := created["tunnel_id"]
	if tunnelID == "" {
		t.Fatal("tunnel_id is empty")
	}

	dialer := websocket.Dialer{}
	baseWS := "ws" + strings.TrimPrefix(ts.URL, "http") + "/tunnel/" + tunnelID

	// 2. Connect sender
	senderConn, _, err := dialer.Dial(baseWS+"?role=sender", nil)
	if err != nil {
		t.Fatalf("sender dial: %v", err)
	}
	defer senderConn.Close()

	// 3. Connect receiver
	receiverConn, _, err := dialer.Dial(baseWS+"?role=receiver", nil)
	if err != nil {
		t.Fatalf("receiver dial: %v", err)
	}
	defer receiverConn.Close()

	// Allow glue goroutines to start
	time.Sleep(50 * time.Millisecond)

	// 4. Send binary data from sender
	payload := []byte{0x01, 0x02, 0x03, 0xDE, 0xAD, 0xBE, 0xEF}
	if err := senderConn.WriteMessage(websocket.BinaryMessage, payload); err != nil {
		t.Fatalf("sender write: %v", err)
	}

	// 5. Receiver must get the same bytes
	receiverConn.SetReadDeadline(time.Now().Add(3 * time.Second))
	mt, got, err := receiverConn.ReadMessage()
	if err != nil {
		t.Fatalf("receiver read: %v", err)
	}
	if mt != websocket.BinaryMessage {
		t.Errorf("expected BinaryMessage, got %d", mt)
	}
	if string(got) != string(payload) {
		t.Errorf("payload mismatch: got %x, want %x", got, payload)
	}

	// 6. Close sender → receiver should also close
	senderConn.WriteMessage(websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	senderConn.Close()

	receiverConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, _, err = receiverConn.ReadMessage()
	if err == nil {
		t.Error("expected receiver to close after sender closed")
	}
}

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
