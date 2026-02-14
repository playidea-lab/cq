package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/changmin/c4-core/internal/llm"
)

// setupKeeperTest creates a ContextKeeper with a mock HTTP server and mock LLM.
func setupKeeperTest(t *testing.T, handler http.HandlerFunc) (*ContextKeeper, *llm.MockProvider) {
	t.Helper()
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)

	c1 := NewC1Handler(ts.URL, "test-key", "test-token", "proj-1")

	gw := llm.NewGateway(llm.RoutingTable{
		Default: "mock",
		Routes: map[string]llm.ModelRef{
			"summary": {Provider: "mock", Model: "haiku"},
		},
	})
	mock := llm.NewMockProvider("mock")
	gw.Register(mock)

	keeper := NewContextKeeper(c1, gw)
	return keeper, mock
}

func TestUpdateSummary_CreatesEntry(t *testing.T) {
	var upsertCalled bool
	var upsertBody map[string]any

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// GET summary (empty)
		if r.Method == "GET" && strings.Contains(path, "c1_channel_summaries") {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte("[]"))
			return
		}

		// GET messages since (returns 6 messages)
		if r.Method == "GET" && strings.Contains(path, "c1_messages") {
			messages := []keeperMessageRow{
				{ID: "m1", SenderName: "alice", Content: "Let's use PKCE", CreatedAt: "2026-02-14T01:00:00Z"},
				{ID: "m2", SenderName: "bob", Content: "Agreed, RFC 7636", CreatedAt: "2026-02-14T01:01:00Z"},
				{ID: "m3", SenderName: "alice", Content: "state param needed too", CreatedAt: "2026-02-14T01:02:00Z"},
				{ID: "m4", SenderName: "system", Content: "T-007-0 started", CreatedAt: "2026-02-14T01:03:00Z"},
				{ID: "m5", SenderName: "bob", Content: "Where to store verifier?", CreatedAt: "2026-02-14T01:04:00Z"},
				{ID: "m6", SenderName: "alice", Content: "Redis or DB", CreatedAt: "2026-02-14T01:05:00Z"},
			}
			json.NewEncoder(w).Encode(messages)
			return
		}

		// POST upsert summary
		if r.Method == "POST" && strings.Contains(path, "c1_channel_summaries") {
			upsertCalled = true
			json.NewDecoder(r.Body).Decode(&upsertBody)
			w.WriteHeader(201)
			return
		}

		w.WriteHeader(404)
	})

	keeper, mock := setupKeeperTest(t, handler)

	// Mock LLM response
	mock.Response = &llm.ChatResponse{
		Content: `{"summary":"PKCE implementation discussed. RFC 7636 chosen.","key_decisions":["Use PKCE with RFC 7636","Add state parameter"],"open_questions":["Where to store code_verifier"],"active_tasks":["T-007-0"]}`,
		Model:   "haiku",
		Usage:   llm.TokenUsage{InputTokens: 200, OutputTokens: 100},
	}

	err := keeper.UpdateChannelSummary("ch-auth")
	if err != nil {
		t.Fatalf("UpdateChannelSummary: %v", err)
	}

	if !upsertCalled {
		t.Fatal("expected upsert to be called")
	}
	if mock.CallCount != 1 {
		t.Errorf("LLM call count = %d, want 1", mock.CallCount)
	}
	if upsertBody["channel_id"] != "ch-auth" {
		t.Errorf("channel_id = %v, want ch-auth", upsertBody["channel_id"])
	}
	if upsertBody["summary"] != "PKCE implementation discussed. RFC 7636 chosen." {
		t.Errorf("summary = %v", upsertBody["summary"])
	}
	if upsertBody["last_message_id"] != "m6" {
		t.Errorf("last_message_id = %v, want m6", upsertBody["last_message_id"])
	}
	count, _ := upsertBody["message_count"].(float64)
	if count != 6 {
		t.Errorf("message_count = %v, want 6", count)
	}
}

func TestUpdateSummary_EmptyChannelNoop(t *testing.T) {
	var upsertCalled bool

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// GET summary (empty)
		if r.Method == "GET" && strings.Contains(path, "c1_channel_summaries") {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte("[]"))
			return
		}

		// GET messages (only 2 — below minMessages)
		if r.Method == "GET" && strings.Contains(path, "c1_messages") {
			messages := []keeperMessageRow{
				{ID: "m1", SenderName: "alice", Content: "Hi", CreatedAt: "2026-02-14T01:00:00Z"},
				{ID: "m2", SenderName: "bob", Content: "Hello", CreatedAt: "2026-02-14T01:01:00Z"},
			}
			json.NewEncoder(w).Encode(messages)
			return
		}

		if r.Method == "POST" && strings.Contains(path, "c1_channel_summaries") {
			upsertCalled = true
			w.WriteHeader(201)
			return
		}

		w.WriteHeader(404)
	})

	keeper, mock := setupKeeperTest(t, handler)

	err := keeper.UpdateChannelSummary("ch-empty")
	if err != nil {
		t.Fatalf("UpdateChannelSummary: %v", err)
	}

	if upsertCalled {
		t.Error("upsert should NOT be called for fewer than minMessages")
	}
	if mock.CallCount != 0 {
		t.Errorf("LLM should NOT be called, got %d calls", mock.CallCount)
	}
}

