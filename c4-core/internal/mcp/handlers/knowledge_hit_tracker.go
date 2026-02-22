package handlers

import "sync"

// KnowledgeHitReport summarises knowledge search hit/miss statistics.
type KnowledgeHitReport struct {
	TotalSearches int     `json:"total_searches"`
	Hits          int     `json:"hits"`
	Misses        int     `json:"misses"`
	HitRate       float64 `json:"hit_rate"` // Hits / TotalSearches (0 if TotalSearches == 0)
}

// KnowledgeHitTracker accumulates knowledge search hit/miss counts in memory.
// It uses running counters (not a slice) to keep memory constant regardless of
// session length. Follows the same concurrency pattern as llm.CostTracker.
type KnowledgeHitTracker struct {
	mu     sync.Mutex
	hits   int
	misses int
}

// NewKnowledgeHitTracker creates a new KnowledgeHitTracker.
func NewKnowledgeHitTracker() *KnowledgeHitTracker {
	return &KnowledgeHitTracker{}
}

// Record logs one knowledge search result.
// resultCount must be >= 0; negative values are treated as 0 (miss).
// resultCount == 0 (or a prior err) counts as a miss; resultCount > 0 is a hit.
func (t *KnowledgeHitTracker) Record(_, _ string, resultCount int) {
	if resultCount < 0 {
		resultCount = 0
	}
	t.mu.Lock()
	if resultCount > 0 {
		t.hits++
	} else {
		t.misses++
	}
	t.mu.Unlock()
}

// Report returns aggregate hit/miss statistics.
func (t *KnowledgeHitTracker) Report() KnowledgeHitReport {
	t.mu.Lock()
	defer t.mu.Unlock()

	report := KnowledgeHitReport{
		TotalSearches: t.hits + t.misses,
		Hits:          t.hits,
		Misses:        t.misses,
	}
	if report.TotalSearches > 0 {
		report.HitRate = float64(report.Hits) / float64(report.TotalSearches)
	}
	return report
}

// EntryCount returns the total number of recorded searches (for testing).
func (t *KnowledgeHitTracker) EntryCount() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.hits + t.misses
}
