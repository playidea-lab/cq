package cloud

import (
	"fmt"
	"sync"
	"time"
)

// refreshMargin is how far before expiry to proactively refresh (5 minutes).
const refreshMargin = 5 * time.Minute

// TokenRefresher is the interface needed to refresh an access token.
// *AuthClient satisfies this interface.
type TokenRefresher interface {
	RefreshToken() (*Session, error)
}

// TokenProvider manages an access token with automatic proactive refresh.
// It is safe for concurrent use.
type TokenProvider struct {
	mu         sync.Mutex
	token      string
	expiresAt  int64          // Unix timestamp
	refresher  TokenRefresher // nil = static mode (no refresh)
	refreshing bool           // dedup concurrent refresh calls
}

// NewTokenProvider creates a TokenProvider with a refresher for automatic token renewal.
// When the token is within refreshMargin of expiry, Token() will proactively refresh.
func NewTokenProvider(token string, expiresAt int64, refresher TokenRefresher) *TokenProvider {
	return &TokenProvider{
		token:     token,
		expiresAt: expiresAt,
		refresher: refresher,
	}
}

// NewStaticTokenProvider creates a TokenProvider with a fixed token that never refreshes.
// Use this when cloud auth is disabled or for testing.
func NewStaticTokenProvider(token string) *TokenProvider {
	return &TokenProvider{
		token:     token,
		expiresAt: 0, // never expires in static mode
	}
}

// Token returns the current access token.
// If the token is near expiry and a refresher is available, it proactively refreshes.
func (p *TokenProvider) Token() string {
	p.mu.Lock()

	// Static mode: no refresher, return as-is
	if p.refresher == nil {
		token := p.token
		p.mu.Unlock()
		return token
	}

	// Check if refresh is needed (within margin of expiry)
	now := time.Now().Unix()
	needsRefresh := p.expiresAt > 0 && now >= p.expiresAt-int64(refreshMargin.Seconds())

	if !needsRefresh || p.refreshing {
		token := p.token
		p.mu.Unlock()
		return token
	}

	// Mark as refreshing to dedup concurrent calls
	p.refreshing = true
	refresher := p.refresher
	p.mu.Unlock()

	// Do refresh outside lock
	session, err := refresher.RefreshToken()

	p.mu.Lock()
	p.refreshing = false
	if err == nil && session != nil {
		p.token = session.AccessToken
		p.expiresAt = session.ExpiresAt
	}
	token := p.token
	p.mu.Unlock()

	return token
}

// Refresh forces an immediate token refresh, regardless of expiry.
// Returns the new token. Concurrent calls are deduplicated — only one refresh runs at a time.
func (p *TokenProvider) Refresh() (string, error) {
	p.mu.Lock()

	if p.refresher == nil {
		token := p.token
		p.mu.Unlock()
		return token, nil
	}

	if p.refreshing {
		// Another goroutine is already refreshing — wait briefly and return current
		token := p.token
		p.mu.Unlock()
		return token, nil
	}

	p.refreshing = true
	refresher := p.refresher
	p.mu.Unlock()

	session, err := refresher.RefreshToken()

	p.mu.Lock()
	p.refreshing = false
	if err != nil {
		token := p.token
		p.mu.Unlock()
		return token, fmt.Errorf("token refresh: %w", err)
	}

	p.token = session.AccessToken
	p.expiresAt = session.ExpiresAt
	token := p.token
	p.mu.Unlock()

	return token, nil
}
