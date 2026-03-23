package serve

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/changmin/c4-core/internal/botstore"
	"github.com/changmin/c4-core/internal/notify"
)

// TestTaskNotifyE2E_DoneToTelegram simulates the full pipeline:
//
//	DB webhook UPDATE (status=done) → handleTaskEvent → sendTaskNotification → Telegram API
//
// A mock Telegram HTTP server captures the outgoing request so we can assert
// the correct payload was sent — no live credentials required.
func TestTaskNotifyE2E_DoneToTelegram(t *testing.T) {
	// 1. Start mock Telegram API server
	var (
		callCount      atomic.Int32
		receivedChatID string
		receivedText   string
	)
	mockTelegram := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		var payload map[string]string
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "bad body", http.StatusBadRequest)
			return
		}
		receivedChatID = payload["chat_id"]
		receivedText = payload["text"]
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`)) //nolint:errcheck
	}))
	defer mockTelegram.Close()

	// Redirect Telegram API calls to the mock server
	notify.OverrideBaseURL(mockTelegram.URL)
	defer notify.ResetBaseURL()

	// 2. Set up temp project dir with bot config and notifications.json
	dir := t.TempDir()

	bs, err := botstore.New(dir)
	if err != nil {
		t.Fatalf("botstore.New: %v", err)
	}
	const botUsername = "e2e_test_bot"
	const chatID int64 = 99887766
	if err := bs.Save(botstore.Bot{
		Username:  botUsername,
		Token:     "e2etoken123",
		Scope:     "project",
		AllowFrom: []int64{chatID},
	}); err != nil {
		t.Fatalf("bs.Save: %v", err)
	}

	notifCfg := map[string]string{"bot_username": botUsername}
	notifData, _ := json.Marshal(notifCfg)
	c4Dir := filepath.Join(dir, ".c4")
	os.MkdirAll(c4Dir, 0755) //nolint:errcheck
	if err := os.WriteFile(filepath.Join(c4Dir, "notifications.json"), notifData, 0644); err != nil {
		t.Fatalf("write notifications.json: %v", err)
	}

	// 3. Create Agent and dispatch a "done" task event (simulates a DB webhook UPDATE)
	a := NewAgent(AgentConfig{
		SupabaseURL: "https://example.supabase.co",
		APIKey:      "test-key",
		ProjectDir:  dir,
	})
	event := makeTaskEvent("UPDATE", "T-E2E-001", "Deploy notification system", "done", "")
	a.handleTaskEvent(event)

	// 4. Assert mock Telegram server received exactly one request with correct payload
	if n := callCount.Load(); n != 1 {
		t.Fatalf("expected 1 Telegram API call, got %d", n)
	}
	if receivedChatID != "99887766" {
		t.Errorf("chat_id: got %q, want %q", receivedChatID, "99887766")
	}
	if !strings.Contains(receivedText, "T-E2E-001") {
		t.Errorf("message missing task ID: %q", receivedText)
	}
	if !strings.Contains(receivedText, "Deploy notification system") {
		t.Errorf("message missing title: %q", receivedText)
	}
}

// TestTaskNotifyE2E_BlockedToTelegram verifies blocked status sends the failure reason.
func TestTaskNotifyE2E_BlockedToTelegram(t *testing.T) {
	var receivedText string
	mockTelegram := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]string
		json.NewDecoder(r.Body).Decode(&payload) //nolint:errcheck
		receivedText = payload["text"]
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`)) //nolint:errcheck
	}))
	defer mockTelegram.Close()

	notify.OverrideBaseURL(mockTelegram.URL)
	defer notify.ResetBaseURL()

	dir := t.TempDir()
	bs, err := botstore.New(dir)
	if err != nil {
		t.Fatalf("botstore.New: %v", err)
	}
	if err := bs.Save(botstore.Bot{
		Username:  "e2e_blocked_bot",
		Token:     "blockedtoken",
		Scope:     "project",
		AllowFrom: []int64{111222333},
	}); err != nil {
		t.Fatalf("bs.Save: %v", err)
	}
	notifCfg := map[string]string{"bot_username": "e2e_blocked_bot"}
	notifData, _ := json.Marshal(notifCfg)
	os.MkdirAll(filepath.Join(dir, ".c4"), 0755) //nolint:errcheck
	os.WriteFile(filepath.Join(dir, ".c4", "notifications.json"), notifData, 0644) //nolint:errcheck

	a := NewAgent(AgentConfig{
		SupabaseURL: "https://example.supabase.co",
		APIKey:      "test-key",
		ProjectDir:  dir,
	})

	event := makeTaskEvent("UPDATE", "T-E2E-002", "Run tests", "blocked", "build_failed")
	a.handleTaskEvent(event)

	if !strings.Contains(receivedText, "build_failed") {
		t.Errorf("expected failure reason in message, got: %q", receivedText)
	}
	if !strings.Contains(receivedText, "T-E2E-002") {
		t.Errorf("expected task ID in message, got: %q", receivedText)
	}
}

// TestTaskNotifyE2E_NoNotificationsConfig verifies that when notifications.json
// is absent the Telegram API is never called (best-effort, silent skip).
func TestTaskNotifyE2E_NoNotificationsConfig(t *testing.T) {
	var called atomic.Int32
	mockTelegram := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer mockTelegram.Close()

	notify.OverrideBaseURL(mockTelegram.URL)
	defer notify.ResetBaseURL()

	dir := t.TempDir()
	// Deliberately NOT creating notifications.json

	a := NewAgent(AgentConfig{
		SupabaseURL: "https://example.supabase.co",
		APIKey:      "test-key",
		ProjectDir:  dir,
	})
	a.handleTaskEvent(makeTaskEvent("UPDATE", "T-E2E-003", "Some task", "done", ""))

	if called.Load() != 0 {
		t.Errorf("expected no Telegram call when notifications.json absent, got %d", called.Load())
	}
}
