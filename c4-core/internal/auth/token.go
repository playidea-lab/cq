package auth

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// Token refresh errors.
var (
	ErrNotAuthenticated = fmt.Errorf("not logged in; run 'c4 login' first")
	ErrTokenExpired     = fmt.Errorf("token expired and refresh failed")
)

// ReloginCallback is called when the token is expired and a refresh is not
// possible (e.g., no refresh token). It should trigger a re-login flow and
// return true if the new session was saved.
type ReloginCallback func() bool

// TokenManager manages the lifecycle of authentication tokens, including
// automatic refresh before expiration.
//
// Thread-safe.
type TokenManager struct {
	sessionManager *SessionManager
	supabaseURL    string
	anonKey        string
	onRelogin      ReloginCallback
	mu             sync.RWMutex

	// RefreshThreshold: refresh when fewer than this many seconds remain.
	// Default: 300 (5 minutes).
	RefreshThreshold time.Duration
}

// NewTokenManager creates a TokenManager backed by the given SessionManager.
// supabaseURL and anonKey can be empty and will be read from environment
// variables SUPABASE_URL and SUPABASE_ANON_KEY respectively.
func NewTokenManager(sm *SessionManager, supabaseURL, anonKey string) *TokenManager {
	return &TokenManager{
		sessionManager:   sm,
		supabaseURL:      supabaseURL,
		anonKey:          anonKey,
		RefreshThreshold: 5 * time.Minute,
	}
}

// SetReloginCallback sets a callback for when re-login is required.
func (t *TokenManager) SetReloginCallback(cb ReloginCallback) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.onRelogin = cb
}

// GetSession returns a valid Session, refreshing if needed.
// Returns ErrNotAuthenticated if no session exists,
// ErrTokenExpired if refresh fails.
func (t *TokenManager) GetSession() (*Session, error) {
	session, err := t.sessionManager.Load()
	if err != nil {
		return nil, fmt.Errorf("load session: %w", err)
	}
	if session == nil {
		return nil, ErrNotAuthenticated
	}

	if t.needsRefresh(session) {
		refreshed, err := t.refreshToken(session)
		if err != nil {
			return nil, err
		}
		return refreshed, nil
	}

	return session, nil
}

// GetAccessToken returns a valid access token string.
func (t *TokenManager) GetAccessToken() (string, error) {
	session, err := t.GetSession()
	if err != nil {
		return "", err
	}
	return session.AccessToken, nil
}

// GetAuthHeaders returns HTTP headers with a valid Bearer token.
func (t *TokenManager) GetAuthHeaders() (map[string]string, error) {
	token, err := t.GetAccessToken()
	if err != nil {
		return nil, err
	}
	return map[string]string{
		"Authorization": fmt.Sprintf("Bearer %s", token),
	}, nil
}

// needsRefresh checks whether the session needs a token refresh.
func (t *TokenManager) needsRefresh(session *Session) bool {
	if session.ExpiresAt == nil {
		return false
	}
	remaining := time.Until(*session.ExpiresAt)
	return remaining < t.RefreshThreshold
}

// refreshToken attempts to refresh the session using the refresh_token.
func (t *TokenManager) refreshToken(session *Session) (*Session, error) {
	if session.RefreshToken == "" {
		return t.handleRefreshFailure(session, "no refresh token")
	}

	supabaseURL := t.getSupabaseURL()
	if supabaseURL == "" {
		return t.handleRefreshFailure(session, "Supabase URL not configured")
	}

	anonKey := t.getAnonKey()

	// POST to /auth/v1/token?grant_type=refresh_token
	body := map[string]string{
		"refresh_token": session.RefreshToken,
	}
	bodyBytes, _ := json.Marshal(body)

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest(
		"POST",
		fmt.Sprintf("%s/auth/v1/token?grant_type=refresh_token", supabaseURL),
		strings.NewReader(string(bodyBytes)),
	)
	if err != nil {
		return t.handleRefreshFailure(session, fmt.Sprintf("build request: %v", err))
	}

	req.Header.Set("Content-Type", "application/json")
	if anonKey != "" {
		req.Header.Set("apikey", anonKey)
	}

	resp, err := client.Do(req)
	if err != nil {
		return t.handleRefreshFailure(session, fmt.Sprintf("network error: %v", err))
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return t.handleRefreshFailure(session, fmt.Sprintf("refresh failed: HTTP %d", resp.StatusCode))
	}

	var data map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return t.handleRefreshFailure(session, fmt.Sprintf("decode response: %v", err))
	}

	accessToken, _ := data["access_token"].(string)
	refreshToken, _ := data["refresh_token"].(string)
	if refreshToken == "" {
		refreshToken = session.RefreshToken
	}
	tokenType, _ := data["token_type"].(string)
	if tokenType == "" {
		tokenType = "bearer"
	}

	expiresIn := 3600.0
	if v, ok := data["expires_in"].(float64); ok {
		expiresIn = v
	}
	expiresAt := time.Now().Add(time.Duration(expiresIn) * time.Second)

	newSession := &Session{
		AccessToken:   accessToken,
		RefreshToken:  refreshToken,
		TokenType:     tokenType,
		ExpiresAt:     &expiresAt,
		UserID:        session.UserID,
		Email:         session.Email,
		Provider:      session.Provider,
		ProviderToken: session.ProviderToken,
		Metadata:      session.Metadata,
	}

	if err := t.sessionManager.Save(newSession); err != nil {
		return newSession, nil // still return even if save fails
	}

	return newSession, nil
}

