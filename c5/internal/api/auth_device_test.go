package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/piqsol/c4/c5/internal/store"
)

func TestDeviceAuthCreate(t *testing.T) {
	srv := newTestServer(t)

	t.Run("success", func(t *testing.T) {
		body := map[string]string{
			"code_challenge":        "test-challenge",
			"supabase_url":          "https://example.supabase.co",
			"code_challenge_method": "S256",
		}
		w := doRequest(t, srv, http.MethodPost, "/v1/auth/device", body)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}

		var resp map[string]any
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("decode: %v", err)
		}

		// Check required fields
		for _, key := range []string{"state", "user_code", "auth_url", "activate_url", "expires_in"} {
			if _, ok := resp[key]; !ok {
				t.Errorf("missing field %q in response", key)
			}
		}

		if resp["expires_in"].(float64) != 600 {
			t.Errorf("expected expires_in=600, got %v", resp["expires_in"])
		}

		// auth_url should contain supabase URL and code_challenge
		authURL := resp["auth_url"].(string)
		if !strings.Contains(authURL, "example.supabase.co") {
			t.Errorf("auth_url should contain supabase URL: %s", authURL)
		}
		if !strings.Contains(authURL, "test-challenge") {
			t.Errorf("auth_url should contain code_challenge: %s", authURL)
		}
	})

	t.Run("missing code_challenge", func(t *testing.T) {
		body := map[string]string{
			"supabase_url": "https://example.supabase.co",
		}
		w := doRequest(t, srv, http.MethodPost, "/v1/auth/device", body)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
		var resp map[string]string
		json.NewDecoder(w.Body).Decode(&resp)
		if resp["error"] != "missing code_challenge" {
			t.Errorf("unexpected error: %s", resp["error"])
		}
	})

	t.Run("missing supabase_url", func(t *testing.T) {
		body := map[string]string{
			"code_challenge": "test-challenge",
		}
		w := doRequest(t, srv, http.MethodPost, "/v1/auth/device", body)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
		var resp map[string]string
		json.NewDecoder(w.Body).Decode(&resp)
		if resp["error"] != "missing supabase_url" {
			t.Errorf("unexpected error: %s", resp["error"])
		}
	})
}

func TestDeviceAuthPoll(t *testing.T) {
	srv := newTestServer(t)

	// Create a session first
	body := map[string]string{
		"code_challenge":        "test-challenge",
		"supabase_url":          "https://example.supabase.co",
		"code_challenge_method": "S256",
	}
	w := doRequest(t, srv, http.MethodPost, "/v1/auth/device", body)
	if w.Code != http.StatusOK {
		t.Fatalf("create failed: %d: %s", w.Code, w.Body.String())
	}
	var createResp map[string]any
	json.NewDecoder(w.Body).Decode(&createResp)
	state := createResp["state"].(string)

	t.Run("pending status", func(t *testing.T) {
		w := doRequest(t, srv, http.MethodGet, "/v1/auth/device/"+state, nil)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		var resp map[string]string
		json.NewDecoder(w.Body).Decode(&resp)
		if resp["status"] != "pending" {
			t.Errorf("expected pending, got %s", resp["status"])
		}
	})

	t.Run("ready after auth_code set", func(t *testing.T) {
		if err := srv.store.SetDeviceSessionAuthCode(state, "test-auth-code"); err != nil {
			t.Fatalf("set auth code: %v", err)
		}
		w := doRequest(t, srv, http.MethodGet, "/v1/auth/device/"+state, nil)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		var resp map[string]string
		json.NewDecoder(w.Body).Decode(&resp)
		if resp["status"] != "ready" {
			t.Errorf("expected ready, got %s", resp["status"])
		}
	})

	t.Run("not found", func(t *testing.T) {
		w := doRequest(t, srv, http.MethodGet, "/v1/auth/device/nonexistent", nil)
		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", w.Code)
		}
	})
}

func TestDeviceAuthPollExpiry(t *testing.T) {
	srv := newTestServer(t)

	// Create a session
	body := map[string]string{
		"code_challenge":        "test-challenge",
		"supabase_url":          "https://example.supabase.co",
		"code_challenge_method": "S256",
	}
	w := doRequest(t, srv, http.MethodPost, "/v1/auth/device", body)
	if w.Code != http.StatusOK {
		t.Fatalf("create failed: %d", w.Code)
	}
	var createResp map[string]any
	json.NewDecoder(w.Body).Decode(&createResp)
	state := createResp["state"].(string)

	// Poll 61 times — should expire after 60 (pending sessions only)
	for i := 0; i < 61; i++ {
		doRequest(t, srv, http.MethodGet, "/v1/auth/device/"+state, nil)
	}

	// 62nd poll should return 404 (expired)
	w = doRequest(t, srv, http.MethodGet, "/v1/auth/device/"+state, nil)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 after expiry, got %d: %s", w.Code, w.Body.String())
	}
}

