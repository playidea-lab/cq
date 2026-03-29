package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/piqsol/c4/c5/internal/store"
)

// newTestServerForAuth creates a server with a pre-populated device session.
func newTestServerForAuth(t *testing.T) (*Server, string) {
	t.Helper()
	dir := t.TempDir()
	st, err := store.New(filepath.Join(dir, "auth_test.db"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	srv := NewServer(Config{
		Store:   st,
		Version: "test",
	})

	state := "teststate123"
	err = st.CreateDeviceSession(state, "TESTCODE", "challenge123", "https://supabase.example.com", time.Now().Add(10*time.Minute))
	if err != nil {
		t.Fatalf("CreateDeviceSession: %v", err)
	}
	return srv, state
}

// =========================================================================
// TestAuthCallback — GET /auth/callback
// =========================================================================

func TestAuthCallback(t *testing.T) {
	t.Run("missing state and code returns 400", func(t *testing.T) {
		srv, _ := newTestServerForAuth(t)
		req := httptest.NewRequest(http.MethodGet, "/auth/callback", nil)
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("want 400, got %d", w.Code)
		}
		if !strings.Contains(w.Body.String(), "누락") {
			t.Fatalf("expected 누락 in body, got: %s", w.Body.String())
		}
	})

	t.Run("missing code returns 400", func(t *testing.T) {
		srv, _ := newTestServerForAuth(t)
		req := httptest.NewRequest(http.MethodGet, "/auth/callback?state=abc", nil)
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("want 400, got %d", w.Code)
		}
	})

	t.Run("missing state returns 400", func(t *testing.T) {
		srv, _ := newTestServerForAuth(t)
		req := httptest.NewRequest(http.MethodGet, "/auth/callback?code=mycode", nil)
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("want 400, got %d", w.Code)
		}
	})

	t.Run("invalid state returns 404", func(t *testing.T) {
		srv, _ := newTestServerForAuth(t)
		req := httptest.NewRequest(http.MethodGet, "/auth/callback?state=nonexistent&code=mycode", nil)
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Fatalf("want 404, got %d", w.Code)
		}
	})

	t.Run("valid state transitions to ready and shows completion page", func(t *testing.T) {
		srv, state := newTestServerForAuth(t)
		req := httptest.NewRequest(http.MethodGet, "/auth/callback?state="+state+"&code=myauthcode", nil)
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("want 200, got %d (body: %s)", w.Code, w.Body.String())
		}
		body := w.Body.String()
		if !strings.Contains(body, "인증 완료") {
			t.Fatalf("expected '인증 완료' in HTML body, got: %s", body)
		}
		if !strings.Contains(body, "window.close()") {
			t.Fatalf("expected window.close() in HTML body, got: %s", body)
		}
	})

	t.Run("idempotent: already ready returns 200", func(t *testing.T) {
		srv, state := newTestServerForAuth(t)

		// First call: set to ready.
		req1 := httptest.NewRequest(http.MethodGet, "/auth/callback?state="+state+"&code=myauthcode", nil)
		w1 := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w1, req1)
		if w1.Code != http.StatusOK {
			t.Fatalf("first call: want 200, got %d", w1.Code)
		}

		// Second call: already ready → 200 OK.
		req2 := httptest.NewRequest(http.MethodGet, "/auth/callback?state="+state+"&code=myauthcode", nil)
		w2 := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w2, req2)
		if w2.Code != http.StatusOK {
			t.Fatalf("second call (idempotent): want 200, got %d", w2.Code)
		}
	})

	t.Run("wrong method returns 405", func(t *testing.T) {
		srv, _ := newTestServerForAuth(t)
		req := httptest.NewRequest(http.MethodPost, "/auth/callback", nil)
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)

		if w.Code != http.StatusMethodNotAllowed {
			t.Fatalf("want 405, got %d", w.Code)
		}
	})
}

// =========================================================================
// TestDeviceToken — POST /v1/auth/device/{state}/token
// =========================================================================

