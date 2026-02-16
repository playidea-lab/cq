package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/changmin/c4-core/internal/cloud"
)

// setupC1Test creates a C1Handler pointing at a mock server.
func setupC1Test(t *testing.T, handler http.HandlerFunc) *C1Handler {
	t.Helper()
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)
	return NewC1Handler(ts.URL, "test-key", cloud.NewStaticTokenProvider("test-token"), "proj-1")
}

// --- resolveChannelID tests ---

func TestResolveChannelID_Found(t *testing.T) {
	h := setupC1Test(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && strings.Contains(r.URL.Path, "c1_channels") {
			json.NewEncoder(w).Encode([]c1ChannelRow{{ID: "ch-123", Name: "general"}})
			return
		}
		http.Error(w, "not found", 404)
	})

	id, err := h.resolveChannelID("general")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "ch-123" {
		t.Fatalf("expected ch-123, got %s", id)
	}
}

func TestResolveChannelID_NotFound(t *testing.T) {
	h := setupC1Test(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]c1ChannelRow{})
	})

	id, err := h.resolveChannelID("nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "" {
		t.Fatalf("expected empty string, got %s", id)
	}
}

// --- Search tests ---

func TestSearch_ReturnsResults(t *testing.T) {
	h := setupC1Test(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && strings.Contains(r.URL.Path, "c1_messages") {
			query := r.URL.RawQuery
			if !strings.Contains(query, "tsv=fts") {
				t.Errorf("expected FTS filter in query: %s", query)
			}
			json.NewEncoder(w).Encode([]c1MessageRow{
				{ID: "m-1", ChannelID: "ch-1", SenderName: "alice", Content: "hello world", CreatedAt: "2026-02-14T10:00:00Z"},
				{ID: "m-2", ChannelID: "ch-1", SenderName: "bob", Content: "hello there", CreatedAt: "2026-02-14T10:01:00Z"},
			})
			return
		}
		http.Error(w, "not found", 404)
	})

	result, err := h.Search("hello", "", "", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	count, ok := result["count"].(int)
	if !ok || count != 2 {
		t.Fatalf("expected count=2, got %v", result["count"])
	}
	results := result["results"].([]map[string]any)
	if results[0]["sender_name"] != "alice" {
		t.Errorf("expected alice, got %v", results[0]["sender_name"])
	}
}

func TestSearch_EmptyResults(t *testing.T) {
	h := setupC1Test(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]c1MessageRow{})
	})

	result, err := h.Search("nonexistent", "", "", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["count"].(int) != 0 {
		t.Fatalf("expected 0 results, got %v", result["count"])
	}
}

func TestSearch_WithChannelFilter(t *testing.T) {
	h := setupC1Test(t, func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.RawQuery
		if strings.Contains(r.URL.Path, "c1_channels") {
			json.NewEncoder(w).Encode([]c1ChannelRow{{ID: "ch-99", Name: "dev"}})
			return
		}
		if strings.Contains(r.URL.Path, "c1_messages") {
			if !strings.Contains(query, "channel_id=eq.ch-99") {
				t.Errorf("expected channel_id filter, got: %s", query)
			}
			json.NewEncoder(w).Encode([]c1MessageRow{
				{ID: "m-1", ChannelID: "ch-99", SenderName: "alice", Content: "fix bug", CreatedAt: "2026-02-14T10:00:00Z"},
			})
			return
		}
		http.Error(w, "not found", 404)
	})

	result, err := h.Search("bug", "dev", "", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["count"].(int) != 1 {
		t.Fatalf("expected 1 result, got %v", result["count"])
	}
}

func TestSearch_ChannelNotFound(t *testing.T) {
	h := setupC1Test(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]c1ChannelRow{}) // empty
	})

	_, err := h.Search("test", "nonexistent-channel", "", 0)
	if err == nil {
		t.Fatal("expected error for nonexistent channel")
	}
	if !strings.Contains(err.Error(), "channel not found") {
		t.Fatalf("expected 'channel not found' error, got: %v", err)
	}
}

// --- CheckMentions tests ---

func TestCheckMentions_Found(t *testing.T) {
	h := setupC1Test(t, func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.RawQuery
		path := r.URL.Path
		if strings.Contains(path, "c1_participants") {
			json.NewEncoder(w).Encode([]c1ParticipantRow{
				{AgentName: "worker-1", ChannelID: "ch-1", LastReadAt: "2026-02-14T09:00:00Z"},
			})
			return
		}
		if strings.Contains(path, "c1_messages") {
			if !strings.Contains(query, "content=like") {
				t.Errorf("expected content like filter, got: %s", query)
			}
			json.NewEncoder(w).Encode([]c1MessageRow{
				{ID: "m-1", ChannelID: "ch-1", SenderName: "alice", Content: "Hey @worker-1 check this", CreatedAt: "2026-02-14T10:00:00Z"},
			})
			return
		}
		if strings.Contains(path, "c1_channels") {
			json.NewEncoder(w).Encode([]c1ChannelRow{{ID: "ch-1", Name: "general"}})
			return
		}
		http.Error(w, "not found", 404)
	})

	results, err := h.CheckMentions("worker-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 mention, got %d", len(results))
	}
	if results[0]["channel_name"] != "general" {
		t.Errorf("expected channel_name=general, got %v", results[0]["channel_name"])
	}
	if results[0]["sender_name"] != "alice" {
		t.Errorf("expected sender=alice, got %v", results[0]["sender_name"])
	}
}

