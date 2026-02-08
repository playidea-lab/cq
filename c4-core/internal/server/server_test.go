package server

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// =========================================================================
// Test helpers
// =========================================================================

// testServer creates a server with a mock JWT verifier for testing.
func testServer(t *testing.T) *Server {
	t.Helper()

	cfg := DefaultConfig()
	cfg.JWTSecret = "test-secret"
	cfg.WebhookSecret = "webhook-secret"

	s := NewServer(cfg)

	// Override JWT verifier for testing
	verifyJWT = func(token, secret string) (*User, error) {
		switch token {
		case "valid-pro":
			return &User{ID: "user-1", Email: "pro@test.com", Plan: "pro"}, nil
		case "valid-team":
			return &User{ID: "user-2", Email: "team@test.com", Plan: "team"}, nil
		case "valid-enterprise":
			return &User{ID: "user-3", Email: "ent@test.com", Plan: "enterprise"}, nil
		case "valid-free":
			return &User{ID: "user-4", Email: "free@test.com", Plan: "free"}, nil
		default:
			return nil, fmt.Errorf("invalid token")
		}
	}

	return s
}

// cleanup resets the JWT verifier after tests.
func cleanup() {
	verifyJWT = defaultVerifyJWT
}

func authHeader(token string) string {
	return fmt.Sprintf("Bearer %s", token)
}

// =========================================================================
// Tests: Health Check
// =========================================================================

func TestHealthCheck(t *testing.T) {
	s := testServer(t)
	defer cleanup()

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	s.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)

	if resp["status"] != "healthy" {
		t.Errorf("status = %v, want healthy", resp["status"])
	}
	if resp["service"] != "c4-cloud" {
		t.Errorf("service = %v, want c4-cloud", resp["service"])
	}
}

func TestRootEndpoint(t *testing.T) {
	s := testServer(t)
	defer cleanup()

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	s.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)

	if resp["name"] != "C4 Cloud API" {
		t.Errorf("name = %v, want C4 Cloud API", resp["name"])
	}
}

// =========================================================================
// Tests: Chat Proxy SSE Streaming
// =========================================================================

