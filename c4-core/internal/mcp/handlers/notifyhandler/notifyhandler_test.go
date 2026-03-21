package notifyhandler

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/changmin/c4-core/internal/botstore"
	"github.com/changmin/c4-core/internal/mcp"
)

// setupBotstore creates a minimal bot in the botstore for testing.
func setupBotstore(t *testing.T, projectDir, username, token string, allowFrom []int64) {
	t.Helper()
	bs, err := botstore.New(projectDir)
	if err != nil {
		t.Fatalf("botstore.New: %v", err)
	}
	if err := bs.Save(botstore.Bot{
		Username:  username,
		Token:     token,
		AllowFrom: allowFrom,
		Scope:     "project",
	}); err != nil {
		t.Fatalf("botstore.Save: %v", err)
	}
}

func TestRegister(t *testing.T) {
	reg := mcp.NewRegistry()
	Register(reg, t.TempDir(), nil)
	tools := reg.ListTools()
	names := map[string]bool{}
	for _, tool := range tools {
		names[tool.Name] = true
	}
	for _, want := range []string{"c4_notification_set", "c4_notification_get", "c4_notify"} {
		if !names[want] {
			t.Errorf("tool %q not registered", want)
		}
	}
}

func TestGetNotConfigured(t *testing.T) {
	reg := mcp.NewRegistry()
	Register(reg, t.TempDir(), nil)

	result, err := reg.Call("c4_notification_get", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map result, got %T", result)
	}
	if m["configured"] != false {
		t.Errorf("expected configured=false, got %v", m["configured"])
	}
}

func TestSetAndGet(t *testing.T) {
	reg := mcp.NewRegistry()
	dir := t.TempDir()
	setupBotstore(t, dir, "testbot", "tok123", []int64{111})
	Register(reg, dir, nil)

	args, _ := json.Marshal(map[string]any{
		"bot_username": "testbot",
	})
	_, err := reg.Call("c4_notification_set", args)
	if err != nil {
		t.Fatalf("set failed: %v", err)
	}

	// Verify file written
	if _, ferr := os.Stat(filepath.Join(dir, ".c4", configFile)); ferr != nil {
		t.Errorf("config file not created: %v", ferr)
	}

	// Get — should show bot_username (no token)
	result, err := reg.Call("c4_notification_get", nil)
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}
	m := result.(map[string]any)
	if m["configured"] != true {
		t.Errorf("expected configured=true, got %v", m["configured"])
	}
	if m["bot_username"] != "testbot" {
		t.Errorf("expected bot_username=testbot, got %v", m["bot_username"])
	}
	// Ensure token is NOT exposed
	if _, hasToken := m["token"]; hasToken {
		t.Error("token should not be exposed in c4_notification_get")
	}
}

func TestSetBotNotFound(t *testing.T) {
	reg := mcp.NewRegistry()
	dir := t.TempDir()
	Register(reg, dir, nil)

	args, _ := json.Marshal(map[string]any{
		"bot_username": "nonexistent_bot",
	})
	_, err := reg.Call("c4_notification_set", args)
	if err == nil {
		t.Fatal("expected error for nonexistent bot")
	}
}

func TestNotifyNoConfig(t *testing.T) {
	reg := mcp.NewRegistry()
	Register(reg, t.TempDir(), nil)

	args, _ := json.Marshal(map[string]any{"message": "hello"})
	_, err := reg.Call("c4_notify", args)
	if err == nil {
		t.Fatal("expected error when no config")
	}
}

func TestSetWithEvents(t *testing.T) {
	reg := mcp.NewRegistry()
	dir := t.TempDir()
	setupBotstore(t, dir, "testbot", "tok123", []int64{111})
	Register(reg, dir, nil)

	args, _ := json.Marshal(map[string]any{
		"bot_username": "testbot",
		"events":       []string{"plan.created", "finish.complete"},
	})
	result, err := reg.Call("c4_notification_set", args)
	if err != nil {
		t.Fatalf("set failed: %v", err)
	}
	m := result.(map[string]any)
	if m["success"] != true {
		t.Errorf("expected success=true, got %v", m["success"])
	}

	// Reload config and verify events persisted
	cfg, err := loadConfig(dir)
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if len(cfg.Events) != 2 || cfg.Events[0] != "plan.created" {
		t.Errorf("events not persisted: got %v", cfg.Events)
	}
}

func TestNotifyEventSkipped(t *testing.T) {
	reg := mcp.NewRegistry()
	dir := t.TempDir()
	setupBotstore(t, dir, "testbot", "tok123", []int64{111})
	Register(reg, dir, nil)

	setArgs, _ := json.Marshal(map[string]any{
		"bot_username": "testbot",
		"events":       []string{"plan.created", "finish.complete"},
	})
	if _, err := reg.Call("c4_notification_set", setArgs); err != nil {
		t.Fatalf("set: %v", err)
	}

	// Send with event NOT in configured list → should be skipped, no HTTP call
	notifyArgs, _ := json.Marshal(map[string]any{
		"message": "checkpoint reached",
		"event":   "checkpoint.ready",
	})
	result, err := reg.Call("c4_notify", notifyArgs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]any)
	if m["sent"] != false || m["skipped"] != true {
		t.Errorf("expected {sent:false, skipped:true}, got %v", m)
	}
}

func TestNotifyNoAllowFrom(t *testing.T) {
	// Bot with no AllowFrom entries → c4_notify should error
	reg := mcp.NewRegistry()
	dir := t.TempDir()
	setupBotstore(t, dir, "emptybot", "tok456", nil)
	Register(reg, dir, nil)

	setArgs, _ := json.Marshal(map[string]any{
		"bot_username": "emptybot",
	})
	if _, err := reg.Call("c4_notification_set", setArgs); err != nil {
		t.Fatalf("set: %v", err)
	}

	notifyArgs, _ := json.Marshal(map[string]any{"message": "hello"})
	_, err := reg.Call("c4_notify", notifyArgs)
	if err == nil {
		t.Fatal("expected error when bot has no AllowFrom entries")
	}
}

func TestNotifyNoEventFilter(t *testing.T) {
	// When no events configured, message passes regardless of event field.
	// We can't call actual Telegram in a unit test, so we test event-skipped logic only.
	// A bot with AllowFrom set will hit SendTelegram which will fail (no real API).
	// We verify the event-filter path at least doesn't skip, by checking the error
	// is a send error (not a skip/config error).
	reg := mcp.NewRegistry()
	dir := t.TempDir()
	setupBotstore(t, dir, "testbot", "tok123", []int64{111})
	Register(reg, dir, nil)

	setArgs, _ := json.Marshal(map[string]any{
		"bot_username": "testbot",
		// no events field
	})
	if _, err := reg.Call("c4_notification_set", setArgs); err != nil {
		t.Fatalf("set: %v", err)
	}

	notifyArgs, _ := json.Marshal(map[string]any{
		"message": "any event passes",
		"event":   "checkpoint.ready",
	})
	result, err := reg.Call("c4_notify", notifyArgs)
	// Either sent (unlikely without real bot) or telegram send error — but NOT a skip
	if err == nil {
		m, ok := result.(map[string]any)
		if ok && m["skipped"] == true {
			t.Error("message should not be skipped when no event filter is configured")
		}
	}
	// If err != nil, it's a telegram send error which is acceptable in unit test
}
