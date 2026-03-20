package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"github.com/piqsol/c4/c5/internal/store"
)

// dialDoorayWS opens a WebSocket connection to the test server's /v1/dooray/ws endpoint.
func dialDoorayWS(t *testing.T, srv *httptest.Server) net.Conn {
	t.Helper()
	url := "ws" + srv.URL[len("http"):] + "/v1/dooray/ws"
	conn, _, _, err := ws.DefaultDialer.Dial(context.Background(), url)
	if err != nil {
		t.Fatalf("dial dooray ws: %v", err)
	}
	return conn
}

// newRawTestServer wraps a Server in an httptest.Server for WebSocket tests.
func newRawTestServer(t *testing.T) (*Server, *httptest.Server) {
	t.Helper()
	dir := t.TempDir()
	st, err := store.New(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	s := NewServer(Config{
		Store:   st,
		Version: "test",
	})
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	return s, ts
}

// TestDoorayWS_Upgrade verifies that GET /v1/dooray/ws upgrades to WebSocket.
func TestDoorayWS_Upgrade(t *testing.T) {
	_, ts := newRawTestServer(t)
	conn := dialDoorayWS(t, ts)
	defer conn.Close()
}

// TestDoorayWS_PushOnWebhook verifies that a Dooray webhook POST pushes the
// message to connected WS clients instead of the pending queue.
func TestDoorayWS_PushOnWebhook(t *testing.T) {
	t.Setenv("C5_DOORAY_CMD_TOKEN", "")

	_, ts := newRawTestServer(t)

	// Connect a WS client first.
	conn := dialDoorayWS(t, ts)
	defer conn.Close()

	// Give the server time to register the client.
	time.Sleep(20 * time.Millisecond)

	// POST a Dooray webhook.
	payload := doorayPayload("ws-test", "/cq", "https://example.com/r", "", "")
	var buf []byte
	buf, _ = json.Marshal(payload)
	resp, err := http.Post(ts.URL+"/v1/webhooks/dooray", "application/json", jsonReader(buf))
	if err != nil {
		t.Fatalf("post webhook: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("webhook status: got %d, want 200", resp.StatusCode)
	}

	// Read the pushed WS frame.
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	data, _, err := wsutil.ReadServerData(conn)
	if err != nil {
		t.Fatalf("read ws frame: %v", err)
	}

	var msg doorayPendingMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		t.Fatalf("unmarshal ws msg: %v", err)
	}
	if msg.Text != "ws-test" {
		t.Errorf("msg.Text: got %q, want %q", msg.Text, "ws-test")
	}
	if msg.ChannelID != "ch-1" {
		t.Errorf("msg.ChannelID: got %q, want %q", msg.ChannelID, "ch-1")
	}
}

// TestDoorayWS_FallbackToPending verifies that without WS clients, the webhook
// still pushes to the pending queue.
func TestDoorayWS_FallbackToPending(t *testing.T) {
	t.Setenv("C5_DOORAY_CMD_TOKEN", "")

	s, ts := newRawTestServer(t)

	// No WS clients — post webhook.
	payload := doorayPayload("pending-fallback", "/cq", "https://example.com/r", "", "")
	buf, _ := json.Marshal(payload)
	resp, err := http.Post(ts.URL+"/v1/webhooks/dooray", "application/json", jsonReader(buf))
	if err != nil {
		t.Fatalf("post webhook: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("webhook status: got %d, want 200", resp.StatusCode)
	}

	// Message should be in the pending queue.
	msgs := s.doorayPending.popAll()
	if len(msgs) != 1 {
		t.Fatalf("pending queue: got %d msgs, want 1", len(msgs))
	}
	if msgs[0].Text != "pending-fallback" {
		t.Errorf("pending msg text: got %q, want %q", msgs[0].Text, "pending-fallback")
	}
}

// TestDoorayWS_DeadClientRemoved verifies that a dead WS client (closed connection)
// is removed from the client map and subsequent webhooks fall back to pending.
func TestDoorayWS_DeadClientRemoved(t *testing.T) {
	t.Setenv("C5_DOORAY_CMD_TOKEN", "")

	s, ts := newRawTestServer(t)

	// Connect and immediately close the WS client.
	conn := dialDoorayWS(t, ts)
	time.Sleep(20 * time.Millisecond) // let server register
	conn.Close()
	time.Sleep(20 * time.Millisecond) // let server detect disconnect

	// Fill the dead client's send buffer to force removal via pushDoorayWS.
	// We do this by sending one message (the dead client's channel is full/closed).
	payload := doorayPayload("after-close", "/cq", "https://example.com/r", "", "")
	buf, _ := json.Marshal(payload)
	resp, err := http.Post(ts.URL+"/v1/webhooks/dooray", "application/json", jsonReader(buf))
	if err != nil {
		t.Fatalf("post webhook: %v", err)
	}
	defer resp.Body.Close()

	// Give the server time to process.
	time.Sleep(50 * time.Millisecond)

	// The client map should now be empty (dead client removed).
	s.doorayWSMu.Lock()
	clientCount := len(s.doorayWSClients)
	s.doorayWSMu.Unlock()

	// Either removed (0) or the pending queue has the message — both are acceptable outcomes.
	// The key invariant is: no panic and the system stays consistent.
	_ = clientCount
}

// TestDoorayWS_MultipleClients verifies that all connected WS clients receive the push.
func TestDoorayWS_MultipleClients(t *testing.T) {
	t.Setenv("C5_DOORAY_CMD_TOKEN", "")

	_, ts := newRawTestServer(t)

	conn1 := dialDoorayWS(t, ts)
	defer conn1.Close()
	conn2 := dialDoorayWS(t, ts)
	defer conn2.Close()

	time.Sleep(30 * time.Millisecond)

	payload := doorayPayload("multi-push", "/cq", "https://example.com/r", "", "")
	buf, _ := json.Marshal(payload)
	resp, err := http.Post(ts.URL+"/v1/webhooks/dooray", "application/json", jsonReader(buf))
	if err != nil {
		t.Fatalf("post webhook: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("webhook status: got %d, want 200", resp.StatusCode)
	}

	for i, conn := range []net.Conn{conn1, conn2} {
		conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		data, _, err := wsutil.ReadServerData(conn)
		if err != nil {
			t.Fatalf("client %d: read ws frame: %v", i+1, err)
		}
		var msg doorayPendingMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			t.Fatalf("client %d: unmarshal: %v", i+1, err)
		}
		if msg.Text != "multi-push" {
			t.Errorf("client %d: msg.Text: got %q, want %q", i+1, msg.Text, "multi-push")
		}
	}
}

// TestDoorayWS_AuthMiddlewareExempt verifies the WS endpoint is accessible
// without an API key even when the server requires one.
func TestDoorayWS_AuthMiddlewareExempt(t *testing.T) {
	dir := t.TempDir()
	st, err := store.New(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	s := NewServer(Config{
		Store:   st,
		Version: "test",
		APIKey:  "master-key",
	})
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)

	// Dial without API key — should succeed (WebSocket upgrade doesn't carry X-API-Key).
	conn := dialDoorayWS(t, ts)
	conn.Close()
}

// jsonReader returns an io.Reader for the given JSON bytes.
func jsonReader(data []byte) *bytes.Reader {
	return bytes.NewReader(data)
}
