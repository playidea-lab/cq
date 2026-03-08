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
	"github.com/piqsol/c4/c5/internal/model"
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

func TestExtractAction_QueryStatus(t *testing.T) {
	action, ok := extractAction(`{"action":"query_status"}`)
	if !ok {
		t.Fatal("extractAction: expected ok=true for query_status")
	}
	if action.Action != "query_status" {
		t.Errorf("action.Action: got %q, want %q", action.Action, "query_status")
	}
}

func TestDooray_ServerSide_QueryStatus(t *testing.T) {
	// LLM returns query_status action; server fetches workers + jobs and responds.
	llmSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"content": `{"action":"query_status"}`}},
			},
		})
	}))
	defer llmSrv.Close()

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
	payload := doorayPayload("현재 실험상황 및 워커 상태 체크", "/cq", "", "", "")
	w := doRequest(t, srv, http.MethodPost, "/v1/webhooks/dooray", payload)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", w.Code, w.Body.String())
	}

	select {
	case text := <-done:
		// Empty store → "워커: 없음" and "대기/실행 중인 잡 없음"
		if !strings.Contains(text, "워커") {
			t.Errorf("query_status text: got %q, expected worker section", text)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for webhook response")
	}
}

func TestExtractAction_QueryWorkers(t *testing.T) {
	action, ok := extractAction(`{"action":"query_workers"}`)
	if !ok {
		t.Fatal("extractAction: expected ok=true for query_workers")
	}
	if action.Action != "query_workers" {
		t.Errorf("action.Action: got %q, want %q", action.Action, "query_workers")
	}
}

func TestExtractAction_QueryJobs(t *testing.T) {
	action, ok := extractAction(`{"action":"query_jobs","limit":5,"status":"FAILED"}`)
	if !ok {
		t.Fatal("extractAction: expected ok=true for query_jobs")
	}
	if action.Action != "query_jobs" {
		t.Errorf("action.Action: got %q, want %q", action.Action, "query_jobs")
	}
	if action.Limit != 5 {
		t.Errorf("action.Limit: got %d, want 5", action.Limit)
	}
	if action.Status != "FAILED" {
		t.Errorf("action.Status: got %q, want FAILED", action.Status)
	}
}

func TestExtractAction_SubmitJob_RequiresCommand(t *testing.T) {
	// submit_job without command must fail.
	if _, ok := extractAction(`{"action":"submit_job","name":"test"}`); ok {
		t.Error("submit_job without command should return ok=false")
	}
	// submit_job with command must succeed.
	action, ok := extractAction(`{"action":"submit_job","name":"test","command":"echo hi"}`)
	if !ok {
		t.Fatal("submit_job with command: expected ok=true")
	}
	if action.Command != "echo hi" {
		t.Errorf("action.Command: got %q, want %q", action.Command, "echo hi")
	}
}

func TestExtractAction_EmptyAction(t *testing.T) {
	if _, ok := extractAction(`{"action":""}`); ok {
		t.Error("empty action should return ok=false")
	}
}

func TestDooray_ServerSide_QueryWorkers(t *testing.T) {
	// LLM returns query_workers action.
	llmSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"content": `{"action":"query_workers"}`}},
			},
		})
	}))
	defer llmSrv.Close()

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
	payload := doorayPayload("서버 상태 봐바", "/cq", "", "", "")
	w := doRequest(t, srv, http.MethodPost, "/v1/webhooks/dooray", payload)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", w.Code, w.Body.String())
	}

	select {
	case text := <-done:
		// No workers registered → "현재 등록된 워커가 없습니다"
		if !strings.Contains(text, "워커") {
			t.Errorf("webhook text: got %q, expected worker-related content", text)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for webhook response")
	}
}

func TestDooray_ServerSide_QueryJobs(t *testing.T) {
	// LLM returns query_jobs action.
	llmSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"content": `{"action":"query_jobs","limit":5}`}},
			},
		})
	}))
	defer llmSrv.Close()

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
	payload := doorayPayload("최근 실험 뭐 있어", "/cq", "", "", "")
	w := doRequest(t, srv, http.MethodPost, "/v1/webhooks/dooray", payload)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", w.Code, w.Body.String())
	}

	select {
	case text := <-done:
		// No jobs → "조건에 맞는 잡이 없습니다"
		if !strings.Contains(text, "잡") {
			t.Errorf("webhook text: got %q, expected job-related content", text)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for webhook response")
	}
}

func TestDooray_ServerSide_SubmitJob_HasDoorayChannel(t *testing.T) {
	// LLM returns submit_job; resulting job must have DOORAY_CHANNEL in env.
	llmSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"content": `{"action":"submit_job","name":"exp401","command":"echo hi","requires_gpu":false}`}},
			},
		})
	}))
	defer llmSrv.Close()

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
	payload := doorayPayload("exp401 실행", "/cq", "", "", "")
	w := doRequest(t, srv, http.MethodPost, "/v1/webhooks/dooray", payload)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", w.Code, w.Body.String())
	}

	select {
	case text := <-done:
		if !strings.Contains(text, "🚀") {
			t.Errorf("webhook text: got %q, expected job submitted message", text)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for webhook response")
	}

	// Verify job has DOORAY_CHANNEL in env.
	wJobs := doRequest(t, srv, http.MethodGet, "/v1/jobs", nil)
	var jobs []map[string]any
	json.NewDecoder(wJobs.Body).Decode(&jobs)
	if len(jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(jobs))
	}
	env, _ := jobs[0]["env"].(map[string]any)
	if env["DOORAY_CHANNEL"] != "ch-1" {
		t.Errorf("DOORAY_CHANNEL: got %v, want %q", env["DOORAY_CHANNEL"], "ch-1")
	}
}

