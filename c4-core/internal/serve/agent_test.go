package serve

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// --- @cq mention regex tests ---

func TestCqMentionRegex(t *testing.T) {
	tests := []struct {
		input string
		match bool
	}{
		{"@cq help me", true},
		{"hey @cq what's up?", true},
		{"@CQ do something", true},
		{"@Cq mixed case", true},
		{"no mention here", false},
		{"email@cq.com", false}, // preceded by non-word-boundary
		{"check @cq", true},
		{"@cq", true},
		{"prefix@cq", false},
	}
	for _, tt := range tests {
		got := cqMentionRe.MatchString(tt.input)
		if got != tt.match {
			t.Errorf("cqMentionRe.MatchString(%q) = %v, want %v", tt.input, got, tt.match)
		}
	}
}

// --- claude -p output parsing tests ---

func TestParseClaudeOutput_Success(t *testing.T) {
	data := `{"type":"result","subtype":"success","result":"Hello world"}`
	got := parseClaudeOutput([]byte(data))
	if got != "Hello world" {
		t.Errorf("parseClaudeOutput = %q, want %q", got, "Hello world")
	}
}

func TestParseClaudeOutput_InvalidJSON(t *testing.T) {
	data := "not json at all"
	got := parseClaudeOutput([]byte(data))
	if got != "not json at all" {
		t.Errorf("parseClaudeOutput fallback = %q, want raw string", got)
	}
}

func TestParseClaudeOutput_EmptyResult(t *testing.T) {
	data := `{"type":"result","subtype":"success","result":""}`
	got := parseClaudeOutput([]byte(data))
	// Should return the raw JSON as fallback
	if !strings.Contains(got, "result") {
		t.Errorf("parseClaudeOutput empty result = %q, want fallback with raw data", got)
	}
}

func TestParseClaudeOutput_Truncation(t *testing.T) {
	// Build a string > 4000 chars
	long := strings.Repeat("a", 5000)
	data := fmt.Sprintf(`{"type":"result","subtype":"success","result":"%s"}`, long)
	got := parseClaudeOutput([]byte(data))
	if len(got) > 4100 { // 4000 + "...(truncated)"
		t.Errorf("parseClaudeOutput should truncate long output, got len=%d", len(got))
	}
	if !strings.HasSuffix(got, "...(truncated)") {
		t.Errorf("should end with truncation marker")
	}
}

// --- ClaimMessage tests ---

func TestAgent_ClaimMessage_Success(t *testing.T) {
	// Mock Supabase RPC that returns a row (claim succeeded)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/rest/v1/rpc/claim_message" {
			body, _ := io.ReadAll(r.Body)
			var params map[string]string
			json.Unmarshal(body, &params)
			if params["p_message_id"] != "msg-123" {
				t.Errorf("unexpected message_id: %s", params["p_message_id"])
			}
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`[{"id":"msg-123","claimed_by":"cq-agent"}]`))
			return
		}
		w.WriteHeader(404)
	}))
	defer server.Close()

	agent := NewAgent(AgentConfig{
		SupabaseURL: server.URL,
		APIKey:      "test-key",
		AuthToken:   "test-token",
		ProjectID:   "test-project",
		WorkerID:    "cq-agent",
	})

	claimed, err := agent.claimMessage("msg-123")
	if err != nil {
		t.Fatalf("claimMessage error: %v", err)
	}
	if !claimed {
		t.Error("expected claim to succeed")
	}
}

func TestAgent_ClaimMessage_AlreadyClaimed(t *testing.T) {
	// RPC returns empty array (already claimed by someone else)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[]`))
	}))
	defer server.Close()

	agent := NewAgent(AgentConfig{
		SupabaseURL: server.URL,
		APIKey:      "test-key",
		ProjectID:   "test-project",
	})

	claimed, err := agent.claimMessage("msg-456")
	if err != nil {
		t.Fatalf("claimMessage error: %v", err)
	}
	if claimed {
		t.Error("expected claim to fail (already claimed)")
	}
}

func TestAgent_ClaimMessage_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		w.Write([]byte(`{"message":"internal error"}`))
	}))
	defer server.Close()

	agent := NewAgent(AgentConfig{
		SupabaseURL: server.URL,
		APIKey:      "test-key",
		ProjectID:   "test-project",
	})

	_, err := agent.claimMessage("msg-789")
	if err == nil {
		t.Fatal("expected error on 500 response")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error should mention status code: %v", err)
	}
}

// --- handleEvent filter tests ---

