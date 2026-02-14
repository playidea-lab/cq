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
