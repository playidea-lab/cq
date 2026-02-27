package cdp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"
)

func TestWebMCPToolJSON(t *testing.T) {
	tool := WebMCPTool{
		Name:        "search",
		Description: "Search products",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"q": map[string]any{"type": "string"},
			},
		},
		Origin: "https://example.com",
	}

	data, err := json.Marshal(tool)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded WebMCPTool
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if decoded.Name != "search" {
		t.Errorf("Name = %q, want %q", decoded.Name, "search")
	}
	if decoded.Description != "Search products" {
		t.Errorf("Description = %q, want %q", decoded.Description, "Search products")
	}
	if decoded.Origin != "https://example.com" {
		t.Errorf("Origin = %q, want %q", decoded.Origin, "https://example.com")
	}
	if decoded.InputSchema == nil {
		t.Error("InputSchema is nil after round-trip")
	}
}

func TestWebMCPToolJSON_OmitEmpty(t *testing.T) {
	tool := WebMCPTool{
		Name:   "simple",
		Origin: "https://example.com",
	}
	data, err := json.Marshal(tool)
	if err != nil {
		t.Fatal(err)
	}
	// inputSchema should be omitted when nil
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	if _, ok := m["inputSchema"]; ok {
		t.Error("inputSchema should be omitted when nil")
	}
}

func TestWebMCPCallResultJSON(t *testing.T) {
	result := WebMCPCallResult{
		Result:    map[string]any{"items": []any{"a", "b"}},
		ToolName:  "search",
		Origin:    "https://shop.example.com",
		ElapsedMs: 150,
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}

	var decoded WebMCPCallResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.ToolName != "search" {
		t.Errorf("ToolName = %q, want %q", decoded.ToolName, "search")
	}
	if decoded.Origin != "https://shop.example.com" {
		t.Errorf("Origin = %q, want %q", decoded.Origin, "https://shop.example.com")
	}
	if decoded.ElapsedMs != 150 {
		t.Errorf("ElapsedMs = %d, want 150", decoded.ElapsedMs)
	}
}

