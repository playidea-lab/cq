package notify_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/changmin/c4-core/internal/notify"
)

// ---------------------------------------------------------------------------
// Profile tests
// ---------------------------------------------------------------------------

func TestLoadProfile_NoFile(t *testing.T) {
	dir := t.TempDir()
	got, err := notify.LoadProfile(dir)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil profile, got %+v", got)
	}
}

func TestLoadProfile_Valid(t *testing.T) {
	dir := t.TempDir()
	data := `{"channel":"dooray","events":["plan.created","finish.complete"],"webhook_secret_key":"notification.dooray.webhook"}`
	if err := os.WriteFile(filepath.Join(dir, "notifications.json"), []byte(data), 0o640); err != nil {
		t.Fatal(err)
	}

	got, err := notify.LoadProfile(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil profile")
	}
	if got.Channel != "dooray" {
		t.Errorf("Channel: got %q, want %q", got.Channel, "dooray")
	}
	if len(got.Events) != 2 {
		t.Errorf("Events len: got %d, want 2", len(got.Events))
	}
	if got.WebhookSecretKey != "notification.dooray.webhook" {
		t.Errorf("WebhookSecretKey: got %q", got.WebhookSecretKey)
	}
}

func TestSaveProfile(t *testing.T) {
	dir := t.TempDir()
	p := &notify.NotificationProfile{
		Channel:          "slack",
		Events:           []string{"checkpoint.ready", "run.task_started"},
		WebhookSecretKey: "notification.slack.webhook",
	}

	if err := p.Save(dir); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// File must exist.
	path := filepath.Join(dir, "notifications.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("file not created: %v", err)
	}

	// Reload must match.
	got, err := notify.LoadProfile(dir)
	if err != nil {
		t.Fatalf("LoadProfile after Save: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil profile after reload")
	}
	if got.Channel != p.Channel {
		t.Errorf("Channel mismatch: got %q, want %q", got.Channel, p.Channel)
	}
	if len(got.Events) != len(p.Events) {
		t.Errorf("Events len mismatch: got %d, want %d", len(got.Events), len(p.Events))
	}
	if got.WebhookSecretKey != p.WebhookSecretKey {
		t.Errorf("WebhookSecretKey mismatch: got %q, want %q", got.WebhookSecretKey, p.WebhookSecretKey)
	}
}

func TestSaveProfile_CreatesDir(t *testing.T) {
	base := t.TempDir()
	soulDir := filepath.Join(base, "souls", "testuser")
	p := &notify.NotificationProfile{
		Channel: "discord",
		Events:  []string{"finish.complete"},
	}
	if err := p.Save(soulDir); err != nil {
		t.Fatalf("Save with non-existent dir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(soulDir, "notifications.json")); err != nil {
		t.Fatalf("file missing: %v", err)
	}
}

// ---------------------------------------------------------------------------
// NewSender tests
// ---------------------------------------------------------------------------

func TestNewSender_InvalidChannel(t *testing.T) {
	_, err := notify.NewSender("telegram", "https://example.com/hook")
	if err == nil {
		t.Fatal("expected error for unknown channel, got nil")
	}
}

func TestNewSender_ValidChannels(t *testing.T) {
	channels := []string{"dooray", "slack", "discord", "teams"}
	for _, ch := range channels {
		s, err := notify.NewSender(ch, "https://example.com/hook")
		if err != nil {
			t.Errorf("channel %q: unexpected error: %v", ch, err)
		}
		if s == nil {
			t.Errorf("channel %q: expected non-nil sender", ch)
		}
	}
}

// ---------------------------------------------------------------------------
// Send mock tests
// ---------------------------------------------------------------------------

// mockSender is an in-process Sender that captures sent messages.
type mockSender struct {
	messages []string
}

func (m *mockSender) Send(_ context.Context, message string) error {
	m.messages = append(m.messages, message)
	return nil
}

func TestSender_Send_Mock(t *testing.T) {
	ms := &mockSender{}
	if err := ms.Send(context.Background(), "hello"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if len(ms.messages) != 1 || ms.messages[0] != "hello" {
		t.Errorf("messages: got %v, want [\"hello\"]", ms.messages)
	}
}

// ---------------------------------------------------------------------------
// Send over httptest server (integration-style per channel)
// ---------------------------------------------------------------------------

func testHTTPSend(t *testing.T, channel string, bodyKey string) {
	t.Helper()

	var received map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			http.Error(w, "bad body", http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	s, err := notify.NewSender(channel, srv.URL)
	if err != nil {
		t.Fatalf("NewSender(%q): %v", channel, err)
	}
	if err := s.Send(context.Background(), "test message"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if received[bodyKey] != "test message" {
		t.Errorf("body key %q: got %q, want %q", bodyKey, received[bodyKey], "test message")
	}
}

func TestSender_Dooray_HTTP(t *testing.T)  { testHTTPSend(t, "dooray", "text") }
func TestSender_Slack_HTTP(t *testing.T)   { testHTTPSend(t, "slack", "text") }
func TestSender_Discord_HTTP(t *testing.T) { testHTTPSend(t, "discord", "content") }
func TestSender_Teams_HTTP(t *testing.T)   { testHTTPSend(t, "teams", "text") }

func TestSender_Send_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	s, err := notify.NewSender("slack", srv.URL)
	if err != nil {
		t.Fatalf("NewSender: %v", err)
	}
	if err := s.Send(context.Background(), "msg"); err == nil {
		t.Fatal("expected error on HTTP 500, got nil")
	}
}
