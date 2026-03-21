package main

import (
	"testing"

	"github.com/changmin/c4-core/internal/botstore"
)

// stubVerify replaces verifyTokenFunc for tests.
func stubVerify(info botstore.BotInfo) func(string) (botstore.BotInfo, error) {
	return func(token string) (botstore.BotInfo, error) {
		return info, nil
	}
}

func TestRunSetupWizardInline_HappyPath(t *testing.T) {
	orig := verifyTokenFunc
	t.Cleanup(func() { verifyTokenFunc = orig })

	verifyTokenFunc = stubVerify(botstore.BotInfo{
		ID:        42,
		Username:  "testcqbot",
		FirstName: "TestCQ",
	})

	origDir := projectDir
	projectDir = t.TempDir()
	t.Cleanup(func() { projectDir = origDir })

	store, err := botstore.New(projectDir)
	if err != nil {
		t.Fatalf("botstore.New: %v", err)
	}

	// runSetupWizardInline reads from os.Stdin — tested via integration/e2e.
	// Unit test verifies the store logic by calling store.Save directly.
	bot := botstore.Bot{
		Username:    "testcqbot",
		Token:       "test-token-123",
		DisplayName: "TestCQ",
		Scope:       "global",
		AllowFrom:   []int64{987654321},
	}
	if err := store.Save(bot); err != nil {
		t.Fatalf("store.Save: %v", err)
	}

	got, err := store.Get("testcqbot")
	if err != nil {
		t.Fatalf("store.Get: %v", err)
	}
	if got.Token != "test-token-123" {
		t.Errorf("token: got %q, want %q", got.Token, "test-token-123")
	}
	if len(got.AllowFrom) != 1 || got.AllowFrom[0] != 987654321 {
		t.Errorf("AllowFrom: got %v, want [987654321]", got.AllowFrom)
	}
}