func TestDiscoverWebMCPTools_ValidationErrors(t *testing.T) {
	r := NewRunner()
	ctx := context.Background()

	t.Run("empty debug url", func(t *testing.T) {
		_, err := r.DiscoverWebMCPTools(ctx, "", "https://example.com", 0)
		if err == nil {
			t.Fatal("expected error for empty debug url")
		}
	})

	t.Run("remote debug url rejected", func(t *testing.T) {
		_, err := r.DiscoverWebMCPTools(ctx, "http://evil.com:9222", "https://example.com", 0)
		if err == nil {
			t.Fatal("expected error for remote debug url")
		}
	})

	t.Run("empty page url", func(t *testing.T) {
		_, err := r.DiscoverWebMCPTools(ctx, "http://localhost:9222", "", 0)
		if err == nil {
			t.Fatal("expected error for empty page url")
		}
		if err.Error() != "cdp: page_url is required for WebMCP discovery" {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

func TestCallWebMCPTool_ValidationErrors(t *testing.T) {
	r := NewRunner()
	ctx := context.Background()

	t.Run("empty debug url", func(t *testing.T) {
		_, err := r.CallWebMCPTool(ctx, "", "https://example.com", "search", nil, 0)
		if err == nil {
			t.Fatal("expected error for empty debug url")
		}
	})

	t.Run("remote debug url rejected", func(t *testing.T) {
		_, err := r.CallWebMCPTool(ctx, "http://remote.host:9222", "https://example.com", "search", nil, 0)
		if err == nil {
			t.Fatal("expected error for remote debug url")
		}
	})

	t.Run("empty page url", func(t *testing.T) {
		_, err := r.CallWebMCPTool(ctx, "http://localhost:9222", "", "search", nil, 0)
		if err == nil {
			t.Fatal("expected error for empty page url")
		}
		if err.Error() != "cdp: page_url is required" {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("empty tool name", func(t *testing.T) {
		_, err := r.CallWebMCPTool(ctx, "http://localhost:9222", "https://example.com", "", nil, 0)
		if err == nil {
			t.Fatal("expected error for empty tool name")
		}
		if err.Error() != "cdp: tool_name is required" {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

func TestCallWebMCPTool_ArgsSerialization(t *testing.T) {
	// Verify that args can be marshaled to JSON (they'll be embedded in JS code).
	args := map[string]any{
		"query":  "test search",
		"limit":  10,
		"nested": map[string]any{"key": "value"},
	}
	data, err := json.Marshal(args)
	if err != nil {
		t.Fatalf("failed to marshal args: %v", err)
	}
	// Should be valid JSON
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("round-trip failed: %v", err)
	}
	if decoded["query"] != "test search" {
		t.Errorf("query = %v, want %q", decoded["query"], "test search")
	}
	if decoded["limit"] != float64(10) {
		t.Errorf("limit = %v, want 10", decoded["limit"])
	}
}

func TestGetWebMCPContext_ValidationErrors(t *testing.T) {
	r := NewRunner()
	ctx := context.Background()

	t.Run("empty debug url", func(t *testing.T) {
		_, err := r.GetWebMCPContext(ctx, "", "https://example.com", 0)
		if err == nil {
			t.Fatal("expected error for empty debug url")
		}
	})

	t.Run("remote debug url rejected", func(t *testing.T) {
		_, err := r.GetWebMCPContext(ctx, "http://evil.com:9222", "https://example.com", 0)
		if err == nil {
			t.Fatal("expected error for remote debug url")
		}
	})

	t.Run("empty page url", func(t *testing.T) {
		_, err := r.GetWebMCPContext(ctx, "http://localhost:9222", "", 0)
		if err == nil {
			t.Fatal("expected error for empty page url")
		}
		if err.Error() != "cdp: page_url is required" {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

func TestProvideWebMCPContext_ValidationErrors(t *testing.T) {
	r := NewRunner()
	ctx := context.Background()

	t.Run("empty debug url", func(t *testing.T) {
		_, err := r.ProvideWebMCPContext(ctx, "", "https://example.com", nil, 0)
		if err == nil {
			t.Fatal("expected error for empty debug url")
		}
	})

	t.Run("remote debug url rejected", func(t *testing.T) {
		_, err := r.ProvideWebMCPContext(ctx, "http://evil.com:9222", "https://example.com", nil, 0)
		if err == nil {
			t.Fatal("expected error for remote debug url")
		}
	})

	t.Run("empty page url", func(t *testing.T) {
		_, err := r.ProvideWebMCPContext(ctx, "http://localhost:9222", "", nil, 0)
		if err == nil {
			t.Fatal("expected error for empty page url")
		}
		if err.Error() != "cdp: page_url is required" {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

func TestClearWebMCPContext_ValidationErrors(t *testing.T) {
	r := NewRunner()
	ctx := context.Background()

	t.Run("empty debug url", func(t *testing.T) {
		_, err := r.ClearWebMCPContext(ctx, "", "https://example.com", 0)
		if err == nil {
			t.Fatal("expected error for empty debug url")
		}
	})

	t.Run("remote debug url rejected", func(t *testing.T) {
		_, err := r.ClearWebMCPContext(ctx, "http://evil.com:9222", "https://example.com", 0)
		if err == nil {
			t.Fatal("expected error for remote debug url")
		}
	})

	t.Run("empty page url", func(t *testing.T) {
		_, err := r.ClearWebMCPContext(ctx, "http://localhost:9222", "", 0)
		if err == nil {
			t.Fatal("expected error for empty page url")
		}
		if err.Error() != "cdp: page_url is required" {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

func TestDiscoverWebMCPToolsWithOpts_ValidationErrors(t *testing.T) {
	r := NewRunner()
	ctx := context.Background()

	t.Run("empty debug url", func(t *testing.T) {
		_, err := r.DiscoverWebMCPToolsWithOpts(ctx, "", "https://example.com", 0, nil)
		if err == nil {
			t.Fatal("expected error for empty debug url")
		}
	})

	t.Run("remote debug url rejected", func(t *testing.T) {
		_, err := r.DiscoverWebMCPToolsWithOpts(ctx, "http://evil.com:9222", "https://example.com", 0, nil)
		if err == nil {
			t.Fatal("expected error for remote debug url")
		}
	})

	t.Run("empty page url", func(t *testing.T) {
		_, err := r.DiscoverWebMCPToolsWithOpts(ctx, "http://localhost:9222", "", 0, nil)
		if err == nil {
			t.Fatal("expected error for empty page url")
		}
	})

	t.Run("with opts wait for tools false", func(t *testing.T) {
		_, err := r.DiscoverWebMCPToolsWithOpts(ctx, "http://localhost:9222", "", 0, &DiscoverOpts{WaitForTools: false})
		if err == nil {
			t.Fatal("expected error for empty page url")
		}
	})
}

func TestWebMCPContextResult_Fields(t *testing.T) {
	result := WebMCPContextResult{
		Context:   map[string]any{"key": "value"},
		Action:    "provide",
		Origin:    "https://example.com",
		Available: true,
	}
	if result.Action != "provide" {
		t.Errorf("Action = %q, want provide", result.Action)
	}
	if result.Origin != "https://example.com" {
		t.Errorf("Origin = %q, want https://example.com", result.Origin)
	}
	if !result.Available {
		t.Error("Available should be true")
	}
}

// --- Integration tests (require a running browser with --remote-debugging-port) ---

// webmcpDebugURL returns the CDP debug URL if a browser is available, or skips.
func webmcpDebugURL(t *testing.T) string {
	t.Helper()
	url := os.Getenv("CDP_DEBUG_URL")
	if url == "" {
		url = "http://localhost:9222"
	}
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(fmt.Sprintf("%s/json/version", url))
	if err != nil {
		t.Skipf("No browser at %s: %v", url, err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Skipf("Browser at %s returned status %d", url, resp.StatusCode)
	}
	return url
}

func TestIntegration_DiscoverWebMCPTools(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	debugURL := webmcpDebugURL(t)
	r := NewRunner()
	ctx := context.Background()

	// about:blank won't have WebMCP — should return empty list, not error
	tools, err := r.DiscoverWebMCPTools(ctx, debugURL, "about:blank", 10)
	if err != nil {
		t.Fatalf("DiscoverWebMCPTools() error: %v", err)
	}
	// about:blank has no WebMCP, expect empty
	t.Logf("Discovered %d tools on about:blank", len(tools))
}

func TestIntegration_CallWebMCPTool(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	debugURL := webmcpDebugURL(t)
	r := NewRunner()
	ctx := context.Background()

	// Calling a tool on about:blank should fail with "WebMCP not available"
	_, err := r.CallWebMCPTool(ctx, debugURL, "about:blank", "search", map[string]any{"q": "test"}, 10)
	if err == nil {
		t.Fatal("expected error calling WebMCP tool on about:blank")
	}
	t.Logf("Expected error: %v", err)
}
