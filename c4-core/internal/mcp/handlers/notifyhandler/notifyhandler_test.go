package notifyhandler

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/changmin/c4-core/internal/mcp"
)

func TestRegister(t *testing.T) {
	reg := mcp.NewRegistry()
	Register(reg, t.TempDir())
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
	Register(reg, t.TempDir())

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
	Register(reg, dir)

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
	Register(reg, t.TempDir())

	args, _ := json.Marshal(map[string]any{"message": "hello"})
	_, err := reg.Call("c4_notify", args)
	if err == nil {
		t.Fatal("expected error when no config")
	}
}
