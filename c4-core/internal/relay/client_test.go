package relay

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
)

// serveWS upgrades an HTTP connection to WebSocket for use in tests.
func serveWS(t *testing.T, w http.ResponseWriter, r *http.Request) net.Conn {
	t.Helper()
	conn, _, _, err := ws.UpgradeHTTP(r, w)
	if err != nil {
		t.Errorf("upgrade: %v", err)
		return nil
	}
	return conn
}

// TestRelayClientConnect verifies that Connect dials the mock WSS server
// and transitions IsConnected to true.
func TestRelayClientConnect(t *testing.T) {
	connected := make(chan struct{})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn := serveWS(t, w, r)
		if conn == nil {
			return
		}
		close(connected)
		// Keep the connection open until the test ends.
		<-r.Context().Done()
		conn.Close()
	}))
	defer srv.Close()

	wsURL := "ws://" + srv.Listener.Addr().String()
	client := NewRelayClient(wsURL, "worker-1", func() string { return "tok" }, nil)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	select {
	case <-connected:
	case <-ctx.Done():
		t.Fatal("server did not receive connection in time")
	}

	if !client.IsConnected() {
		t.Fatal("expected IsConnected() == true after Connect")
	}
}

// TestRelayClientReconnect verifies that when the server closes the connection
// the client reconnects automatically.
func TestRelayClientReconnect(t *testing.T) {
	var connCount atomic.Int32

	// First connection: server immediately closes.
	// Second connection: server keeps open.
	reconnected := make(chan struct{})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn := serveWS(t, w, r)
		if conn == nil {
			return
		}
		n := connCount.Add(1)
		if n == 1 {
			// Close immediately to trigger reconnect.
			conn.Close()
			return
		}
		// Second connection: signal and keep open.
		select {
		case reconnected <- struct{}{}:
		default:
		}
		<-r.Context().Done()
		conn.Close()
	}))
	defer srv.Close()

	wsURL := "ws://" + srv.Listener.Addr().String()
	client := NewRelayClient(wsURL, "worker-2", func() string { return "tok" }, nil)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	select {
	case <-reconnected:
		// Reconnect succeeded.
	case <-ctx.Done():
		t.Fatalf("client did not reconnect within timeout (connections=%d)", connCount.Load())
	}
}

// TestRelayClientMCPHandler verifies that an incoming relay envelope triggers
// the MCPHandler and the response is sent back with the correct relay_id.
func TestRelayClientMCPHandler(t *testing.T) {
	handlerCalled := make(chan json.RawMessage, 1)
	responseCh := make(chan relayEnvelope, 1)

	handler := func(ctx context.Context, req json.RawMessage) (json.RawMessage, error) {
		handlerCalled <- req
		return json.RawMessage(`{"jsonrpc":"2.0","result":{"ok":true}}`), nil
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn := serveWS(t, w, r)
		if conn == nil {
			return
		}
		defer conn.Close()

		// Send a relay request to the client.
		req := relayEnvelope{
			RelayID: "test-relay-id-123",
			Body:    json.RawMessage(`{"jsonrpc":"2.0","method":"tools/call","params":{"name":"ping"}}`),
		}
		data, _ := json.Marshal(req)
		if err := wsutil.WriteServerText(conn, data); err != nil {
			t.Errorf("server write: %v", err)
			return
		}

		// Read the response from the client.
		respData, _, err := wsutil.ReadClientData(conn)
		if err != nil {
			t.Errorf("server read response: %v", err)
			return
		}
		var env relayEnvelope
		if err := json.Unmarshal(respData, &env); err != nil {
			t.Errorf("server unmarshal response: %v", err)
			return
		}
		responseCh <- env
	}))
	defer srv.Close()

	wsURL := "ws://" + srv.Listener.Addr().String()
	client := NewRelayClient(wsURL, "worker-3", func() string { return "tok" }, handler)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	// Wait for handler to be called.
	select {
	case req := <-handlerCalled:
		if !strings.Contains(string(req), "tools/call") {
			t.Errorf("unexpected request body: %s", req)
		}
	case <-ctx.Done():
		t.Fatal("MCPHandler was not called within timeout")
	}

	// Verify response envelope.
	select {
	case env := <-responseCh:
		if env.RelayID != "test-relay-id-123" {
			t.Errorf("relay_id: got %q, want %q", env.RelayID, "test-relay-id-123")
		}
		if !strings.Contains(string(env.Body), `"ok":true`) {
			t.Errorf("unexpected response body: %s", env.Body)
		}
	case <-ctx.Done():
		t.Fatal("no response received from client within timeout")
	}
}
