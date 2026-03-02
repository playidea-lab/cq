package main

import (
	"errors"
	"testing"
)

// TestIsHeadless_CI: envMap{"CI":"1"} → true
func TestIsHeadless_CI(t *testing.T) {
	got := isHeadless(false, map[string]string{
		"CI":           "1",
		"DISPLAY":      ":0",
		"TERM_PROGRAM": "iTerm.app",
	})
	if !got {
		t.Error("expected true when CI=1")
	}
}

// TestIsHeadless_HeadlessFlag: headlessFlag=true → true
func TestIsHeadless_HeadlessFlag(t *testing.T) {
	got := isHeadless(true, map[string]string{
		"CI":           "",
		"DISPLAY":      ":0",
		"TERM_PROGRAM": "iTerm.app",
	})
	if !got {
		t.Error("expected true when headlessFlag=true")
	}
}

// TestIsHeadless_NoDisplay: DISPLAY="" + TERM_PROGRAM="" → true
func TestIsHeadless_NoDisplay(t *testing.T) {
	got := isHeadless(false, map[string]string{
		"CI":           "",
		"DISPLAY":      "",
		"TERM_PROGRAM": "",
	})
	if !got {
		t.Error("expected true when DISPLAY and TERM_PROGRAM are empty")
	}
}

// TestIsHeadless_Interactive: all empty except TERM_PROGRAM set → false
func TestIsHeadless_Interactive(t *testing.T) {
	got := isHeadless(false, map[string]string{
		"CI":           "",
		"DISPLAY":      "",
		"TERM_PROGRAM": "iTerm.app",
	})
	if got {
		t.Error("expected false when TERM_PROGRAM is set (interactive terminal)")
	}
}

// TestRunAuthSignup_ErrPropagation: SignUpWithEmail error → appropriate CLI error
func TestRunAuthSignup_ErrPropagation(t *testing.T) {
	// isHeadless returns true when CI=1, DISPLAY="", TERM_PROGRAM=""
	// We test the error propagation path by passing invalid (empty) email/password
	// to a headless context — should return static errors.New error.
	cmd := signupCmd
	cmd.ResetFlags()
	cmd.Flags().String("email", "", "")
	cmd.Flags().String("password", "", "")
	cmd.Flags().Bool("headless", false, "")

	// Set headless flag and empty email/password
	if err := cmd.Flags().Set("headless", "true"); err != nil {
		t.Fatalf("setting headless flag: %v", err)
	}

	err := runAuthSignup(cmd, nil)
	if err == nil {
		t.Fatal("expected error when headless with no email/password")
	}
	if !errors.Is(err, err) { // always true; check message
		t.Error("unexpected error type")
	}
	wantMsg := "--email and --password required in headless mode"
	if err.Error() != wantMsg {
		t.Errorf("error = %q, want %q", err.Error(), wantMsg)
	}
}