func TestAgent_HandleEvent_Filters(t *testing.T) {
	var claimed bool
	var claimMu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/rest/v1/rpc/claim_message" {
			claimMu.Lock()
			claimed = true
			claimMu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`[{"id":"test"}]`))
			return
		}
		// For postMessage and ensureMember
		if r.URL.Path == "/rest/v1/c1_members" {
			w.Header().Set("Content-Type", "application/json")
			if r.Method == "GET" {
				w.Write([]byte(`[{"id":"member-1"}]`))
			} else {
				w.Write([]byte(`[{"id":"member-1"}]`))
			}
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write([]byte(`{}`))
	}))
	defer server.Close()

	agent := NewAgent(AgentConfig{
		SupabaseURL: server.URL,
		APIKey:      "test-key",
		ProjectID:   "proj-1",
		WorkerID:    "cq-agent",
	})

	tests := []struct {
		name     string
		event    RealtimeEvent
		wantClaim bool
	}{
		{
			name: "wrong table",
			event: RealtimeEvent{
				Table:      "c4_tasks",
				ChangeType: "INSERT",
				Record:     json.RawMessage(`{"id":"1","content":"@cq help","project_id":"proj-1","sender_type":"user"}`),
			},
			wantClaim: false,
		},
		{
			name: "wrong change type",
			event: RealtimeEvent{
				Table:      "c1_messages",
				ChangeType: "UPDATE",
				Record:     json.RawMessage(`{"id":"1","content":"@cq help","project_id":"proj-1","sender_type":"user"}`),
			},
			wantClaim: false,
		},
		{
			name: "no @cq mention",
			event: RealtimeEvent{
				Table:      "c1_messages",
				ChangeType: "INSERT",
				Record:     json.RawMessage(`{"id":"1","content":"hello world","project_id":"proj-1","sender_type":"user"}`),
			},
			wantClaim: false,
		},
		{
			name: "agent message (loop prevention)",
			event: RealtimeEvent{
				Table:      "c1_messages",
				ChangeType: "INSERT",
				Record:     json.RawMessage(`{"id":"1","content":"@cq response","project_id":"proj-1","sender_type":"agent"}`),
			},
			wantClaim: false,
		},
		{
			name: "system message",
			event: RealtimeEvent{
				Table:      "c1_messages",
				ChangeType: "INSERT",
				Record:     json.RawMessage(`{"id":"1","content":"@cq system","project_id":"proj-1","sender_type":"system"}`),
			},
			wantClaim: false,
		},
		{
			name: "wrong project",
			event: RealtimeEvent{
				Table:      "c1_messages",
				ChangeType: "INSERT",
				Record:     json.RawMessage(`{"id":"1","content":"@cq help","project_id":"other-proj","sender_type":"user"}`),
			},
			wantClaim: false,
		},
		{
			name: "already claimed",
			event: RealtimeEvent{
				Table:      "c1_messages",
				ChangeType: "INSERT",
				Record:     json.RawMessage(`{"id":"1","content":"@cq help","project_id":"proj-1","sender_type":"user","claimed_by":"other-worker"}`),
			},
			wantClaim: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claimMu.Lock()
			claimed = false
			claimMu.Unlock()

			agent.handleEvent(tt.event)
			// Give async processing a moment
			time.Sleep(50 * time.Millisecond)

			claimMu.Lock()
			gotClaim := claimed
			claimMu.Unlock()

			if gotClaim != tt.wantClaim {
				t.Errorf("claimed = %v, want %v", gotClaim, tt.wantClaim)
			}
		})
	}
}

// --- Realtime WS URL and message tests ---

func TestRealtimeClient_BuildWSURL(t *testing.T) {
	rc := NewRealtimeClient("https://abc.supabase.co", "my-key", "")
	url := rc.buildWSURL()
	want := "wss://abc.supabase.co/realtime/v1/websocket?apikey=my-key&vsn=1.0.0"
	if url != want {
		t.Errorf("buildWSURL = %q, want %q", url, want)
	}
}

func TestRealtimeClient_BuildWSURL_TrailingSlash(t *testing.T) {
	rc := NewRealtimeClient("https://abc.supabase.co/", "key", "")
	url := rc.buildWSURL()
	if !strings.HasPrefix(url, "wss://abc.supabase.co/realtime/") {
		t.Errorf("trailing slash not handled: %s", url)
	}
}

func TestRealtimeClient_BuildWSURL_HTTP(t *testing.T) {
	rc := NewRealtimeClient("http://localhost:54321", "key", "")
	url := rc.buildWSURL()
	if !strings.HasPrefix(url, "ws://localhost:54321/") {
		t.Errorf("http not converted to ws: %s", url)
	}
}

