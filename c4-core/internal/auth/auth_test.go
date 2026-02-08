package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// =========================================================================
// Tests: Session
// =========================================================================

func TestSessionNotExpired(t *testing.T) {
	future := time.Now().Add(1 * time.Hour)
	s := &Session{
		AccessToken:  "token",
		RefreshToken: "refresh",
		ExpiresAt:    &future,
	}

	if s.IsExpired() {
		t.Error("session should not be expired")
	}
}

func TestSessionExpired(t *testing.T) {
	past := time.Now().Add(-1 * time.Hour)
	s := &Session{
		AccessToken:  "token",
		RefreshToken: "refresh",
		ExpiresAt:    &past,
	}

	if !s.IsExpired() {
		t.Error("session should be expired")
	}
}

func TestSessionNoExpiry(t *testing.T) {
	s := &Session{
		AccessToken:  "token",
		RefreshToken: "refresh",
	}

	if s.IsExpired() {
		t.Error("session with no expiry should not be expired")
	}
}

func TestExpiresInSeconds(t *testing.T) {
	future := time.Now().Add(30 * time.Minute)
	s := &Session{
		AccessToken:  "token",
		RefreshToken: "refresh",
		ExpiresAt:    &future,
	}

	secs := s.ExpiresInSeconds()
	if secs < 1750 || secs > 1850 {
		t.Errorf("ExpiresInSeconds = %d, want ~1800", secs)
	}
}

func TestExpiresInSecondsNoExpiry(t *testing.T) {
	s := &Session{AccessToken: "token", RefreshToken: "refresh"}
	if s.ExpiresInSeconds() != -1 {
		t.Errorf("expected -1 for no expiry, got %d", s.ExpiresInSeconds())
	}
}

func TestFromSupabaseResponse(t *testing.T) {
	resp := map[string]any{
		"session": map[string]any{
			"access_token":  "sb_token",
			"refresh_token": "sb_refresh",
			"expires_in":    float64(3600),
		},
		"user": map[string]any{
			"id":    "user-uuid",
			"email": "user@example.com",
			"identities": []any{
				map[string]any{
					"provider":      "github",
					"identity_data": map[string]any{},
				},
			},
		},
	}

	session, err := FromSupabaseResponse(resp)
	if err != nil {
		t.Fatalf("FromSupabaseResponse: %v", err)
	}

	if session.AccessToken != "sb_token" {
		t.Errorf("access_token = %q, want sb_token", session.AccessToken)
	}
	if session.UserID != "user-uuid" {
		t.Errorf("user_id = %q, want user-uuid", session.UserID)
	}
	if session.Email != "user@example.com" {
		t.Errorf("email = %q, want user@example.com", session.Email)
	}
	if session.Provider != "github" {
		t.Errorf("provider = %q, want github", session.Provider)
	}
	if session.IsExpired() {
		t.Error("newly created session should not be expired")
	}
}

// =========================================================================
// Tests: SessionManager (credential file)
// =========================================================================

func TestLoginStoresCredentials(t *testing.T) {
	dir := t.TempDir()
	sm, err := NewSessionManager(dir)
	if err != nil {
		t.Fatalf("NewSessionManager: %v", err)
	}

	future := time.Now().Add(1 * time.Hour)
	session := &Session{
		AccessToken:  "test_token",
		RefreshToken: "refresh_token",
		ExpiresAt:    &future,
		Email:        "test@example.com",
	}

	if err := sm.Save(session); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(sm.SessionFile()); os.IsNotExist(err) {
		t.Error("credentials file should exist after Save")
	}

	// Load and verify
	loaded, err := sm.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded == nil {
		t.Fatal("loaded session should not be nil")
	}
	if loaded.AccessToken != "test_token" {
		t.Errorf("access_token = %q, want test_token", loaded.AccessToken)
	}
	if loaded.Email != "test@example.com" {
		t.Errorf("email = %q, want test@example.com", loaded.Email)
	}
}

