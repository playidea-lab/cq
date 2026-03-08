package notifyhandler

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/changmin/c4-core/internal/notify"
)

// soulDirForTest creates a temp project dir and returns the soul dir path.
func soulDirForTest(t *testing.T) (projectDir, soulDir string) {
	t.Helper()
	projectDir = t.TempDir()
	soulDir = filepath.Join(projectDir, ".c4", "souls", "default")
	return
}

// --- c4_notification_get tests ---

func TestNotificationGet_NotConfigured(t *testing.T) {
	_, soulDir := soulDirForTest(t)
	// No notifications.json exists → configured: false
	result, err := handleGet(soulDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]any)
	if m["configured"] != false {
		t.Errorf("want configured=false, got %v", m["configured"])
	}
	if len(m) != 1 {
		t.Errorf("want only 'configured' key when not configured, got %v", m)
	}
}

func TestNotificationGet_Configured(t *testing.T) {
	_, soulDir := soulDirForTest(t)

	// Save a profile first
	p := &notify.NotificationProfile{
		Channel:          "dooray",
		Events:           []string{"plan.created", "finish.complete"},
		WebhookSecretKey: "notification.dooray.webhook",
	}
	if err := p.Save(soulDir); err != nil {
		t.Fatalf("save profile: %v", err)
	}

	result, err := handleGet(soulDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]any)

	if m["configured"] != true {
		t.Errorf("want configured=true, got %v", m["configured"])
	}
	if m["channel"] != "dooray" {
		t.Errorf("want channel=dooray, got %v", m["channel"])
	}
	if m["webhook_secret_key"] != "notification.dooray.webhook" {
		t.Errorf("want webhook_secret_key set, got %v", m["webhook_secret_key"])
	}
	// Webhook URL must NOT be returned (security)
	if _, hasURL := m["webhook_url"]; hasURL {
		t.Error("webhook_url must not be present in response")
	}
	events, ok := m["events"].([]string)
	if !ok {
		t.Fatalf("events should be []string, got %T", m["events"])
	}
	if len(events) != 2 {
		t.Errorf("want 2 events, got %d", len(events))
	}
}

// --- c4_notification_set tests ---

func TestNotificationSet_SavesProfile(t *testing.T) {
	_, soulDir := soulDirForTest(t)

	args, _ := json.Marshal(map[string]any{
		"channel":            "slack",
		"events":             []string{"checkpoint.ready"},
		"webhook_secret_key": "notification.slack.webhook",
	})
	result, err := handleSet(soulDir, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]any)
	if m["ok"] != true {
		t.Errorf("want ok=true, got %v", m["ok"])
	}

	// Verify the file was written
	profilePath := filepath.Join(soulDir, "notifications.json")
	if _, err := os.Stat(profilePath); err != nil {
		t.Errorf("notifications.json not written: %v", err)
	}
}

func TestNotificationSet_MissingChannel(t *testing.T) {
	_, soulDir := soulDirForTest(t)
	args, _ := json.Marshal(map[string]any{
		"webhook_secret_key": "notification.slack.webhook",
	})
	_, err := handleSet(soulDir, args)
	if err == nil {
		t.Error("expected error for missing channel")
	}
}

func TestNotificationSet_MissingWebhookSecretKey(t *testing.T) {
	_, soulDir := soulDirForTest(t)
	args, _ := json.Marshal(map[string]any{
		"channel": "slack",
	})
	_, err := handleSet(soulDir, args)
	if err == nil {
		t.Error("expected error for missing webhook_secret_key")
	}
}
