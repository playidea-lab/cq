package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/piqsol/c4/c5/internal/llmclient"
	"github.com/piqsol/c4/c5/internal/store"
)

// doorayPayload builds a minimal Dooray slash command POST body.
func doorayPayload(text, cmd, responseURL, appToken, cmdToken string) map[string]any {
	return map[string]any{
		"tenantId":    "tenant-1",
		"channelId":   "ch-1",
		"channelName": "test-channel",
		"userId":      "user-1",
		"command":     cmd,
		"text":        text,
		"responseUrl": responseURL,
		"appToken":    appToken,
		"cmdToken":    cmdToken,
	}
}

// doRequestNoAPIKey sends a request without any X-API-Key header.
func doRequestNoAPIKey(t *testing.T, srv *Server, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var b bytes.Buffer
	if body != nil {
		json.NewEncoder(&b).Encode(body)
	}
	req := httptest.NewRequest(method, path, &b)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	return w
}

// newTestServerWithLLM creates a test server with LLM client and Dooray webhook configured.
func newTestServerWithLLM(t *testing.T, llmCli *llmclient.Client, webhookURL string) *Server {
	t.Helper()
	dir := t.TempDir()
	st, err := store.New(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return NewServer(Config{
		Store:            st,
		Version:          "test",
		LLMClient:        llmCli,
		DoorayWebhookURL: webhookURL,
	})
}

func TestDoorayGET(t *testing.T) {
	srv := newTestServer(t)
	w := doRequest(t, srv, http.MethodGet, "/v1/webhooks/dooray", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /v1/webhooks/dooray: got %d, want 200", w.Code)
	}
}

func TestDoorayPOST_NoToken(t *testing.T) {
	// When C5_DOORAY_CMD_TOKEN is unset, any request should be accepted.
	t.Setenv("C5_DOORAY_CMD_TOKEN", "")

	srv := newTestServer(t)
	payload := doorayPayload("hello world", "/cq", "https://example.com/response", "", "")
	w := doRequest(t, srv, http.MethodPost, "/v1/webhooks/dooray", payload)

	if w.Code != http.StatusOK {
		t.Fatalf("POST without token: got %d, want 200 — body: %s", w.Code, w.Body.String())
	}

	var resp doorayResponse
	decodeJSON(t, w, &resp)
	if resp.Text != "⏳ 수신: hello world" {
		t.Errorf("response text: got %q, want %q", resp.Text, "⏳ 수신: hello world")
	}
	if resp.ResponseType != "ephemeral" {
		t.Errorf("responseType: got %q, want %q", resp.ResponseType, "ephemeral")
	}
}

func TestDoorayPOST_ValidToken(t *testing.T) {
	t.Setenv("C5_DOORAY_CMD_TOKEN", "secret-token")

	srv := newTestServer(t)
	// Token matches cmdToken field.
	payload := doorayPayload("test", "/cq", "https://example.com/r", "", "secret-token")
	w := doRequest(t, srv, http.MethodPost, "/v1/webhooks/dooray", payload)

	if w.Code != http.StatusOK {
		t.Fatalf("valid cmdToken: got %d, want 200 — body: %s", w.Code, w.Body.String())
	}
}

func TestDoorayPOST_ValidAppToken(t *testing.T) {
	t.Setenv("C5_DOORAY_CMD_TOKEN", "app-secret")

	srv := newTestServer(t)
	// Token matches appToken field.
	payload := doorayPayload("test", "/cq", "https://example.com/r", "app-secret", "")
	w := doRequest(t, srv, http.MethodPost, "/v1/webhooks/dooray", payload)

	if w.Code != http.StatusOK {
		t.Fatalf("valid appToken: got %d, want 200 — body: %s", w.Code, w.Body.String())
	}
}

func TestDoorayPOST_InvalidToken(t *testing.T) {
	t.Setenv("C5_DOORAY_CMD_TOKEN", "correct-token")

	srv := newTestServer(t)
	payload := doorayPayload("test", "/cq", "https://example.com/r", "wrong", "also-wrong")
	w := doRequest(t, srv, http.MethodPost, "/v1/webhooks/dooray", payload)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("invalid token: got %d, want 401", w.Code)
	}
}