func TestCredentialFilePermissions(t *testing.T) {
	dir := t.TempDir()
	sm, err := NewSessionManager(dir)
	if err != nil {
		t.Fatalf("NewSessionManager: %v", err)
	}

	session := &Session{
		AccessToken:  "secret_token",
		RefreshToken: "refresh",
	}

	if err := sm.Save(session); err != nil {
		t.Fatalf("Save: %v", err)
	}

	info, err := os.Stat(sm.SessionFile())
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}

	mode := info.Mode().Perm()
	if mode != 0600 {
		t.Errorf("credential file permissions = %o, want 0600", mode)
	}
}

func TestLoadNonexistent(t *testing.T) {
	dir := t.TempDir()
	sm, err := NewSessionManager(dir)
	if err != nil {
		t.Fatalf("NewSessionManager: %v", err)
	}

	loaded, err := sm.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded != nil {
		t.Error("expected nil for nonexistent credentials")
	}
}

func TestLogoutCleansCredentials(t *testing.T) {
	dir := t.TempDir()
	sm, err := NewSessionManager(dir)
	if err != nil {
		t.Fatalf("NewSessionManager: %v", err)
	}

	// Save first
	session := &Session{
		AccessToken:  "token",
		RefreshToken: "refresh",
	}
	sm.Save(session)

	// Verify exists
	if _, err := os.Stat(sm.SessionFile()); os.IsNotExist(err) {
		t.Fatal("credentials file should exist before clear")
	}

	// Clear
	if err := sm.Clear(); err != nil {
		t.Fatalf("Clear: %v", err)
	}

	// Verify gone
	if _, err := os.Stat(sm.SessionFile()); !os.IsNotExist(err) {
		t.Error("credentials file should not exist after Clear")
	}
}

func TestClearNonexistent(t *testing.T) {
	dir := t.TempDir()
	sm, err := NewSessionManager(dir)
	if err != nil {
		t.Fatalf("NewSessionManager: %v", err)
	}

	if err := sm.Clear(); err != nil {
		t.Errorf("Clear nonexistent should not error: %v", err)
	}
}

func TestIsLoggedIn(t *testing.T) {
	dir := t.TempDir()
	sm, _ := NewSessionManager(dir)

	if sm.IsLoggedIn() {
		t.Error("should not be logged in initially")
	}

	future := time.Now().Add(1 * time.Hour)
	sm.Save(&Session{
		AccessToken:  "token",
		RefreshToken: "refresh",
		ExpiresAt:    &future,
	})

	if !sm.IsLoggedIn() {
		t.Error("should be logged in after save")
	}
}

func TestIsLoggedInExpired(t *testing.T) {
	dir := t.TempDir()
	sm, _ := NewSessionManager(dir)

	past := time.Now().Add(-1 * time.Hour)
	sm.Save(&Session{
		AccessToken:  "token",
		RefreshToken: "refresh",
		ExpiresAt:    &past,
	})

	if sm.IsLoggedIn() {
		t.Error("expired session should not count as logged in")
	}
}

func TestGetValidSession(t *testing.T) {
	dir := t.TempDir()
	sm, _ := NewSessionManager(dir)

	future := time.Now().Add(1 * time.Hour)
	sm.Save(&Session{
		AccessToken:  "valid_token",
		RefreshToken: "refresh",
		ExpiresAt:    &future,
	})

	session, err := sm.GetValidSession()
	if err != nil {
		t.Fatalf("GetValidSession: %v", err)
	}
	if session == nil || session.AccessToken != "valid_token" {
		t.Error("expected valid session")
	}
}

func TestGetValidSessionExpired(t *testing.T) {
	dir := t.TempDir()
	sm, _ := NewSessionManager(dir)

	past := time.Now().Add(-1 * time.Hour)
	sm.Save(&Session{
		AccessToken:  "expired_token",
		RefreshToken: "refresh",
		ExpiresAt:    &past,
	})

	session, err := sm.GetValidSession()
	if err != nil {
		t.Fatalf("GetValidSession: %v", err)
	}
	if session != nil {
		t.Error("expired session should return nil")
	}
}

