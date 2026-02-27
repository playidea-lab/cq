package webcontent

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	c2webcontent "github.com/changmin/c4-core/internal/c2/webcontent"
	"github.com/changmin/c4-core/internal/mcp"
)

// testWebFetchHandler creates a handler with SSRF check disabled for testing.
func testWebFetchHandler() mcp.HandlerFunc {
	return makeWebFetchHandler(&c2webcontent.FetchOpts{SkipSSRFCheck: true})
}

func TestHandleWebFetch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<html><head><title>Test</title></head><body><h1>Hello</h1><p>World</p></body></html>`))
	}))
	defer srv.Close()

	handler := testWebFetchHandler()
	raw, _ := json.Marshal(map[string]any{"url": srv.URL})
	result, err := handler(json.RawMessage(raw))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map result, got %T", result)
	}

	if m["success"] != true {
		t.Errorf("expected success=true, got %v", m["success"])
	}
	if m["method"] != "html_converted" {
		t.Errorf("expected method html_converted, got %v", m["method"])
	}
	if m["title"] != "Test" {
		t.Errorf("expected title 'Test', got %v", m["title"])
	}
	content, _ := m["content"].(string)
	if content == "" {
		t.Error("expected non-empty content")
	}
}

func TestHandleWebFetchMaxLength(t *testing.T) {
	bigContent := strings.Repeat("Hello ", 10000)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte(bigContent))
	}))
	defer srv.Close()

	handler := testWebFetchHandler()
	raw, _ := json.Marshal(map[string]any{"url": srv.URL, "max_length": 100})
	result, err := handler(json.RawMessage(raw))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	content, _ := m["content"].(string)
	if len(content) > 100 {
		t.Errorf("expected content truncated to 100, got %d", len(content))
	}
	if m["truncated"] != true {
		t.Error("expected truncated=true")
	}
}

func TestHandleWebFetchInvalidURL(t *testing.T) {
	handler := testWebFetchHandler()
	raw, _ := json.Marshal(map[string]any{"url": "not-a-valid-url"})
	result, err := handler(json.RawMessage(raw))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	if _, ok := m["error"]; !ok {
		t.Error("expected error in response for invalid URL")
	}
}

func TestHandleWebFetchMissingURL(t *testing.T) {
	handler := testWebFetchHandler()
	raw, _ := json.Marshal(map[string]any{})
	result, err := handler(json.RawMessage(raw))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	errMsg, _ := m["error"].(string)
	if errMsg != "url is required" {
		t.Errorf("expected 'url is required' error, got %q", errMsg)
	}
}

func TestHandleWebFetchWithLLMSTxt(t *testing.T) {
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	defer srv.Close()

	mux.HandleFunc("/page", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<html><head><title>Test</title></head><body><p>Hello</p></body></html>`))
	})
	mux.HandleFunc("/.well-known/llms.txt", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte("# My Project\n\n> A test project\n\n## Docs\n\n- [API](https://example.com/api): API docs\n"))
	})

	handler := testWebFetchHandler()
	raw, _ := json.Marshal(map[string]any{"url": srv.URL + "/page", "include_llms_txt": true})
	result, err := handler(json.RawMessage(raw))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	if m["success"] != true {
		t.Errorf("expected success=true, got %v", m["success"])
	}

	llms, ok := m["llms_txt"].(map[string]any)
	if !ok {
		t.Fatal("expected llms_txt in response")
	}
	if llms["title"] != "My Project" {
		t.Errorf("expected llms_txt title 'My Project', got %v", llms["title"])
	}
}

func TestRegisterWebContentHandlers(t *testing.T) {
	reg := mcp.NewRegistry()
	Register(reg)

	tools := reg.ListTools()
	found := false
	for _, tool := range tools {
		if tool.Name == "c4_web_fetch" {
			found = true
			break
		}
	}
	if !found {
		t.Error("c4_web_fetch not registered")
	}
}
