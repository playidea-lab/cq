package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/piqsol/c4/c5/internal/store"
)

// newTestServerWithSupabase creates a test server wired to a mock Supabase endpoint.
func newTestServerWithSupabase(t *testing.T, supabaseURL string) *Server {
	t.Helper()
	dir := t.TempDir()
	st, err := store.New(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return NewServer(Config{
		Store:       st,
		Version:     "test",
		SupabaseURL: supabaseURL,
	})
}

// TestAuthCallback tests GET /auth/callback
func TestAuthCallback(t *testing.T) {
	srv := newTestServer(t)

	t.Run("missing_params", func(t *testing.T) {
		w := doRequest(t, srv, "GET", "/auth/callback", nil)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("want 400, got %d", w.Code)
		}
		if !strings.Contains(w.Body.String(), "누락") {
			t.Fatalf("want missing-param message, got %q", w.Body.String())
		}
	})

	t.Run("unknown_state", func(t *testing.T) {
		w := doRequest(t, srv, "GET", "/auth/callback?state=unknown&code=abc", nil)
		if w.Code != http.StatusNotFound {
			t.Fatalf("want 404, got %d", w.Code)
		}
	})

	t.Run("valid_callback", func(t *testing.T) {
		// Create a device session first
		err := srv.store.CreateDeviceSession("state123", "TESTCODE", "challenge123", "https://test.supabase.co", time.Now().Add(deviceSessionTTL))
		if err != nil {
			t.Fatalf("create session: %v", err)
		}

		w := doRequest(t, srv, "GET", "/auth/callback?state=state123&code=myauthcode", nil)
		if w.Code != http.StatusOK {
			t.Fatalf("want 200, got %d; body: %s", w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), "인증 완료") {
			t.Fatalf("want success HTML, got %q", w.Body.String())
		}

		// Verify session is ready
		ds, err := srv.store.GetDeviceSession("state123")
		if err != nil {
			t.Fatalf("get session: %v", err)
		}
		if ds.Status != "ready" {
			t.Fatalf("want status=ready, got %q", ds.Status)
		}
	})

	t.Run("idempotent_callback", func(t *testing.T) {
		err := srv.store.CreateDeviceSession("state-idem", "IDEMCODE", "challenge456", "https://test.supabase.co", time.Now().Add(deviceSessionTTL))
		if err != nil {
			t.Fatalf("create session: %v", err)
		}
		// First call
		w := doRequest(t, srv, "GET", "/auth/callback?state=state-idem&code=code1", nil)
		if w.Code != http.StatusOK {
			t.Fatalf("first call: want 200, got %d", w.Code)
		}
		// Second call (already ready) — should return 200 (idempotent)
		w = doRequest(t, srv, "GET", "/auth/callback?state=state-idem&code=code1", nil)
		if w.Code != http.StatusOK {
			t.Fatalf("second call: want 200, got %d; body: %s", w.Code, w.Body.String())
		}
	})

	t.Run("wrong_method", func(t *testing.T) {
		w := doRequest(t, srv, "POST", "/auth/callback?state=x&code=y", nil)
		if w.Code != http.StatusMethodNotAllowed {
			t.Fatalf("want 405, got %d", w.Code)
		}
	})
}

// TestDeviceToken tests POST /v1/auth/device/{state}/token
func TestDeviceToken(t *testing.T) {
	// Mock Supabase server
	mockSupabase := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/auth/v1/token" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["auth_code"] == "" || body["code_verifier"] == "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "missing fields"})
			return
		}
		// Return a mock session
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "tok_abc",
			"refresh_token": "ref_xyz",
			"expires_at":    9999999999,
			"user": map[string]any{
				"id":    "user-1",
				"email": "user@example.com",
			},
		})
	}))
	defer mockSupabase.Close()

	srv := newTestServerWithSupabase(t, mockSupabase.URL)

	t.Run("session_not_found", func(t *testing.T) {
		body := `{"code_verifier":"verifier123"}`
		req := httptest.NewRequest("POST", "/v1/auth/device/nostate/token", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Fatalf("want 404, got %d; body: %s", w.Code, w.Body.String())
		}
	})

	t.Run("not_ready", func(t *testing.T) {
		err := srv.store.CreateDeviceSession("pending-state", "PENDCODE", "challenge789", "https://test.supabase.co", time.Now().Add(deviceSessionTTL))
		if err != nil {
			t.Fatalf("create session: %v", err)
		}
		body := `{"code_verifier":"verifier123"}`
		req := httptest.NewRequest("POST", "/v1/auth/device/pending-state/token", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("want 400, got %d; body: %s", w.Code, w.Body.String())
		}
		var resp map[string]string
		json.NewDecoder(w.Body).Decode(&resp)
		if resp["error"] != "not ready" {
			t.Fatalf("want 'not ready', got %q", resp["error"])
		}
	})

	t.Run("successful_exchange", func(t *testing.T) {
		// Create session and set it to ready
		err := srv.store.CreateDeviceSession("ready-state", "READCODE", "challengeABC", "https://test.supabase.co", time.Now().Add(deviceSessionTTL))
		if err != nil {
			t.Fatalf("create session: %v", err)
		}
		if err := srv.store.SetDeviceSessionAuthCode("ready-state", "authcode_123"); err != nil {
			t.Fatalf("set auth code: %v", err)
		}

		body := `{"code_verifier":"verifier_abc"}`
		req := httptest.NewRequest("POST", "/v1/auth/device/ready-state/token", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("want 200, got %d; body: %s", w.Code, w.Body.String())
		}

		var resp map[string]any
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if resp["access_token"] != "tok_abc" {
			t.Fatalf("want access_token=tok_abc, got %v", resp["access_token"])
		}
		// auth_code must not be in response
		if _, ok := resp["auth_code"]; ok {
			t.Fatal("auth_code must not be in response")
		}
	})

	t.Run("rate_limit_token_attempts", func(t *testing.T) {
		// Uses dedicated token_attempts counter (not shared with job poll_count)
		err := srv.store.CreateDeviceSession("rate-limited-state", "RATELCODE", "challengeDEF", "https://test.supabase.co", time.Now().Add(deviceSessionTTL))
		if err != nil {
			t.Fatalf("create session: %v", err)
		}

		body := `{"code_verifier":"verifier"}`
		for i := 0; i < tokenAttemptLimit; i++ {
			req := httptest.NewRequest("POST", "/v1/auth/device/rate-limited-state/token", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			srv.Handler().ServeHTTP(w, req)
			// All should return 400 (not ready) or hit the limit
		}

		// After limit exhausted, a further attempt returns 404 (session expired/not found).
		req := httptest.NewRequest("POST", "/v1/auth/device/rate-limited-state/token", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)
		if w.Code != http.StatusNotFound && w.Code != http.StatusBadRequest {
			t.Fatalf("want 404 or 400 after rate limit exhausted, got %d; body: %s", w.Code, w.Body.String())
		}
	})

	t.Run("missing_code_verifier", func(t *testing.T) {
		err := srv.store.CreateDeviceSession("ready-state-2", "READCODE2", "challengeGHI", "https://test.supabase.co", time.Now().Add(deviceSessionTTL))
		if err != nil {
			t.Fatalf("create session: %v", err)
		}
		if err := srv.store.SetDeviceSessionAuthCode("ready-state-2", "authcode_456"); err != nil {
			t.Fatalf("set auth code: %v", err)
		}

		body := `{"code_verifier":""}`
		req := httptest.NewRequest("POST", "/v1/auth/device/ready-state-2/token", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("want 400, got %d; body: %s", w.Code, w.Body.String())
		}
	})
}