func TestAuthStatus(t *testing.T) {
	dir := t.TempDir()
	sm, _ := NewSessionManager(dir)

	// Not logged in
	status := sm.Status()
	if status.LoggedIn {
		t.Error("should not be logged in")
	}
	if status.Valid {
		t.Error("should not be valid")
	}

	// Logged in
	future := time.Now().Add(1 * time.Hour)
	sm.Save(&Session{
		AccessToken:  "token",
		RefreshToken: "refresh",
		ExpiresAt:    &future,
		UserID:       "user-123",
		Email:        "test@example.com",
		Provider:     "github",
	})

	status = sm.Status()
	if !status.LoggedIn {
		t.Error("should be logged in")
	}
	if !status.Valid {
		t.Error("should be valid")
	}
	if status.UserID != "user-123" {
		t.Errorf("user_id = %q, want user-123", status.UserID)
	}
	if status.Email != "test@example.com" {
		t.Errorf("email = %q, want test@example.com", status.Email)
	}
}

// =========================================================================
// Tests: Session serialization roundtrip
// =========================================================================

func TestSessionJSONRoundtrip(t *testing.T) {
	future := time.Now().Add(1 * time.Hour).Truncate(time.Second)
	original := &Session{
		AccessToken:  "token123",
		RefreshToken: "refresh456",
		TokenType:    "bearer",
		ExpiresAt:    &future,
		UserID:       "user-uuid",
		Email:        "test@example.com",
		Provider:     "github",
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var loaded Session
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if loaded.AccessToken != original.AccessToken {
		t.Errorf("AccessToken = %q, want %q", loaded.AccessToken, original.AccessToken)
	}
	if loaded.UserID != original.UserID {
		t.Errorf("UserID = %q, want %q", loaded.UserID, original.UserID)
	}
	if loaded.Email != original.Email {
		t.Errorf("Email = %q, want %q", loaded.Email, original.Email)
	}
}

// =========================================================================
// Tests: TokenManager - auto refresh on expiry
// =========================================================================

func TestAutoRefreshOnExpiry(t *testing.T) {
	dir := t.TempDir()
	sm, _ := NewSessionManager(dir)

	// Save a session that's about to expire (within refresh threshold)
	nearExpiry := time.Now().Add(1 * time.Minute)
	sm.Save(&Session{
		AccessToken:  "old_token",
		RefreshToken: "valid_refresh",
		ExpiresAt:    &nearExpiry,
		UserID:       "user-1",
	})

	// Mock Supabase refresh endpoint
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/auth/v1/token" && r.URL.Query().Get("grant_type") == "refresh_token" {
			json.NewEncoder(w).Encode(map[string]any{
				"access_token":  "refreshed_token",
				"refresh_token": "new_refresh",
				"token_type":    "bearer",
				"expires_in":    3600,
			})
			return
		}
		w.WriteHeader(404)
	}))
	defer server.Close()

	tm := NewTokenManager(sm, server.URL, "test-anon-key")

	session, err := tm.GetSession()
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}

	if session.AccessToken != "refreshed_token" {
		t.Errorf("access_token = %q, want refreshed_token", session.AccessToken)
	}

	// Verify saved
	saved, _ := sm.Load()
	if saved.AccessToken != "refreshed_token" {
		t.Errorf("saved token = %q, want refreshed_token", saved.AccessToken)
	}
	// Verify user info preserved
	if saved.UserID != "user-1" {
		t.Errorf("user_id lost after refresh: %q", saved.UserID)
	}
}

func TestRefreshFailurePromptsReauth(t *testing.T) {
	dir := t.TempDir()
	sm, _ := NewSessionManager(dir)

	// Save an expired session with no refresh token
	past := time.Now().Add(-1 * time.Hour)
	sm.Save(&Session{
		AccessToken:  "expired",
		RefreshToken: "",
		ExpiresAt:    &past,
	})

	tm := NewTokenManager(sm, "", "")

	// Set relogin callback that creates new session
	reauthCalled := false
	tm.SetReloginCallback(func() bool {
		reauthCalled = true
		future := time.Now().Add(1 * time.Hour)
		sm.Save(&Session{
			AccessToken:  "reauthed_token",
			RefreshToken: "new_refresh",
			ExpiresAt:    &future,
		})
		return true
	})

	session, err := tm.GetSession()
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if !reauthCalled {
		t.Error("relogin callback should have been called")
	}
	if session.AccessToken != "reauthed_token" {
		t.Errorf("access_token = %q, want reauthed_token", session.AccessToken)
	}
}