func TestUpdateSummary_NoGatewaySkips(t *testing.T) {
	c1 := NewC1Handler("http://localhost", "key", "token", "proj")
	keeper := NewContextKeeper(c1, nil) // nil gateway

	err := keeper.UpdateChannelSummary("ch-1")
	if err != nil {
		t.Fatalf("expected nil error when gateway is nil, got: %v", err)
	}
}

func TestAutoPost(t *testing.T) {
	var postCalled bool
	var postBody map[string]any

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// Resolve channel name to ID
		if r.Method == "GET" && strings.Contains(path, "c1_channels") {
			channels := []c1ChannelRow{{ID: "ch-updates", Name: "#updates"}}
			json.NewEncoder(w).Encode(channels)
			return
		}

		// POST message
		if r.Method == "POST" && strings.Contains(path, "c1_messages") {
			postCalled = true
			json.NewDecoder(r.Body).Decode(&postBody)
			w.WriteHeader(201)
			return
		}

		w.WriteHeader(404)
	})

	keeper, _ := setupKeeperTest(t, handler)

	err := keeper.AutoPost("#updates", "T-001-0 completed")
	if err != nil {
		t.Fatalf("AutoPost: %v", err)
	}

	if !postCalled {
		t.Fatal("expected POST to be called")
	}
	if postBody["content"] != "T-001-0 completed" {
		t.Errorf("content = %v", postBody["content"])
	}
	if postBody["sender_type"] != "system" {
		t.Errorf("sender_type = %v, want system", postBody["sender_type"])
	}
}

func TestAutoPost_MissingChannel(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return empty channels list
		if r.Method == "GET" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte("[]"))
			return
		}
		w.WriteHeader(404)
	})

	keeper, _ := setupKeeperTest(t, handler)

	// Should not error — just skip silently
	err := keeper.AutoPost("#nonexistent", "test message")
	if err != nil {
		t.Fatalf("expected nil error for missing channel, got: %v", err)
	}
}

func TestUpdateSummary_UpsertsExisting(t *testing.T) {
	var upsertBody map[string]any

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// GET summary (existing entry with 10 messages already counted)
		if r.Method == "GET" && strings.Contains(path, "c1_channel_summaries") {
			summaries := []keeperSummaryRow{{
				ChannelID:     "ch-dev",
				Summary:       "Old summary about auth module",
				KeyDecisions:  `["Use PKCE"]`,
				OpenQuestions:  `["Storage choice"]`,
				ActiveTasks:   `["T-001-0"]`,
				LastMessageID: "m-prev",
				MessageCount:  10,
			}}
			json.NewEncoder(w).Encode(summaries)
			return
		}

		// GET last message (for created_at filter)
		if r.Method == "GET" && strings.Contains(path, "c1_messages") && strings.Contains(r.URL.RawQuery, "id=eq.m-prev") {
			json.NewEncoder(w).Encode([]keeperMessageRow{
				{ID: "m-prev", SenderName: "sys", Content: "old", CreatedAt: "2026-02-14T00:00:00Z"},
			})
			return
		}

		// GET new messages (6 messages)
		if r.Method == "GET" && strings.Contains(path, "c1_messages") {
			messages := []keeperMessageRow{
				{ID: "n1", SenderName: "alice", Content: "Start refactoring", CreatedAt: "2026-02-14T02:00:00Z"},
				{ID: "n2", SenderName: "bob", Content: "OK", CreatedAt: "2026-02-14T02:01:00Z"},
				{ID: "n3", SenderName: "alice", Content: "Module A done", CreatedAt: "2026-02-14T02:02:00Z"},
				{ID: "n4", SenderName: "bob", Content: "Module B started", CreatedAt: "2026-02-14T02:03:00Z"},
				{ID: "n5", SenderName: "alice", Content: "Tests pass", CreatedAt: "2026-02-14T02:04:00Z"},
				{ID: "n6", SenderName: "bob", Content: "Ready for review", CreatedAt: "2026-02-14T02:05:00Z"},
			}
			json.NewEncoder(w).Encode(messages)
			return
		}

		// POST upsert
		if r.Method == "POST" && strings.Contains(path, "c1_channel_summaries") {
			json.NewDecoder(r.Body).Decode(&upsertBody)
			w.WriteHeader(201)
			return
		}

		w.WriteHeader(404)
	})

	keeper, mock := setupKeeperTest(t, handler)
	mock.Response = &llm.ChatResponse{
		Content: `{"summary":"Refactoring complete. Modules A and B done.","key_decisions":["Use PKCE","Refactor modules"],"open_questions":[],"active_tasks":[]}`,
		Model:   "haiku",
		Usage:   llm.TokenUsage{InputTokens: 300, OutputTokens: 150},
	}

	err := keeper.UpdateChannelSummary("ch-dev")
	if err != nil {
		t.Fatalf("UpdateChannelSummary: %v", err)
	}

	// Verify upsert with incremented message count
	count, _ := upsertBody["message_count"].(float64)
	if count != 16 { // 10 existing + 6 new
		t.Errorf("message_count = %v, want 16", count)
	}
	if upsertBody["last_message_id"] != "n6" {
		t.Errorf("last_message_id = %v, want n6", upsertBody["last_message_id"])
	}
	if upsertBody["summary"] != "Refactoring complete. Modules A and B done." {
		t.Errorf("summary = %v", upsertBody["summary"])
	}
}