func TestDoorayPOST_JobCreated(t *testing.T) {
	t.Setenv("C5_DOORAY_CMD_TOKEN", "")

	srv := newTestServer(t)
	payload := doorayPayload("my task", "/cq", "https://example.com/resp", "", "")
	w := doRequest(t, srv, http.MethodPost, "/v1/webhooks/dooray", payload)

	if w.Code != http.StatusOK {
		t.Fatalf("POST: got %d — body: %s", w.Code, w.Body.String())
	}

	// Verify a job was created with the expected tags and env vars.
	wJobs := doRequest(t, srv, http.MethodGet, "/v1/jobs", nil)
	if wJobs.Code != http.StatusOK {
		t.Fatalf("GET /v1/jobs: got %d", wJobs.Code)
	}

	var jobs []map[string]any
	if err := json.NewDecoder(wJobs.Body).Decode(&jobs); err != nil {
		t.Fatalf("decode jobs: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(jobs))
	}

	job := jobs[0]

	// Check tags contain "dooray".
	tags, _ := job["tags"].([]any)
	found := false
	for _, tag := range tags {
		if tag == "dooray" {
			found = true
		}
	}
	if !found {
		t.Errorf("job tags: %v — expected 'dooray'", tags)
	}

	// Check env vars.
	env, _ := job["env"].(map[string]any)
	if env["DOORAY_TEXT"] != "my task" {
		t.Errorf("DOORAY_TEXT: got %v, want %q", env["DOORAY_TEXT"], "my task")
	}
	if env["DOORAY_CMD"] != "/cq" {
		t.Errorf("DOORAY_CMD: got %v, want %q", env["DOORAY_CMD"], "/cq")
	}
	if env["DOORAY_RESPONSE_URL"] != "https://example.com/resp" {
		t.Errorf("DOORAY_RESPONSE_URL: got %v", env["DOORAY_RESPONSE_URL"])
	}
	if env["DOORAY_CHANNEL"] != "ch-1" {
		t.Errorf("DOORAY_CHANNEL: got %v, want %q", env["DOORAY_CHANNEL"], "ch-1")
	}
}

func TestDoorayPOST_InvalidMethod(t *testing.T) {
	srv := newTestServer(t)
	w := doRequest(t, srv, http.MethodPut, "/v1/webhooks/dooray", nil)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("PUT /v1/webhooks/dooray: got %d, want 405", w.Code)
	}
}

func TestDoorayAuthMiddlewareExempt(t *testing.T) {
	// The /v1/webhooks/dooray endpoint must be reachable without an API key
	// even when the server has apiKey configured.
	t.Setenv("C5_DOORAY_CMD_TOKEN", "")

	dir := t.TempDir()
	st, err := store.New(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	srv := NewServer(Config{
		Store:   st,
		Version: "test",
		APIKey:  "master-key", // auth middleware active
	})

	payload := doorayPayload("hi", "/cq", "https://example.com/r", "", "")
	// doRequestNoAPIKey sends no X-API-Key header.
	w := doRequestNoAPIKey(t, srv, http.MethodPost, "/v1/webhooks/dooray", payload)

	if w.Code != http.StatusOK {
		t.Fatalf("without api-key on exempt endpoint: got %d, want 200 — body: %s", w.Code, w.Body.String())
	}
}

// =========================================================================
// Server-side LLM tests
// =========================================================================

func TestDooray_ServerSide_LLMResponse(t *testing.T) {
	// Mock LLM server returns a canned answer.
	llmSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"content": "AI 응답입니다."}},
			},
		})
	}))
	defer llmSrv.Close()

	// Mock Dooray webhook signals via channel when it receives the response.
	done := make(chan string, 1)
	webhookSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		done <- body["text"]
		w.WriteHeader(http.StatusOK)
	}))
	defer webhookSrv.Close()

	llmCli := llmclient.New(llmSrv.URL, "test-key", "test-model", 100)
	srv := newTestServerWithLLM(t, llmCli, webhookSrv.URL)

	t.Setenv("C5_DOORAY_CMD_TOKEN", "")
	payload := doorayPayload("AI 질문", "/cq", "", "", "")
	w := doRequest(t, srv, http.MethodPost, "/v1/webhooks/dooray", payload)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", w.Code, w.Body.String())
	}

	// Ephemeral ack must mention the question text.
	var resp doorayResponse
	decodeJSON(t, w, &resp)
	if !strings.Contains(resp.Text, "AI 질문") {
		t.Errorf("ack text: got %q, expected to contain 'AI 질문'", resp.Text)
	}

	// Wait for the goroutine to post to the webhook.
	select {
	case text := <-done:
		if !strings.Contains(text, "AI 응답입니다.") {
			t.Errorf("webhook text: got %q, expected LLM answer", text)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for webhook response")
	}
}

