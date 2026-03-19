package notifyhandler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/changmin/c4-core/internal/mcp"
)

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
	Register(reg, dir, nil)

	args, _ := json.Marshal(map[string]any{
		"channel":     "slack",
		"webhook_url": "https://hooks.example.com/abc123",
	})
	_, err := reg.Call("c4_notification_set", args)
	if err != nil {
		t.Fatalf("set failed: %v", err)
	}

	// Verify file written
	if _, ferr := os.Stat(filepath.Join(dir, ".c4", configFile)); ferr != nil {
		t.Errorf("config file not created: %v", ferr)
	}

	// Get — should show masked URL
	result, err := reg.Call("c4_notification_get", nil)
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}
	m := result.(map[string]any)
	if m["configured"] != true {
		t.Errorf("expected configured=true, got %v", m["configured"])
	}
	if m["channel"] != "slack" {
		t.Errorf("expected channel=slack, got %v", m["channel"])
	}
	url, _ := m["webhook_url"].(string)
	if url == "https://hooks.example.com/abc123" {
		t.Error("webhook_url should be masked")
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
	Register(reg, dir, nil)

	args, _ := json.Marshal(map[string]any{
		"channel":     "slack",
		"webhook_url": "https://hooks.example.com/abc",
		"events":      []string{"plan.created", "finish.complete"},
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
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("webhook should not be called for filtered-out event")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	reg := mcp.NewRegistry()
	dir := t.TempDir()
	Register(reg, dir, nil)

	setArgs, _ := json.Marshal(map[string]any{
		"channel":     "slack",
		"webhook_url": srv.URL,
		"events":      []string{"plan.created", "finish.complete"},
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

func TestNotifyEventMatched(t *testing.T) {
	var received string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		received = body["text"]
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	reg := mcp.NewRegistry()
	dir := t.TempDir()
	Register(reg, dir, nil)

	setArgs, _ := json.Marshal(map[string]any{
		"channel":     "slack",
		"webhook_url": srv.URL,
		"events":      []string{"plan.created", "finish.complete"},
	})
	if _, err := reg.Call("c4_notification_set", setArgs); err != nil {
		t.Fatalf("set: %v", err)
	}

	// Send with event IN configured list → should send
	notifyArgs, _ := json.Marshal(map[string]any{
		"message": "plan done",
		"event":   "plan.created",
	})
	result, err := reg.Call("c4_notify", notifyArgs)
	if err != nil {
		t.Fatalf("c4_notify: %v", err)
	}
	m := result.(map[string]any)
	if m["sent"] != true {
		t.Errorf("expected sent=true, got %v", m)
	}
	if received != "plan done" {
		t.Errorf("webhook received %q, want %q", received, "plan done")
	}
}

func TestNotifyTitleAndMessage(t *testing.T) {
	var received string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		received = body["text"]
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	reg := mcp.NewRegistry()
	dir := t.TempDir()
	Register(reg, dir, nil)

	setArgs, _ := json.Marshal(map[string]any{
		"channel":     "slack",
		"webhook_url": srv.URL,
	})
	if _, err := reg.Call("c4_notification_set", setArgs); err != nil {
		t.Fatalf("set: %v", err)
	}

	notifyArgs, _ := json.Marshal(map[string]any{
		"message": "body text",
		"title":   "My Title",
	})
	if _, err := reg.Call("c4_notify", notifyArgs); err != nil {
		t.Fatalf("c4_notify: %v", err)
	}
	want := "My Title\nbody text"
	if received != want {
		t.Errorf("text: got %q, want %q", received, want)
	}
}

func TestNotifyNoEventFilter(t *testing.T) {
	// When no events configured, all messages pass regardless of event field.
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	reg := mcp.NewRegistry()
	dir := t.TempDir()
	Register(reg, dir, nil)

	setArgs, _ := json.Marshal(map[string]any{
		"channel":     "slack",
		"webhook_url": srv.URL,
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
	if err != nil {
		t.Fatalf("c4_notify: %v", err)
	}
	m := result.(map[string]any)
	if m["sent"] != true {
		t.Errorf("expected sent=true, got %v", m)
	}
	if !called {
		t.Error("webhook not called despite no event filter")
	}
}
