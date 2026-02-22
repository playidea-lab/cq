package handlers

import "sync"

// KnowledgeHitEntry records a single knowledge search event.
type KnowledgeHitEntry struct {
	TaskID      string
	Query       string
	Hit         bool
	ResultCount int
}

// KnowledgeHitReport summarises knowledge search hit/miss statistics.
type KnowledgeHitReport struct {
	TotalSearches int     `json:"total_searches"`
	Hits          int     `json:"hits"`
	Misses        int     `json:"misses"`
	HitRate       float64 `json:"hit_rate"` // Hits / TotalSearches (0 if TotalSearches == 0)
}

// KnowledgeHitTracker accumulates knowledge search hit/miss counts in memory.
// It follows the same pattern as llm.CostTracker.
type KnowledgeHitTracker struct {
	mu      sync.Mutex
	entries []KnowledgeHitEntry
}

// NewKnowledgeHitTracker creates a new KnowledgeHitTracker.
func NewKnowledgeHitTracker() *KnowledgeHitTracker {
	return &KnowledgeHitTracker{}
}

// Record logs one knowledge search result.
// resultCount must be >= 0; negative values are treated as 0 (miss).
// resultCount == 0 (or a prior err) counts as a miss; resultCount > 0 is a hit.
func (t *KnowledgeHitTracker) Record(taskID, query string, resultCount int) {
	if resultCount < 0 {
		resultCount = 0
	}
	t.mu.Lock()
	t.entries = append(t.entries, KnowledgeHitEntry{
		TaskID:      taskID,
		Query:       query,
		Hit:         resultCount > 0,
		ResultCount: resultCount,
	})
	t.mu.Unlock()
}

// Report returns aggregate hit/miss statistics.
func (t *KnowledgeHitTracker) Report() KnowledgeHitReport {
	t.mu.Lock()
	defer t.mu.Unlock()

	report := KnowledgeHitReport{
		TotalSearches: len(t.entries),
	}
	for _, e := range t.entries {
		if e.Hit {
			report.Hits++
		} else {
			report.Misses++
		}
	}
	if report.TotalSearches > 0 {
		report.HitRate = float64(report.Hits) / float64(report.TotalSearches)
	}
	return report
}

// EntryCount returns the number of recorded entries (for testing).
func (t *KnowledgeHitTracker) EntryCount() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.entries)
}