func TestActivateGet(t *testing.T) {
	srv := newTestServer(t)

	// Create a device session to get a valid state.
	createBody := map[string]string{
		"code_challenge":        "test-challenge",
		"supabase_url":          "https://example.supabase.co",
		"code_challenge_method": "S256",
	}
	cw := doRequest(t, srv, http.MethodPost, "/v1/auth/device", createBody)
	if cw.Code != http.StatusOK {
		t.Fatalf("create device session failed: %d", cw.Code)
	}
	var createResp map[string]any
	json.NewDecoder(cw.Body).Decode(&createResp)
	state := createResp["state"].(string)

	t.Run("success with valid state", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/auth/activate?state="+state, nil)
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}

		ct := w.Header().Get("Content-Type")
		if !strings.Contains(ct, "text/html") {
			t.Errorf("expected text/html content-type, got %s", ct)
		}

		bodyStr := w.Body.String()
		if !strings.Contains(bodyStr, `name="user_code"`) {
			t.Error("HTML should contain user_code input")
		}
		// Form action should include state parameter.
		if !strings.Contains(bodyStr, `state=`+state) {
			t.Errorf("form action should contain state=%s", state)
		}
		// No CSRF cookie should be set.
		for _, c := range w.Result().Cookies() {
			if c.Name == "csrf" {
				t.Error("unexpected csrf cookie — state-based CSRF should not set cookies")
			}
		}
	})

	t.Run("missing state returns 400", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/auth/activate", nil)
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})

	t.Run("unknown state returns 404", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/auth/activate?state=unknownstate123", nil)
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", w.Code)
		}
	})
}

func TestActivatePost(t *testing.T) {
	srv := newTestServer(t)

	// Create a device session first.
	createBody := map[string]string{
		"code_challenge":        "test-challenge",
		"supabase_url":          "https://example.supabase.co",
		"code_challenge_method": "S256",
	}
	w := doRequest(t, srv, http.MethodPost, "/v1/auth/device", createBody)
	if w.Code != http.StatusOK {
		t.Fatalf("create failed: %d", w.Code)
	}
	var createResp map[string]any
	json.NewDecoder(w.Body).Decode(&createResp)
	userCode := createResp["user_code"].(string)
	state := createResp["state"].(string)

	t.Run("success redirect", func(t *testing.T) {
		form := url.Values{}
		form.Set("user_code", userCode)

		req := httptest.NewRequest(http.MethodPost, "/auth/activate?state="+state, strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)

		if w.Code != http.StatusFound {
			t.Fatalf("expected 302, got %d: %s", w.Code, w.Body.String())
		}

		location := w.Header().Get("Location")
		if !strings.Contains(location, "example.supabase.co") {
			t.Errorf("redirect should point to supabase auth URL: %s", location)
		}
		if !strings.Contains(location, "test-challenge") {
			t.Errorf("redirect should contain code_challenge: %s", location)
		}
	})

	t.Run("state mismatch returns 403", func(t *testing.T) {
		form := url.Values{}
		form.Set("user_code", userCode)

		req := httptest.NewRequest(http.MethodPost, "/auth/activate?state=wrongstate000000000000000000000000000000000000000000000000000000", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)

		if w.Code != http.StatusForbidden {
			t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("missing state returns 400", func(t *testing.T) {
		form := url.Values{}
		form.Set("user_code", userCode)

		req := httptest.NewRequest(http.MethodPost, "/auth/activate", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})

	t.Run("invalid user code shows form again", func(t *testing.T) {
		form := url.Values{}
		form.Set("user_code", "INVALID1")

		req := httptest.NewRequest(http.MethodPost, "/auth/activate?state="+state, strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 (form re-render), got %d: %s", w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), `name="user_code"`) {
			t.Error("response should re-render the activation form")
		}
		if !strings.Contains(w.Body.String(), "잘못된") {
			t.Error("response should contain an error message")
		}
	})
}

func TestDeviceAuthPublicEndpoint(t *testing.T) {
	// Verify device auth endpoints bypass API key auth
	srv := NewServer(Config{
		Store:  newTestStore(t),
		APIKey: "secret-key",
	})
	defer srv.Close()

	t.Run("POST /v1/auth/device without API key", func(t *testing.T) {
		body := map[string]string{
			"code_challenge":        "test-challenge",
			"supabase_url":          "https://example.supabase.co",
			"code_challenge_method": "S256",
		}
		data, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, "/v1/auth/device", strings.NewReader(string(data)))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)

		// Should NOT get 401
		if w.Code == http.StatusUnauthorized {
			t.Fatal("device auth endpoint should be public (bypass API key)")
		}
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("GET /auth/activate without API key", func(t *testing.T) {
		// Without a valid state we get 400, but NOT 401 — endpoint is public.
		req := httptest.NewRequest(http.MethodGet, "/auth/activate", nil)
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)

		if w.Code == http.StatusUnauthorized {
			t.Fatal("activate endpoint should be public (bypass API key)")
		}
	})
}

// newTestStore creates a test store and registers cleanup.
func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	dir := t.TempDir()
	st, err := store.New(dir + "/test.db")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}
