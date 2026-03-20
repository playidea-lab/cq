package doorayhandler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/changmin/c4-core/internal/mcp"
	"github.com/changmin/c4-core/internal/mcp/handlers/doorayhandler"
	"github.com/changmin/c4-core/internal/serve"
	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
)

// startWSServer starts a test server that upgrades /v1/dooray/ws to WebSocket
// and sends the given JSON payloads, then holds the connection open.
func startWSServer(t *testing.T, payloads []map[string]any) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/dooray/ws" {
			http.NotFound(w, r)
			return
		}
		conn, _, _, err := ws.UpgradeHTTP(r, w)
		if err != nil {
			t.Logf("ws upgrade: %v", err)
			return
		}
		defer conn.Close()
		for _, p := range payloads {
			data, _ := json.Marshal(p)
			conn.SetWriteDeadline(time.Now().Add(5 * time.Second)) //nolint:errcheck
			if err := wsutil.WriteServerMessage(conn, ws.OpText, data); err != nil {
				return
			}
		}
		// Hold open.
		for {
			conn.SetReadDeadline(time.Now().Add(5 * time.Second)) //nolint:errcheck
			if _, _, err := wsutil.ReadClientData(conn); err != nil {
				return
			}
		}
	}))
	return srv
}

// waitMessages polls comp.GetMessages until count is reached or deadline passes.
func waitMessages(comp *serve.DoorayChannelComponent, count int, timeout time.Duration) []serve.DoorayMessage {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if msgs := comp.GetMessages(); len(msgs) >= count {
			return msgs
		}
		time.Sleep(20 * time.Millisecond)
	}
	return comp.GetMessages()
}

// TestPollFromLocalBuffer verifies that c4_dooray_poll reads from the local buffer
// (via DoorayChannelComponent) when Channel is set, and auto-acks each message.
func TestPollFromLocalBuffer(t *testing.T) {
	payloads := []map[string]any{
		{"id": "msg-1", "channelId": "ch1", "senderId": "u1", "text": "hello"},
		{"id": "msg-2", "channelId": "ch1", "senderId": "u2", "text": "world"},
	}
	wsSrv := startWSServer(t, payloads)
	defer wsSrv.Close()

	comp := serve.NewDoorayChannel(serve.DoorayChannelConfig{HubURL: wsSrv.URL})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := comp.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer comp.Stop(context.Background()) //nolint:errcheck

	// Wait for messages to arrive.
	if msgs := waitMessages(comp, 2, 3*time.Second); len(msgs) < 2 {
		t.Fatalf("expected 2 messages in buffer, got %d", len(msgs))
	}

	reg := mcp.NewRegistry()
	doorayhandler.Register(reg, doorayhandler.DoorayConfig{
		HubURL:  wsSrv.URL,
		Channel: comp,
	})

	result, err := reg.Call("c4_dooray_poll", json.RawMessage("{}"))
	if err != nil {
		t.Fatalf("c4_dooray_poll: %v", err)
	}

	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("result type = %T, want map[string]any", result)
	}
	if m["count"] != 2 {
		t.Errorf("count = %v, want 2", m["count"])
	}

	// After poll, buffer should be empty (auto-acked).
	if remaining := comp.GetMessages(); len(remaining) != 0 {
		t.Errorf("expected buffer empty after poll, got %d messages", len(remaining))
	}
}

// TestPollFromLocalBufferEmpty verifies that c4_dooray_poll returns empty when no messages.
func TestPollFromLocalBufferEmpty(t *testing.T) {
	wsSrv := startWSServer(t, nil)
	defer wsSrv.Close()

	comp := serve.NewDoorayChannel(serve.DoorayChannelConfig{HubURL: wsSrv.URL})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := comp.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer comp.Stop(context.Background()) //nolint:errcheck

	reg := mcp.NewRegistry()
	doorayhandler.Register(reg, doorayhandler.DoorayConfig{
		HubURL:  wsSrv.URL,
		Channel: comp,
	})

	result, err := reg.Call("c4_dooray_poll", json.RawMessage("{}"))
	if err != nil {
		t.Fatalf("c4_dooray_poll: %v", err)
	}
	m := result.(map[string]any)
	if m["count"] != 0 {
		t.Errorf("count = %v, want 0", m["count"])
	}
}

// TestPollStandaloneMode verifies that when Channel is nil, c4_dooray_poll
// falls back to the Hub HTTP endpoint.
func TestPollStandaloneMode(t *testing.T) {
	// Serve a mock Hub /v1/dooray/pending endpoint.
	hubMsgs := []map[string]any{
		{"id": "hub-1", "text": "from hub"},
	}
	httpSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/dooray/pending" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(hubMsgs) //nolint:errcheck
			return
		}
		http.NotFound(w, r)
	}))
	defer httpSrv.Close()

	reg := mcp.NewRegistry()
	// Channel is nil → standalone mode.
	doorayhandler.Register(reg, doorayhandler.DoorayConfig{
		HubURL: httpSrv.URL,
	})

	result, err := reg.Call("c4_dooray_poll", json.RawMessage("{}"))
	if err != nil {
		t.Fatalf("c4_dooray_poll: %v", err)
	}
	m := result.(map[string]any)
	if m["count"] != 1 {
		t.Errorf("count = %v, want 1", m["count"])
	}
}
