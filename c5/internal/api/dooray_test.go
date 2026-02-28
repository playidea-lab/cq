package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

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
