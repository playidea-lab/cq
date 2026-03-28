package persona

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRecordSessionMetrics_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "metrics.yaml")

	if err := RecordSessionMetrics(path, "sess-1", 2, 5); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected file to exist after RecordSessionMetrics, got: %v", err)
	}
}

func TestRecordSessionMetrics_AppendsEntry(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "metrics.yaml")

	if err := RecordSessionMetrics(path, "sess-1", 1, 3); err != nil {
		t.Fatalf("first record: %v", err)
	}
	if err := RecordSessionMetrics(path, "sess-2", 2, 4); err != nil {
		t.Fatalf("second record: %v", err)
	}

	entries, err := loadMetrics(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].SessionID != "sess-1" {
		t.Errorf("entry[0].SessionID = %q, want %q", entries[0].SessionID, "sess-1")
	}
	if entries[1].SessionID != "sess-2" {
		t.Errorf("entry[1].SessionID = %q, want %q", entries[1].SessionID, "sess-2")
	}
	if entries[1].Corrections != 2 || entries[1].Suggestions != 4 {
		t.Errorf("entry[1] = {%d, %d}, want {2, 4}", entries[1].Corrections, entries[1].Suggestions)
	}
}

func TestCalcCorrectionRate_BasicCalculation(t *testing.T) {
	now := time.Now().UTC()
	entries := []MetricEntry{
		{SessionID: "a", Corrections: 2, Suggestions: 10, Date: now.AddDate(0, 0, -1)},
		{SessionID: "b", Corrections: 3, Suggestions: 10, Date: now.AddDate(0, 0, -2)},
	}

	rate := CalcCorrectionRate(entries, 30)
	// 5 corrections / 20 suggestions = 0.25
	const want = 0.25
	if rate != want {
		t.Errorf("CalcCorrectionRate = %f, want %f", rate, want)
	}
}

func TestCalcCorrectionRate_NoEntries_ReturnsZero(t *testing.T) {
	rate := CalcCorrectionRate(nil, 30)
	if rate != 0.0 {
		t.Errorf("CalcCorrectionRate(nil) = %f, want 0.0", rate)
	}
}

func TestCalcCorrectionRate_ZeroSuggestions_ReturnsZero(t *testing.T) {
	now := time.Now().UTC()
	entries := []MetricEntry{
		{SessionID: "a", Corrections: 5, Suggestions: 0, Date: now.AddDate(0, 0, -1)},
	}
	rate := CalcCorrectionRate(entries, 30)
	if rate != 0.0 {
		t.Errorf("CalcCorrectionRate with zero suggestions = %f, want 0.0", rate)
	}
}

func TestLoadGrowthSummary_WithTrend(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "metrics.yaml")
	now := time.Now().UTC()

	// Previous 30-day window (days -60 to -31): high correction rate (0.5).
	entries := []MetricEntry{
		{SessionID: "old-1", Corrections: 5, Suggestions: 10, Date: now.AddDate(0, 0, -45)},
	}
	// Recent 30-day window: lower correction rate (0.1) → should be "improving".
	entries = append(entries,
		MetricEntry{SessionID: "new-1", Corrections: 1, Suggestions: 10, Date: now.AddDate(0, 0, -5)},
	)

	if err := saveMetrics(entries, path); err != nil {
		t.Fatalf("save: %v", err)
	}

	summary, err := LoadGrowthSummary(path)
	if err != nil {
		t.Fatalf("LoadGrowthSummary: %v", err)
	}

	if summary.TotalSessions != 2 {
		t.Errorf("TotalSessions = %d, want 2", summary.TotalSessions)
	}
	// current30 = 1/10 = 0.1
	const wantRate = 0.1
	if summary.CorrectionRate30d != wantRate {
		t.Errorf("CorrectionRate30d = %f, want %f", summary.CorrectionRate30d, wantRate)
	}
	if summary.TrendDirection != "improving" {
		t.Errorf("TrendDirection = %q, want %q", summary.TrendDirection, "improving")
	}
}
