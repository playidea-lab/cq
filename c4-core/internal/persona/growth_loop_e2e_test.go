package persona

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestGrowthLoop_E2E simulates the full growth loop cycle:
// 1. Accumulate preferences (3 sessions with same preference)
// 2. Verify hint promotion to claude.md
// 3. Accumulate more (5 total)
// 4. Verify rule promotion to auto-learned.md
// 5. Suppress a key and verify it's skipped
// 6. Record growth metrics and verify summary
func TestGrowthLoop_E2E(t *testing.T) {
	tmpDir := t.TempDir()
	ledgerPath := filepath.Join(tmpDir, "preference_ledger.yaml")
	claudeMdPath := filepath.Join(tmpDir, "CLAUDE.md")
	rulesDir := filepath.Join(tmpDir, "rules")
	metricsPath := filepath.Join(tmpDir, "growth_metrics.yaml")

	// Seed claude.md with minimal content
	os.WriteFile(claudeMdPath, []byte("# My Project\n\nSome instructions.\n"), 0644)

	// === Phase 1: Accumulate 3 sessions with same preferences ===
	prefs := []string{"매직 넘버 상수로 추출", "에러 경로 테스트 필수"}
	for i := 0; i < 3; i++ {
		if err := IncrementAndSaveAt(prefs, ledgerPath); err != nil {
			t.Fatalf("session %d: increment failed: %v", i+1, err)
		}
	}

	// Verify ledger counts
	ledger, err := LoadLedgerAt(ledgerPath)
	if err != nil {
		t.Fatalf("load ledger: %v", err)
	}
	for _, key := range prefs {
		k := normalizeKey(key)
		entry := ledger[k]
		if entry == nil {
			t.Fatalf("key %q not found in ledger", k)
		}
		if entry.Count != 3 {
			t.Fatalf("key %q count = %d, want 3", k, entry.Count)
		}
	}
	t.Log("✓ Phase 1: ledger counts = 3 for both preferences")

	// === Phase 2: Promote hints (count >= 3) ===
	hints, err := PromoteHints(ledger, claudeMdPath)
	if err != nil {
		t.Fatalf("promote hints: %v", err)
	}
	if len(hints) != 2 {
		t.Fatalf("expected 2 hints promoted, got %d", len(hints))
	}

	// Verify claude.md has Auto-Learned section
	content, _ := os.ReadFile(claudeMdPath)
	if got := string(content); !containsString(got, "## Auto-Learned") {
		t.Fatal("claude.md missing Auto-Learned section")
	}
	if !containsString(string(content), "[auto] 매직 넘버 상수로 추출") {
		t.Fatal("claude.md missing hint for 매직 넘버")
	}
	t.Log("✓ Phase 2: 2 hints promoted to claude.md")

	// === Phase 3: Accumulate to 5 total ===
	for i := 0; i < 2; i++ {
		if err := IncrementAndSaveAt(prefs, ledgerPath); err != nil {
			t.Fatalf("session %d: increment failed: %v", i+4, err)
		}
	}
	ledger, _ = LoadLedgerAt(ledgerPath)
	if ledger[normalizeKey(prefs[0])].Count != 5 {
		t.Fatal("expected count 5 after 5 sessions")
	}

	// === Phase 4: Promote rules (count >= 5) ===
	rules, err := PromoteRules(ledger, rulesDir)
	if err != nil {
		t.Fatalf("promote rules: %v", err)
	}
	if len(rules) != 2 {
		t.Fatalf("expected 2 rules promoted, got %d", len(rules))
	}

	// Verify auto-learned.md exists
	rulesFile := filepath.Join(rulesDir, "auto-learned.md")
	rulesContent, err := os.ReadFile(rulesFile)
	if err != nil {
		t.Fatalf("read rules file: %v", err)
	}
	if !containsString(string(rulesContent), "매직 넘버 상수로 추출") {
		t.Fatal("auto-learned.md missing rule")
	}
	t.Log("✓ Phase 4: 2 rules promoted to auto-learned.md")

	// === Phase 5: Suppress a key ===
	if err := SuppressKey(ledgerPath, prefs[0]); err != nil {
		t.Fatalf("suppress: %v", err)
	}
	ledger, _ = LoadLedgerAt(ledgerPath)
	if !ledger[normalizeKey(prefs[0])].Suppressed {
		t.Fatal("expected key to be suppressed")
	}

	// Re-promote should skip suppressed key
	hints2, _ := PromoteHints(ledger, claudeMdPath)
	if len(hints2) != 0 {
		t.Fatalf("expected 0 new hints (all either existing or suppressed), got %d", len(hints2))
	}
	t.Log("✓ Phase 5: suppressed key skipped in promotion")

	// === Phase 6: Growth Metrics ===
	for i := 0; i < 5; i++ {
		corrections := 3 - i/2 // decreasing corrections = improving
		if err := RecordSessionMetrics(metricsPath, "session-"+string(rune('a'+i)), corrections, 10); err != nil {
			t.Fatalf("record metrics: %v", err)
		}
	}

	summary, err := LoadGrowthSummary(metricsPath)
	if err != nil {
		t.Fatalf("load growth summary: %v", err)
	}
	if summary.TotalSessions != 5 {
		t.Fatalf("expected 5 sessions, got %d", summary.TotalSessions)
	}
	if summary.CorrectionRate30d <= 0 {
		t.Fatalf("expected positive correction rate, got %f", summary.CorrectionRate30d)
	}
	t.Logf("✓ Phase 6: growth summary — %d sessions, correction rate %.2f, trend %s",
		summary.TotalSessions, summary.CorrectionRate30d, summary.TrendDirection)

	t.Log("✅ Growth Loop E2E: all phases passed")
	_ = time.Now() // use time package to prevent unused import
}

func containsString(haystack, needle string) bool {
	return len(haystack) > 0 && len(needle) > 0 &&
		// simple contains check
		findSubstring(haystack, needle)
}

func findSubstring(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