func TestMakeJoinPayload_WithToken(t *testing.T) {
	payload := makeJoinPayload("c1_messages", "test-token")
	if payload["access_token"] != "test-token" {
		t.Error("access_token not set")
	}
	config, ok := payload["config"].(map[string]interface{})
	if !ok {
		t.Fatal("config not found")
	}
	changes, ok := config["postgres_changes"].([]map[string]interface{})
	if !ok || len(changes) == 0 {
		t.Fatal("postgres_changes not found")
	}
	if changes[0]["table"] != "c1_messages" {
		t.Errorf("table = %v, want c1_messages", changes[0]["table"])
	}
}

func TestMakeJoinPayload_WithoutToken(t *testing.T) {
	payload := makeJoinPayload("c4_tasks", "")
	if _, ok := payload["access_token"]; ok {
		t.Error("access_token should not be set for empty token")
	}
}

func TestPhxMessage_Serialization(t *testing.T) {
	ref := "1"
	msg := phxMessage{
		Topic:   "realtime:public:c1_messages",
		Event:   "phx_join",
		Payload: map[string]interface{}{"test": true},
		Ref:     &ref,
	}
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	var parsed map[string]interface{}
	json.Unmarshal(data, &parsed)
	if parsed["topic"] != "realtime:public:c1_messages" {
		t.Errorf("topic = %v", parsed["topic"])
	}
	if parsed["event"] != "phx_join" {
		t.Errorf("event = %v", parsed["event"])
	}
	if parsed["ref"] != "1" {
		t.Errorf("ref = %v", parsed["ref"])
	}
}

func TestPhxMessage_HeartbeatSerialization(t *testing.T) {
	ref := "hb"
	msg := phxMessage{
		Topic:   "phoenix",
		Event:   "heartbeat",
		Payload: map[string]interface{}{},
		Ref:     &ref,
	}
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	var parsed map[string]interface{}
	json.Unmarshal(data, &parsed)
	if parsed["topic"] != "phoenix" {
		t.Errorf("topic = %v", parsed["topic"])
	}
	if parsed["event"] != "heartbeat" {
		t.Errorf("event = %v", parsed["event"])
	}
}

// --- Realtime handleMessage tests ---

func TestRealtimeClient_HandleMessage_PostgresChanges(t *testing.T) {
	rc := NewRealtimeClient("https://example.com", "key", "")
	var received RealtimeEvent
	var mu sync.Mutex

	callback := func(event RealtimeEvent) {
		mu.Lock()
		received = event
		mu.Unlock()
	}

	// Simulate a postgres_changes message from Supabase Realtime
	msg := `{
		"topic": "realtime:public:c1_messages",
		"event": "postgres_changes",
		"payload": {
			"data": {
				"type": "INSERT",
				"record": {"id": "msg-1", "content": "hello @cq"},
				"old_record": null
			}
		},
		"ref": null
	}`

	rc.handleMessage([]byte(msg), callback)

	mu.Lock()
	defer mu.Unlock()

	if received.Table != "c1_messages" {
		t.Errorf("table = %q, want c1_messages", received.Table)
	}
	if received.ChangeType != "INSERT" {
		t.Errorf("change_type = %q, want INSERT", received.ChangeType)
	}
}

func TestRealtimeClient_HandleMessage_NonPostgresChanges(t *testing.T) {
	rc := NewRealtimeClient("https://example.com", "key", "")
	called := false
	callback := func(event RealtimeEvent) {
		called = true
	}

	// Non-postgres_changes events should be ignored
	msg := `{"topic":"phoenix","event":"phx_reply","payload":{},"ref":"hb"}`
	rc.handleMessage([]byte(msg), callback)

	if called {
		t.Error("callback should not be called for non-postgres_changes events")
	}
}

func TestRealtimeClient_HandleMessage_InvalidJSON(t *testing.T) {
	rc := NewRealtimeClient("https://example.com", "key", "")
	called := false
	callback := func(event RealtimeEvent) {
		called = true
	}

	rc.handleMessage([]byte("not json"), callback)

	if called {
		t.Error("callback should not be called for invalid JSON")
	}
}

// --- Reconnection backoff tests ---

func TestRealtimeClient_ConnectNoTables(t *testing.T) {
	rc := NewRealtimeClient("https://example.com", "key", "")
	rc.OnMessage(func(event RealtimeEvent) {})
	err := rc.Connect(context.Background())
	if err == nil || !strings.Contains(err.Error(), "no tables") {
		t.Errorf("expected 'no tables' error, got: %v", err)
	}
}

