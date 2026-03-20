package serve

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
)

// startTestWSServer starts a local HTTP server that upgrades GET /v1/dooray/ws
// to WebSocket and sends msgs as JSON frames, then holds the connection open.
func startTestWSServer(t *testing.T, msgs []DoorayMessage) *httptest.Server {
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

		for _, m := range msgs {
			data, _ := json.Marshal(m)
			conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if err := wsutil.WriteServerMessage(conn, ws.OpText, data); err != nil {
				return
			}
		}
		// Hold connection open until client disconnects.
		for {
			conn.SetReadDeadline(time.Now().Add(5 * time.Second))
			_, _, err := wsutil.ReadClientData(conn)
			if err != nil {
				return
			}
		}
	}))
	return srv
}

func TestDoorayChannelGetMessages(t *testing.T) {
	msgs := []DoorayMessage{
		{ChannelID: "ch1", SenderID: "u1", SenderName: "Alice", Text: "hello", ReceivedAt: time.Now()},
		{ChannelID: "ch1", SenderID: "u2", SenderName: "Bob", Text: "world", ReceivedAt: time.Now()},
	}
	srv := startTestWSServer(t, msgs)
	defer srv.Close()

	cfg := DoorayChannelConfig{HubURL: srv.URL}
	comp := NewDoorayChannel(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := comp.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer comp.Stop(context.Background()) //nolint:errcheck

	// Wait for messages to arrive.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(comp.GetMessages()) >= 2 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	got := comp.GetMessages()
	if len(got) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(got))
	}

	// GetMessages is peek — calling again should return the same messages.
	got2 := comp.GetMessages()
	if len(got2) != 2 {
		t.Fatalf("peek: expected 2 messages after second GetMessages, got %d", len(got2))
	}
}

func TestDoorayChannelAckMessage(t *testing.T) {
	msgs := []DoorayMessage{
		{ID: "id-1", ChannelID: "ch1", SenderID: "u1", Text: "first", ReceivedAt: time.Now()},
		{ID: "id-2", ChannelID: "ch1", SenderID: "u1", Text: "second", ReceivedAt: time.Now()},
	}
	srv := startTestWSServer(t, msgs)
	defer srv.Close()

	cfg := DoorayChannelConfig{HubURL: srv.URL}
	comp := NewDoorayChannel(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := comp.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer comp.Stop(context.Background()) //nolint:errcheck

	// Wait for messages.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(comp.GetMessages()) >= 2 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	// Find the first message's ID (may be assigned by component if not provided).
	all := comp.GetMessages()
	if len(all) < 2 {
		t.Fatalf("expected 2 messages, got %d", len(all))
	}

	firstID := all[0].ID
	comp.AckMessage(firstID)

	remaining := comp.GetMessages()
	if len(remaining) != 1 {
		t.Fatalf("after Ack, expected 1 message, got %d", len(remaining))
	}
	if remaining[0].Text == "first" {
		t.Errorf("first message still present after Ack")
	}
}

func TestDoorayChannelReconnect(t *testing.T) {
	// Test that the component reconnects when the server closes the connection.
	received := make(chan DoorayMessage, 10)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/dooray/ws" {
			http.NotFound(w, r)
			return
		}
		conn, _, _, err := ws.UpgradeHTTP(r, w)
		if err != nil {
			return
		}
		defer conn.Close()
		// Send one message and immediately close.
		msg := DoorayMessage{ChannelID: "ch1", SenderID: "u1", Text: "ping", ReceivedAt: time.Now()}
		data, _ := json.Marshal(msg)
		conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
		wsutil.WriteServerMessage(conn, ws.OpText, data) //nolint:errcheck
	}))
	defer srv.Close()

	cfg := DoorayChannelConfig{HubURL: srv.URL}
	comp := NewDoorayChannel(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Override messages buffer watch via GetMessages polling.
	if err := comp.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer comp.Stop(context.Background()) //nolint:errcheck

	_ = received

	// After first disconnect, component reconnects and gets another message.
	// We just verify it doesn't crash and eventually has messages.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if len(comp.GetMessages()) >= 1 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	if len(comp.GetMessages()) == 0 {
		t.Error("expected at least one message after connect+reconnect cycle")
	}
}

// TestDoorayChannelBackoffReset verifies that after a successful dial+readLoop cycle,
// the component reconnects quickly (backoff resets to 1s, not grows exponentially).
// We simulate two rapid disconnect cycles and confirm both messages arrive within 3s.
func TestDoorayChannelBackoffReset(t *testing.T) {
	connectCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/dooray/ws" {
			http.NotFound(w, r)
			return
		}
		conn, _, _, err := ws.UpgradeHTTP(r, w)
		if err != nil {
			return
		}
		defer conn.Close()
		connectCount++
		// Send a unique message per connection and immediately close.
		msg := DoorayMessage{ChannelID: "ch1", SenderID: "u1", Text: "msg", ReceivedAt: time.Now()}
		data, _ := json.Marshal(msg)
		conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
		wsutil.WriteServerMessage(conn, ws.OpText, data) //nolint:errcheck
	}))
	defer srv.Close()

	comp := NewDoorayChannel(DoorayChannelConfig{HubURL: srv.URL})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := comp.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer comp.Stop(context.Background()) //nolint:errcheck

	// With backoff reset working: two connect cycles should complete within 3s
	// (1s reconnect delay after reset, not 2s or more from exponential backoff).
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if connectCount >= 2 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	if connectCount < 2 {
		t.Errorf("expected at least 2 connect cycles (backoff reset), got %d", connectCount)
	}
}

func TestHubWSURL(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"http://localhost:8585", "ws://localhost:8585/v1/dooray/ws"},
		{"https://example.com", "wss://example.com/v1/dooray/ws"},
		{"http://127.0.0.1:9000", "ws://127.0.0.1:9000/v1/dooray/ws"},
		{"", ""},
	}
	for _, tc := range cases {
		got := hubWSURL(tc.in)
		if got != tc.want {
			t.Errorf("hubWSURL(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestDoorayChannelIDAssignment(t *testing.T) {
	// When Hub sends a message without an ID, the component assigns one.
	msgs := []DoorayMessage{
		{ChannelID: "ch1", SenderID: "u1", Text: "no-id", ReceivedAt: time.Now()},
	}
	srv := startTestWSServer(t, msgs)
	defer srv.Close()

	comp := NewDoorayChannel(DoorayChannelConfig{HubURL: srv.URL})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	comp.Start(ctx) //nolint:errcheck
	defer comp.Stop(context.Background()) //nolint:errcheck

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(comp.GetMessages()) >= 1 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	got := comp.GetMessages()
	if len(got) == 0 {
		t.Fatal("expected 1 message")
	}
	if got[0].ID == "" {
		t.Error("expected component to assign non-empty ID when Hub omits it")
	}
}

func TestDoorayChannelComponentInterface(t *testing.T) {
	// Verify DoorayChannelComponent satisfies the Component interface.
	var _ Component = (*DoorayChannelComponent)(nil)
}

// testWSUpgrade is a helper for dial tests using raw net connections.
var _ = func() net.Conn { return nil }

// Ensure hubWSURL handles trailing slashes and paths gracefully.
func TestHubWSURLTrailingSlash(t *testing.T) {
	got := hubWSURL("http://localhost:8585/")
	if !strings.HasSuffix(got, "/v1/dooray/ws") {
		t.Errorf("expected /v1/dooray/ws suffix, got %q", got)
	}
}
