package persona

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRecordSessionMetrics_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", "growth_metrics.yaml")

	err := RecordSessionMetrics(path, "sess-001", 3, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatal("file was not created")
	}
}

func TestRecordSessionMetrics_AppendsEntry(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "growth_metrics.yaml")

	if err := RecordSessionMetrics(path, "sess-001", 2, 8); err != nil {
		t.Fatalf("first record: %v", err)
	}
	if err := RecordSessionMetrics(path, "sess-002", 1, 5); err != nil {
		t.Fatalf("second record: %v", err)
	}

	entries, err := loadMetricEntries(path)
	if err != nil {
		t.Fatalf("load entries: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].SessionID != "sess-001" {
		t.Errorf("expected sess-001, got %s", entries[0].SessionID)
	}
	if entries[1].SessionID != "sess-002" {
		t.Errorf("expected sess-002, got %s", entries[1].SessionID)
	}
	if entries[0].Corrections != 2 || entries[0].Suggestions != 8 {
		t.Errorf("entry[0] corrections=%d suggestions=%d", entries[0].Corrections, entries[0].Suggestions)
	}
}

func TestCalcCorrectionRate_BasicCalculation(t *testing.T) {
	now := time.Now().UTC()
	entries := []MetricEntry{
		{SessionID: "s1", Corrections: 4, Suggestions: 20, Date: now.Add(-24 * time.Hour)},
		{SessionID: "s2", Corrections: 6, Suggestions: 30, Date: now.Add(-48 * time.Hour)},
	}

	rate := CalcCorrectionRate(entries, 30)
	// (4+6)/(20+30) = 10/50 = 0.2
	if rate < 0.199 || rate > 0.201 {
		t.Errorf("expected rate ~0.2, got %f", rate)
	}
}

func TestCalcCorrectionRate_NoEntries_ReturnsZero(t *testing.T) {
	rate := CalcCorrectionRate(nil, 30)
	if rate != 0.0 {
		t.Errorf("expected 0.0, got %f", rate)
	}
}

func TestCalcCorrectionRate_ZeroSuggestions_ReturnsZero(t *testing.T) {
	now := time.Now().UTC()
	entries := []MetricEntry{
		{SessionID: "s1", Corrections: 5, Suggestions: 0, Date: now.Add(-24 * time.Hour)},
	}

	rate := CalcCorrectionRate(entries, 30)
	if rate != 0.0 {
		t.Errorf("expected 0.0, got %f", rate)
	}
}

func TestCalcCorrectionRate_FiltersOutsideWindow(t *testing.T) {
	now := time.Now().UTC()
	entries := []MetricEntry{
		{SessionID: "s1", Corrections: 10, Suggestions: 20, Date: now.Add(-40 * 24 * time.Hour)}, // outside 30d
		{SessionID: "s2", Corrections: 2, Suggestions: 10, Date: now.Add(-5 * 24 * time.Hour)},   // inside 30d
	}

	rate := CalcCorrectionRate(entries, 30)
	// only s2: 2/10 = 0.2
	if rate < 0.199 || rate > 0.201 {
		t.Errorf("expected rate ~0.2, got %f", rate)
	}
}

func TestLoadGrowthSummary_WithTrend(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "growth_metrics.yaml")

	now := time.Now().UTC()
	// previous 30d: high correction rate
	// current 30d: lower correction rate => improving
	entries := []MetricEntry{
		// 31-60 days ago (previous period)
		{SessionID: "old1", Corrections: 8, Suggestions: 10, Date: now.Add(-45 * 24 * time.Hour)},
		// last 30 days (current period)
		{SessionID: "new1", Corrections: 2, Suggestions: 10, Date: now.Add(-5 * 24 * time.Hour)},
	}

	if err := saveMetricEntries(entries, path); err != nil {
		t.Fatalf("save entries: %v", err)
	}

	summary, err := LoadGrowthSummary(path)
	if err != nil {
		t.Fatalf("load summary: %v", err)
	}

	if summary.TotalSessions != 2 {
		t.Errorf("expected 2 sessions, got %d", summary.TotalSessions)
	}
	// current rate 0.2, previous rate 0.8 => improving
	if summary.TrendDirection != "improving" {
		t.Errorf("expected improving, got %s", summary.TrendDirection)
	}
	if summary.CorrectionRate30d < 0.199 || summary.CorrectionRate30d > 0.201 {
		t.Errorf("expected CorrectionRate30d ~0.2, got %f", summary.CorrectionRate30d)
	}
}

func TestLoadGrowthSummary_NoEntries(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "growth_metrics.yaml")

	summary, err := LoadGrowthSummary(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if summary.TotalSessions != 0 {
		t.Errorf("expected 0 sessions, got %d", summary.TotalSessions)
	}
	if summary.TrendDirection != "stable" {
		t.Errorf("expected stable, got %s", summary.TrendDirection)
	}
	if summary.CorrectionRate30d != 0.0 {
		t.Errorf("expected 0.0 rate, got %f", summary.CorrectionRate30d)
	}
}