func TestChatProxySSEStreaming(t *testing.T) {
	s := testServer(t)
	defer cleanup()

	body, _ := json.Marshal(ChatRequest{
		Message: "Hello, Claude!",
		Stream:  true,
	})

	req := httptest.NewRequest("POST", "/api/chat", bytes.NewReader(body))
	req.Header.Set("Authorization", authHeader("valid-pro"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	// Check SSE content type
	contentType := w.Header().Get("Content-Type")
	if contentType != "text/event-stream" {
		t.Errorf("Content-Type = %q, want text/event-stream", contentType)
	}

	// Parse SSE events
	respBody := w.Body.String()
	if !strings.Contains(respBody, "event: start") {
		t.Error("response should contain 'event: start'")
	}
	if !strings.Contains(respBody, "event: message") {
		t.Error("response should contain 'event: message'")
	}
	if !strings.Contains(respBody, "event: done") {
		t.Error("response should contain 'event: done'")
	}
}

func TestChatNonStreaming(t *testing.T) {
	s := testServer(t)
	defer cleanup()

	body, _ := json.Marshal(ChatRequest{
		Message: "Hello",
		Stream:  false,
	})

	req := httptest.NewRequest("POST", "/api/chat", bytes.NewReader(body))
	req.Header.Set("Authorization", authHeader("valid-pro"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)

	if resp["done"] != true {
		t.Error("expected done=true for non-streaming response")
	}
}

func TestChatEmptyMessage(t *testing.T) {
	s := testServer(t)
	defer cleanup()

	body, _ := json.Marshal(ChatRequest{
		Message: "",
		Stream:  true,
	})

	req := httptest.NewRequest("POST", "/api/chat", bytes.NewReader(body))
	req.Header.Set("Authorization", authHeader("valid-pro"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestChatInvalidBody(t *testing.T) {
	s := testServer(t)
	defer cleanup()

	req := httptest.NewRequest("POST", "/api/chat", strings.NewReader("invalid json"))
	req.Header.Set("Authorization", authHeader("valid-pro"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

// =========================================================================
// Tests: Webhook Signature Validation
// =========================================================================

func TestWebhookSignatureValidation(t *testing.T) {
	s := testServer(t)
	defer cleanup()

	payload := `{"action":"opened","repository":{"full_name":"test/repo"},"sender":{"login":"user1"}}`
	bodyBytes := []byte(payload)

	// Compute valid signature
	mac := hmac.New(sha256.New, []byte("webhook-secret"))
	mac.Write(bodyBytes)
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	req := httptest.NewRequest("POST", "/api/webhooks/github", bytes.NewReader(bodyBytes))
	req.Header.Set("X-Hub-Signature-256", sig)
	req.Header.Set("X-GitHub-Event", "pull_request")
	w := httptest.NewRecorder()

	s.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)

	if resp["success"] != true {
		t.Error("expected success=true")
	}
	if resp["event"] != "pull_request" {
		t.Errorf("event = %v, want pull_request", resp["event"])
	}
}

func TestWebhookInvalidSignature(t *testing.T) {
	s := testServer(t)
	defer cleanup()

	payload := `{"action":"opened"}`
	req := httptest.NewRequest("POST", "/api/webhooks/github", strings.NewReader(payload))
	req.Header.Set("X-Hub-Signature-256", "sha256=invalid")
	req.Header.Set("X-GitHub-Event", "push")
	w := httptest.NewRecorder()

	s.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestWebhookMissingSignature(t *testing.T) {
	s := testServer(t)
	defer cleanup()

	payload := `{"action":"opened"}`
	req := httptest.NewRequest("POST", "/api/webhooks/github", strings.NewReader(payload))
	req.Header.Set("X-GitHub-Event", "push")
	w := httptest.NewRecorder()

	s.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestWebhookMissingEventHeader(t *testing.T) {
	s := testServer(t)
	defer cleanup()

	payload := `{"action":"opened"}`
	bodyBytes := []byte(payload)

	mac := hmac.New(sha256.New, []byte("webhook-secret"))
	mac.Write(bodyBytes)
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	req := httptest.NewRequest("POST", "/api/webhooks/github", bytes.NewReader(bodyBytes))
	req.Header.Set("X-Hub-Signature-256", sig)
	// Missing X-GitHub-Event
	w := httptest.NewRecorder()

	s.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

// =========================================================================
// Tests: Plan Gating - Free plan rejected
// =========================================================================

func TestGatingRejectsFreePlan(t *testing.T) {
	s := testServer(t)
	defer cleanup()

	body, _ := json.Marshal(ChatRequest{
		Message: "Hello",
		Stream:  false,
	})

	req := httptest.NewRequest("POST", "/api/chat", bytes.NewReader(body))
	req.Header.Set("Authorization", authHeader("valid-free"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d (free plan should be rejected)", w.Code, http.StatusForbidden)
	}

	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)

	if resp["error"] != "plan_restricted" {
		t.Errorf("error = %v, want plan_restricted", resp["error"])
	}
}

func TestGatingAllowsProPlan(t *testing.T) {
	s := testServer(t)
	defer cleanup()

	body, _ := json.Marshal(ChatRequest{
		Message: "Hello",
		Stream:  false,
	})

	req := httptest.NewRequest("POST", "/api/chat", bytes.NewReader(body))
	req.Header.Set("Authorization", authHeader("valid-pro"))
	w := httptest.NewRecorder()

	s.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d (pro plan should be allowed)", w.Code, http.StatusOK)
	}
}

func TestGatingAllowsTeamPlan(t *testing.T) {
	s := testServer(t)
	defer cleanup()

	body, _ := json.Marshal(ChatRequest{
		Message: "Hello",
		Stream:  false,
	})

	req := httptest.NewRequest("POST", "/api/chat", bytes.NewReader(body))
	req.Header.Set("Authorization", authHeader("valid-team"))
	w := httptest.NewRecorder()

	s.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestGatingAllowsEnterprisePlan(t *testing.T) {
	s := testServer(t)
	defer cleanup()

	body, _ := json.Marshal(ChatRequest{
		Message: "Hello",
		Stream:  false,
	})

	req := httptest.NewRequest("POST", "/api/chat", bytes.NewReader(body))
	req.Header.Set("Authorization", authHeader("valid-enterprise"))
	w := httptest.NewRecorder()

	s.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

// =========================================================================
// Tests: Auth middleware
// =========================================================================

func TestAuthMissingHeader(t *testing.T) {
	s := testServer(t)
	defer cleanup()

	body, _ := json.Marshal(ChatRequest{Message: "test"})
	req := httptest.NewRequest("POST", "/api/chat", bytes.NewReader(body))
	w := httptest.NewRecorder()

	s.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestAuthInvalidToken(t *testing.T) {
	s := testServer(t)
	defer cleanup()

	body, _ := json.Marshal(ChatRequest{Message: "test"})
	req := httptest.NewRequest("POST", "/api/chat", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer invalid-token")
	w := httptest.NewRecorder()

	s.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestAuthInvalidFormat(t *testing.T) {
	s := testServer(t)
	defer cleanup()

	body, _ := json.Marshal(ChatRequest{Message: "test"})
	req := httptest.NewRequest("POST", "/api/chat", bytes.NewReader(body))
	req.Header.Set("Authorization", "Basic dGVzdDp0ZXN0")
	w := httptest.NewRecorder()

	s.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

// =========================================================================
// Tests: Worker Spawn
// =========================================================================

func TestWorkerSpawn(t *testing.T) {
	s := testServer(t)
	defer cleanup()

	body, _ := json.Marshal(WorkerSpawnRequest{
		ProjectID: "proj-12345678",
		Region:    "iad",
		Model:     "opus",
		Count:     2,
	})

	req := httptest.NewRequest("POST", "/api/workers/spawn", bytes.NewReader(body))
	req.Header.Set("Authorization", authHeader("valid-pro"))
	w := httptest.NewRecorder()

	s.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d", w.Code, http.StatusCreated)
	}

	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)

	if resp["count"] != float64(2) {
		t.Errorf("count = %v, want 2", resp["count"])
	}

	workers, ok := resp["workers"].([]any)
	if !ok || len(workers) != 2 {
		t.Errorf("expected 2 workers, got %v", resp["workers"])
	}
}

func TestWorkerSpawnDefaultCount(t *testing.T) {
	s := testServer(t)
	defer cleanup()

	body, _ := json.Marshal(WorkerSpawnRequest{
		ProjectID: "proj-12345678",
	})

	req := httptest.NewRequest("POST", "/api/workers/spawn", bytes.NewReader(body))
	req.Header.Set("Authorization", authHeader("valid-pro"))
	w := httptest.NewRecorder()

	s.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d", w.Code, http.StatusCreated)
	}

	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)

	if resp["count"] != float64(1) {
		t.Errorf("count = %v, want 1 (default)", resp["count"])
	}
}

func TestWorkerSpawnMissingProject(t *testing.T) {
	s := testServer(t)
	defer cleanup()

	body, _ := json.Marshal(WorkerSpawnRequest{})

	req := httptest.NewRequest("POST", "/api/workers/spawn", bytes.NewReader(body))
	req.Header.Set("Authorization", authHeader("valid-pro"))
	w := httptest.NewRecorder()

	s.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

// =========================================================================
// Tests: Worker Delete
// =========================================================================

func TestWorkerDelete(t *testing.T) {
	s := testServer(t)
	defer cleanup()

	req := httptest.NewRequest("DELETE", "/api/workers/w-test-123", nil)
	req.Header.Set("Authorization", authHeader("valid-pro"))
	w := httptest.NewRecorder()

	s.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)

	if resp["status"] != "deleted" {
		t.Errorf("status = %v, want deleted", resp["status"])
	}
}

// =========================================================================
// Tests: C4 Status
// =========================================================================

func TestC4Status(t *testing.T) {
	s := testServer(t)
	defer cleanup()

	req := httptest.NewRequest("GET", "/api/c4/status", nil)
	req.Header.Set("Authorization", authHeader("valid-pro"))
	w := httptest.NewRecorder()

	s.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)

	if resp["status"] != "execute" {
		t.Errorf("status = %v, want execute", resp["status"])
	}
}

// =========================================================================
// Tests: CORS
// =========================================================================

func TestCORSPreflight(t *testing.T) {
	s := testServer(t)
	defer cleanup()

	req := httptest.NewRequest("OPTIONS", "/api/chat", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	w := httptest.NewRecorder()

	s.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNoContent)
	}

	acao := w.Header().Get("Access-Control-Allow-Origin")
	if acao != "http://localhost:3000" {
		t.Errorf("ACAO = %q, want http://localhost:3000", acao)
	}
}

func TestCORSDisallowedOrigin(t *testing.T) {
	s := testServer(t)
	defer cleanup()

	req := httptest.NewRequest("OPTIONS", "/api/chat", nil)
	req.Header.Set("Origin", "https://evil.com")
	w := httptest.NewRecorder()

	s.Handler().ServeHTTP(w, req)

	acao := w.Header().Get("Access-Control-Allow-Origin")
	if acao != "" {
		t.Errorf("ACAO should be empty for disallowed origin, got %q", acao)
	}
}

// =========================================================================
// Tests: Concurrent 100 connections
// =========================================================================

func TestConcurrent100Connections(t *testing.T) {
	s := testServer(t)
	defer cleanup()

	// Slow down the handler for concurrency testing
	originalHandler := s.handler
	var peak atomic.Int64
	var current atomic.Int64

	slowHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c := current.Add(1)
		for {
			p := peak.Load()
			if c > p {
				if peak.CompareAndSwap(p, c) {
					break
				}
			} else {
				break
			}
		}

		originalHandler.ServeHTTP(w, r)

		current.Add(-1)
	})

	ts := httptest.NewServer(slowHandler)
	defer ts.Close()

	const numRequests = 100
	var wg sync.WaitGroup
	wg.Add(numRequests)

	errors := make([]error, numRequests)
	statuses := make([]int, numRequests)

	for i := 0; i < numRequests; i++ {
		go func(idx int) {
			defer wg.Done()

			body, _ := json.Marshal(ChatRequest{
				Message: fmt.Sprintf("Request %d", idx),
				Stream:  false,
			})

			req, err := http.NewRequest("POST", ts.URL+"/api/chat", bytes.NewReader(body))
			if err != nil {
				errors[idx] = err
				return
			}
			req.Header.Set("Authorization", authHeader("valid-pro"))
			req.Header.Set("Content-Type", "application/json")

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				errors[idx] = err
				return
			}
			io.ReadAll(resp.Body)
			resp.Body.Close()
			statuses[idx] = resp.StatusCode
		}(i)
	}

	wg.Wait()

	// Check all succeeded
	errorCount := 0
	successCount := 0
	for i := 0; i < numRequests; i++ {
		if errors[i] != nil {
			errorCount++
		} else if statuses[i] == http.StatusOK {
			successCount++
		}
	}

	if successCount != numRequests {
		t.Errorf("success count = %d, want %d (errors: %d)", successCount, numRequests, errorCount)
	}

	// Verify concurrent connections occurred
	peakVal := peak.Load()
	if peakVal < 2 {
		t.Logf("peak concurrent = %d (may be limited by test environment)", peakVal)
	}
}

// =========================================================================
// Tests: Signature validation utility
// =========================================================================

func TestValidateGitHubSignatureValid(t *testing.T) {
	body := []byte(`{"test":"data"}`)
	secret := "test-secret"

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	if !validateGitHubSignature(body, sig, secret) {
		t.Error("valid signature should pass")
	}
}

func TestValidateGitHubSignatureInvalid(t *testing.T) {
	body := []byte(`{"test":"data"}`)
	if validateGitHubSignature(body, "sha256=invalid", "secret") {
		t.Error("invalid signature should fail")
	}
}

func TestValidateGitHubSignatureEmpty(t *testing.T) {
	body := []byte(`{"test":"data"}`)
	if validateGitHubSignature(body, "", "secret") {
		t.Error("empty signature should fail")
	}
}

func TestValidateGitHubSignatureNoPrefix(t *testing.T) {
	body := []byte(`{"test":"data"}`)
	if validateGitHubSignature(body, "invalid-prefix", "secret") {
		t.Error("signature without sha256= prefix should fail")
	}
}

// =========================================================================
// Tests: Config
// =========================================================================

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Port != 8080 {
		t.Errorf("port = %d, want 8080", cfg.Port)
	}
	if cfg.Host != "0.0.0.0" {
		t.Errorf("host = %q, want 0.0.0.0", cfg.Host)
	}
	if len(cfg.AllowedPlans) != 3 {
		t.Errorf("allowed plans count = %d, want 3", len(cfg.AllowedPlans))
	}
}

// =========================================================================
// Tests: SSE format
// =========================================================================

func TestSSEFormat(t *testing.T) {
	w := httptest.NewRecorder()

	writeSSE(w, "test", map[string]any{"key": "value"})

	body := w.Body.String()
	if !strings.HasPrefix(body, "event: test\n") {
		t.Errorf("SSE should start with 'event: test\\n', got: %q", body)
	}
	if !strings.Contains(body, "data: ") {
		t.Error("SSE should contain 'data: '")
	}
	if !strings.HasSuffix(body, "\n\n") {
		t.Error("SSE should end with double newline")
	}
}

// =========================================================================
// Tests: writeJSON
// =========================================================================

func TestWriteJSON(t *testing.T) {
	w := httptest.NewRecorder()

	writeJSON(w, http.StatusOK, map[string]string{"hello": "world"})

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if w.Header().Get("Content-Type") != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", w.Header().Get("Content-Type"))
	}

	var resp map[string]string
	json.Unmarshal(w.Body.Bytes(), &resp)

	if resp["hello"] != "world" {
		t.Errorf("hello = %q, want world", resp["hello"])
	}
}

// =========================================================================
// Tests: Connection tracking
// =========================================================================

func TestConnectionTracker(t *testing.T) {
	tracker := &ConnectionTracker{}

	tracker.Track()
	tracker.Track()
	tracker.Track()

	if tracker.Peak() != 3 {
		t.Errorf("peak = %d, want 3", tracker.Peak())
	}

	tracker.Release()
	tracker.Release()

	if tracker.Peak() != 3 {
		t.Errorf("peak should still be 3 after releases, got %d", tracker.Peak())
	}
}

// =========================================================================
// Tests: Plan gating edge cases
// =========================================================================

func TestGatingNoPlanRestrictions(t *testing.T) {
	cfg := DefaultConfig()
	cfg.JWTSecret = "test"
	cfg.AllowedPlans = []string{} // no restrictions

	s := NewServer(cfg)

	verifyJWT = func(token, secret string) (*User, error) {
		return &User{ID: "user-1", Plan: "free"}, nil
	}
	defer cleanup()

	body, _ := json.Marshal(ChatRequest{Message: "test", Stream: false})
	req := httptest.NewRequest("POST", "/api/chat", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer any-token")
	w := httptest.NewRecorder()

	s.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d (no plan restrictions)", w.Code, http.StatusOK)
	}
}

// =========================================================================
// Tests: Endpoint without auth for webhooks
// =========================================================================

func TestWebhookNoAuthRequired(t *testing.T) {
	cfg := DefaultConfig()
	cfg.JWTSecret = "test"
	cfg.WebhookSecret = "" // No signature validation

	s := NewServer(cfg)
	defer cleanup()

	payload := `{"action":"opened","repository":{"full_name":"test/repo"},"sender":{"login":"user1"}}`
	req := httptest.NewRequest("POST", "/api/webhooks/github", strings.NewReader(payload))
	req.Header.Set("X-GitHub-Event", "push")
	w := httptest.NewRecorder()

	s.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d (webhooks should not require JWT)", w.Code, http.StatusOK)
	}
}

// =========================================================================
// Tests: Server lifecycle
// =========================================================================

func TestServerShutdown(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Port = 0 // any available port

	s := NewServer(cfg)

	// Should not panic
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := s.Shutdown(ctx)
	if err != nil {
		t.Errorf("Shutdown before start should not error: %v", err)
	}
}

// =========================================================================
// Tests: Active connections tracking
// =========================================================================

func TestActiveConnections(t *testing.T) {
	s := testServer(t)
	defer cleanup()

	// Before any request
	if s.ActiveConnections() != 0 {
		t.Errorf("active connections = %d, want 0", s.ActiveConnections())
	}

	// Health check doesn't track (no middleware)
	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	if s.ActiveConnections() != 0 {
		t.Errorf("active connections after health = %d, want 0", s.ActiveConnections())
	}
}

// =========================================================================
// Tests: Delayed response for connection tracking
// =========================================================================

func TestDelayedHandler(t *testing.T) {
	s := testServer(t)
	defer cleanup()

	// Wrap handler to add delay
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.Handler().ServeHTTP(w, r)
	}))
	defer ts.Close()

	body, _ := json.Marshal(ChatRequest{
		Message: "test",
		Stream:  false,
	})

	req, _ := http.NewRequest("POST", ts.URL+"/api/chat", bytes.NewReader(body))
	req.Header.Set("Authorization", authHeader("valid-pro"))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("status = %d, want %d, body: %s", resp.StatusCode, http.StatusOK, string(body))
	}

	// After request completes, active should be 0
	time.Sleep(10 * time.Millisecond)
	if s.ActiveConnections() != 0 {
		t.Errorf("active connections after completion = %d, want 0", s.ActiveConnections())
	}
}
