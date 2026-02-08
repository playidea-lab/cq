// Package auth provides Supabase OAuth authentication for the C4 CLI.
//
// Implements PKCE-based OAuth flow with GitHub as the identity provider,
// credential storage at ~/.c4/credentials.json (0600 permissions), and
// automatic token refresh.
package auth

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Session represents an authenticated user session with Supabase.
type Session struct {
	AccessToken   string            `json:"access_token"`
	RefreshToken  string            `json:"refresh_token"`
	TokenType     string            `json:"token_type"`
	ExpiresAt     *time.Time        `json:"expires_at,omitempty"`
	UserID        string            `json:"user_id,omitempty"`
	Email         string            `json:"email,omitempty"`
	Provider      string            `json:"provider,omitempty"`
	ProviderToken string            `json:"provider_token,omitempty"`
	Metadata      map[string]any    `json:"metadata,omitempty"`
}

// IsExpired returns true if the session has expired.
func (s *Session) IsExpired() bool {
	if s.ExpiresAt == nil {
		return false
	}
	return time.Now().After(*s.ExpiresAt)
}

// ExpiresInSeconds returns the number of seconds until expiration.
// Returns -1 if no expiry is set.
func (s *Session) ExpiresInSeconds() int {
	if s.ExpiresAt == nil {
		return -1
	}
	d := time.Until(*s.ExpiresAt)
	if d < 0 {
		return 0
	}
	return int(d.Seconds())
}

// FromSupabaseResponse creates a Session from a Supabase auth API response.
func FromSupabaseResponse(data map[string]any) (*Session, error) {
	sessionData, _ := data["session"].(map[string]any)
	if sessionData == nil {
		sessionData = data // fallback: top-level
	}

	accessToken, _ := sessionData["access_token"].(string)
	if accessToken == "" {
		return nil, fmt.Errorf("no access_token in response")
	}

	refreshToken, _ := sessionData["refresh_token"].(string)
	tokenType, _ := sessionData["token_type"].(string)
	if tokenType == "" {
		tokenType = "bearer"
	}

	// Parse expiration
	var expiresAt *time.Time
	if expiresAtUnix, ok := sessionData["expires_at"].(float64); ok {
		t := time.Unix(int64(expiresAtUnix), 0)
		expiresAt = &t
	} else if expiresIn, ok := sessionData["expires_in"].(float64); ok {
		t := time.Now().Add(time.Duration(expiresIn) * time.Second)
		expiresAt = &t
	}

	// Extract user info
	userData, _ := data["user"].(map[string]any)
	var userID, email, provider, providerToken string

	if userData != nil {
		userID, _ = userData["id"].(string)
		email, _ = userData["email"].(string)

		identities, _ := userData["identities"].([]any)
		if len(identities) > 0 {
			identity, _ := identities[0].(map[string]any)
			if identity != nil {
				provider, _ = identity["provider"].(string)
				identityData, _ := identity["identity_data"].(map[string]any)
				if identityData != nil {
					providerToken, _ = identityData["provider_token"].(string)
				}
			}
		}
	}

	return &Session{
		AccessToken:   accessToken,
		RefreshToken:  refreshToken,
		TokenType:     tokenType,
		ExpiresAt:     expiresAt,
		UserID:        userID,
		Email:         email,
		Provider:      provider,
		ProviderToken: providerToken,
	}, nil
}

// AuthStatus holds the result of `c4 auth status`.
type AuthStatus struct {
	LoggedIn  bool   `json:"logged_in"`
	UserID    string `json:"user_id,omitempty"`
	Email     string `json:"email,omitempty"`
	Provider  string `json:"provider,omitempty"`
	ExpiresIn int    `json:"expires_in,omitempty"` // seconds until expiry
	Valid     bool   `json:"valid"`
}

// SessionManager handles credential persistence at ~/.c4/credentials.json.
//
// Thread-safe: all operations are guarded by a mutex.
type SessionManager struct {
	configDir   string
	sessionFile string
	mu          sync.Mutex
}

// NewSessionManager creates a SessionManager using the given config directory.
// If configDir is empty, defaults to ~/.c4/.
func NewSessionManager(configDir string) (*SessionManager, error) {
	if configDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("get home directory: %w", err)
		}
		configDir = filepath.Join(home, ".c4")
	}

	return &SessionManager{
		configDir:   configDir,
		sessionFile: filepath.Join(configDir, "credentials.json"),
	}, nil
}

// Save persists the session to credentials.json with 0600 permissions.
func (m *SessionManager) Save(session *Session) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := os.MkdirAll(m.configDir, 0700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal session: %w", err)
	}

	if err := os.WriteFile(m.sessionFile, data, 0600); err != nil {
		return fmt.Errorf("write credentials: %w", err)
	}

	return nil
}

// Load reads the session from credentials.json.
// Returns nil, nil if no credentials file exists.
func (m *SessionManager) Load() (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	data, err := os.ReadFile(m.sessionFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read credentials: %w", err)
	}

	var session Session
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, fmt.Errorf("parse credentials: %w", err)
	}

	return &session, nil
}

// Clear removes the credentials file.
func (m *SessionManager) Clear() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	err := os.Remove(m.sessionFile)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove credentials: %w", err)
	}
	return nil
}

// GetValidSession returns the session only if it is not expired.
// Returns nil, nil if session is expired or does not exist.
func (m *SessionManager) GetValidSession() (*Session, error) {
	session, err := m.Load()
	if err != nil {
		return nil, err
	}
	if session == nil {
		return nil, nil
	}
	if session.IsExpired() {
		return nil, nil
	}
	return session, nil
}

// IsLoggedIn checks whether a valid (non-expired) session exists.
func (m *SessionManager) IsLoggedIn() bool {
	session, err := m.GetValidSession()
	return err == nil && session != nil
}

// Status returns AuthStatus for `c4 auth status`.
func (m *SessionManager) Status() AuthStatus {
	session, err := m.Load()
	if err != nil || session == nil {
		return AuthStatus{LoggedIn: false, Valid: false}
	}

	return AuthStatus{
		LoggedIn:  true,
		UserID:    session.UserID,
		Email:     session.Email,
		Provider:  session.Provider,
		ExpiresIn: session.ExpiresInSeconds(),
		Valid:     !session.IsExpired(),
	}
}

// SessionFile returns the path to the credentials file (for testing).
func (m *SessionManager) SessionFile() string {
	return m.sessionFile
}
