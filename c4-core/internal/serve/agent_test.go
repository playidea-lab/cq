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

// --- updateMemberPresence tests ---

func TestAgent_UpdateMemberPresence_Success(t *testing.T) {
	var patchPath string
	var patchBody map[string]interface{}
	var mu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "PATCH" && strings.HasPrefix(r.URL.Path, "/rest/v1/c1_members") {
			mu.Lock()
			patchPath = r.URL.String()
			body, _ := io.ReadAll(r.Body)
			json.Unmarshal(body, &patchBody)
			mu.Unlock()
			w.WriteHeader(204)
			return
		}
		w.WriteHeader(404)
	}))
	defer server.Close()

	agent := NewAgent(AgentConfig{
		SupabaseURL: server.URL,
		APIKey:      "test-key",
		ProjectID:   "proj-1",
		WorkerID:    "cq-agent",
	})
	// Inject a known memberID so the PATCH is called
	agent.memberID = "member-abc"

	agent.updateMemberPresence("typing")

	mu.Lock()
	defer mu.Unlock()

	if !strings.Contains(patchPath, "id=eq.member-abc") {
		t.Errorf("PATCH URL should filter by memberID, got: %s", patchPath)
	}
	if patchBody["status"] != "typing" {
		t.Errorf("status = %v, want 'typing'", patchBody["status"])
	}
	if _, ok := patchBody["last_seen_at"]; !ok {
		t.Error("last_seen_at should be set")
	}
}

func TestAgent_UpdateMemberPresence_NoMemberID(t *testing.T) {
	patchCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "PATCH" {
			patchCalled = true
		}
		w.WriteHeader(200)
	}))
	defer server.Close()

	agent := NewAgent(AgentConfig{
		SupabaseURL: server.URL,
		APIKey:      "test-key",
		ProjectID:   "proj-1",
	})
	// memberID is empty — updateMemberPresence should be a no-op
	agent.updateMemberPresence("typing")

	if patchCalled {
		t.Error("PATCH should not be called when memberID is empty")
	}
}

func TestAgent_UpdateMemberPresence_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		w.Write([]byte(`{"message":"error"}`))
	}))
	defer server.Close()

	agent := NewAgent(AgentConfig{
		SupabaseURL: server.URL,
		APIKey:      "test-key",
		ProjectID:   "proj-1",
	})
	agent.memberID = "member-abc"

	// Should not panic — errors are logged and swallowed
	agent.updateMemberPresence("online")
}

// --- ProjectDir passthrough ---

func TestAgent_ProjectDirConfig(t *testing.T) {
	agent := NewAgent(AgentConfig{
		SupabaseURL: "https://example.supabase.co",
		APIKey:      "key",
		ProjectID:   "proj-1",
		ProjectDir:  "/home/user/myproject",
	})
	agent.mu.Lock()
	dir := agent.cfg.ProjectDir
	agent.mu.Unlock()
	if dir != "/home/user/myproject" {
		t.Errorf("ProjectDir = %q, want '/home/user/myproject'", dir)
	}
}

// --- buildA2UIPrompt tests ---

func TestBuildA2UIPrompt_WithContext(t *testing.T) {
	msgs := []channelMsg{
		{
			ID:         "m1",
			Content:    "user message",
			SenderType: "user",
			Metadata:   json.RawMessage(`{}`),
		},
		{
			ID:         "m2",
			Content:    "Here are your options:",
			SenderType: "agent",
			Metadata:   json.RawMessage(`{"a2ui":{"items":[{"id":"act-1","label":"Option A"}]}}`),
		},
	}
	result := buildA2UIPrompt(msgs, "act-1", "Option A")
	if !strings.Contains(result, "act-1") {
		t.Errorf("result should contain actionID, got: %s", result)
	}
	if !strings.Contains(result, "Option A") {
		t.Errorf("result should contain label, got: %s", result)
	}
	if !strings.Contains(result, "Here are your options:") {
		t.Errorf("result should contain original agent content, got: %s", result)
	}
	if !strings.Contains(result, "[A2UI action selected]") {
		t.Errorf("result should contain A2UI header, got: %s", result)
	}
}

