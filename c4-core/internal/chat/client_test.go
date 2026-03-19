package chat

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
)

// --- Unit tests for SendMessage ---

func TestSendMessage_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if !strings.Contains(r.URL.Path, "c1_messages") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("apikey") == "" {
			t.Error("missing apikey header")
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	c := New(srv.URL, "anon-key", "access-token")
	ctx := context.Background()
	if err := c.SendMessage(ctx, "chan-uuid", "hello", "user"); err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
}

func TestSendMessage_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := New(srv.URL, "anon-key", "")
	ctx := context.Background()
	err := c.SendMessage(ctx, "chan-uuid", "hello", "agent")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("expected status 500 in error, got: %v", err)
	}
}

func TestSendMessage_EmptyChannelID(t *testing.T) {
	c := New("http://example.com", "anon-key", "")
	ctx := context.Background()
	err := c.SendMessage(ctx, "", "hello", "user")
	if err == nil {
		t.Fatal("expected error for empty channelID, got nil")
	}
}

func TestSendMessage_AuthHeader(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	// With access token — should use access token, not anon key
	c := New(srv.URL, "anon-key", "my-jwt")
	if err := c.SendMessage(context.Background(), "chan-uuid", "hi", "user"); err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if gotAuth != "Bearer my-jwt" {
		t.Errorf("expected 'Bearer my-jwt', got %q", gotAuth)
	}
}

// --- Unit tests for Subscribe / Connect lifecycle ---

func TestSubscribe_RequiresHandlerBeforeConnect(t *testing.T) {
	c := New("wss://example.supabase.co", "anon-key", "")
	// No Subscribe call — Connect should fail
	err := c.Connect(context.Background())
	if err == nil {
		t.Fatal("expected error when no handler set")
	}
	if !strings.Contains(err.Error(), "Subscribe") {
		t.Errorf("expected Subscribe in error, got: %v", err)
	}
}

func TestConnect_AlreadyRunning(t *testing.T) {
	c := New("wss://example.supabase.co", "anon-key", "")
	c.Subscribe("", func(Message) {})
	// Force running=true to simulate already-connected state
	c.mu.Lock()
	c.running = true
	c.mu.Unlock()

	err := c.Connect(context.Background())
	if err == nil {
		t.Fatal("expected error when already connected")
	}
	if !strings.Contains(err.Error(), "already connected") {
		t.Errorf("expected 'already connected', got: %v", err)
	}
}

// --- WebSocket integration test ---

// TestRealtimeSubscribe verifies that c1_messages INSERT events are delivered to the handler.
func TestRealtimeSubscribe(t *testing.T) {
	// Spin up a minimal Phoenix Channel server
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, _, _, err := ws.UpgradeHTTP(r, w)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		defer conn.Close()

		// Read phx_join
		data, _, err := wsutil.ReadClientData(conn)
		if err != nil {
			return
		}
		var msg phxMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			return
		}
		// Send phx_reply for join
		ref := msg.Ref
		reply := phxMessage{
			Topic:   msg.Topic,
			Event:   "phx_reply",
			Payload: map[string]interface{}{"status": "ok", "response": map[string]interface{}{}},
			Ref:     ref,
		}
		replyData, _ := json.Marshal(reply)
		_ = wsutil.WriteServerText(conn, replyData)

		// Send a fake postgres_changes INSERT event
		record := Message{
			ID:         "msg-1",
			ChannelID:  "chan-1",
			SenderName: "alice",
			SenderType: "user",
			Content:    "hello world",
			CreatedAt:  "2024-01-01T00:00:00Z",
		}
		recordJSON, _ := json.Marshal(record)
		event := map[string]interface{}{
			"topic": "realtime:public:c1_messages",
			"event": "postgres_changes",
			"payload": map[string]interface{}{
				"data": map[string]interface{}{
					"type":   "INSERT",
					"record": json.RawMessage(recordJSON),
				},
			},
			"ref": nil,
		}
		eventData, _ := json.Marshal(event)
		_ = wsutil.WriteServerText(conn, eventData)

		// Keep connection open briefly
		time.Sleep(200 * time.Millisecond)
	}))
	defer srv.Close()

	// Convert http to ws URL
	wsURL := "ws://" + srv.Listener.Addr().String()

	received := make(chan Message, 1)
	c := New(wsURL, "anon-key", "")
	// Override supabaseURL to point to test server directly
	c.supabaseURL = wsURL

	c.Subscribe("", func(msg Message) {
		received <- msg
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := c.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer c.Close()

	select {
	case msg := <-received:
		if msg.Content != "hello world" {
			t.Errorf("expected 'hello world', got %q", msg.Content)
		}
		if msg.SenderType != "user" {
			t.Errorf("expected sender_type 'user', got %q", msg.SenderType)
		}
	case <-ctx.Done():
		t.Fatal("timeout waiting for message")
	}
}

// TestHandleMessage_IgnoresNonInsert verifies non-INSERT events are dropped.
func TestHandleMessage_IgnoresNonInsert(t *testing.T) {
	c := New("http://example.com", "anon-key", "")
	called := false
	handler := func(Message) { called = true }

	// UPDATE event — should be ignored
	payload, _ := json.Marshal(map[string]interface{}{
		"data": map[string]interface{}{
			"type":   "UPDATE",
			"record": map[string]string{"id": "x"},
		},
	})
	data, _ := json.Marshal(map[string]interface{}{
		"event":   "postgres_changes",
		"payload": json.RawMessage(payload),
	})
	c.handleMessage(data, handler)
	if called {
		t.Error("handler should not be called for UPDATE events")
	}

	// Non-postgres_changes event — should be ignored
	data2, _ := json.Marshal(map[string]interface{}{
		"event":   "phx_reply",
		"payload": map[string]interface{}{},
	})
	c.handleMessage(data2, handler)
	if called {
		t.Error("handler should not be called for non-postgres_changes events")
	}
}

// TestBuildWSURL verifies URL transformation.
func TestBuildWSURL(t *testing.T) {
	cases := []struct {
		supabaseURL string
		wantPrefix  string
	}{
		{"https://abc.supabase.co", "wss://abc.supabase.co"},
		{"http://localhost:54321", "ws://localhost:54321"},
	}
	for _, tc := range cases {
		c := New(tc.supabaseURL, "key", "")
		got := c.buildWSURL()
		if !strings.HasPrefix(got, tc.wantPrefix) {
			t.Errorf("buildWSURL(%q) = %q, want prefix %q", tc.supabaseURL, got, tc.wantPrefix)
		}
		if !strings.Contains(got, "realtime/v1/websocket") {
			t.Errorf("buildWSURL missing realtime path: %q", got)
		}
	}
}
