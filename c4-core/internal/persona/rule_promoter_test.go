package persona

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// buildLedger creates a PreferenceLedger from a simple map of key→(count, suppressed).
func buildLedger(entries map[string]struct {
	count      int
	suppressed bool
}) PreferenceLedger {
	now := time.Now().UTC()
	ledger := make(PreferenceLedger, len(entries))
	for k, v := range entries {
		ledger[k] = &LedgerEntry{
			Count:      v.count,
			FirstSeen:  now,
			LastSeen:   now,
			Suppressed: v.suppressed,
		}
	}
	return ledger
}

// TestPromoteHints_AddsNewHints verifies that an entry with count >= 3 that is
// not suppressed gets added to the Auto-Learned section of claude.md.
func TestPromoteHints_AddsNewHints(t *testing.T) {
	dir := t.TempDir()
	claudeMd := filepath.Join(dir, "CLAUDE.md")

	ledger := buildLedger(map[string]struct {
		count      int
		suppressed bool
	}{
		"prefer-short-functions": {count: 3, suppressed: false},
	})

	added, err := PromoteHints(ledger, claudeMd)
	if err != nil {
		t.Fatalf("PromoteHints: %v", err)
	}
	if len(added) != 1 {
		t.Fatalf("expected 1 added hint, got %d: %v", len(added), added)
	}
	if added[0] != "prefer-short-functions" {
		t.Errorf("expected key 'prefer-short-functions', got %q", added[0])
	}

	content, err := os.ReadFile(claudeMd)
	if err != nil {
		t.Fatalf("read claude.md: %v", err)
	}
	if !strings.Contains(string(content), "## Auto-Learned") {
		t.Error("claude.md missing '## Auto-Learned' section")
	}
	if !strings.Contains(string(content), "- [auto] prefer-short-functions") {
		t.Errorf("claude.md missing hint line; content:\n%s", content)
	}
}

// TestPromoteHints_SkipsSuppressed verifies that a suppressed entry is not
// added even if count >= 3.
func TestPromoteHints_SkipsSuppressed(t *testing.T) {
	dir := t.TempDir()
	claudeMd := filepath.Join(dir, "CLAUDE.md")

	ledger := buildLedger(map[string]struct {
		count      int
		suppressed bool
	}{
		"no-trailing-newlines": {count: 5, suppressed: true},
	})

	added, err := PromoteHints(ledger, claudeMd)
	if err != nil {
		t.Fatalf("PromoteHints: %v", err)
	}
	if len(added) != 0 {
		t.Errorf("expected 0 added hints for suppressed entry, got %d: %v", len(added), added)
	}
}

// TestPromoteHints_SkipsDuplicates verifies that calling PromoteHints twice
// does not duplicate a hint already present in the Auto-Learned section.
func TestPromoteHints_SkipsDuplicates(t *testing.T) {
	dir := t.TempDir()
	claudeMd := filepath.Join(dir, "CLAUDE.md")

	// Pre-populate claude.md with the hint already present.
	existing := "# Project\n\n## Auto-Learned\n\n- [auto] prefer-short-functions\n"
	if err := os.WriteFile(claudeMd, []byte(existing), 0644); err != nil {
		t.Fatalf("write pre-existing claude.md: %v", err)
	}

	ledger := buildLedger(map[string]struct {
		count      int
		suppressed bool
	}{
		"prefer-short-functions": {count: 4, suppressed: false},
	})

	added, err := PromoteHints(ledger, claudeMd)
	if err != nil {
		t.Fatalf("PromoteHints: %v", err)
	}
	if len(added) != 0 {
		t.Errorf("expected 0 added hints (duplicate), got %d: %v", len(added), added)
	}

	// Verify the section was not duplicated.
	content, err := os.ReadFile(claudeMd)
	if err != nil {
		t.Fatalf("read claude.md: %v", err)
	}
	count := strings.Count(string(content), "- [auto] prefer-short-functions")
	if count != 1 {
		t.Errorf("expected hint to appear exactly once, got %d times:\n%s", count, content)
	}
}

// TestPromoteHints_BelowThreshold verifies that entries with count < 3 are not
// promoted.
func TestPromoteHints_BelowThreshold(t *testing.T) {
	dir := t.TempDir()
	claudeMd := filepath.Join(dir, "CLAUDE.md")

	ledger := buildLedger(map[string]struct {
		count      int
		suppressed bool
	}{
		"not-yet-promoted": {count: 2, suppressed: false},
	})

	added, err := PromoteHints(ledger, claudeMd)
	if err != nil {
		t.Fatalf("PromoteHints: %v", err)
	}
	if len(added) != 0 {
		t.Errorf("expected 0 added hints for count<3, got %d: %v", len(added), added)
	}
}

