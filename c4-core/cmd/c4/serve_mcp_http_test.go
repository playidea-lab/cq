package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/changmin/c4-core/internal/config"
)

// newTestMCPHTTPComponent creates an mcpHTTPComponent wired to a test mcpServer.
// apiKey is pre-injected into the component (skips secret/env resolution).
func newTestMCPHTTPComponent(t *testing.T, apiKey string) *mcpHTTPComponent {
	t.Helper()
	db, cleanup := setupTestDB(t)
	t.Cleanup(cleanup)
	srv := newTestMCPServer(t, db)
	comp := newMCPHTTPComponent(srv, config.ServeMCPHTTPConfig{
		Port:   4142,
		Bind:   "127.0.0.1",
		APIKey: apiKey,
	})
	comp.apiKey = apiKey // pre-inject so withAuth works without Start()
	return comp
}

// doMCPRequest sends a JSON-RPC request through the component's HTTP handler
// and returns the response recorder.
func doMCPRequest(t *testing.T, comp *mcpHTTPComponent, apiKey string, body any) *httptest.ResponseRecorder {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("X-API-Key", apiKey)
	}
	w := httptest.NewRecorder()
	http.HandlerFunc(comp.withAuth(comp.handleMCP)).ServeHTTP(w, req)
	return w
}

// TestMCPHTTP_NoAPIKey_401 verifies that a request without an API key is rejected.
func TestMCPHTTP_NoAPIKey_401(t *testing.T) {
	comp := newTestMCPHTTPComponent(t, "secret-key")
	w := doMCPRequest(t, comp, "" /* no key */, map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]any{},
	})
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

// TestMCPHTTP_WrongAPIKey_401 verifies that an incorrect API key is rejected.
func TestMCPHTTP_WrongAPIKey_401(t *testing.T) {
	comp := newTestMCPHTTPComponent(t, "secret-key")
	w := doMCPRequest(t, comp, "wrong-key", map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]any{},
	})
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

// TestMCPHTTP_Initialize verifies the initialize method returns serverInfo.
func TestMCPHTTP_Initialize(t *testing.T) {
	comp := newTestMCPHTTPComponent(t, "secret-key")
	w := doMCPRequest(t, comp, "secret-key", map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]any{},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}
	var resp mcpResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
	resultMap, ok := resp.Result.(map[string]any)
	if !ok {
		t.Fatalf("result is not a map: %T", resp.Result)
	}
	if _, hasServerInfo := resultMap["serverInfo"]; !hasServerInfo {
		t.Errorf("expected serverInfo in initialize result, got: %v", resultMap)
	}
}

// TestMCPHTTP_ToolsList verifies tools/list returns a non-empty tools array.
func TestMCPHTTP_ToolsList(t *testing.T) {
	comp := newTestMCPHTTPComponent(t, "secret-key")
	w := doMCPRequest(t, comp, "secret-key", map[string]any{
		"jsonrpc": "2.0", "id": 2, "method": "tools/list",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}
	var resp mcpResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
	resultMap, ok := resp.Result.(map[string]any)
	if !ok {
		t.Fatalf("result is not a map: %T", resp.Result)
	}
	tools, ok := resultMap["tools"].([]any)
	if !ok || len(tools) == 0 {
		t.Errorf("expected non-empty tools array, got: %v", resultMap["tools"])
	}
}

// TestMCPHTTP_ToolsCall verifies tools/call dispatches a tool and returns a result.
func TestMCPHTTP_ToolsCall(t *testing.T) {
	comp := newTestMCPHTTPComponent(t, "secret-key")
	w := doMCPRequest(t, comp, "secret-key", map[string]any{
		"jsonrpc": "2.0",
		"id":      3,
		"method":  "tools/call",
		"params": map[string]any{
			"name":      "c4_status",
			"arguments": map[string]any{},
		},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}
	var resp mcpResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected tool error: %+v", resp.Error)
	}
	// Result should have content array with text.
	resultMap, ok := resp.Result.(map[string]any)
	if !ok {
		t.Fatalf("result is not a map: %T", resp.Result)
	}
	if _, hasContent := resultMap["content"]; !hasContent {
		t.Errorf("expected content key in tools/call result, got: %v", resultMap)
	}
}

// TestMCPHTTP_EmptyAPIKey_RefuseStart verifies that Start() fails when no API key is configured.
func TestMCPHTTP_EmptyAPIKey_RefuseStart(t *testing.T) {
	db, cleanup := setupTestDB(t)
	t.Cleanup(cleanup)
	srv := newTestMCPServer(t, db)

	comp := newMCPHTTPComponent(srv, config.ServeMCPHTTPConfig{
		Port:   4142,
		Bind:   "127.0.0.1",
		APIKey: "", // empty — no secrets.db, no env
	})
	// Ensure CQ_MCP_API_KEY is not set in the test environment.
	t.Setenv("CQ_MCP_API_KEY", "")

	err := comp.Start(t.Context())
	if err == nil {
		comp.Stop(t.Context()) //nolint:errcheck
		t.Fatal("expected Start() to fail with empty api_key, got nil")
	}
}

// TestMCPHTTP_BearerAuth verifies that Authorization: Bearer <key> is accepted.
func TestMCPHTTP_BearerAuth(t *testing.T) {
	comp := newTestMCPHTTPComponent(t, "bearer-key")

	b, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": 4, "method": "initialize", "params": map[string]any{},
	})
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer bearer-key")

	w := httptest.NewRecorder()
	http.HandlerFunc(comp.withAuth(comp.handleMCP)).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 with Bearer auth, got %d", w.Code)
	}
}