func TestExpiredNoCallbackErrors(t *testing.T) {
	dir := t.TempDir()
	sm, _ := NewSessionManager(dir)

	past := time.Now().Add(-1 * time.Hour)
	sm.Save(&Session{
		AccessToken:  "expired",
		RefreshToken: "",
		ExpiresAt:    &past,
	})

	tm := NewTokenManager(sm, "", "")

	_, err := tm.GetSession()
	if err == nil {
		t.Error("expected error for expired session without callback")
	}
}

func TestNotAuthenticatedError(t *testing.T) {
	dir := t.TempDir()
	sm, _ := NewSessionManager(dir)
	tm := NewTokenManager(sm, "", "")

	_, err := tm.GetSession()
	if err == nil {
		t.Error("expected error when no session")
	}
}

func TestGetAccessToken(t *testing.T) {
	dir := t.TempDir()
	sm, _ := NewSessionManager(dir)

	future := time.Now().Add(1 * time.Hour)
	sm.Save(&Session{
		AccessToken:  "my_token",
		RefreshToken: "refresh",
		ExpiresAt:    &future,
	})

	tm := NewTokenManager(sm, "", "")
	token, err := tm.GetAccessToken()
	if err != nil {
		t.Fatalf("GetAccessToken: %v", err)
	}
	if token != "my_token" {
		t.Errorf("token = %q, want my_token", token)
	}
}

func TestGetAuthHeaders(t *testing.T) {
	dir := t.TempDir()
	sm, _ := NewSessionManager(dir)

	future := time.Now().Add(1 * time.Hour)
	sm.Save(&Session{
		AccessToken:  "bearer_token",
		RefreshToken: "refresh",
		ExpiresAt:    &future,
	})

	tm := NewTokenManager(sm, "", "")
	headers, err := tm.GetAuthHeaders()
	if err != nil {
		t.Fatalf("GetAuthHeaders: %v", err)
	}
	if headers["Authorization"] != "Bearer bearer_token" {
		t.Errorf("Authorization = %q, want Bearer bearer_token", headers["Authorization"])
	}
}

func TestNeedsRefreshFalse(t *testing.T) {
	future := time.Now().Add(1 * time.Hour)
	tm := &TokenManager{RefreshThreshold: 5 * time.Minute}
	session := &Session{ExpiresAt: &future}

	if tm.needsRefresh(session) {
		t.Error("should not need refresh with 1 hour remaining")
	}
}

func TestNeedsRefreshTrue(t *testing.T) {
	nearExpiry := time.Now().Add(1 * time.Minute)
	tm := &TokenManager{RefreshThreshold: 5 * time.Minute}
	session := &Session{ExpiresAt: &nearExpiry}

	if !tm.needsRefresh(session) {
		t.Error("should need refresh with 1 minute remaining")
	}
}

func TestNeedsRefreshNoExpiry(t *testing.T) {
	tm := &TokenManager{RefreshThreshold: 5 * time.Minute}
	session := &Session{}

	if tm.needsRefresh(session) {
		t.Error("should not need refresh with no expiry set")
	}
}

// =========================================================================
// Tests: OAuth PKCE
// =========================================================================

func TestPKCEVerifier(t *testing.T) {
	pkce, err := NewPKCEVerifier()
	if err != nil {
		t.Fatalf("NewPKCEVerifier: %v", err)
	}

	if pkce.Verifier == "" {
		t.Error("verifier should not be empty")
	}
	if pkce.Challenge == "" {
		t.Error("challenge should not be empty")
	}
	if pkce.Method != "S256" {
		t.Errorf("method = %q, want S256", pkce.Method)
	}

	// Verifier and challenge should be different
	if pkce.Verifier == pkce.Challenge {
		t.Error("verifier and challenge should differ")
	}
}

