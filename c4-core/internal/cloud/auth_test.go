package cloud

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestNewAuthClient verifies construction with required parameters.
func TestNewAuthClient(t *testing.T) {
	t.Run("valid construction", func(t *testing.T) {
		client := NewAuthClient("https://abc.supabase.co", "anon-key-123")
		if client == nil {
			t.Fatal("NewAuthClient returned nil")
		}
		if client.supabaseURL != "https://abc.supabase.co" {
			t.Errorf("supabaseURL = %q, want %q", client.supabaseURL, "https://abc.supabase.co")
		}
		if client.anonKey != "anon-key-123" {
			t.Errorf("anonKey = %q, want %q", client.anonKey, "anon-key-123")
		}
	})

	t.Run("custom session path", func(t *testing.T) {
		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "session.json")
		client := NewAuthClient("https://abc.supabase.co", "key")
		client.SetSessionPath(path)
		if client.sessionPath != path {
			t.Errorf("sessionPath = %q, want %q", client.sessionPath, path)
		}
	})
}

// TestSessionSaveLoad verifies round-trip persistence of session data.
func TestSessionSaveLoad(t *testing.T) {
	tmpDir := t.TempDir()
	sessionPath := filepath.Join(tmpDir, ".c4", "session.json")

	client := NewAuthClient("https://test.supabase.co", "key")
	client.SetSessionPath(sessionPath)

	session := &Session{
		AccessToken:  "access-token-abc",
		RefreshToken: "refresh-token-xyz",
		ExpiresAt:    time.Now().Add(1 * time.Hour).Unix(),
		User: User{
			ID:    "user-123",
			Email: "test@example.com",
			Name:  "Test User",
		},
	}

	// Save
	if err := client.saveSession(session); err != nil {
		t.Fatalf("saveSession failed: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(sessionPath); os.IsNotExist(err) {
		t.Fatal("session file was not created")
	}

	// Verify file permissions are restrictive (owner only)
	info, err := os.Stat(sessionPath)
	if err != nil {
		t.Fatalf("stat session file: %v", err)
	}
	perm := info.Mode().Perm()
	if perm != 0o600 {
		t.Errorf("session file permissions = %o, want %o", perm, 0o600)
	}

	// Load
	loaded, err := client.loadSession()
	if err != nil {
		t.Fatalf("loadSession failed: %v", err)
	}

	if loaded.AccessToken != session.AccessToken {
		t.Errorf("AccessToken = %q, want %q", loaded.AccessToken, session.AccessToken)
	}
	if loaded.RefreshToken != session.RefreshToken {
		t.Errorf("RefreshToken = %q, want %q", loaded.RefreshToken, session.RefreshToken)
	}
	if loaded.User.Email != session.User.Email {
		t.Errorf("User.Email = %q, want %q", loaded.User.Email, session.User.Email)
	}
	if loaded.User.ID != session.User.ID {
		t.Errorf("User.ID = %q, want %q", loaded.User.ID, session.User.ID)
	}
	if loaded.User.Name != session.User.Name {
		t.Errorf("User.Name = %q, want %q", loaded.User.Name, session.User.Name)
	}
}

// TestLoadSessionMissing verifies behavior when no session file exists.
func TestLoadSessionMissing(t *testing.T) {
	tmpDir := t.TempDir()
	sessionPath := filepath.Join(tmpDir, "nonexistent", "session.json")

	client := NewAuthClient("https://test.supabase.co", "key")
	client.SetSessionPath(sessionPath)

	session, err := client.loadSession()
	if err != nil {
		t.Fatalf("loadSession should not error for missing file: %v", err)
	}
	if session != nil {
		t.Error("loadSession should return nil for missing file")
	}
}

// TestGetSession returns stored session data.
func TestGetSession(t *testing.T) {
	tmpDir := t.TempDir()
	sessionPath := filepath.Join(tmpDir, "session.json")

	client := NewAuthClient("https://test.supabase.co", "key")
	client.SetSessionPath(sessionPath)

	t.Run("no session", func(t *testing.T) {
		session, err := client.GetSession()
		if err != nil {
			t.Fatalf("GetSession failed: %v", err)
		}
		if session != nil {
			t.Error("GetSession should return nil when no session exists")
		}
	})

	t.Run("valid session", func(t *testing.T) {
		session := &Session{
			AccessToken:  "token",
			RefreshToken: "refresh",
			ExpiresAt:    time.Now().Add(1 * time.Hour).Unix(),
			User:         User{ID: "u1", Email: "a@b.com", Name: "A"},
		}
		if err := client.saveSession(session); err != nil {
			t.Fatal(err)
		}

		got, err := client.GetSession()
		if err != nil {
			t.Fatalf("GetSession failed: %v", err)
		}
		if got == nil {
			t.Fatal("GetSession returned nil")
		}
		if got.AccessToken != "token" {
			t.Errorf("AccessToken = %q, want %q", got.AccessToken, "token")
		}
	})
}

// TestIsAuthenticated checks the quick auth status check.
func TestIsAuthenticated(t *testing.T) {
	tmpDir := t.TempDir()
	sessionPath := filepath.Join(tmpDir, "session.json")

	client := NewAuthClient("https://test.supabase.co", "key")
	client.SetSessionPath(sessionPath)

	t.Run("not authenticated when no session", func(t *testing.T) {
		if client.IsAuthenticated() {
			t.Error("IsAuthenticated should be false with no session")
		}
	})

	t.Run("not authenticated when expired", func(t *testing.T) {
		session := &Session{
			AccessToken:  "token",
			RefreshToken: "refresh",
			ExpiresAt:    time.Now().Add(-1 * time.Hour).Unix(), // expired
			User:         User{ID: "u1"},
		}
		if err := client.saveSession(session); err != nil {
			t.Fatal(err)
		}
		if client.IsAuthenticated() {
			t.Error("IsAuthenticated should be false with expired session")
		}
	})

	t.Run("authenticated when valid", func(t *testing.T) {
		session := &Session{
			AccessToken:  "token",
			RefreshToken: "refresh",
			ExpiresAt:    time.Now().Add(1 * time.Hour).Unix(),
			User:         User{ID: "u1"},
		}
		if err := client.saveSession(session); err != nil {
			t.Fatal(err)
		}
		if !client.IsAuthenticated() {
			t.Error("IsAuthenticated should be true with valid session")
		}
	})
}

// TestLogout verifies token clearing.
func TestLogout(t *testing.T) {
	tmpDir := t.TempDir()
	sessionPath := filepath.Join(tmpDir, "session.json")

	client := NewAuthClient("https://test.supabase.co", "key")
	client.SetSessionPath(sessionPath)

	// Save a session first
	session := &Session{
		AccessToken:  "token",
		RefreshToken: "refresh",
		ExpiresAt:    time.Now().Add(1 * time.Hour).Unix(),
		User:         User{ID: "u1"},
	}
	if err := client.saveSession(session); err != nil {
		t.Fatal(err)
	}

	// Verify file exists
	if _, err := os.Stat(sessionPath); os.IsNotExist(err) {
		t.Fatal("session file should exist before logout")
	}

	// Logout
	if err := client.Logout(); err != nil {
		t.Fatalf("Logout failed: %v", err)
	}

	// Verify file is gone
	if _, err := os.Stat(sessionPath); !os.IsNotExist(err) {
		t.Error("session file should not exist after logout")
	}

	// IsAuthenticated should be false
	if client.IsAuthenticated() {
		t.Error("IsAuthenticated should be false after logout")
	}
}

// TestLogoutNoSession verifies logout is idempotent.
func TestLogoutNoSession(t *testing.T) {
	tmpDir := t.TempDir()
	sessionPath := filepath.Join(tmpDir, "session.json")

	client := NewAuthClient("https://test.supabase.co", "key")
	client.SetSessionPath(sessionPath)

	// Logout without any session should not error
	if err := client.Logout(); err != nil {
		t.Fatalf("Logout without session should not error: %v", err)
	}
}

// TestExchangeCodeForToken tests the OAuth code exchange against a mock Supabase.
func TestExchangeCodeForToken(t *testing.T) {
	// Mock Supabase token endpoint
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/auth/v1/token" {
			t.Errorf("path = %s, want /auth/v1/token", r.URL.Path)
		}
		if got := r.URL.Query().Get("grant_type"); got != "authorization_code" {
			t.Errorf("grant_type = %q, want authorization_code", got)
		}
		if got := r.Header.Get("apikey"); got != "test-anon-key" {
			t.Errorf("apikey header = %q, want test-anon-key", got)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("content-type = %q, want application/json", got)
		}

		// Parse request body
		var body struct {
			AuthCode     string `json:"auth_code"`
			CodeVerifier string `json:"code_verifier"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode body: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if body.AuthCode != "test-code-123" {
			t.Errorf("auth_code = %q, want test-code-123", body.AuthCode)
		}

		// Return mock token response
		resp := supabaseTokenResponse{
			AccessToken:  "new-access-token",
			RefreshToken: "new-refresh-token",
			ExpiresIn:    3600,
			TokenType:    "bearer",
			User: supabaseUser{
				ID:    "user-abc",
				Email: "test@example.com",
				UserMetadata: map[string]any{
					"full_name": "Test User",
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer mockServer.Close()

	tmpDir := t.TempDir()
	sessionPath := filepath.Join(tmpDir, "session.json")

	client := NewAuthClient(mockServer.URL, "test-anon-key")
	client.SetSessionPath(sessionPath)

	session, err := client.exchangeCodeForToken("test-code-123")
	if err != nil {
		t.Fatalf("exchangeCodeForToken failed: %v", err)
	}

	if session.AccessToken != "new-access-token" {
		t.Errorf("AccessToken = %q, want %q", session.AccessToken, "new-access-token")
	}
	if session.RefreshToken != "new-refresh-token" {
		t.Errorf("RefreshToken = %q, want %q", session.RefreshToken, "new-refresh-token")
	}
	if session.User.ID != "user-abc" {
		t.Errorf("User.ID = %q, want %q", session.User.ID, "user-abc")
	}
	if session.User.Email != "test@example.com" {
		t.Errorf("User.Email = %q, want %q", session.User.Email, "test@example.com")
	}
	if session.User.Name != "Test User" {
		t.Errorf("User.Name = %q, want %q", session.User.Name, "Test User")
	}
}

// TestExchangeCodeForTokenError tests error handling for failed token exchange.
func TestExchangeCodeForTokenError(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"invalid_grant","error_description":"Invalid authorization code"}`))
	}))
	defer mockServer.Close()

	client := NewAuthClient(mockServer.URL, "key")

	_, err := client.exchangeCodeForToken("bad-code")
	if err == nil {
		t.Fatal("exchangeCodeForToken should fail with bad code")
	}
}