func TestCheckMentions_NoMentions(t *testing.T) {
	h := setupC1Test(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "c1_participants") {
			json.NewEncoder(w).Encode([]c1ParticipantRow{})
			return
		}
		if strings.Contains(r.URL.Path, "c1_messages") {
			json.NewEncoder(w).Encode([]c1MessageRow{})
			return
		}
		http.Error(w, "not found", 404)
	})

	results, err := h.CheckMentions("nobody")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 mentions, got %d", len(results))
	}
}

// --- GetBriefing tests ---

func TestGetBriefing_WithSummariesAndMessages(t *testing.T) {
	h := setupC1Test(t, func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if strings.Contains(path, "c1_channel_summaries") {
			json.NewEncoder(w).Encode([]c1ChannelSummaryRow{
				{ChannelID: "ch-1", Summary: "Project kickoff discussion", KeyDecisions: "[]", OpenQuestions: "[]"},
				{ChannelID: "ch-2", Summary: "Bug triage session", KeyDecisions: "[\"fix auth\"]", OpenQuestions: "[]"},
			})
			return
		}
		if strings.Contains(path, "c1_channels") {
			json.NewEncoder(w).Encode([]c1ChannelRow{
				{ID: "ch-1", Name: "general"},
				{ID: "ch-2", Name: "bugs"},
			})
			return
		}
		if strings.Contains(path, "c1_messages") {
			json.NewEncoder(w).Encode([]c1MessageRow{
				{ID: "m-1", ChannelID: "ch-1", SenderName: "alice", Content: "hello", CreatedAt: "2026-02-14T10:00:00Z"},
				{ID: "m-2", ChannelID: "ch-2", SenderName: "bob", Content: "found bug", CreatedAt: "2026-02-14T10:01:00Z"},
			})
			return
		}
		http.Error(w, "not found", 404)
	})

	result, err := h.GetBriefing()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	summaries := result["channel_summaries"].([]map[string]any)
	if len(summaries) != 2 {
		t.Fatalf("expected 2 summaries, got %d", len(summaries))
	}
	if summaries[0]["channel_name"] != "general" {
		t.Errorf("expected general, got %v", summaries[0]["channel_name"])
	}
	if summaries[1]["summary"] != "Bug triage session" {
		t.Errorf("expected 'Bug triage session', got %v", summaries[1]["summary"])
	}

	messages := result["recent_messages"].([]map[string]any)
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(messages))
	}
}

func TestGetBriefing_NoChannels(t *testing.T) {
	h := setupC1Test(t, func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if strings.Contains(path, "c1_channels") {
			json.NewEncoder(w).Encode([]c1ChannelRow{})
			return
		}
		if strings.Contains(path, "c1_messages") {
			json.NewEncoder(w).Encode([]c1MessageRow{})
			return
		}
		http.Error(w, "not found", 404)
	})

	result, err := h.GetBriefing()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	summaries := result["channel_summaries"].([]map[string]any)
	if len(summaries) != 0 {
		t.Fatalf("expected 0 summaries, got %d", len(summaries))
	}
}

// --- httpGet error handling ---