func TestNotifyDoorayJobComplete_Succeeded(t *testing.T) {
	done := make(chan string, 1)
	webhookSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		done <- body["text"]
		w.WriteHeader(http.StatusOK)
	}))
	defer webhookSrv.Close()

	dir := t.TempDir()
	st, err := store.New(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	srv := NewServer(Config{
		Store:            st,
		Version:          "test",
		DoorayWebhookURL: webhookSrv.URL,
	})

	// Create a job with DOORAY_CHANNEL in env.
	job, err := st.CreateJob(&model.JobSubmitRequest{
		Name:    "test-exp",
		Command: "echo hi",
		Workdir: ".",
		Env:     map[string]string{"DOORAY_CHANNEL": "ch-1"},
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}

	completedJob, err := st.GetJob(job.ID)
	if err != nil {
		t.Fatalf("get job: %v", err)
	}
	srv.notifyDoorayJobComplete(completedJob, model.StatusSucceeded, 0)

	select {
	case text := <-done:
		if !strings.Contains(text, "✅") {
			t.Errorf("completion text: got %q, expected success marker", text)
		}
		if !strings.Contains(text, "test-exp") {
			t.Errorf("completion text: got %q, expected job name", text)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for Dooray completion notification")
	}
}

func TestNotifyDoorayJobComplete_Failed(t *testing.T) {
	done := make(chan string, 1)
	webhookSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		done <- body["text"]
		w.WriteHeader(http.StatusOK)
	}))
	defer webhookSrv.Close()

	dir := t.TempDir()
	st, err := store.New(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	srv := NewServer(Config{
		Store:            st,
		Version:          "test",
		DoorayWebhookURL: webhookSrv.URL,
	})

	job, err := st.CreateJob(&model.JobSubmitRequest{
		Name:    "fail-exp",
		Command: "exit 1",
		Workdir: ".",
		Env:     map[string]string{"DOORAY_CHANNEL": "ch-1"},
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}

	failedJob, err := st.GetJob(job.ID)
	if err != nil {
		t.Fatalf("get job: %v", err)
	}
	srv.notifyDoorayJobComplete(failedJob, model.StatusFailed, 1)

	select {
	case text := <-done:
		if !strings.Contains(text, "❌") {
			t.Errorf("failure text: got %q, expected failure marker", text)
		}
		if !strings.Contains(text, "fail-exp") {
			t.Errorf("failure text: got %q, expected job name", text)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for Dooray failure notification")
	}
}

func TestNotifyDoorayJobComplete_NoDoorayChannel(t *testing.T) {
	// Job without DOORAY_CHANNEL must not post to webhook.
	posted := make(chan struct{}, 1)
	webhookSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		posted <- struct{}{}
		w.WriteHeader(http.StatusOK)
	}))
	defer webhookSrv.Close()

	dir := t.TempDir()
	st, err := store.New(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	srv := NewServer(Config{
		Store:            st,
		Version:          "test",
		DoorayWebhookURL: webhookSrv.URL,
	})

	job, err := st.CreateJob(&model.JobSubmitRequest{
		Name:    "non-dooray",
		Command: "echo hi",
		Workdir: ".",
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}

	nonDoorayJob, err := st.GetJob(job.ID)
	if err != nil {
		t.Fatalf("get job: %v", err)
	}
	srv.notifyDoorayJobComplete(nonDoorayJob, model.StatusSucceeded, 0)

	// Give goroutine time to post (if it were to post).
	select {
	case <-posted:
		t.Error("webhook was called for job without DOORAY_CHANNEL")
	case <-time.After(200 * time.Millisecond):
		// Expected: no webhook call.
	}
}

func TestBuildDooraySystemPrompt_NoHardcodedPaths(t *testing.T) {
	prompt := buildDooraySystemPrompt("", "", "")
	if strings.Contains(prompt, "hmr_postproc") {
		t.Error("prompt should not contain hardcoded hmr_postproc path")
	}
	if strings.Contains(prompt, "exp401") {
		t.Error("prompt should not contain hardcoded experiment IDs")
	}
}

func TestBuildDooraySystemPrompt_WithKnowledge(t *testing.T) {
	prompt := buildDooraySystemPrompt("proj1", "some knowledge context", "")
	if !strings.Contains(prompt, "some knowledge context") {
		t.Error("prompt should include knowledge context")
	}
}

func TestBuildDooraySystemPrompt_WithCaps(t *testing.T) {
	prompt := buildDooraySystemPrompt("", "", "gpu.train: GPU training")
	if !strings.Contains(prompt, "gpu.train") {
		t.Error("prompt should include capability context")
	}
}

func TestBuildDooraySystemPrompt_NoProjectID(t *testing.T) {
	// Should not panic
	prompt := buildDooraySystemPrompt("", "", "")
	if prompt == "" {
		t.Error("prompt should not be empty")
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