// TestRefreshToken tests the token refresh flow.
func TestRefreshToken(t *testing.T) {
	callCount := 0
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if r.URL.Path != "/auth/v1/token" {
			t.Errorf("path = %s, want /auth/v1/token", r.URL.Path)
		}
		if got := r.URL.Query().Get("grant_type"); got != "refresh_token" {
			t.Errorf("grant_type = %q, want refresh_token", got)
		}

		var body struct {
			RefreshToken string `json:"refresh_token"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode body: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if body.RefreshToken != "old-refresh" {
			t.Errorf("refresh_token = %q, want old-refresh", body.RefreshToken)
		}

		resp := supabaseTokenResponse{
			AccessToken:  "refreshed-access",
			RefreshToken: "refreshed-refresh",
			ExpiresIn:    3600,
			TokenType:    "bearer",
			User: supabaseUser{
				ID:    "user-abc",
				Email: "test@example.com",
				UserMetadata: map[string]any{
					"full_name": "Test User",
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer mockServer.Close()

	tmpDir := t.TempDir()
	sessionPath := filepath.Join(tmpDir, "session.json")

	client := NewAuthClient(mockServer.URL, "key")
	client.SetSessionPath(sessionPath)

	// Save an expired session with a refresh token
	expired := &Session{
		AccessToken:  "old-access",
		RefreshToken: "old-refresh",
		ExpiresAt:    time.Now().Add(-1 * time.Hour).Unix(),
		User:         User{ID: "user-abc", Email: "test@example.com"},
	}
	if err := client.saveSession(expired); err != nil {
		t.Fatal(err)
	}

	// Refresh
	session, err := client.RefreshToken()
	if err != nil {
		t.Fatalf("RefreshToken failed: %v", err)
	}

	if callCount != 1 {
		t.Errorf("expected 1 API call, got %d", callCount)
	}
	if session.AccessToken != "refreshed-access" {
		t.Errorf("AccessToken = %q, want %q", session.AccessToken, "refreshed-access")
	}
	if session.RefreshToken != "refreshed-refresh" {
		t.Errorf("RefreshToken = %q, want %q", session.RefreshToken, "refreshed-refresh")
	}

	// Verify persisted
	loaded, err := client.loadSession()
	if err != nil {
		t.Fatalf("loadSession failed: %v", err)
	}
	if loaded.AccessToken != "refreshed-access" {
		t.Errorf("persisted AccessToken = %q, want %q", loaded.AccessToken, "refreshed-access")
	}
}

// TestRefreshTokenNoSession tests refresh without existing session.
func TestRefreshTokenNoSession(t *testing.T) {
	tmpDir := t.TempDir()
	sessionPath := filepath.Join(tmpDir, "session.json")

	client := NewAuthClient("https://test.supabase.co", "key")
	client.SetSessionPath(sessionPath)

	_, err := client.RefreshToken()
	if err == nil {
		t.Fatal("RefreshToken should fail without existing session")
	}
}

// TestOAuthCallbackServer tests the temporary localhost callback server.
func TestOAuthCallbackServer(t *testing.T) {
	// Mock Supabase for token exchange
	mockSupabase := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := supabaseTokenResponse{
			AccessToken:  "oauth-access",
			RefreshToken: "oauth-refresh",
			ExpiresIn:    3600,
			TokenType:    "bearer",
			User: supabaseUser{
				ID:    "github-user",
				Email: "dev@github.com",
				UserMetadata: map[string]any{
					"full_name": "GitHub Dev",
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer mockSupabase.Close()

	tmpDir := t.TempDir()
	sessionPath := filepath.Join(tmpDir, "session.json")

	client := NewAuthClient(mockSupabase.URL, "anon-key")
	client.SetSessionPath(sessionPath)

	// Start the callback server on a test port
	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)
	srv, port, err := client.startCallbackServer(codeCh, errCh)
	if err != nil {
		t.Fatalf("startCallbackServer failed: %v", err)
	}
	defer srv.Close()

	if port <= 0 {
		t.Fatalf("port should be > 0, got %d", port)
	}

	// Simulate the OAuth redirect hitting the callback
	callbackURL := "http://localhost:" + itoa(port) + "/auth/callback?code=github-auth-code"
	resp, err := http.Get(callbackURL)
	if err != nil {
		t.Fatalf("callback request failed: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("callback status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	// Read the code from the channel
	select {
	case code := <-codeCh:
		if code != "github-auth-code" {
			t.Errorf("code = %q, want %q", code, "github-auth-code")
		}
	case err := <-errCh:
		t.Fatalf("errCh received: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for code")
	}
}

// TestOAuthCallbackServerMissingCode tests callback without code parameter.
func TestOAuthCallbackServerMissingCode(t *testing.T) {
	client := NewAuthClient("https://test.supabase.co", "key")

	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)
	srv, port, err := client.startCallbackServer(codeCh, errCh)
	if err != nil {
		t.Fatalf("startCallbackServer failed: %v", err)
	}
	defer srv.Close()

	// Hit callback without code
	callbackURL := "http://localhost:" + itoa(port) + "/auth/callback"
	resp, err := http.Get(callbackURL)
	if err != nil {
		t.Fatalf("callback request failed: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("callback status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
}

// TestOAuthURL verifies the constructed authorization URL.
func TestOAuthURL(t *testing.T) {
	client := NewAuthClient("https://abc.supabase.co", "anon-key")

	url := client.oauthURL(19823)

	// Should contain the Supabase auth endpoint
	wantPrefix := "https://abc.supabase.co/auth/v1/authorize"
	if len(url) < len(wantPrefix) || url[:len(wantPrefix)] != wantPrefix {
		t.Errorf("URL should start with %q, got %q", wantPrefix, url)
	}

	// Should contain provider=github
	if !containsSubstring(url, "provider=github") {
		t.Errorf("URL should contain provider=github: %s", url)
	}

	// Should contain redirect_to with the port
	if !containsSubstring(url, "redirect_to=") {
		t.Errorf("URL should contain redirect_to: %s", url)
	}
	if !containsSubstring(url, "19823") {
		t.Errorf("URL should contain port 19823: %s", url)
	}
}

// TestSessionJSON verifies JSON serialization format.
func TestSessionJSON(t *testing.T) {
	session := Session{
		AccessToken:  "at",
		RefreshToken: "rt",
		ExpiresAt:    1700000000,
		User: User{
			ID:    "uid",
			Email: "a@b.com",
			Name:  "Test",
		},
	}

	data, err := json.Marshal(session)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded Session
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.AccessToken != "at" {
		t.Errorf("AccessToken = %q", decoded.AccessToken)
	}
	if decoded.User.Email != "a@b.com" {
		t.Errorf("User.Email = %q", decoded.User.Email)
	}
}

// containsSubstring is a simple helper to avoid importing strings in tests.
func containsSubstring(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// itoa converts int to string without importing strconv.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	digits := make([]byte, 0, 10)
	for n > 0 {
		digits = append(digits, byte('0'+n%10))
		n /= 10
	}
	if neg {
		digits = append(digits, '-')
	}
	// reverse
	for i, j := 0, len(digits)-1; i < j; i, j = i+1, j-1 {
		digits[i], digits[j] = digits[j], digits[i]
	}
	return string(digits)
}
