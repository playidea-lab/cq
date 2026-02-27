package store

import (
	"database/sql"
	"testing"
	"time"
)

func TestDeviceSessionCreate(t *testing.T) {
	s := newTestStore(t)

	state := "test-state-1"
	userCode := "ABCD1234"
	codeChallenge := "challenge123"
	supabaseURL := "https://example.supabase.co"
	expiresAt := time.Now().Add(10 * time.Minute)

	if err := s.CreateDeviceSession(state, userCode, codeChallenge, supabaseURL, expiresAt); err != nil {
		t.Fatalf("CreateDeviceSession: %v", err)
	}
}

func TestDeviceSessionGetByState(t *testing.T) {
	s := newTestStore(t)

	state := "test-state-get"
	userCode := "EFGH5678"
	expiresAt := time.Now().Add(10 * time.Minute)

	if err := s.CreateDeviceSession(state, userCode, "challenge", "https://example.supabase.co", expiresAt); err != nil {
		t.Fatalf("CreateDeviceSession: %v", err)
	}

	ds, err := s.GetDeviceSession(state)
	if err != nil {
		t.Fatalf("GetDeviceSession: %v", err)
	}
	if ds.State != state {
		t.Errorf("expected state %q, got %q", state, ds.State)
	}
	if ds.UserCode != userCode {
		t.Errorf("expected user_code %q, got %q", userCode, ds.UserCode)
	}
	if ds.Status != "pending" {
		t.Errorf("expected status pending, got %q", ds.Status)
	}
	if ds.PollCount != 1 {
		t.Errorf("expected poll_count 1 after first get, got %d", ds.PollCount)
	}
}

func TestDeviceSessionPollCountIncrement(t *testing.T) {
	s := newTestStore(t)

	state := "test-state-poll"
	if err := s.CreateDeviceSession(state, "POLL1234", "challenge", "https://example.supabase.co", time.Now().Add(10*time.Minute)); err != nil {
		t.Fatalf("CreateDeviceSession: %v", err)
	}

	for i := 1; i <= 5; i++ {
		ds, err := s.GetDeviceSession(state)
		if err != nil {
			t.Fatalf("GetDeviceSession call %d: %v", i, err)
		}
		if ds.PollCount != i {
			t.Errorf("expected poll_count %d, got %d", i, ds.PollCount)
		}
	}
}

func TestDeviceSessionExpireAfterMaxPolls(t *testing.T) {
	s := newTestStore(t)

	state := "test-state-maxpoll"
	if err := s.CreateDeviceSession(state, "MAXP1234", "challenge", "https://example.supabase.co", time.Now().Add(10*time.Minute)); err != nil {
		t.Fatalf("CreateDeviceSession: %v", err)
	}

	// Poll 20 times — on the 21st get, status should be expired (poll_count > 20).
	for i := 0; i < 20; i++ {
		_, err := s.GetDeviceSession(state)
		if err != nil {
			t.Fatalf("GetDeviceSession call %d: %v", i+1, err)
		}
	}

	// 21st call: poll_count becomes 21 > 20 → status = expired → should return ErrNoRows.
	_, err := s.GetDeviceSession(state)
	if err != sql.ErrNoRows {
		t.Errorf("expected sql.ErrNoRows after max polls, got %v", err)
	}
}

func TestDeviceSessionExpired(t *testing.T) {
	s := newTestStore(t)

	state := "test-state-expired"
	// Already expired.
	expiresAt := time.Now().Add(-1 * time.Minute)
	if err := s.CreateDeviceSession(state, "EXPR1234", "challenge", "https://example.supabase.co", expiresAt); err != nil {
		t.Fatalf("CreateDeviceSession: %v", err)
	}

	_, err := s.GetDeviceSession(state)
	if err != sql.ErrNoRows {
		t.Errorf("expected sql.ErrNoRows for expired session, got %v", err)
	}
}

func TestDeviceSessionNotFound(t *testing.T) {
	s := newTestStore(t)

	_, err := s.GetDeviceSession("nonexistent-state")
	if err != sql.ErrNoRows {
		t.Errorf("expected sql.ErrNoRows, got %v", err)
	}
}

func TestDeviceSessionGetByUserCode(t *testing.T) {
	s := newTestStore(t)

	state := "test-state-bycode"
	userCode := "BYCO1234"
	if err := s.CreateDeviceSession(state, userCode, "challenge", "https://example.supabase.co", time.Now().Add(10*time.Minute)); err != nil {
		t.Fatalf("CreateDeviceSession: %v", err)
	}

	ds, err := s.GetDeviceSessionByUserCode(userCode)
	if err != nil {
		t.Fatalf("GetDeviceSessionByUserCode: %v", err)
	}
	if ds.State != state {
		t.Errorf("expected state %q, got %q", state, ds.State)
	}
}