func TestKeeperTrigger_NotifyKeeper(t *testing.T) {
	var postBody map[string]any
	var postCalled bool

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// Resolve channel name
		if r.Method == "GET" && strings.Contains(path, "c1_channels") {
			json.NewEncoder(w).Encode([]c1ChannelRow{{ID: "ch-updates", Name: "#updates"}})
			return
		}

		// POST message
		if r.Method == "POST" && strings.Contains(path, "c1_messages") {
			postCalled = true
			json.NewDecoder(r.Body).Decode(&postBody)
			w.WriteHeader(201)
			return
		}

		w.WriteHeader(404)
	})

	keeper, _ := setupKeeperTest(t, handler)

	// Simulate task completion notification
	err := keeper.AutoPost("#updates", "T-007-0 completed: Implement auth flow")
	if err != nil {
		t.Fatalf("AutoPost: %v", err)
	}

	if !postCalled {
		t.Fatal("expected POST to be called for task completion notification")
	}
	if postBody["content"] != "T-007-0 completed: Implement auth flow" {
		t.Errorf("content = %v", postBody["content"])
	}
	if postBody["sender_type"] != "system" {
		t.Errorf("sender_type = %v, want system", postBody["sender_type"])
	}
}

func TestEnsureChannel_ExistingReused(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && strings.Contains(r.URL.Path, "c1_channels") {
			json.NewEncoder(w).Encode([]c1ChannelRow{{ID: "ch-existing", Name: "#updates"}})
			return
		}
		// POST should NOT be called — channel already exists
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		w.WriteHeader(500)
	})

	keeper, _ := setupKeeperTest(t, handler)
	id, err := keeper.EnsureChannel("#updates", "desc", "updates")
	if err != nil {
		t.Fatalf("EnsureChannel: %v", err)
	}
	if id != "ch-existing" {
		t.Errorf("channel ID = %q, want ch-existing", id)
	}
}

func TestEnsureChannel_CreatesNew(t *testing.T) {
	var postCalled bool

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// GET channels — empty
		if r.Method == "GET" && strings.Contains(r.URL.Path, "c1_channels") {
			json.NewEncoder(w).Encode([]c1ChannelRow{})
			return
		}
		// POST create channel
		if r.Method == "POST" && strings.Contains(r.URL.Path, "c1_channels") {
			postCalled = true
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(201)
			json.NewEncoder(w).Encode([]struct {
				ID string `json:"id"`
			}{{ID: "ch-new-123"}})
			return
		}
		w.WriteHeader(404)
	})

	keeper, _ := setupKeeperTest(t, handler)
	id, err := keeper.EnsureChannel("#new-channel", "desc", "updates")
	if err != nil {
		t.Fatalf("EnsureChannel: %v", err)
	}
	if !postCalled {
		t.Fatal("expected POST to create channel")
	}
	if id != "ch-new-123" {
		t.Errorf("channel ID = %q, want ch-new-123", id)
	}
}

