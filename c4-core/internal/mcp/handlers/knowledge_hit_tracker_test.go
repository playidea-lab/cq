package handlers

import (
	"testing"
)

func TestKnowledgeHitTrackerRecord(t *testing.T) {
	tracker := NewKnowledgeHitTracker()

	if tracker.EntryCount() != 0 {
		t.Fatalf("expected 0 entries, got %d", tracker.EntryCount())
	}

	tracker.Record("T-001-0", "database migration", 3)
	tracker.Record("T-002-0", "unknown concept xyz", 0)
	tracker.Record("T-003-0", "error handling", 2)

	if tracker.EntryCount() != 3 {
		t.Fatalf("expected 3 entries, got %d", tracker.EntryCount())
	}
}

func TestKnowledgeHitTrackerReport(t *testing.T) {
	tracker := NewKnowledgeHitTracker()

	// Empty report
	r := tracker.Report()
	if r.TotalSearches != 0 {
		t.Errorf("expected TotalSearches=0, got %d", r.TotalSearches)
	}
	if r.HitRate != 0.0 {
		t.Errorf("expected HitRate=0.0, got %f", r.HitRate)
	}

	// 2 hits, 1 miss
	tracker.Record("T-001-0", "query with results", 5)
	tracker.Record("T-002-0", "another hit", 1)
	tracker.Record("T-003-0", "no results", 0)

	r = tracker.Report()
	if r.TotalSearches != 3 {
		t.Errorf("expected TotalSearches=3, got %d", r.TotalSearches)
	}
	if r.Hits != 2 {
		t.Errorf("expected Hits=2, got %d", r.Hits)
	}
	if r.Misses != 1 {
		t.Errorf("expected Misses=1, got %d", r.Misses)
	}
	const wantRate = 2.0 / 3.0
	if r.HitRate < wantRate-0.001 || r.HitRate > wantRate+0.001 {
		t.Errorf("expected HitRate=%.4f, got %.4f", wantRate, r.HitRate)
	}
}

func TestKnowledgeHitTrackerAllMisses(t *testing.T) {
	tracker := NewKnowledgeHitTracker()
	tracker.Record("T-001-0", "miss1", 0)
	tracker.Record("T-002-0", "miss2", 0)

	r := tracker.Report()
	if r.Hits != 0 {
		t.Errorf("expected 0 hits, got %d", r.Hits)
	}
	if r.Misses != 2 {
		t.Errorf("expected 2 misses, got %d", r.Misses)
	}
	if r.HitRate != 0.0 {
		t.Errorf("expected HitRate=0.0, got %f", r.HitRate)
	}
}

func TestKnowledgeHitTrackerAllHits(t *testing.T) {
	tracker := NewKnowledgeHitTracker()
	tracker.Record("T-001-0", "hit1", 3)
	tracker.Record("T-002-0", "hit2", 1)

	r := tracker.Report()
	if r.HitRate != 1.0 {
		t.Errorf("expected HitRate=1.0, got %f", r.HitRate)
	}
}

func TestKnowledgeHitTrackerNegativeResultCount(t *testing.T) {
	tracker := NewKnowledgeHitTracker()
	tracker.Record("T-x", "q", -1)
	r := tracker.Report()
	if r.Hits != 0 || r.Misses != 1 {
		t.Errorf("negative resultCount: expected 0 hits / 1 miss, got %d/%d", r.Hits, r.Misses)
	}
}

func TestKnowledgeHitTrackerManualSimulation(t *testing.T) {
	// Simulate the call pattern used by enrichWithKnowledge: hit then miss.
	tracker := NewKnowledgeHitTracker()

	tracker.Record("T-010-0", "knowledge engineering", 2) // hit
	tracker.Record("T-011-0", "no such concept", 0)       // miss

	r := tracker.Report()
	if r.TotalSearches != 2 {
		t.Errorf("expected 2 total searches, got %d", r.TotalSearches)
	}
	if r.Hits != 1 || r.Misses != 1 {
		t.Errorf("expected 1 hit / 1 miss, got %d/%d", r.Hits, r.Misses)
	}
}

func TestKnowledgeHitTrackerConcurrent(t *testing.T) {
	tracker := NewKnowledgeHitTracker()
	const n = 100
	done := make(chan struct{})
	for i := 0; i < n; i++ {
		go func(i int) {
			tracker.Record("T-concurrent", "query", i%2) // alternating hit/miss
			done <- struct{}{}
		}(i)
	}
	for i := 0; i < n; i++ {
		<-done
	}
	r := tracker.Report()
	if r.TotalSearches != n {
		t.Errorf("concurrent: expected %d total searches, got %d", n, r.TotalSearches)
	}
	if r.Hits+r.Misses != n {
		t.Errorf("concurrent: hits+misses=%d, want %d", r.Hits+r.Misses, n)
	}
}
