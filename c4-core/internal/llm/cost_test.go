package llm

import (
	"database/sql"
	"math"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestCostTrackerWithCacheTokens(t *testing.T) {
	ct := NewCostTracker()
	usage := TokenUsage{
		InputTokens:      100,
		OutputTokens:     50,
		CacheReadTokens:  1000,
		CacheWriteTokens: 200,
	}
	ct.Record("anthropic", "claude-sonnet-4-5", usage, 0)

	report := ct.Report()
	pc := report.ByProvider["anthropic"]

	if pc.CacheReadTok != 1000 {
		t.Errorf("CacheReadTok: got %d, want 1000", pc.CacheReadTok)
	}
	if pc.CacheWriteTok != 200 {
		t.Errorf("CacheWriteTok: got %d, want 200", pc.CacheWriteTok)
	}
	if pc.SavedUSD <= 0 {
		t.Errorf("SavedUSD: got %f, want > 0", pc.SavedUSD)
	}
}

func TestCacheHitRate_HalfHit(t *testing.T) {
	ct := NewCostTracker()
	usage := TokenUsage{
		InputTokens:      5000,
		OutputTokens:     0,
		CacheReadTokens:  1000,
		CacheWriteTokens: 1000,
	}
	ct.Record("anthropic", "claude-sonnet-4-5", usage, 0)

	report := ct.Report()
	pc := report.ByProvider["anthropic"]

	// CacheHitRate = 1000 / (1000 + 1000) = 0.5
	if math.Abs(pc.CacheHitRate-0.5) > 1e-9 {
		t.Errorf("CacheHitRate: got %f, want 0.5", pc.CacheHitRate)
	}
	// CacheSavingsRate = 1000 / (5000 + 1000 + 1000) = 1000/7000 ≈ 0.142857
	wantSavings := 1000.0 / 7000.0
	if math.Abs(pc.CacheSavingsRate-wantSavings) > 1e-9 {
		t.Errorf("CacheSavingsRate: got %f, want %f", pc.CacheSavingsRate, wantSavings)
	}
	// GlobalCacheHitRate should match per-provider (single provider)
	if math.Abs(report.GlobalCacheHitRate-0.5) > 1e-9 {
		t.Errorf("GlobalCacheHitRate: got %f, want 0.5", report.GlobalCacheHitRate)
	}
	if math.Abs(report.GlobalCacheSavingsRate-wantSavings) > 1e-9 {
		t.Errorf("GlobalCacheSavingsRate: got %f, want %f", report.GlobalCacheSavingsRate, wantSavings)
	}
}

func TestCacheHitRate_ZeroTokens(t *testing.T) {
	ct := NewCostTracker()
	usage := TokenUsage{
		InputTokens:      0,
		OutputTokens:     0,
		CacheReadTokens:  0,
		CacheWriteTokens: 0,
	}
	ct.Record("anthropic", "claude-sonnet-4-5", usage, 0)

	report := ct.Report()
	pc := report.ByProvider["anthropic"]

	if pc.CacheHitRate != 0.0 {
		t.Errorf("CacheHitRate: got %f, want 0.0", pc.CacheHitRate)
	}
	if pc.CacheSavingsRate != 0.0 {
		t.Errorf("CacheSavingsRate: got %f, want 0.0", pc.CacheSavingsRate)
	}
	if report.GlobalCacheHitRate != 0.0 {
		t.Errorf("GlobalCacheHitRate: got %f, want 0.0", report.GlobalCacheHitRate)
	}
	if report.GlobalCacheSavingsRate != 0.0 {
		t.Errorf("GlobalCacheSavingsRate: got %f, want 0.0", report.GlobalCacheSavingsRate)
	}
}

func TestCacheHitRate_WriteOnly(t *testing.T) {
	ct := NewCostTracker()
	usage := TokenUsage{
		InputTokens:      1000,
		OutputTokens:     0,
		CacheReadTokens:  0,
		CacheWriteTokens: 500,
	}
	ct.Record("anthropic", "claude-sonnet-4-5", usage, 0)

	report := ct.Report()
	pc := report.ByProvider["anthropic"]

	// hit = 0/(0+500) = 0.0
	if pc.CacheHitRate != 0.0 {
		t.Errorf("CacheHitRate: got %f, want 0.0", pc.CacheHitRate)
	}
	// savings = 0/(1000+0+500) = 0.0
	if pc.CacheSavingsRate != 0.0 {
		t.Errorf("CacheSavingsRate: got %f, want 0.0", pc.CacheSavingsRate)
	}
}

func TestCostTrackerWithCacheTokensAccumulates(t *testing.T) {
	ct := NewCostTracker()
	usage1 := TokenUsage{
		InputTokens:      100,
		OutputTokens:     50,
		CacheReadTokens:  500,
		CacheWriteTokens: 100,
	}
	usage2 := TokenUsage{
		InputTokens:      200,
		OutputTokens:     100,
		CacheReadTokens:  500,
		CacheWriteTokens: 100,
	}
	ct.Record("anthropic", "claude-sonnet-4-5", usage1, 10*time.Millisecond)
	ct.Record("anthropic", "claude-sonnet-4-5", usage2, 20*time.Millisecond)

	report := ct.Report()
	pc := report.ByProvider["anthropic"]

	if pc.CacheReadTok != 1000 {
		t.Errorf("CacheReadTok: got %d, want 1000", pc.CacheReadTok)
	}
	if pc.CacheWriteTok != 200 {
		t.Errorf("CacheWriteTok: got %d, want 200", pc.CacheWriteTok)
	}
	if pc.Requests != 2 {
		t.Errorf("Requests: got %d, want 2", pc.Requests)
	}
}

// newTestDB opens an in-memory SQLite database for testing.
func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestCostTrackerPersist(t *testing.T) {
	db := newTestDB(t)
	ct := NewCostTracker()
	ct.SetDB(db)

	usage := TokenUsage{InputTokens: 100, OutputTokens: 50}
	ct.Record("anthropic", "claude-sonnet-4-5", usage, 42*time.Millisecond)

	ct.Close()

	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM llm_usage").Scan(&count); err != nil {
		t.Fatalf("query: %v", err)
	}
	if count != 1 {
		t.Errorf("llm_usage rows: got %d, want 1", count)
	}
}