func TestBuildA2UIPrompt_NoContext(t *testing.T) {
	msgs := []channelMsg{
		{
			ID:         "m1",
			Content:    "user message",
			SenderType: "user",
			Metadata:   json.RawMessage(`{}`),
		},
		{
			ID:         "m2",
			Content:    "regular agent reply",
			SenderType: "agent",
			Metadata:   json.RawMessage(`{"other_key":"value"}`),
		},
	}
	result := buildA2UIPrompt(msgs, "act-1", "My Label")
	if result != "My Label" {
		t.Errorf("result = %q, want %q (fallback to label)", result, "My Label")
	}
}

func TestBuildA2UIPrompt_EmptyMessages(t *testing.T) {
	result := buildA2UIPrompt([]channelMsg{}, "act-1", "Button Text")
	if result != "Button Text" {
		t.Errorf("result = %q, want %q (fallback to label)", result, "Button Text")
	}
}

// --- handleEvent A2UI response tests ---

func TestAgent_HandleEvent_A2UIResponse_Triggers(t *testing.T) {
	var claimed bool
	var claimMu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/rest/v1/rpc/claim_message" {
			claimMu.Lock()
			claimed = true
			claimMu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`[{"id":"msg-a2u"}]`))
			return
		}
		if r.URL.Path == "/rest/v1/c1_members" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`[{"id":"member-1"}]`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write([]byte(`[]`))
	}))
	defer server.Close()

	agent := NewAgent(AgentConfig{
		SupabaseURL: server.URL,
		APIKey:      "test-key",
		ProjectID:   "proj-1",
		WorkerID:    "cq-agent",
	})

	event := RealtimeEvent{
		Table:      "c1_messages",
		ChangeType: "INSERT",
		Record:     json.RawMessage(`{"id":"msg-a2u","channel_id":"ch-1","content":"Option A","sender_name":"alice","sender_type":"user","project_id":"proj-1","metadata":{"a2ui_response":{"action_id":"act-1"}}}`),
	}
	agent.handleEvent(event)
	time.Sleep(50 * time.Millisecond)

	claimMu.Lock()
	gotClaim := claimed
	claimMu.Unlock()

	if !gotClaim {
		t.Error("expected claim to be called for a2ui_response with action_id")
	}
}

func TestAgent_HandleEvent_A2UIResponse_AgentSkipped(t *testing.T) {
	var claimed bool
	var claimMu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/rest/v1/rpc/claim_message" {
			claimMu.Lock()
			claimed = true
			claimMu.Unlock()
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[]`))
	}))
	defer server.Close()

	agent := NewAgent(AgentConfig{
		SupabaseURL: server.URL,
		APIKey:      "test-key",
		ProjectID:   "proj-1",
	})

	event := RealtimeEvent{
		Table:      "c1_messages",
		ChangeType: "INSERT",
		Record:     json.RawMessage(`{"id":"msg-1","channel_id":"ch-1","content":"Option A","sender_type":"agent","project_id":"proj-1","metadata":{"a2ui_response":{"action_id":"act-1"}}}`),
	}
	agent.handleEvent(event)
	time.Sleep(50 * time.Millisecond)

	claimMu.Lock()
	gotClaim := claimed
	claimMu.Unlock()

	if gotClaim {
		t.Error("agent sender_type should be filtered before A2UI check — claim must NOT be called")
	}
}

func TestAgent_HandleEvent_A2UIResponse_SystemSkipped(t *testing.T) {
	var claimed bool
	var claimMu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/rest/v1/rpc/claim_message" {
			claimMu.Lock()
			claimed = true
			claimMu.Unlock()
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[]`))
	}))
	defer server.Close()

	agent := NewAgent(AgentConfig{
		SupabaseURL: server.URL,
		APIKey:      "test-key",
		ProjectID:   "proj-1",
	})

	event := RealtimeEvent{
		Table:      "c1_messages",
		ChangeType: "INSERT",
		Record:     json.RawMessage(`{"id":"msg-1","channel_id":"ch-1","content":"Option A","sender_type":"system","project_id":"proj-1","metadata":{"a2ui_response":{"action_id":"act-1"}}}`),
	}
	agent.handleEvent(event)
	time.Sleep(50 * time.Millisecond)

	claimMu.Lock()
	gotClaim := claimed
	claimMu.Unlock()

	if gotClaim {
		t.Error("system sender_type should be filtered before A2UI check — claim must NOT be called")
	}
}