// handleRefreshFailure handles a failed token refresh.
// If the session is still valid, it returns it. Otherwise, tries relogin.
func (t *TokenManager) handleRefreshFailure(session *Session, reason string) (*Session, error) {
	if session.IsExpired() {
		// Try relogin callback
		t.mu.RLock()
		cb := t.onRelogin
		t.mu.RUnlock()

		if cb != nil && cb() {
			newSession, err := t.sessionManager.Load()
			if err == nil && newSession != nil && !newSession.IsExpired() {
				return newSession, nil
			}
		}

		return nil, fmt.Errorf("%w: %s", ErrTokenExpired, reason)
	}

	// Not expired yet, continue with existing
	return session, nil
}

// getSupabaseURL resolves the Supabase URL from config or environment.
func (t *TokenManager) getSupabaseURL() string {
	if t.supabaseURL != "" {
		return t.supabaseURL
	}
	return os.Getenv("SUPABASE_URL")
}

// getAnonKey resolves the anon key from config or environment.
func (t *TokenManager) getAnonKey() string {
	if t.anonKey != "" {
		return t.anonKey
	}
	return os.Getenv("SUPABASE_ANON_KEY")
}

// Login runs the full login flow: OAuth PKCE -> save credentials.
// If noBrowser is true, prints the URL instead of opening the browser.
func Login(config *OAuthConfig, sm *SessionManager, noBrowser bool, onURL func(string)) (*Session, error) {
	flow, err := NewOAuthFlow(config)
	if err != nil {
		return nil, fmt.Errorf("create OAuth flow: %w", err)
	}

	// Use context with 2 minute timeout
	ctx, cancel := withTimeout(2 * time.Minute)
	defer cancel()

	result, err := flow.Run(ctx, !noBrowser, onURL)
	if err != nil {
		return nil, fmt.Errorf("OAuth flow: %w", err)
	}

	if !result.Success {
		return nil, fmt.Errorf("authentication failed: %s", result.Error)
	}

	// Build session
	expiresAt := time.Now().Add(time.Duration(result.ExpiresIn) * time.Second)
	session := &Session{
		AccessToken:  result.AccessToken,
		RefreshToken: result.RefreshToken,
		TokenType:    "bearer",
		ExpiresAt:    &expiresAt,
		Provider:     config.Provider,
	}

	// Save credentials
	if err := sm.Save(session); err != nil {
		return nil, fmt.Errorf("save credentials: %w", err)
	}

	return session, nil
}

// Logout clears the stored session and optionally revokes the token
// on the Supabase server.
func Logout(sm *SessionManager, supabaseURL, anonKey string) error {
	session, err := sm.Load()
	if err != nil {
		return fmt.Errorf("load session: %w", err)
	}

	// Revoke on server if we have a session
	if session != nil && supabaseURL != "" {
		_ = revokeToken(supabaseURL, anonKey, session.AccessToken)
	}

	return sm.Clear()
}

// revokeToken calls Supabase /auth/v1/logout to invalidate the session.
func revokeToken(supabaseURL, anonKey, accessToken string) error {
	client := &http.Client{Timeout: 10 * time.Second}

	req, err := http.NewRequest("POST", fmt.Sprintf("%s/auth/v1/logout", supabaseURL), nil)
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", accessToken))
	if anonKey != "" {
		req.Header.Set("apikey", anonKey)
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()

	return nil
}

// withTimeout creates a context with timeout (extracted for testing).
var withTimeout = func(d time.Duration) (ctx interface {
	Done() <-chan struct{}
	Err() error
	Deadline() (time.Time, bool)
	Value(key any) any
}, cancel func()) {
	return newTimeoutContext(d)
}

func newTimeoutContext(d time.Duration) (interface {
	Done() <-chan struct{}
	Err() error
	Deadline() (time.Time, bool)
	Value(key any) any
}, func()) {
	done := make(chan struct{})
	deadline := time.Now().Add(d)
	timer := time.AfterFunc(d, func() { close(done) })

	ctx := &timerContext{done: done, deadline: deadline}
	cancel := func() {
		timer.Stop()
		select {
		case <-done:
		default:
			close(done)
		}
	}

	return ctx, cancel
}

type timerContext struct {
	done     chan struct{}
	deadline time.Time
}

func (c *timerContext) Done() <-chan struct{}        { return c.done }
func (c *timerContext) Err() error                   { return nil }
func (c *timerContext) Deadline() (time.Time, bool)  { return c.deadline, true }
func (c *timerContext) Value(key any) any            { return nil }