func TestHttpGet_ServerError(t *testing.T) {
	h := setupC1Test(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"internal error"}`, 500)
	})

	_, err := h.Search("test", "", "", 0)
	if err == nil {
		t.Fatal("expected error on 500 response")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Fatalf("expected 500 in error, got: %v", err)
	}
}

// --- setHeaders verification ---

func TestSetHeaders(t *testing.T) {
	var capturedHeaders http.Header
	h := setupC1Test(t, func(w http.ResponseWriter, r *http.Request) {
		capturedHeaders = r.Header.Clone()
		json.NewEncoder(w).Encode([]c1MessageRow{})
	})

	_, _ = h.Search("test", "", "", 0)

	if capturedHeaders.Get("apikey") != "test-key" {
		t.Errorf("expected apikey=test-key, got %s", capturedHeaders.Get("apikey"))
	}
	if capturedHeaders.Get("Authorization") != "Bearer test-token" {
		t.Errorf("expected Bearer test-token, got %s", capturedHeaders.Get("Authorization"))
	}
	if capturedHeaders.Get("Content-Type") != "application/json" {
		t.Errorf("expected Content-Type=application/json, got %s", capturedHeaders.Get("Content-Type"))
	}
}

// --- EnsureMember tests ---

func TestEnsureMember_ExistingReturned(t *testing.T) {
	h := setupC1Test(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && strings.Contains(r.URL.Path, "c1_members") {
			json.NewEncoder(w).Encode([]c1MemberRow{{ID: "mem-123"}})
			return
		}
		http.Error(w, "not found", 404)
	})

	id, err := h.EnsureMember("agent", "worker-1", "Worker 1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "mem-123" {
		t.Fatalf("expected mem-123, got %s", id)
	}
}

func TestEnsureMember_CreatesNew(t *testing.T) {
	var postCalled bool
	h := setupC1Test(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && strings.Contains(r.URL.Path, "c1_members") {
			json.NewEncoder(w).Encode([]c1MemberRow{})
			return
		}
		if r.Method == "POST" && strings.Contains(r.URL.Path, "c1_members") {
			postCalled = true
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(201)
			json.NewEncoder(w).Encode([]struct {
				ID string `json:"id"`
			}{{ID: "mem-new-456"}})
			return
		}
		http.Error(w, "not found", 404)
	})

	id, err := h.EnsureMember("agent", "worker-2", "Worker 2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !postCalled {
		t.Fatal("expected POST to create member")
	}
	if id != "mem-new-456" {
		t.Fatalf("expected mem-new-456, got %s", id)
	}
}

// --- SendMessage tests ---

func TestSendMessage_Success(t *testing.T) {
	var msgPayload map[string]any
	h := setupC1Test(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && strings.Contains(r.URL.Path, "c1_channels") {
			json.NewEncoder(w).Encode([]c1ChannelRow{{ID: "ch-gen", Name: "general"}})
			return
		}
		if r.Method == "GET" && strings.Contains(r.URL.Path, "c1_members") {
			json.NewEncoder(w).Encode([]c1MemberRow{{ID: "mem-agent"}})
			return
		}
		if r.Method == "POST" && strings.Contains(r.URL.Path, "c1_messages") {
			json.NewDecoder(r.Body).Decode(&msgPayload)
			w.WriteHeader(201)
			return
		}
		http.Error(w, "not found", 404)
	})

	result, err := h.SendMessage("general", "Hello from agent", "test-agent", "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["status"] != "sent" {
		t.Errorf("expected status=sent, got %v", result["status"])
	}
	if msgPayload["member_id"] != "mem-agent" {
		t.Errorf("expected member_id=mem-agent, got %v", msgPayload["member_id"])
	}
	if msgPayload["sender_type"] != "agent" {
		t.Errorf("expected sender_type=agent, got %v", msgPayload["sender_type"])
	}
}

func TestSendMessage_ChannelNotFound(t *testing.T) {
	h := setupC1Test(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]c1ChannelRow{})
	})

	_, err := h.SendMessage("nonexistent", "hello", "", "", nil)
	if err == nil {
		t.Fatal("expected error for nonexistent channel")
	}
	if !strings.Contains(err.Error(), "channel not found") {
		t.Fatalf("expected 'channel not found' error, got: %v", err)
	}
}

func TestSendMessage_EmptyContent(t *testing.T) {
	h := setupC1Test(t, func(w http.ResponseWriter, r *http.Request) {})

	_, err := h.SendMessage("general", "", "agent", "", nil)
	if err == nil {
		t.Fatal("expected error for empty content")
	}
}

// --- UpdatePresence tests ---

func TestUpdatePresence_ValidStatus(t *testing.T) {
	var patchPayload map[string]any
	h := setupC1Test(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "PATCH" && strings.Contains(r.URL.Path, "c1_members") {
			json.NewDecoder(r.Body).Decode(&patchPayload)
			w.WriteHeader(200)
			return
		}
		http.Error(w, "not found", 404)
	})

	err := h.UpdatePresence("agent", "worker-1", "working", "T-003-0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if patchPayload["status"] != "working" {
		t.Errorf("expected status=working, got %v", patchPayload["status"])
	}
	if patchPayload["status_text"] != "T-003-0" {
		t.Errorf("expected status_text=T-003-0, got %v", patchPayload["status_text"])
	}
}

func TestUpdatePresence_InvalidStatus(t *testing.T) {
	h := setupC1Test(t, func(w http.ResponseWriter, r *http.Request) {})

	err := h.UpdatePresence("agent", "worker-1", "invalid-status", "")
	if err == nil {
		t.Fatal("expected error for invalid status")
	}
	if !strings.Contains(err.Error(), "invalid status") {
		t.Fatalf("expected 'invalid status' error, got: %v", err)
	}
}

// --- httpPatch tests ---

func TestHttpPatch_ServerError(t *testing.T) {
	h := setupC1Test(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "PATCH" {
			http.Error(w, `{"message":"internal error"}`, 500)
			return
		}
	})

	err := h.httpPatch("c1_members", "id=eq.123", map[string]any{"status": "online"})
	if err == nil {
		t.Fatal("expected error on 500 response")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Fatalf("expected 500 in error, got: %v", err)
	}
}