func TestRealtimeClient_ConnectNoCallback(t *testing.T) {
	rc := NewRealtimeClient("https://example.com", "key", "")
	rc.Subscribe("c1_messages")
	err := rc.Connect(context.Background())
	if err == nil || !strings.Contains(err.Error(), "no message callback") {
		t.Errorf("expected 'no message callback' error, got: %v", err)
	}
}

func TestRealtimeClient_DoubleConnect(t *testing.T) {
	rc := NewRealtimeClient("https://example.com", "key", "")
	rc.Subscribe("c1_messages")
	rc.OnMessage(func(event RealtimeEvent) {})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// First connect will fail to dial but that's OK - we're testing the double-connect guard
	_ = rc.Connect(ctx)
	defer rc.Close()

	err := rc.Connect(ctx)
	if err == nil || !strings.Contains(err.Error(), "already connected") {
		t.Errorf("expected 'already connected' error, got: %v", err)
	}
}

// --- Agent lifecycle tests ---

func TestAgent_StartNoConfig(t *testing.T) {
	agent := NewAgent(AgentConfig{})
	err := agent.Start(context.Background())
	if err == nil {
		t.Fatal("expected error with empty config")
	}
	if !strings.Contains(err.Error(), "required") {
		t.Errorf("error = %v, want 'required'", err)
	}
}

func TestAgent_DefaultWorkerID(t *testing.T) {
	agent := NewAgent(AgentConfig{})
	if agent.cfg.WorkerID != "cq-agent" {
		t.Errorf("default worker_id = %q, want 'cq-agent'", agent.cfg.WorkerID)
	}
}

func TestAgent_Name(t *testing.T) {
	agent := NewAgent(AgentConfig{})
	if agent.Name() != "agent" {
		t.Errorf("Name() = %q, want 'agent'", agent.Name())
	}
}

func TestAgent_InitialHealth(t *testing.T) {
	agent := NewAgent(AgentConfig{})
	h := agent.Health()
	if h.Status != "ok" {
		t.Errorf("initial health status = %q, want %q", h.Status, "ok")
	}
}

// --- Truncate tests ---

func TestTruncate(t *testing.T) {
	if got := truncate("short", 100); got != "short" {
		t.Errorf("truncate short = %q", got)
	}
	if got := truncate("abcde", 3); got != "abc...(truncated)" {
		t.Errorf("truncate long = %q", got)
	}
}

// --- postMessage integration test ---

func TestAgent_PostMessage(t *testing.T) {
	var postedContent string
	var mu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/rest/v1/c1_messages" && r.Method == "POST" {
			body, _ := io.ReadAll(r.Body)
			var msg map[string]interface{}
			json.Unmarshal(body, &msg)
			mu.Lock()
			postedContent = msg["content"].(string)
			mu.Unlock()
			w.WriteHeader(201)
			return
		}
		if r.URL.Path == "/rest/v1/c1_members" && r.Method == "GET" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`[{"id":"member-1"}]`))
			return
		}
		w.WriteHeader(200)
	}))
	defer server.Close()

	agent := NewAgent(AgentConfig{
		SupabaseURL: server.URL,
		APIKey:      "test-key",
		ProjectID:   "proj-1",
		WorkerID:    "cq-agent",
	})

	agent.postMessage("channel-1", "Hello from test")

	mu.Lock()
	defer mu.Unlock()
	if postedContent != "Hello from test" {
		t.Errorf("posted content = %q, want 'Hello from test'", postedContent)
	}
}

// --- Stop() context-awareness tests ---

func TestAgent_Stop_NoGoroutines_ReturnsImmediately(t *testing.T) {
	agent := NewAgent(AgentConfig{WorkerID: "test-worker"})
	// No goroutines running — Stop should return immediately even with a tight deadline.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- agent.Stop(ctx) }()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Stop() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Stop() blocked for >2s with no goroutines")
	}
}

func TestAgent_Stop_CancelledCtx_ReturnsBeforeWg(t *testing.T) {
	agent := NewAgent(AgentConfig{WorkerID: "test-worker"})

	// Simulate an in-flight goroutine that will never finish.
	agent.wg.Add(1)
	agent.inFlight.Add(1)
	defer func() {
		// Clean up: release the WaitGroup after the test so the leaked goroutine exits.
		agent.wg.Done()
		agent.inFlight.Add(-1)
	}()

	// Stop with an already-cancelled context — must return within a short time.
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled

	start := time.Now()
	err := agent.Stop(ctx)
	elapsed := time.Since(start)

	if err != nil {
		t.Errorf("Stop() error = %v", err)
	}
	if elapsed > 2*time.Second {
		t.Errorf("Stop() took %v with cancelled ctx, want <2s", elapsed)
	}
}

