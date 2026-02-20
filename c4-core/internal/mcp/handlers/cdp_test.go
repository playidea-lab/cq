//go:build cdp


package handlers

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/changmin/c4-core/internal/cdp"
	"github.com/changmin/c4-core/internal/mcp"
)

func setupCDPHandlers(t *testing.T) *mcp.Registry {
	t.Helper()
	reg := mcp.NewRegistry()
	runner := cdp.NewRunner()
	RegisterCDPHandlers(reg, runner)
	return reg
}

func TestCDPToolsRegistered(t *testing.T) {
	reg := setupCDPHandlers(t)

	tools := reg.ListTools()
	names := make(map[string]bool)
	for _, tool := range tools {
		names[tool.Name] = true
	}

	expected := []string{"c4_cdp_run", "c4_cdp_list"}
	for _, name := range expected {
		if !names[name] {
			t.Errorf("tool %q not registered", name)
		}
	}
}

func TestCDPRunMissingScript(t *testing.T) {
	reg := setupCDPHandlers(t)

	params := `{}`
	_, err := reg.Call("c4_cdp_run", json.RawMessage(params))
	if err == nil {
		t.Fatal("expected error for missing script")
	}
	if !strings.Contains(err.Error(), "script is required") {
		t.Errorf("error = %q, want to contain 'script is required'", err.Error())
	}
}

func TestCDPRunEmptyScript(t *testing.T) {
	reg := setupCDPHandlers(t)

	params := `{"script": ""}`
	_, err := reg.Call("c4_cdp_run", json.RawMessage(params))
	if err == nil {
		t.Fatal("expected error for empty script")
	}
	if !strings.Contains(err.Error(), "script is required") {
		t.Errorf("error = %q, want to contain 'script is required'", err.Error())
	}
}

func TestCDPRunInvalidJSON(t *testing.T) {
	reg := setupCDPHandlers(t)

	params := `{invalid json}`
	_, err := reg.Call("c4_cdp_run", json.RawMessage(params))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
	if !strings.Contains(err.Error(), "parsing params") {
		t.Errorf("error = %q, want to contain 'parsing params'", err.Error())
	}
}

func TestCDPRunNonLocalhostURL(t *testing.T) {
	reg := setupCDPHandlers(t)

	// cdp.Runner.Execute validates localhost-only connections
	params := `{"script": "1+1", "url": "http://evil.com:9222"}`
	_, err := reg.Call("c4_cdp_run", json.RawMessage(params))
	if err == nil {
		t.Fatal("expected error for non-localhost URL")
	}
	if !strings.Contains(err.Error(), "only localhost") {
		t.Errorf("error = %q, want to contain 'only localhost'", err.Error())
	}
}

func TestCDPListInvalidJSON(t *testing.T) {
	reg := setupCDPHandlers(t)

	params := `{bad}`
	_, err := reg.Call("c4_cdp_list", json.RawMessage(params))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
	if !strings.Contains(err.Error(), "parsing params") {
		t.Errorf("error = %q, want to contain 'parsing params'", err.Error())
	}
}

func TestCDPListNonLocalhostURL(t *testing.T) {
	reg := setupCDPHandlers(t)

	params := `{"url": "http://remote-host:9222"}`
	_, err := reg.Call("c4_cdp_list", json.RawMessage(params))
	if err == nil {
		t.Fatal("expected error for non-localhost URL")
	}
	if !strings.Contains(err.Error(), "only localhost") {
		t.Errorf("error = %q, want to contain 'only localhost'", err.Error())
	}
}

func TestCDPRunDefaultURL(t *testing.T) {
	// This test verifies the default URL is used when none provided.
	// It will fail to connect (no browser running) but should NOT fail on URL validation.
	reg := setupCDPHandlers(t)

	params := `{"script": "document.title"}`
	_, err := reg.Call("c4_cdp_run", json.RawMessage(params))
	if err == nil {
		t.Skip("browser running on localhost:9222, cannot test connection failure")
	}
	// Should fail with a connection error, NOT a URL validation error
	if strings.Contains(err.Error(), "only localhost") {
		t.Errorf("error = %q, should not be a URL validation error", err.Error())
	}
}

func TestCDPListDefaultURL(t *testing.T) {
	// Same as above: verifies default URL is used when none provided.
	reg := setupCDPHandlers(t)

	params := `{}`
	_, err := reg.Call("c4_cdp_list", json.RawMessage(params))
	if err == nil {
		t.Skip("browser running on localhost:9222, cannot test connection failure")
	}
	// Should fail with a connection error, NOT a URL validation error
	if strings.Contains(err.Error(), "only localhost") {
		t.Errorf("error = %q, should not be a URL validation error", err.Error())
	}
}
