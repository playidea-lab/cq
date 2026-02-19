package llm

import (
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
