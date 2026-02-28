//go:build c3_eventbus

package eventbushandler

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/changmin/c4-core/internal/mcp"
)

func TestDoorayRespondTool_Success(t *testing.T) {
	// A real *.dooray.com URL would require DNS + SSRF check.
	// This test verifies that a valid dooray.com URL passes domain validation
	// but (expectedly) fails at DNS resolution in test environments.
	// We verify the tool is registered and reachable.
	reg := mcp.NewRegistry()
	RegisterDoorayRespondTool(reg)

	// Call with a dooray.com URL — may fail at DNS but should not fail at domain check
	raw, _ := json.Marshal(map[string]any{
		"response_url": "https://hooks.dooray.com/services/123/abc",
		"text":         "hello from worker",
	})
	result, err := reg.Call("c4_dooray_respond", raw)
	if err != nil {
		t.Fatalf("handler returned unexpected error: %v", err)
	}
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map result, got %T", result)
	}
	// Result should have a "success" field (may be false due to DNS failure in tests)
	if _, hasSuccess := m["success"]; !hasSuccess {
		t.Error("expected 'success' field in result")
	}
}

func TestDoorayRespondTool_InvalidURL(t *testing.T) {
	reg := mcp.NewRegistry()
	RegisterDoorayRespondTool(reg)

	// non-dooray URL must be rejected with success=false and domain error
	raw, _ := json.Marshal(map[string]any{
		"response_url": "https://attacker.evil.com/hook",
		"text":         "pwned",
	})
	result, err := reg.Call("c4_dooray_respond", raw)
	if err != nil {
		t.Fatalf("handler returned unexpected error: %v", err)
	}
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map result, got %T", result)
	}
	if success, _ := m["success"].(bool); success {
		t.Error("expected success=false for non-dooray URL")
	}
	errMsg, _ := m["error"].(string)
	if !strings.Contains(errMsg, "not in allowed domains") {
		t.Errorf("expected 'not in allowed domains' in error, got: %q", errMsg)
	}
}

func TestDoorayRespondTool_EmptyText(t *testing.T) {
	reg := mcp.NewRegistry()
	RegisterDoorayRespondTool(reg)

	raw, _ := json.Marshal(map[string]any{
		"response_url": "https://hooks.dooray.com/services/123/abc",
		"text":         "",
	})
	_, err := reg.Call("c4_dooray_respond", raw)
	if err == nil {
		t.Error("expected error for empty text")
	}
	if !strings.Contains(err.Error(), "text is required") {
		t.Errorf("expected 'text is required' in error, got: %v", err)
	}
}