// TestPromoteRules_CreatesFileAndAddsRules verifies that an entry with
// count >= 5 causes auto-learned.md to be created with the rule inside.
func TestPromoteRules_CreatesFileAndAddsRules(t *testing.T) {
	dir := t.TempDir()

	ledger := buildLedger(map[string]struct {
		count      int
		suppressed bool
	}{
		"always-add-context-to-errors": {count: 5, suppressed: false},
	})

	added, err := PromoteRules(ledger, dir)
	if err != nil {
		t.Fatalf("PromoteRules: %v", err)
	}
	if len(added) != 1 {
		t.Fatalf("expected 1 added rule, got %d: %v", len(added), added)
	}
	if added[0] != "always-add-context-to-errors" {
		t.Errorf("expected key 'always-add-context-to-errors', got %q", added[0])
	}

	content, err := os.ReadFile(filepath.Join(dir, "auto-learned.md"))
	if err != nil {
		t.Fatalf("read auto-learned.md: %v", err)
	}
	if !strings.Contains(string(content), "# Auto-Learned Rules") {
		t.Error("auto-learned.md missing header")
	}
	if !strings.Contains(string(content), "- always-add-context-to-errors") {
		t.Errorf("auto-learned.md missing rule line; content:\n%s", content)
	}
}

// TestPromoteRules_SkipsExisting verifies that a rule already present in
// auto-learned.md is not added a second time.
func TestPromoteRules_SkipsExisting(t *testing.T) {
	dir := t.TempDir()
	rulesFile := filepath.Join(dir, "auto-learned.md")

	existing := "# Auto-Learned Rules\n\n> Rules automatically generated from repeated preferences.\n\n- always-add-context-to-errors\n"
	if err := os.WriteFile(rulesFile, []byte(existing), 0644); err != nil {
		t.Fatalf("write pre-existing auto-learned.md: %v", err)
	}

	ledger := buildLedger(map[string]struct {
		count      int
		suppressed bool
	}{
		"always-add-context-to-errors": {count: 7, suppressed: false},
	})

	added, err := PromoteRules(ledger, dir)
	if err != nil {
		t.Fatalf("PromoteRules: %v", err)
	}
	if len(added) != 0 {
		t.Errorf("expected 0 added rules (duplicate), got %d: %v", len(added), added)
	}

	content, err := os.ReadFile(rulesFile)
	if err != nil {
		t.Fatalf("read auto-learned.md: %v", err)
	}
	cnt := strings.Count(string(content), "- always-add-context-to-errors")
	if cnt != 1 {
		t.Errorf("expected rule to appear once, got %d times:\n%s", cnt, content)
	}
}

// TestPromoteRules_SkipsSuppressed verifies that suppressed entries are not
// promoted to rules even when count >= 5.
func TestPromoteRules_SkipsSuppressed(t *testing.T) {
	dir := t.TempDir()

	ledger := buildLedger(map[string]struct {
		count      int
		suppressed bool
	}{
		"suppressed-rule": {count: 10, suppressed: true},
	})

	added, err := PromoteRules(ledger, dir)
	if err != nil {
		t.Fatalf("PromoteRules: %v", err)
	}
	if len(added) != 0 {
		t.Errorf("expected 0 added rules for suppressed entry, got %d: %v", len(added), added)
	}
}

// TestSuppressKey verifies that SuppressKey loads the ledger, marks the key
// suppressed, and persists the change.
func TestSuppressKey(t *testing.T) {
	dir := t.TempDir()
	ledgerPath := filepath.Join(dir, "ledger.yaml")

	// Seed a ledger with one active entry.
	initial := PreferenceLedger{
		"no-magic-numbers": {Count: 4, Suppressed: false},
	}
	if err := saveLedger(initial, ledgerPath); err != nil {
		t.Fatalf("saveLedger: %v", err)
	}

	if err := SuppressKey(ledgerPath, "no-magic-numbers"); err != nil {
		t.Fatalf("SuppressKey: %v", err)
	}

	loaded, err := loadLedger(ledgerPath)
	if err != nil {
		t.Fatalf("loadLedger after suppress: %v", err)
	}
	entry, ok := loaded["no-magic-numbers"]
	if !ok {
		t.Fatal("entry missing after SuppressKey")
	}
	if !entry.Suppressed {
		t.Error("expected Suppressed=true after SuppressKey")
	}
}

// TestSuppressKey_NormalizesKey verifies that the key is normalized before
// suppression (whitespace + case).
func TestSuppressKey_NormalizesKey(t *testing.T) {
	dir := t.TempDir()
	ledgerPath := filepath.Join(dir, "ledger.yaml")

	initial := PreferenceLedger{
		"no-magic-numbers": {Count: 4, Suppressed: false},
	}
	if err := saveLedger(initial, ledgerPath); err != nil {
		t.Fatalf("saveLedger: %v", err)
	}

	// Supply the key with mixed case and leading/trailing spaces.
	if err := SuppressKey(ledgerPath, "  No-Magic-Numbers  "); err != nil {
		t.Fatalf("SuppressKey: %v", err)
	}

	loaded, err := loadLedger(ledgerPath)
	if err != nil {
		t.Fatalf("loadLedger: %v", err)
	}
	entry, ok := loaded["no-magic-numbers"]
	if !ok {
		t.Fatal("normalized entry missing after SuppressKey")
	}
	if !entry.Suppressed {
		t.Error("expected Suppressed=true after SuppressKey with normalized key")
	}
}