func TestDeviceToken(t *testing.T) {
	t.Run("session not ready returns 400", func(t *testing.T) {
		srv, state := newTestServerForAuth(t)

		w := doDeviceTokenRequest(t, srv, state, "verifier123")
		if w.Code != http.StatusBadRequest {
			t.Fatalf("want 400, got %d (body: %s)", w.Code, w.Body.String())
		}
		var resp map[string]string
		json.NewDecoder(w.Body).Decode(&resp) //nolint:errcheck
		if resp["error"] != "not ready" {
			t.Fatalf("want 'not ready', got %q", resp["error"])
		}
	})

	t.Run("session not found returns 404", func(t *testing.T) {
		srv, _ := newTestServerForAuth(t)

		w := doDeviceTokenRequest(t, srv, "nonexistentstate", "verifier123")
		if w.Code != http.StatusNotFound {
			t.Fatalf("want 404, got %d (body: %s)", w.Code, w.Body.String())
		}
		var resp map[string]string
		json.NewDecoder(w.Body).Decode(&resp) //nolint:errcheck
		if resp["error"] != "session not found or expired" {
			t.Fatalf("want 'session not found or expired', got %q", resp["error"])
		}
	})

	t.Run("missing code_verifier returns 400", func(t *testing.T) {
		srv, state := newTestServerForAuth(t)
		// Mark as ready first.
		srv.store.SetDeviceSessionAuthCode(state, "authcode123") //nolint:errcheck

		req := httptest.NewRequest(http.MethodPost, "/v1/auth/device/"+state+"/token",
			strings.NewReader(`{}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("want 400, got %d", w.Code)
		}
	})

	t.Run("wrong method returns 405", func(t *testing.T) {
		srv, state := newTestServerForAuth(t)
		req := httptest.NewRequest(http.MethodGet, "/v1/auth/device/"+state+"/token", nil)
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)

		if w.Code != http.StatusMethodNotAllowed {
			t.Fatalf("want 405, got %d", w.Code)
		}
	})

	t.Run("ready session calls supabase and returns token", func(t *testing.T) {
		// Start a mock Supabase server.
		mockSupabase := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/auth/v1/token" || r.URL.Query().Get("grant_type") != "pkce" {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			var body map[string]string
			json.NewDecoder(r.Body).Decode(&body) //nolint:errcheck
			if body["auth_code"] == "" || body["code_verifier"] == "" {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{ //nolint:errcheck
				"access_token":  "access-tok-123",
				"refresh_token": "refresh-tok-456",
				"expires_at":    1800000000,
				"user": map[string]interface{}{
					"id":    "user-uuid",
					"email": "test@example.com",
					"user_metadata": map[string]interface{}{
						"full_name": "Test User",
					},
				},
			})
		}))
		defer mockSupabase.Close()

		// Create a session with the mock Supabase URL.
		dir := t.TempDir()
		st, err := store.New(filepath.Join(dir, "supabase_test.db"))
		if err != nil {
			t.Fatalf("new store: %v", err)
		}
		defer st.Close()

		srv := NewServer(Config{Store: st, Version: "test"})
		state := "supabasestate"
		err = st.CreateDeviceSession(state, "SBCODE", "challenge999", mockSupabase.URL, time.Now().Add(10*time.Minute))
		if err != nil {
			t.Fatalf("CreateDeviceSession: %v", err)
		}
		// Mark as ready.
		if err := st.SetDeviceSessionAuthCode(state, "authcode-from-supabase"); err != nil {
			t.Fatalf("SetDeviceSessionAuthCode: %v", err)
		}

		w := doDeviceTokenRequest(t, srv, state, "verifier-xyz")
		if w.Code != http.StatusOK {
			t.Fatalf("want 200, got %d (body: %s)", w.Code, w.Body.String())
		}

		var resp deviceTokenResponse
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if resp.AccessToken != "access-tok-123" {
			t.Errorf("AccessToken: want 'access-tok-123', got %q", resp.AccessToken)
		}
		if resp.RefreshToken != "refresh-tok-456" {
			t.Errorf("RefreshToken: want 'refresh-tok-456', got %q", resp.RefreshToken)
		}
		if resp.User.Email != "test@example.com" {
			t.Errorf("User.Email: want 'test@example.com', got %q", resp.User.Email)
		}
		if resp.User.Name != "Test User" {
			t.Errorf("User.Name: want 'Test User', got %q", resp.User.Name)
		}
		// auth_code must NOT be in response (security: never expose auth_code).
	})

	t.Run("supabase returns error → 400", func(t *testing.T) {
		mockSupabase := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintln(w, `{"msg":"invalid code verifier"}`)
		}))
		defer mockSupabase.Close()

		dir := t.TempDir()
		st, err := store.New(filepath.Join(dir, "err_test.db"))
		if err != nil {
			t.Fatalf("new store: %v", err)
		}
		defer st.Close()

		srv := NewServer(Config{Store: st, Version: "test"})
		state := "errstate"
		err = st.CreateDeviceSession(state, "ERRCODE", "challenge", mockSupabase.URL, time.Now().Add(10*time.Minute))
		if err != nil {
			t.Fatalf("CreateDeviceSession: %v", err)
		}
		st.SetDeviceSessionAuthCode(state, "auth-err") //nolint:errcheck

		w := doDeviceTokenRequest(t, srv, state, "bad-verifier")
		if w.Code != http.StatusBadRequest {
			t.Fatalf("want 400, got %d (body: %s)", w.Code, w.Body.String())
		}
		var resp map[string]string
		json.NewDecoder(w.Body).Decode(&resp) //nolint:errcheck
		if !strings.Contains(resp["error"], "token exchange failed") {
			t.Fatalf("expected 'token exchange failed', got %q", resp["error"])
		}
	})

	t.Run("supabase unreachable → 502", func(t *testing.T) {
		dir := t.TempDir()
		st, err := store.New(filepath.Join(dir, "unreachable_test.db"))
		if err != nil {
			t.Fatalf("new store: %v", err)
		}
		defer st.Close()

		srv := NewServer(Config{Store: st, Version: "test"})
		state := "unreachstate"
		// Point to a port that will refuse connections.
		err = st.CreateDeviceSession(state, "UCODE", "challenge", "http://127.0.0.1:1", time.Now().Add(10*time.Minute))
		if err != nil {
			t.Fatalf("CreateDeviceSession: %v", err)
		}
		st.SetDeviceSessionAuthCode(state, "auth-unreachable") //nolint:errcheck

		w := doDeviceTokenRequest(t, srv, state, "verifier")
		if w.Code != http.StatusBadGateway {
			t.Fatalf("want 502, got %d (body: %s)", w.Code, w.Body.String())
		}
		var resp map[string]string
		json.NewDecoder(w.Body).Decode(&resp) //nolint:errcheck
		if !strings.Contains(resp["error"], "supabase unreachable") {
			t.Fatalf("expected 'supabase unreachable', got %q", resp["error"])
		}
	})
}

// =========================================================================
// Helpers
// =========================================================================

func doDeviceTokenRequest(t *testing.T, srv *Server, state, codeVerifier string) *httptest.ResponseRecorder {
	t.Helper()
	body := fmt.Sprintf(`{"code_verifier":%q}`, codeVerifier)
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/device/"+state+"/token", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	return w
}
