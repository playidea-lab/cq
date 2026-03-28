package persona

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLedger_NormalizeKey(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"  Hello  ", "hello"},
		{"UPPERCASE", "uppercase"},
		{"already_lower", "already_lower"},
		{" Mixed Case ", "mixed case"},
		{"", ""},
	}
	for _, tc := range cases {
		got := normalizeKey(tc.input)
		if got != tc.expected {
			t.Errorf("normalizeKey(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}

func TestLedger_LoadEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ledger.yaml")

	ledger, err := loadLedger(path)
	if err != nil {
		t.Fatalf("loadLedger on missing file: %v", err)
	}
	if len(ledger) != 0 {
		t.Errorf("expected empty ledger, got %d entries", len(ledger))
	}
}

func TestLedger_SaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ledger.yaml")

	ledger := make(PreferenceLedger)
	now := time.Now().UTC().Truncate(time.Second)
	ledger["bullet_points"] = &LedgerEntry{
		Count:     3,
		FirstSeen: now,
		LastSeen:  now,
	}

	if err := saveLedger(ledger, path); err != nil {
		t.Fatalf("saveLedger: %v", err)
	}

	loaded, err := loadLedger(path)
	if err != nil {
		t.Fatalf("loadLedger: %v", err)
	}

	entry, ok := loaded["bullet_points"]
	if !ok {
		t.Fatal("expected 'bullet_points' key in loaded ledger")
	}
	if entry.Count != 3 {
		t.Errorf("Count = %d, want 3", entry.Count)
	}
}

func TestLedger_IncrementPreferences_NewKey(t *testing.T) {
	ledger := make(PreferenceLedger)
	before := time.Now().UTC()

	incrementPreferences(ledger, []string{"concise", "bullets"})

	after := time.Now().UTC()

	if len(ledger) != 2 {
		t.Errorf("expected 2 entries, got %d", len(ledger))
	}

	e := ledger["concise"]
	if e == nil {
		t.Fatal("missing 'concise' entry")
	}
	if e.Count != 1 {
		t.Errorf("Count = %d, want 1", e.Count)
	}
	if e.FirstSeen.Before(before) || e.FirstSeen.After(after) {
		t.Errorf("FirstSeen %v not within [%v, %v]", e.FirstSeen, before, after)
	}
	if e.LastSeen.Before(before) || e.LastSeen.After(after) {
		t.Errorf("LastSeen %v not within [%v, %v]", e.LastSeen, before, after)
	}
}

func TestLedger_IncrementPreferences_ExistingKey(t *testing.T) {
	ledger := make(PreferenceLedger)
	first := time.Now().UTC().Add(-1 * time.Hour)
	ledger["tone_soft"] = &LedgerEntry{
		Count:     2,
		FirstSeen: first,
		LastSeen:  first,
	}

	incrementPreferences(ledger, []string{"tone_soft"})

	e := ledger["tone_soft"]
	if e.Count != 3 {
		t.Errorf("Count = %d, want 3", e.Count)
	}
	if !e.FirstSeen.Equal(first) {
		t.Errorf("FirstSeen changed: got %v, want %v", e.FirstSeen, first)
	}
	if !e.LastSeen.After(first) {
		t.Errorf("LastSeen should be after first increment time")
	}
}

func TestLedger_IncrementPreferences_NormalizesKeys(t *testing.T) {
	ledger := make(PreferenceLedger)

	incrementPreferences(ledger, []string{"  Tone Soft  ", "TONE SOFT"})

	// Both should normalize to "tone soft"
	if len(ledger) != 1 {
		t.Errorf("expected 1 unique normalized entry, got %d", len(ledger))
	}
	e, ok := ledger["tone soft"]
	if !ok {
		t.Fatal("expected key 'tone soft'")
	}
	if e.Count != 2 {
		t.Errorf("Count = %d, want 2", e.Count)
	}
}

func TestLedger_IncrementPreferences_EmptyKeySkipped(t *testing.T) {
	ledger := make(PreferenceLedger)

	incrementPreferences(ledger, []string{"", "   ", "valid"})

	if len(ledger) != 1 {
		t.Errorf("expected 1 entry (empty keys skipped), got %d", len(ledger))
	}
}

func TestLedger_IncrementAndSaveAt(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "ledger.yaml")

	err := IncrementAndSaveAt([]string{"brevity", "bullets"}, path)
	if err != nil {
		t.Fatalf("IncrementAndSaveAt: %v", err)
	}

	// File should exist
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("ledger file not created: %v", err)
	}

	err = IncrementAndSaveAt([]string{"brevity"}, path)
	if err != nil {
		t.Fatalf("second IncrementAndSaveAt: %v", err)
	}

	ledger, err := LoadLedgerAt(path)
	if err != nil {
		t.Fatalf("LoadLedgerAt: %v", err)
	}

	if ledger["brevity"].Count != 2 {
		t.Errorf("brevity Count = %d, want 2", ledger["brevity"].Count)
	}
	if ledger["bullets"].Count != 1 {
		t.Errorf("bullets Count = %d, want 1", ledger["bullets"].Count)
	}
}

func TestLedger_SuppressedFieldPreserved(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ledger.yaml")

	ledger := make(PreferenceLedger)
	now := time.Now().UTC()
	ledger["verbose"] = &LedgerEntry{
		Count:      5,
		FirstSeen:  now,
		LastSeen:   now,
		Suppressed: true,
	}
	if err := saveLedger(ledger, path); err != nil {
		t.Fatalf("saveLedger: %v", err)
	}

	loaded, err := loadLedger(path)
	if err != nil {
		t.Fatalf("loadLedger: %v", err)
	}
	if !loaded["verbose"].Suppressed {
		t.Error("Suppressed flag not preserved after save/load")
	}
}