func TestDooray_ServerSide_KnowledgeContext(t *testing.T) {
	// Verify system prompt contains projectID from channel mapping.
	systemPromptReceived := make(chan string, 1)
	llmSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if msgs, ok := body["messages"].([]any); ok && len(msgs) > 0 {
			if m, ok := msgs[0].(map[string]any); ok {
				if content, ok := m["content"].(string); ok {
					select {
					case systemPromptReceived <- content:
					default:
					}
				}
			}
		}
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"content": "answer"}},
			},
		})
	}))
	defer llmSrv.Close()

	webhookSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer webhookSrv.Close()

	dir := t.TempDir()
	st, err := store.New(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	llmCli := llmclient.New(llmSrv.URL, "test-key", "test-model", 100)
	srv := NewServer(Config{
		Store:            st,
		Version:          "test",
		LLMClient:        llmCli,
		DoorayWebhookURL: webhookSrv.URL,
		ChannelMap: map[string]DoorayChannel{
			"ch-mapped": {ProjectID: "proj-xyz"},
		},
	})

	t.Setenv("C5_DOORAY_CMD_TOKEN", "")
	payload := map[string]any{
		"tenantId":    "t1",
		"channelId":   "ch-mapped",
		"channelName": "mapped-channel",
		"userId":      "u1",
		"command":     "/cq",
		"text":        "질문",
		"responseUrl": "",
		"appToken":    "",
		"cmdToken":    "",
	}
	var buf bytes.Buffer
	json.NewEncoder(&buf).Encode(payload)
	req := httptest.NewRequest(http.MethodPost, "/v1/webhooks/dooray", &buf)
	req.Header.Set("Content-Type", "application/json")
	rw := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rw, req)

	if rw.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rw.Code)
	}

	select {
	case sysPrompt := <-systemPromptReceived:
		if !strings.Contains(sysPrompt, "proj-xyz") {
			t.Errorf("system prompt should contain projectID, got: %q", sysPrompt)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for LLM call")
	}
}

func TestDooray_ServerSide_Fallback(t *testing.T) {
	// When llmClient is nil, handler must fall back to Hub Job creation.
	t.Setenv("C5_DOORAY_CMD_TOKEN", "")

	srv := newTestServer(t) // no LLM client
	payload := doorayPayload("fallback task", "/cq", "https://example.com/resp", "", "")
	w := doRequest(t, srv, http.MethodPost, "/v1/webhooks/dooray", payload)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", w.Code, w.Body.String())
	}

	wJobs := doRequest(t, srv, http.MethodGet, "/v1/jobs", nil)
	if wJobs.Code != http.StatusOK {
		t.Fatalf("GET /v1/jobs: got %d", wJobs.Code)
	}
	var jobs []map[string]any
	if err := json.NewDecoder(wJobs.Body).Decode(&jobs); err != nil {
		t.Fatalf("decode jobs: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("expected 1 fallback job, got %d", len(jobs))
	}
	tags, _ := jobs[0]["tags"].([]any)
	foundDooray := false
	for _, tag := range tags {
		if tag == "dooray" {
			foundDooray = true
		}
	}
	if !foundDooray {
		t.Errorf("fallback job missing 'dooray' tag: %v", tags)
	}
}

func TestDooray_SanitizeText(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "clean text unchanged",
			input: "Hello, world!",
			want:  "Hello, world!",
		},
		{
			name:  "zero-width space removed",
			input: "Hello\u200bWorld",
			want:  "HelloWorld",
		},
		{
			name:  "BOM removed",
			input: "\ufeffHello",
			want:  "Hello",
		},
		{
			name:  "Korean text preserved",
			input: "안녕하세요! 질문이 있습니다.",
			want:  "안녕하세요! 질문이 있습니다.",
		},
		{
			name:  "multiple bad chars",
			input: "a\u200bb\ufeffc\u200dd",
			want:  "abcd",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeDoorayText(tt.input)
			if got != tt.want {
				t.Errorf("sanitizeDoorayText(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestDooray_ChannelRouting(t *testing.T) {
	dir := t.TempDir()
	st, err := store.New(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	srv := NewServer(Config{
		Store:            st,
		Version:          "test",
		DoorayWebhookURL: "https://default.webhook/",
		ChannelMap: map[string]DoorayChannel{
			"ch-custom": {ProjectID: "proj-1", WebhookURL: "https://custom.webhook/ch"},
			"ch-proj":   {ProjectID: "proj-2"},
		},
	})

	// Per-channel webhook takes precedence over default.
	if url := srv.resolveWebhookURL("ch-custom"); url != "https://custom.webhook/ch" {
		t.Errorf("ch-custom webhook: got %q, want custom URL", url)
	}
	// No per-channel webhook → falls back to default.
	if url := srv.resolveWebhookURL("ch-proj"); url != "https://default.webhook/" {
		t.Errorf("ch-proj webhook: got %q, want default URL", url)
	}
	// Unknown channel → falls back to default.
	if url := srv.resolveWebhookURL("ch-unknown"); url != "https://default.webhook/" {
		t.Errorf("ch-unknown webhook: got %q, want default URL", url)
	}
	// Channel projectID mapping is correct.
	if ch, ok := srv.channelMap["ch-proj"]; !ok || ch.ProjectID != "proj-2" {
		t.Errorf("ch-proj projectID: got %q, want proj-2", srv.channelMap["ch-proj"].ProjectID)
	}
}
