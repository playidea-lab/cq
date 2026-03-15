package mcphttp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/changmin/c4-core/internal/config"
	"github.com/changmin/c4-core/internal/serve"
)

// compile-time interface assertion
var _ serve.Component = (*Component)(nil)

// stubHandler is a minimal RequestHandler for testing.
type stubHandler struct{}

func (s *stubHandler) HandleRawRequest(body []byte, _ context.Context) []byte {
	var req Request
	if err := json.Unmarshal(body, &req); err != nil {
		resp, _ := json.Marshal(Response{
			JSONRPC: "2.0",
			Error:   &Error{Code: -32700, Message: "parse error"},
		})
		return resp
	}

	var result interface{}
	switch req.Method {
	case "initialize":
		result = map[string]any{
			"protocolVersion": "2025-03-26",
			"serverInfo":      map[string]any{"name": "test", "version": "0.0.1"},
			"capabilities":    map[string]any{},
		}
	case "tools/list":
		result = map[string]any{
			"tools": []any{map[string]any{"name": "test_tool"}},
		}
	case "tools/call":
		result = map[string]any{
			"content": []any{map[string]any{"type": "text", "text": "ok"}},
		}
	default:
		resp, _ := json.Marshal(Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &Error{Code: -32601, Message: "method not found"},
		})
		return resp
	}

	resp, _ := json.Marshal(Response{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  result,
	})
	return resp
}

// stubSecretGetter is a no-op SecretGetter.
type stubSecretGetter struct{}

func (s *stubSecretGetter) Get(key string) (string, error) { return "", nil }

func newTestComponent(t *testing.T, apiKey string) *Component {
	t.Helper()
	comp := New(&stubHandler{}, &stubSecretGetter{}, config.ServeMCPHTTPConfig{
		Port:   4142,
		Bind:   "127.0.0.1",
		APIKey: apiKey,
	})
	comp.apiKey = apiKey // pre-inject so withAuth works without Start()
	return comp
}

func doRequest(t *testing.T, comp *Component, apiKey string, body any) *httptest.ResponseRecorder {
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
	comp := newTestComponent(t, "secret-key")
	w := doRequest(t, comp, "" /* no key */, map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]any{},
	})
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

// TestMCPHTTP_WrongAPIKey_401 verifies that an incorrect API key is rejected.
func TestMCPHTTP_WrongAPIKey_401(t *testing.T) {
	comp := newTestComponent(t, "secret-key")
	w := doRequest(t, comp, "wrong-key", map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]any{},
	})
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

// TestMCPHTTP_Initialize verifies the initialize method returns serverInfo.
func TestMCPHTTP_Initialize(t *testing.T) {
	comp := newTestComponent(t, "secret-key")
	w := doRequest(t, comp, "secret-key", map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]any{},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}
	var resp Response
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
	comp := newTestComponent(t, "secret-key")
	w := doRequest(t, comp, "secret-key", map[string]any{
		"jsonrpc": "2.0", "id": 2, "method": "tools/list",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}
	var resp Response
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

// TestMCPHTTP_EmptyAPIKey_RefuseStart verifies that Start() fails when no API key is configured.
func TestMCPHTTP_EmptyAPIKey_RefuseStart(t *testing.T) {
	comp := New(&stubHandler{}, &stubSecretGetter{}, config.ServeMCPHTTPConfig{
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
	comp := newTestComponent(t, "bearer-key")

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

// TestMCPHTTP_ComponentName verifies the component Name().
func TestMCPHTTP_ComponentName(t *testing.T) {
	comp := newTestComponent(t, "key")
	if comp.Name() != "mcp-http" {
		t.Errorf("Name() = %q, want mcp-http", comp.Name())
	}
}