func TestPKCEVerifierUniqueness(t *testing.T) {
	p1, _ := NewPKCEVerifier()
	p2, _ := NewPKCEVerifier()

	if p1.Verifier == p2.Verifier {
		t.Error("two verifiers should be unique")
	}
}

func TestOAuthFlowCreation(t *testing.T) {
	config := &OAuthConfig{
		SupabaseURL: "https://test.supabase.co",
		Provider:    "github",
	}

	flow, err := NewOAuthFlow(config)
	if err != nil {
		t.Fatalf("NewOAuthFlow: %v", err)
	}

	if flow.State == "" {
		t.Error("state should not be empty")
	}
	if flow.PKCE == nil {
		t.Fatal("PKCE should not be nil")
	}
}

func TestAuthorizationURL(t *testing.T) {
	config := &OAuthConfig{
		SupabaseURL:  "https://test.supabase.co",
		RedirectPort: 8765,
		RedirectPath: "/auth/callback",
		Provider:     "github",
	}

	flow, _ := NewOAuthFlow(config)
	authURL := flow.AuthorizationURL()

	// Verify URL structure
	if authURL == "" {
		t.Fatal("authorization URL should not be empty")
	}

	// Should contain expected parts
	expected := []string{
		"https://test.supabase.co/auth/v1/authorize",
		"provider=github",
		"redirect_to=http%3A%2F%2Flocalhost%3A8765%2Fauth%2Fcallback",
		"code_challenge=",
		"code_challenge_method=S256",
		"state=",
	}

	for _, part := range expected {
		found := false
		if len(authURL) > 0 {
			for i := 0; i <= len(authURL)-len(part); i++ {
				if authURL[i:i+len(part)] == part {
					found = true
					break
				}
			}
		}
		if !found {
			t.Errorf("authorization URL missing %q\nURL: %s", part, authURL)
		}
	}
}

func TestOAuthConfigRedirectURI(t *testing.T) {
	config := &OAuthConfig{
		SupabaseURL:  "https://test.supabase.co",
		RedirectPort: 9000,
		RedirectPath: "/auth/callback",
	}

	if config.RedirectURI() != "http://localhost:9000/auth/callback" {
		t.Errorf("RedirectURI = %q, want http://localhost:9000/auth/callback", config.RedirectURI())
	}
}

func TestDefaultOAuthConfig(t *testing.T) {
	config := DefaultOAuthConfig()
	if config.RedirectPort != 8765 {
		t.Errorf("default port = %d, want 8765", config.RedirectPort)
	}
	if config.Provider != "github" {
		t.Errorf("default provider = %q, want github", config.Provider)
	}
}

// =========================================================================
// Tests: OAuth callback handling
// =========================================================================

func TestHandleCallbackSuccess(t *testing.T) {
	config := &OAuthConfig{
		SupabaseURL:  "https://test.supabase.co",
		RedirectPort: 0,
		RedirectPath: "/auth/callback",
		Provider:     "github",
	}

	flow, _ := NewOAuthFlow(config)

	// Simulate callback with access_token directly
	req := httptest.NewRequest("GET", fmt.Sprintf("/auth/callback?access_token=test_token&refresh_token=test_refresh&state=%s&expires_in=3600", flow.State), nil)

	result := flow.handleCallback(req)

	if !result.Success {
		t.Errorf("expected success, got error: %s", result.Error)
	}
	if result.AccessToken != "test_token" {
		t.Errorf("access_token = %q, want test_token", result.AccessToken)
	}
	if result.RefreshToken != "test_refresh" {
		t.Errorf("refresh_token = %q, want test_refresh", result.RefreshToken)
	}
}

func TestHandleCallbackStateMismatch(t *testing.T) {
	config := &OAuthConfig{
		SupabaseURL: "https://test.supabase.co",
	}
	flow, _ := NewOAuthFlow(config)

	req := httptest.NewRequest("GET", "/auth/callback?access_token=token&state=wrong_state", nil)
	result := flow.handleCallback(req)

	if result.Success {
		t.Error("should fail with state mismatch")
	}
	if result.Error == "" {
		t.Error("error message should not be empty")
	}
}

