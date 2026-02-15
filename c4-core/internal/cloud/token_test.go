package cloud

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// mockRefresher implements TokenRefresher for testing.
type mockRefresher struct {
	mu       sync.Mutex
	calls    int
	session  *Session
	err      error
	delay    time.Duration
	callback func() // called on each RefreshToken call
}

func (m *mockRefresher) RefreshToken() (*Session, error) {
	m.mu.Lock()
	m.calls++
	d := m.delay
	cb := m.callback
	s := m.session
	e := m.err
	m.mu.Unlock()

	if cb != nil {
		cb()
	}
	if d > 0 {
		time.Sleep(d)
	}
	return s, e
}

func (m *mockRefresher) callCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}

func TestTokenProviderStaticMode(t *testing.T) {
	tp := NewStaticTokenProvider("static-token")

	got := tp.Token()
	if got != "static-token" {
		t.Fatalf("expected 'static-token', got %q", got)
	}

	// Refresh in static mode returns current token without error
	refreshed, err := tp.Refresh()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if refreshed != "static-token" {
		t.Fatalf("expected 'static-token', got %q", refreshed)
	}
}

func TestTokenProviderReturnsCurrentToken(t *testing.T) {
	// Token that expires far in the future — no refresh needed
	expiresAt := time.Now().Add(1 * time.Hour).Unix()
	refresher := &mockRefresher{
		session: &Session{AccessToken: "new-token", ExpiresAt: time.Now().Add(2 * time.Hour).Unix()},
	}

	tp := NewTokenProvider("current-token", expiresAt, refresher)

	got := tp.Token()
	if got != "current-token" {
		t.Fatalf("expected 'current-token', got %q", got)
	}

	// Refresher should NOT have been called (token not near expiry)
	if refresher.callCount() != 0 {
		t.Fatalf("expected 0 refresh calls, got %d", refresher.callCount())
	}
}

func TestTokenProviderRefreshesNearExpiry(t *testing.T) {
	// Token expires in 2 minutes — within the 5-minute refresh margin
	expiresAt := time.Now().Add(2 * time.Minute).Unix()
	newExpiry := time.Now().Add(1 * time.Hour).Unix()
	refresher := &mockRefresher{
		session: &Session{AccessToken: "refreshed-token", ExpiresAt: newExpiry},
	}

	tp := NewTokenProvider("old-token", expiresAt, refresher)

	got := tp.Token()
	if got != "refreshed-token" {
		t.Fatalf("expected 'refreshed-token', got %q", got)
	}

	if refresher.callCount() != 1 {
		t.Fatalf("expected 1 refresh call, got %d", refresher.callCount())
	}

	// Subsequent call should use the new token without refreshing again
	got2 := tp.Token()
	if got2 != "refreshed-token" {
		t.Fatalf("expected 'refreshed-token' on second call, got %q", got2)
	}

	// No additional refresh calls (new expiry is far in future)
	if refresher.callCount() != 1 {
		t.Fatalf("expected 1 total refresh calls, got %d", refresher.callCount())
	}
}

func TestTokenProviderConcurrentRefresh(t *testing.T) {
	// Token already expired
	expiresAt := time.Now().Add(-1 * time.Minute).Unix()
	var refreshCount atomic.Int32

	refresher := &mockRefresher{
		session: &Session{
			AccessToken: "concurrent-token",
			ExpiresAt:   time.Now().Add(1 * time.Hour).Unix(),
		},
		delay: 50 * time.Millisecond, // simulate slow refresh
		callback: func() {
			refreshCount.Add(1)
		},
	}

	tp := NewTokenProvider("expired-token", expiresAt, refresher)

	// Launch concurrent Token() calls
	const n = 10
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			tp.Token()
		}()
	}
	wg.Wait()

	// Due to dedup, only 1 refresh should have executed
	// (others see refreshing=true and return old token)
	count := refreshCount.Load()
	if count > 2 {
		t.Fatalf("expected at most 2 refresh calls (dedup), got %d", count)
	}
}

func TestTokenProviderRefreshFailure(t *testing.T) {
	// Token near expiry
	expiresAt := time.Now().Add(1 * time.Minute).Unix()
	refresher := &mockRefresher{
		err: fmt.Errorf("network error"),
	}

	tp := NewTokenProvider("old-token", expiresAt, refresher)

	// Token() should return old token even if refresh fails
	got := tp.Token()
	if got != "old-token" {
		t.Fatalf("expected 'old-token' on refresh failure, got %q", got)
	}

	// Explicit Refresh() should return error
	_, err := tp.Refresh()
	if err == nil {
		t.Fatal("expected error from Refresh()")
	}
}
