package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/changmin/c4-core/internal/botstore"
)

// stubVerifyToken replaces verifyTokenFunc for tests.
func stubVerify(info botstore.BotInfo) func(string) (botstore.BotInfo, error) {
	return func(token string) (botstore.BotInfo, error) {
		return info, nil
	}
}

func TestSetupWizard_HappyPath(t *testing.T) {
	// Arrange: stub token verify, redirect store to temp dir
	orig := verifyTokenFunc
	t.Cleanup(func() { verifyTokenFunc = orig })

	verifyTokenFunc = stubVerify(botstore.BotInfo{
		ID:        42,
		Username:  "testcqbot",
		FirstName: "TestCQ",
	})

	// Save original projectDir and redirect to temp
	origDir := projectDir
	projectDir = t.TempDir()
	t.Cleanup(func() { projectDir = origDir })

	// Simulate user input: Enter (step 1), token, chat ID
	input := "\ntest-token-123\n987654321\n"
	r := strings.NewReader(input)
	var out bytes.Buffer

	err := runSetupWizard(r, &out)
	if err != nil {
		t.Fatalf("runSetupWizard returned error: %v", err)
	}

	outStr := out.String()

	// Verify wizard output contains key checkpoints
	if !strings.Contains(outStr, "BotFather") {
		t.Error("output missing BotFather instructions")
	}
	if !strings.Contains(outStr, "@testcqbot") {
		t.Error("output missing bot username")
	}
	if !strings.Contains(outStr, "페어링 완료") {
		t.Error("output missing pairing completion message")
	}
	if !strings.Contains(outStr, "설정 완료") {
		t.Error("output missing final completion message")
	}

	// Verify bot was persisted
	store, err := botstore.New(projectDir)
	if err != nil {
		t.Fatalf("botstore.New: %v", err)
	}
	bot, err := store.Get("testcqbot")
	if err != nil {
		t.Fatalf("store.Get: %v", err)
	}
	if bot.Token != "test-token-123" {
		t.Errorf("token: got %q, want %q", bot.Token, "test-token-123")
	}
	if len(bot.AllowFrom) != 1 || bot.AllowFrom[0] != 987654321 {
		t.Errorf("AllowFrom: got %v, want [987654321]", bot.AllowFrom)
	}
}

func TestSetupWizard_EmptyToken(t *testing.T) {
	orig := verifyTokenFunc
	t.Cleanup(func() { verifyTokenFunc = orig })
	verifyTokenFunc = stubVerify(botstore.BotInfo{})

	origDir := projectDir
	projectDir = t.TempDir()
	t.Cleanup(func() { projectDir = origDir })

	// Step 1 Enter, then empty token
	input := "\n   \n"
	r := strings.NewReader(input)
	var out bytes.Buffer

	err := runSetupWizard(r, &out)
	if err == nil {
		t.Fatal("expected error for empty token, got nil")
	}
	if !strings.Contains(err.Error(), "비어있습니다") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSetupWizard_InvalidChatID(t *testing.T) {
	orig := verifyTokenFunc
	t.Cleanup(func() { verifyTokenFunc = orig })
	verifyTokenFunc = stubVerify(botstore.BotInfo{
		ID:        1,
		Username:  "mybot",
		FirstName: "My",
	})

	origDir := projectDir
	projectDir = t.TempDir()
	t.Cleanup(func() { projectDir = origDir })

	// Step 1 Enter, valid token, invalid chat ID
	input := "\nsome-token\nnot-a-number\n"
	r := strings.NewReader(input)
	var out bytes.Buffer

	err := runSetupWizard(r, &out)
	if err == nil {
		t.Fatal("expected error for invalid chat ID, got nil")
	}
	if !strings.Contains(err.Error(), "파싱 실패") {
		t.Errorf("unexpected error: %v", err)
	}
}