func TestDeviceSessionSetAuthCode(t *testing.T) {
	s := newTestStore(t)

	state := "test-state-authcode"
	if err := s.CreateDeviceSession(state, "AUTH1234", "challenge", "https://example.supabase.co", time.Now().Add(10*time.Minute)); err != nil {
		t.Fatalf("CreateDeviceSession: %v", err)
	}

	if err := s.SetDeviceSessionAuthCode(state, "auth-code-xyz"); err != nil {
		t.Fatalf("SetDeviceSessionAuthCode: %v", err)
	}

	ds, err := s.GetDeviceSessionByUserCode("AUTH1234")
	if err != nil {
		t.Fatalf("GetDeviceSessionByUserCode: %v", err)
	}
	if ds.Status != "ready" {
		t.Errorf("expected status ready, got %q", ds.Status)
	}
	if ds.AuthCode != "auth-code-xyz" {
		t.Errorf("expected auth_code %q, got %q", "auth-code-xyz", ds.AuthCode)
	}
}

func TestDeviceSessionSetAuthCodeIdempotent(t *testing.T) {
	s := newTestStore(t)

	state := "test-state-idempotent"
	if err := s.CreateDeviceSession(state, "IDEM1234", "challenge", "https://example.supabase.co", time.Now().Add(10*time.Minute)); err != nil {
		t.Fatalf("CreateDeviceSession: %v", err)
	}

	// Set twice — should not error.
	for i := 0; i < 2; i++ {
		if err := s.SetDeviceSessionAuthCode(state, "auth-code-xyz"); err != nil {
			t.Fatalf("SetDeviceSessionAuthCode call %d: %v", i+1, err)
		}
	}
}

func TestDeviceSessionSetCSRF(t *testing.T) {
	s := newTestStore(t)

	state := "test-state-csrf"
	if err := s.CreateDeviceSession(state, "CSRF1234", "challenge", "https://example.supabase.co", time.Now().Add(10*time.Minute)); err != nil {
		t.Fatalf("CreateDeviceSession: %v", err)
	}

	csrfToken := "deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef"
	if err := s.SetDeviceSessionCSRF(state, csrfToken); err != nil {
		t.Fatalf("SetDeviceSessionCSRF: %v", err)
	}

	ds, err := s.GetDeviceSessionByUserCode("CSRF1234")
	if err != nil {
		t.Fatalf("GetDeviceSessionByUserCode: %v", err)
	}
	if ds.CSRFToken != csrfToken {
		t.Errorf("expected csrf_token %q, got %q", csrfToken, ds.CSRFToken)
	}
}

func TestDeviceSessionUserCodeUnique(t *testing.T) {
	s := newTestStore(t)

	// Create first session with a specific user_code.
	userCode := "UNIQ1234"
	if err := s.CreateDeviceSession("state-1", userCode, "challenge", "https://example.supabase.co", time.Now().Add(10*time.Minute)); err != nil {
		t.Fatalf("CreateDeviceSession 1: %v", err)
	}

	// Try to create a second session with the same user_code — should retry and succeed with a new code.
	// (The retry logic generates a new userCode on conflict.)
	if err := s.CreateDeviceSession("state-2", userCode, "challenge2", "https://example.supabase.co", time.Now().Add(10*time.Minute)); err != nil {
		t.Fatalf("CreateDeviceSession 2 (retry on conflict): %v", err)
	}

	// Verify both sessions exist with distinct user_codes.
	ds1, err := s.GetDeviceSession("state-1")
	if err != nil {
		t.Fatalf("GetDeviceSession state-1: %v", err)
	}
	ds2, err := s.GetDeviceSession("state-2")
	if err != nil {
		t.Fatalf("GetDeviceSession state-2: %v", err)
	}
	if ds1.UserCode == ds2.UserCode {
		t.Errorf("expected distinct user_codes, both got %q", ds1.UserCode)
	}
}

func TestDeviceSessionCleanupExpired(t *testing.T) {
	s := newTestStore(t)

	// Create an expired session.
	expiredState := "expired-state"
	if err := s.CreateDeviceSession(expiredState, "CLEA1234", "challenge", "https://example.supabase.co", time.Now().Add(-time.Minute)); err != nil {
		t.Fatalf("CreateDeviceSession expired: %v", err)
	}

	// Create a valid session — triggers cleanup of expired rows.
	if err := s.CreateDeviceSession("valid-state", "VALI1234", "challenge", "https://example.supabase.co", time.Now().Add(10*time.Minute)); err != nil {
		t.Fatalf("CreateDeviceSession valid: %v", err)
	}

	// Expired session should be cleaned up.
	ds, err := s.GetDeviceSessionByUserCode("CLEA1234")
	if err == nil {
		t.Errorf("expected error for cleaned-up expired session, got ds: %+v", ds)
	}
}