func TestHandleCallbackError(t *testing.T) {
	config := &OAuthConfig{
		SupabaseURL: "https://test.supabase.co",
	}
	flow, _ := NewOAuthFlow(config)

	req := httptest.NewRequest("GET", "/auth/callback?error=access_denied&error_description=User+denied+access", nil)
	result := flow.handleCallback(req)

	if result.Success {
		t.Error("should fail with access_denied")
	}
	if result.Error != "User denied access" {
		t.Errorf("error = %q, want User denied access", result.Error)
	}
}

func TestHandleCallbackWithCode(t *testing.T) {
	// Mock Supabase token exchange endpoint
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/auth/v1/token" {
			json.NewEncoder(w).Encode(map[string]any{
				"access_token":  "exchanged_token",
				"refresh_token": "exchanged_refresh",
				"token_type":    "bearer",
				"expires_in":    3600,
			})
			return
		}
		w.WriteHeader(404)
	}))
	defer server.Close()

	config := &OAuthConfig{
		SupabaseURL: server.URL,
		AnonKey:     "test-key",
	}
	flow, _ := NewOAuthFlow(config)

	req := httptest.NewRequest("GET", fmt.Sprintf("/auth/callback?code=auth_code_123&state=%s", flow.State), nil)
	result := flow.handleCallback(req)

	if !result.Success {
		t.Errorf("expected success, got error: %s", result.Error)
	}
	if result.AccessToken != "exchanged_token" {
		t.Errorf("access_token = %q, want exchanged_token", result.AccessToken)
	}
}

// =========================================================================
// Tests: Logout
// =========================================================================

func TestLogoutCleansUp(t *testing.T) {
	dir := t.TempDir()
	sm, _ := NewSessionManager(dir)

	// Save session
	sm.Save(&Session{
		AccessToken:  "to_revoke",
		RefreshToken: "refresh",
	})

	// Mock revoke endpoint
	revokeCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/auth/v1/logout" {
			revokeCalled = true
			w.WriteHeader(204)
			return
		}
		w.WriteHeader(404)
	}))
	defer server.Close()

	err := Logout(sm, server.URL, "test-key")
	if err != nil {
		t.Fatalf("Logout: %v", err)
	}

	if !revokeCalled {
		t.Error("revoke endpoint should have been called")
	}

	// Verify credentials gone
	if _, err := os.Stat(sm.SessionFile()); !os.IsNotExist(err) {
		t.Error("credentials file should be removed after logout")
	}
}

func TestLogoutNoSession(t *testing.T) {
	dir := t.TempDir()
	sm, _ := NewSessionManager(dir)

	err := Logout(sm, "", "")
	if err != nil {
		t.Errorf("Logout with no session should not error: %v", err)
	}
}

// =========================================================================
// Tests: Concurrent access
// =========================================================================

func TestSessionManagerConcurrent(t *testing.T) {
	dir := t.TempDir()
	sm, _ := NewSessionManager(dir)

	var wg sync.WaitGroup

	// 10 concurrent saves and loads
	for i := 0; i < 10; i++ {
		wg.Add(2)

		go func(idx int) {
			defer wg.Done()
			future := time.Now().Add(1 * time.Hour)
			sm.Save(&Session{
				AccessToken:  fmt.Sprintf("token-%d", idx),
				RefreshToken: "refresh",
				ExpiresAt:    &future,
			})
		}(i)

		go func() {
			defer wg.Done()
			sm.Load()
		}()
	}

	wg.Wait()

	// Should not panic or corrupt data
	session, err := sm.Load()
	if err != nil {
		t.Fatalf("Load after concurrent access: %v", err)
	}
	if session == nil {
		t.Error("session should not be nil after concurrent saves")
	}
}

// =========================================================================
// Tests: Token refresh with mock server failures
// =========================================================================