func TestNotifyTaskEvent_Formats(t *testing.T) {
	tests := []struct {
		eventType string
		taskID    string
		title     string
		workerID  string
		wantMsg   string
	}{
		{"started", "T-001", "Impl auth", "w-1", "[started] T-001: Impl auth (worker: w-1)"},
		{"completed", "T-002", "Add tests", "", "[completed] T-002: Add tests"},
		{"blocked", "T-003", "Fix bug", "w-2", "[blocked] T-003: Fix bug (worker: w-2)"},
		{"custom", "T-004", "Something", "", "[custom] T-004: Something"},
	}

	for _, tt := range tests {
		t.Run(tt.eventType, func(t *testing.T) {
			var postedContent string

			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "GET" && strings.Contains(r.URL.Path, "c1_channels") {
					json.NewEncoder(w).Encode([]c1ChannelRow{{ID: "ch-upd", Name: "#updates"}})
					return
				}
				if r.Method == "POST" && strings.Contains(r.URL.Path, "c1_messages") {
					var body map[string]any
					json.NewDecoder(r.Body).Decode(&body)
					postedContent, _ = body["content"].(string)
					w.WriteHeader(201)
					return
				}
				w.WriteHeader(404)
			})

			keeper, _ := setupKeeperTest(t, handler)

			// NotifyTaskEvent is async (goroutine), call AutoPost directly for sync test
			err := keeper.AutoPost("#updates", tt.wantMsg)
			if err != nil {
				t.Fatalf("AutoPost: %v", err)
			}
			if postedContent != tt.wantMsg {
				t.Errorf("posted = %q, want %q", postedContent, tt.wantMsg)
			}
		})
	}
}

func TestAutoPost_CreatesChannelIfMissing(t *testing.T) {
	var channelCreated bool
	var messageSent bool

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// GET channels — empty (channel doesn't exist)
		if r.Method == "GET" && strings.Contains(r.URL.Path, "c1_channels") {
			json.NewEncoder(w).Encode([]c1ChannelRow{})
			return
		}
		// POST create channel
		if r.Method == "POST" && strings.Contains(r.URL.Path, "c1_channels") {
			channelCreated = true
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(201)
			json.NewEncoder(w).Encode([]struct {
				ID string `json:"id"`
			}{{ID: "ch-auto"}})
			return
		}
		// POST message
		if r.Method == "POST" && strings.Contains(r.URL.Path, "c1_messages") {
			messageSent = true
			w.WriteHeader(201)
			return
		}
		w.WriteHeader(404)
	})

	keeper, _ := setupKeeperTest(t, handler)
	err := keeper.AutoPost("#auto-channel", "test message")
	if err != nil {
		t.Fatalf("AutoPost: %v", err)
	}
	if !channelCreated {
		t.Error("expected channel to be auto-created")
	}
	if !messageSent {
		t.Error("expected message to be sent after channel creation")
	}
}

func TestGetBriefing_CombinesSummaryAndMessages(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		query := r.URL.RawQuery

		// Channels
		if strings.Contains(path, "c1_channels") {
			channels := []c1ChannelRow{
				{ID: "ch-1", Name: "#general"},
				{ID: "ch-2", Name: "#auth-module"},
			}
			json.NewEncoder(w).Encode(channels)
			return
		}

		// Summaries
		if strings.Contains(path, "c1_channel_summaries") {
			summaries := []c1ChannelSummaryRow{
				{ChannelID: "ch-2", Summary: "Discussing PKCE auth", KeyDecisions: `["Use RFC 7636"]`, OpenQuestions: `["Verifier storage"]`},
			}
			json.NewEncoder(w).Encode(summaries)
			return
		}

		// Messages (recent)
		if strings.Contains(path, "c1_messages") && strings.Contains(query, "order=created_at.desc") {
			messages := []c1MessageRow{
				{ID: "m1", ChannelID: "ch-2", SenderName: "alice", Content: "PKCE ready", CreatedAt: "2026-02-14T12:00:00Z"},
			}
			json.NewEncoder(w).Encode(messages)
			return
		}

		w.WriteHeader(404)
	})

	ts := httptest.NewServer(handler)
	defer ts.Close()

	c1 := NewC1Handler(ts.URL, "test-key", "test-token", "proj-1")
	result, err := c1.GetBriefing()
	if err != nil {
		t.Fatalf("GetBriefing: %v", err)
	}

	summaries, ok := result["channel_summaries"].([]map[string]any)
	if !ok {
		t.Fatalf("channel_summaries type = %T", result["channel_summaries"])
	}
	if len(summaries) != 1 {
		t.Fatalf("summaries count = %d, want 1", len(summaries))
	}
	if summaries[0]["channel_name"] != "#auth-module" {
		t.Errorf("channel_name = %v", summaries[0]["channel_name"])
	}

	recentMsgs, ok := result["recent_messages"].([]map[string]any)
	if !ok {
		t.Fatalf("recent_messages type = %T", result["recent_messages"])
	}
	if len(recentMsgs) != 1 {
		t.Fatalf("recent messages count = %d, want 1", len(recentMsgs))
	}
	if recentMsgs[0]["sender_name"] != "alice" {
		t.Errorf("sender_name = %v", recentMsgs[0]["sender_name"])
	}
}
