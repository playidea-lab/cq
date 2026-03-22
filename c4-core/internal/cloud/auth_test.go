package cloud

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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

// TestOAuthCallbackServer2 tests the fragment-based OAuth callback server.
func TestOAuthCallbackServer2(t *testing.T) {
	// Mock Supabase /auth/v1/user endpoint
	mockSupabase := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/auth/v1/user" {
			user := supabaseUser{
				ID:    "github-user",
				Email: "dev@github.com",
				UserMetadata: map[string]any{
					"full_name": "GitHub Dev",
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(user)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer mockSupabase.Close()

	tmpDir := t.TempDir()
	sessionPath := filepath.Join(tmpDir, "session.json")

	client := NewAuthClient(mockSupabase.URL, "anon-key")
	client.SetSessionPath(sessionPath)

	// Start the callback server
	sessionCh := make(chan *Session, 1)
	errCh := make(chan error, 1)
	srv, port, err := client.startCallbackServer2(sessionCh, errCh)
	if err != nil {
		t.Fatalf("startCallbackServer2 failed: %v", err)
	}
	defer srv.Close()

	if port <= 0 {
		t.Fatalf("port should be > 0, got %d", port)
	}

	// The callback page serves HTML (fragment extraction via JS).
	// We test the /auth/token endpoint directly (simulating the JS POST).
	tokenURL := "http://localhost:" + itoa(port) + "/auth/token"
	tokenBody := `{"access_token":"test-access","refresh_token":"test-refresh","expires_in":3600,"token_type":"bearer"}`
	resp, err := http.Post(tokenURL, "application/json", bytes.NewBufferString(tokenBody))
	if err != nil {
		t.Fatalf("token POST failed: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("token status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	// Read the session from the channel
	select {
	case session := <-sessionCh:
		if session.AccessToken != "test-access" {
			t.Errorf("AccessToken = %q, want %q", session.AccessToken, "test-access")
		}
		if session.RefreshToken != "test-refresh" {
			t.Errorf("RefreshToken = %q, want %q", session.RefreshToken, "test-refresh")
		}
		if session.User.ID != "github-user" {
			t.Errorf("User.ID = %q, want %q", session.User.ID, "github-user")
		}
		if session.User.Email != "dev@github.com" {
			t.Errorf("User.Email = %q, want %q", session.User.Email, "dev@github.com")
		}
		if session.User.Name != "GitHub Dev" {
			t.Errorf("User.Name = %q, want %q", session.User.Name, "GitHub Dev")
		}
	case err := <-errCh:
		t.Fatalf("errCh received: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for session")
	}
}

// TestOAuthCallbackServer2MissingToken tests /auth/token without access_token.
func TestOAuthCallbackServer2MissingToken(t *testing.T) {
	client := NewAuthClient("https://test.supabase.co", "key")

	sessionCh := make(chan *Session, 1)
	errCh := make(chan error, 1)
	srv, port, err := client.startCallbackServer2(sessionCh, errCh)
	if err != nil {
		t.Fatalf("startCallbackServer2 failed: %v", err)
	}
	defer srv.Close()

	// POST without access_token
	tokenURL := "http://localhost:" + itoa(port) + "/auth/token"
	resp, err := http.Post(tokenURL, "application/json", bytes.NewBufferString(`{"refresh_token":"rt"}`))
	if err != nil {
		t.Fatalf("token POST failed: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("token status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
}

// TestGetUserInfo tests fetching user info from Supabase.
func TestGetUserInfo(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/auth/v1/user" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("Authorization = %q, want 'Bearer test-token'", got)
		}
		user := supabaseUser{
			ID:    "u-123",
			Email: "user@test.com",
			UserMetadata: map[string]any{
				"full_name": "Test User",
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(user)
	}))
	defer mockServer.Close()

	client := NewAuthClient(mockServer.URL, "anon-key")
	user, err := client.getUserInfo("test-token")
	if err != nil {
		t.Fatalf("getUserInfo failed: %v", err)
	}
	if user.ID != "u-123" {
		t.Errorf("ID = %q, want u-123", user.ID)
	}
	if user.Email != "user@test.com" {
		t.Errorf("Email = %q, want user@test.com", user.Email)
	}
	if user.Name != "Test User" {
		t.Errorf("Name = %q, want 'Test User'", user.Name)
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

// TestSessionUnmarshal_Int64 verifies that expires_at as a numeric Unix timestamp is parsed correctly.
func TestSessionUnmarshal_Int64(t *testing.T) {
	data := `{"access_token":"at","refresh_token":"rt","expires_at":1700000000,"user":{"id":"u1","email":"a@b.com","name":""}}`
	var s Session
	if err := json.Unmarshal([]byte(data), &s); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if s.ExpiresAt != 1700000000 {
		t.Errorf("ExpiresAt = %d, want 1700000000", s.ExpiresAt)
	}
	if s.AccessToken != "at" {
		t.Errorf("AccessToken = %q, want at", s.AccessToken)
	}
}

// TestSessionUnmarshal_ISO8601 verifies that expires_at as an ISO 8601 string (Rust c1 format) is parsed correctly.
func TestSessionUnmarshal_ISO8601(t *testing.T) {
	data := `{"access_token":"at","refresh_token":"rt","expires_at":"2026-02-18T00:35:21.824664+00:00","user":{"id":"u1","email":"a@b.com","name":""}}`
	var s Session
	if err := json.Unmarshal([]byte(data), &s); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	// 2026-02-18T00:35:21+00:00 → Unix = 1771374921
	want := int64(1771374921)
	if s.ExpiresAt != want {
		t.Errorf("ExpiresAt = %d, want %d", s.ExpiresAt, want)
	}
}

// TestSessionUnmarshal_InvalidStr verifies that an unparseable expires_at string returns an error.
func TestSessionUnmarshal_InvalidStr(t *testing.T) {
	data := `{"access_token":"at","refresh_token":"rt","expires_at":"not-a-date","user":{"id":"u1","email":"a@b.com","name":""}}`
	var s Session
	err := json.Unmarshal([]byte(data), &s)
	if err == nil {
		t.Fatal("expected error for invalid expires_at string, got nil")
	}
}

// TestLoginNoBrowser verifies that NoBrowser=true skips openBrowserFunc and
// prints the OAuth URL and SSH hint to stderr.
func TestLoginNoBrowser(t *testing.T) {
	// Stub openBrowserFunc to detect if it is called.
	orig := openBrowserFunc
	defer func() { openBrowserFunc = orig }()

	browserCalled := false
	openBrowserFunc = func(url string) error {
		browserCalled = true
		return nil
	}

	// Mock Supabase /auth/v1/user endpoint.
	mockSupabase := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/auth/v1/user" {
			user := supabaseUser{
				ID:    "nb-user",
				Email: "nb@example.com",
				UserMetadata: map[string]any{
					"full_name": "NoBrowser User",
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(user)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer mockSupabase.Close()

	tmpDir := t.TempDir()
	sessionPath := tmpDir + "/session.json"

	client := NewAuthClient(mockSupabase.URL, "anon-key")
	client.SetSessionPath(sessionPath)
	client.NoBrowser = true
	client.callbackTimeout = 200 * time.Millisecond // keep test fast; no real browser

	// Capture stderr output by redirecting os.Stderr.
	origStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stderr = w

	// Run LoginWithGitHub in a goroutine so we can POST the token concurrently.
	loginErr := make(chan error, 1)
	go func() {
		loginErr <- client.LoginWithGitHub()
	}()

	// Give the callback server a moment to start, then find its port from stderr.
	// We need to read stderr output to get the port.
	// But LoginWithGitHub writes to stderr before blocking — read it.
	// Use a small buffer read with a timeout via the channel approach.
	// Strategy: wait briefly, then scan stderr for the port number.
	time.Sleep(50 * time.Millisecond)

	// Read what's been written to stderr so far (non-blocking read from pipe).
	w.Close()
	os.Stderr = origStderr

	stderrBuf := new(strings.Builder)
	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	stderrBuf.Write(buf[:n])
	r.Close()

	stderrOutput := stderrBuf.String()

	// Verify URL is printed to stderr.
	if !strings.Contains(stderrOutput, mockSupabase.URL) {
		t.Errorf("expected stderr to contain OAuth URL %q, got:\n%s", mockSupabase.URL, stderrOutput)
	}
	// Verify SSH hint is printed.
	if !strings.Contains(stderrOutput, "ssh -L") {
		t.Errorf("expected stderr to contain SSH hint, got:\n%s", stderrOutput)
	}
	// Verify "Waiting for authorization" is printed.
	if !strings.Contains(stderrOutput, "Waiting for authorization") {
		t.Errorf("expected stderr to contain 'Waiting for authorization', got:\n%s", stderrOutput)
	}

	// Verify browser was NOT called.
	if browserCalled {
		t.Error("expected openBrowserFunc NOT to be called with NoBrowser=true")
	}

	// Wait for LoginWithGitHub to finish (it will time out via callbackTimeout=200ms).
	select {
	case <-loginErr:
		// Returned (timeout error expected — no callback was sent).
	case <-time.After(2 * time.Second):
		t.Error("LoginWithGitHub did not return within 2s; possible goroutine leak")
	}
}

// TestRefreshTokenRotationRace verifies that when a refresh fails because another
// process already rotated the refresh token, the client falls back to the
// session.json on disk (which the winning process already updated).
func TestRefreshTokenRotationRace(t *testing.T) {
	// Mock Supabase that always rejects the refresh (simulating token already rotated).
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(supabaseErrorResponse{
			Error:            "invalid_grant",
			ErrorDescription: "Invalid Refresh Token: Already Used",
		})
	}))
	defer mockServer.Close()

	tmpDir := t.TempDir()
	sessionPath := filepath.Join(tmpDir, "session.json")

	client := NewAuthClient(mockServer.URL, "key")
	client.SetSessionPath(sessionPath)

	// Save an expired session (triggers refresh attempt).
	expired := &Session{
		AccessToken:  "old-access",
		RefreshToken: "old-refresh",
		ExpiresAt:    time.Now().Add(-1 * time.Hour).Unix(),
		User:         User{ID: "u1", Email: "a@b.com"},
	}
	if err := client.saveSession(expired); err != nil {
		t.Fatal(err)
	}

	// Simulate another process writing a fresh session to disk.
	fresh := &Session{
		AccessToken:  "winner-access",
		RefreshToken: "winner-refresh",
		ExpiresAt:    time.Now().Add(1 * time.Hour).Unix(),
		User:         User{ID: "u1", Email: "a@b.com"},
	}
	freshData, _ := json.MarshalIndent(fresh, "", "  ")
	// We need to write this after the client reads the expired session but before
	// it checks disk on failure. Since the mock returns immediately, write it now.
	if err := os.WriteFile(sessionPath, freshData, 0o600); err != nil {
		t.Fatal(err)
	}

	// RefreshToken should detect the disk session was updated by another process.
	session, err := client.RefreshToken()
	if err != nil {
		t.Fatalf("RefreshToken should have fallen back to disk session: %v", err)
	}
	if session.AccessToken != "winner-access" {
		t.Errorf("AccessToken = %q, want winner-access", session.AccessToken)
	}
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