func TestRefreshServerError(t *testing.T) {
	dir := t.TempDir()
	sm, _ := NewSessionManager(dir)

	// Session still valid but within refresh threshold
	nearExpiry := time.Now().Add(2 * time.Minute)
	sm.Save(&Session{
		AccessToken:  "current_token",
		RefreshToken: "valid_refresh",
		ExpiresAt:    &nearExpiry,
	})

	// Mock server that returns 500
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer server.Close()

	tm := NewTokenManager(sm, server.URL, "test-key")

	// Should return existing session since it's not expired yet
	session, err := tm.GetSession()
	if err != nil {
		t.Fatalf("GetSession should succeed with valid session: %v", err)
	}
	if session.AccessToken != "current_token" {
		t.Errorf("should return existing valid token, got %q", session.AccessToken)
	}
}

func TestRefreshNetworkError(t *testing.T) {
	dir := t.TempDir()
	sm, _ := NewSessionManager(dir)

	nearExpiry := time.Now().Add(2 * time.Minute)
	sm.Save(&Session{
		AccessToken:  "current_token",
		RefreshToken: "valid_refresh",
		ExpiresAt:    &nearExpiry,
	})

	// Use unreachable URL
	tm := NewTokenManager(sm, "http://localhost:1", "test-key")

	// Should return existing session
	session, err := tm.GetSession()
	if err != nil {
		t.Fatalf("GetSession should succeed: %v", err)
	}
	if session.AccessToken != "current_token" {
		t.Errorf("should return existing valid token, got %q", session.AccessToken)
	}
}

// =========================================================================
// Tests: Full OAuth flow integration (with mock server)
// =========================================================================

func TestOAuthFlowRunWithTokenCallback(t *testing.T) {
	config := &OAuthConfig{
		SupabaseURL:  "https://test.supabase.co",
		RedirectPort: 0, // will find free port
		RedirectPath: "/auth/callback",
		Provider:     "github",
	}

	flow, err := NewOAuthFlow(config)
	if err != nil {
		t.Fatalf("NewOAuthFlow: %v", err)
	}

	// Use a free port
	config.RedirectPort = findFreePort(t)

	// Create context with short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var receivedURL string
	go func() {
		// Simulate browser callback after a short delay
		time.Sleep(200 * time.Millisecond)

		callbackURL := fmt.Sprintf(
			"http://localhost:%d/auth/callback?access_token=flow_token&refresh_token=flow_refresh&state=%s&expires_in=3600",
			config.RedirectPort,
			flow.State,
		)
		http.Get(callbackURL)
	}()

	result, err := flow.Run(ctx, false, func(url string) {
		receivedURL = url
	})

	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if !result.Success {
		t.Errorf("expected success, got error: %s", result.Error)
	}
	if result.AccessToken != "flow_token" {
		t.Errorf("access_token = %q, want flow_token", result.AccessToken)
	}
	if receivedURL == "" {
		t.Error("onURL callback should have been called")
	}
}

func TestOAuthFlowTimeout(t *testing.T) {
	config := &OAuthConfig{
		SupabaseURL:  "https://test.supabase.co",
		RedirectPort: findFreePort(t),
		RedirectPath: "/auth/callback",
		Provider:     "github",
	}

	flow, _ := NewOAuthFlow(config)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	result, err := flow.Run(ctx, false, func(url string) {})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if result.Success {
		t.Error("should not succeed on timeout")
	}
	if result.Error != "authentication timed out" {
		t.Errorf("error = %q, want 'authentication timed out'", result.Error)
	}
}

// =========================================================================
// Tests: DefaultSessionManager path
// =========================================================================

func TestDefaultSessionManagerPath(t *testing.T) {
	sm, err := NewSessionManager("")
	if err != nil {
		t.Fatalf("NewSessionManager: %v", err)
	}

	home, _ := os.UserHomeDir()
	expected := filepath.Join(home, ".c4", "credentials.json")
	if sm.SessionFile() != expected {
		t.Errorf("session file = %q, want %q", sm.SessionFile(), expected)
	}
}

// =========================================================================
// Helpers
// =========================================================================

func findFreePort(t *testing.T) int {
	t.Helper()

	l, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("find free port: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return port
}
