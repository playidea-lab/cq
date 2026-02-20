package llm

import (
	"math"
	"testing"
	"time"
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
