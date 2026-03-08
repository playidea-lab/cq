package notifyhandler

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/changmin/c4-core/internal/mcp"
	"github.com/changmin/c4-core/internal/notify"
	"github.com/changmin/c4-core/internal/secrets"
)

func newTestStore(t *testing.T) (*secrets.Store, string) {
	t.Helper()
	dir := t.TempDir()
	store, err := secrets.NewWithPaths(
		filepath.Join(dir, "secrets.db"),
		filepath.Join(dir, "master.key"),
	)
	if err != nil {
		t.Fatalf("secrets.NewWithPaths: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store, dir
}

func callHandler(t *testing.T, opts *Opts, args map[string]any) (map[string]any, error) {
	t.Helper()
	reg := mcp.NewRegistry()
	Register(reg, opts)

	raw, err := json.Marshal(args)
	if err != nil {
		t.Fatalf("marshal args: %v", err)
	}

	result, err := reg.Call("c4_notification_set", raw)
	if err != nil {
		return nil, err
	}
	m, _ := result.(map[string]any)
	return m, nil
}

func TestNotificationSet_Valid(t *testing.T) {
	store, dir := newTestStore(t)
	opts := &Opts{ProjectDir: dir, SecretStore: store}

	result, err := callHandler(t, opts, map[string]any{
		"channel":     "dooray",
		"webhook_url": "https://example.com/hook",
		"events":      []string{"plan.created", "finish.complete"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["success"] != true {
		t.Errorf("expected success=true, got %v", result["success"])
	}
	if result["channel"] != "dooray" {
		t.Errorf("expected channel=dooray, got %v", result["channel"])
	}

	// Verify soul JSON was saved.
	user := currentUser()
	soulDir := filepath.Join(dir, ".c4", "souls", user)
	profile, err := notify.LoadProfile(soulDir)
	if err != nil {
		t.Fatalf("LoadProfile: %v", err)
	}
	if profile == nil {
		t.Fatal("profile is nil after save")
	}
	if profile.Channel != "dooray" {
		t.Errorf("profile.Channel=%q, want dooray", profile.Channel)
	}
	if profile.WebhookSecretKey != "notification.dooray.webhook" {
		t.Errorf("profile.WebhookSecretKey=%q", profile.WebhookSecretKey)
	}

	// Verify secret was saved.
	val, err := store.Get("notification.dooray.webhook")
	if err != nil {
		t.Fatalf("secret.Get: %v", err)
	}
	if val != "https://example.com/hook" {
		t.Errorf("secret value=%q, want https://example.com/hook", val)
	}
}

func TestNotificationSet_InvalidChannel(t *testing.T) {
	store, dir := newTestStore(t)
	opts := &Opts{ProjectDir: dir, SecretStore: store}

	_, err := callHandler(t, opts, map[string]any{
		"channel":     "telegram",
		"webhook_url": "https://example.com/hook",
	})
	if err == nil {
		t.Fatal("expected error for invalid channel, got nil")
	}
}

func TestNotificationSet_EmptyWebhook(t *testing.T) {
	store, dir := newTestStore(t)
	opts := &Opts{ProjectDir: dir, SecretStore: store}

	_, err := callHandler(t, opts, map[string]any{
		"channel":     "slack",
		"webhook_url": "",
	})
	if err == nil {
		t.Fatal("expected error for empty webhook_url, got nil")
	}
}

func TestNotificationSet_DefaultEvents(t *testing.T) {
	store, dir := newTestStore(t)
	opts := &Opts{ProjectDir: dir, SecretStore: store}

	result, err := callHandler(t, opts, map[string]any{
		"channel":     "slack",
		"webhook_url": "https://example.com/hook",
		// events omitted — should default to 3 standard events
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	events, ok := result["events"].([]string)
	if !ok {
		// JSON round-trip may produce []any
		raw, _ := result["events"].([]any)
		if len(raw) != 3 {
			t.Errorf("expected 3 default events, got %v", result["events"])
		}
		return
	}
	if len(events) != 3 {
		t.Errorf("expected 3 default events, got %v", events)
	}

	// Verify soul JSON reflects defaults.
	user := currentUser()
	soulDir := filepath.Join(dir, ".c4", "souls", user)
	profile, err := notify.LoadProfile(soulDir)
	if err != nil {
		t.Fatalf("LoadProfile: %v", err)
	}
	if profile == nil || len(profile.Events) != 3 {
		t.Errorf("expected 3 events in profile, got %v", profile)
	}

	// Check the notifications.json file exists.
	jsonPath := filepath.Join(soulDir, "notifications.json")
	if _, err := os.Stat(jsonPath); err != nil {
		t.Errorf("notifications.json not found: %v", err)
	}
}