func TestAgent_HandleEvent_A2UIResponse_NoActionID(t *testing.T) {
	var claimed bool
	var claimMu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/rest/v1/rpc/claim_message" {
			claimMu.Lock()
			claimed = true
			claimMu.Unlock()
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[]`))
	}))
	defer server.Close()

	agent := NewAgent(AgentConfig{
		SupabaseURL: server.URL,
		APIKey:      "test-key",
		ProjectID:   "proj-1",
	})

	// a2ui_response present but action_id is empty string
	event := RealtimeEvent{
		Table:      "c1_messages",
		ChangeType: "INSERT",
		Record:     json.RawMessage(`{"id":"msg-1","channel_id":"ch-1","content":"something","sender_type":"user","project_id":"proj-1","metadata":{"a2ui_response":{"action_id":""}}}`),
	}
	agent.handleEvent(event)
	time.Sleep(50 * time.Millisecond)

	claimMu.Lock()
	gotClaim := claimed
	claimMu.Unlock()

	if gotClaim {
		t.Error("empty action_id with no @cq mention should NOT trigger claim")
	}
}

// --- fetchChannelContext tests ---

func TestAgent_FetchChannelContext_Success(t *testing.T) {
	var capturedURL string
	var capturedMu sync.Mutex

	returnedMsgs := []map[string]interface{}{
		{"id": "m1", "content": "hello", "sender_type": "user", "metadata": nil},
		{"id": "m2", "content": "world", "sender_type": "agent", "metadata": map[string]interface{}{"a2ui": true}},
	}
	returnedJSON, _ := json.Marshal(returnedMsgs)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedMu.Lock()
		capturedURL = r.URL.String()
		capturedMu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.Write(returnedJSON)
	}))
	defer server.Close()

	agent := NewAgent(AgentConfig{
		SupabaseURL: server.URL,
		APIKey:      "test-key",
		ProjectID:   "proj-1",
	})

	msgs, err := agent.fetchChannelContext("chan-xyz", 20)
	if err != nil {
		t.Fatalf("fetchChannelContext error: %v", err)
	}
	if len(msgs) != 2 {
		t.Errorf("expected 2 messages, got %d", len(msgs))
	}

	capturedMu.Lock()
	u := capturedURL
	capturedMu.Unlock()

	if !strings.Contains(u, "channel_id=eq.chan-xyz") {
		t.Errorf("URL should filter by channel_id, got: %s", u)
	}
	if !strings.Contains(u, "order=created_at.desc") {
		t.Errorf("URL should have order param, got: %s", u)
	}
	if !strings.Contains(u, "limit=20") {
		t.Errorf("URL should have limit param, got: %s", u)
	}
	if !strings.Contains(u, "select=id,content,sender_type,metadata") {
		t.Errorf("URL should have select param, got: %s", u)
	}
}

func TestAgent_FetchChannelContext_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		w.Write([]byte(`{"message":"internal server error"}`))
	}))
	defer server.Close()

	agent := NewAgent(AgentConfig{
		SupabaseURL: server.URL,
		APIKey:      "test-key",
		ProjectID:   "proj-1",
	})

	_, err := agent.fetchChannelContext("ch-1", 10)
	if err == nil {
		t.Fatal("expected error on 500 response")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error should mention status code, got: %v", err)
	}
}