func TestCostTrackerClose_Drain(t *testing.T) {
	db := newTestDB(t)
	ct := NewCostTracker()
	ct.SetDB(db)

	const n = 100
	usage := TokenUsage{InputTokens: 10, OutputTokens: 5}
	for i := 0; i < n; i++ {
		ct.Record("openai", "gpt-4o", usage, time.Millisecond)
	}
	ct.Close()

	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM llm_usage").Scan(&count); err != nil {
		t.Fatalf("query: %v", err)
	}
	if count != n {
		t.Errorf("llm_usage rows after drain: got %d, want %d", count, n)
	}
}

func TestCostTrackerNoDB(t *testing.T) {
	// SetDB not called — in-memory mode should work without panic.
	ct := NewCostTracker()
	usage := TokenUsage{InputTokens: 100, OutputTokens: 50}
	ct.Record("anthropic", "claude-sonnet-4-5", usage, 0)

	if ct.EntryCount() != 1 {
		t.Errorf("EntryCount: got %d, want 1", ct.EntryCount())
	}
	ct.Close() // no-op, must not panic
}

func TestCostTrackerBufferOverflow(t *testing.T) {
	// Fill a tracker whose channel is blocked to verify drop+warn without panic.
	ct := NewCostTracker()
	// Use a small-capacity channel by directly setting it (white-box: same package).
	ct.ch = make(chan dbRow, 2)
	// Do NOT start the writer goroutine — channel will fill immediately.
	// WaitGroup is zero so Close() returns immediately.

	usage := TokenUsage{InputTokens: 10, OutputTokens: 5}
	// Send more records than channel capacity; overflow must not panic.
	for i := 0; i < 10; i++ {
		ct.Record("anthropic", "claude-sonnet-4-5", usage, 0)
	}
	// In-memory entries still recorded.
	if ct.EntryCount() != 10 {
		t.Errorf("EntryCount: got %d, want 10", ct.EntryCount())
	}
	// Drain remaining items so no goroutine leak.
	for len(ct.ch) > 0 {
		<-ct.ch
	}
}
