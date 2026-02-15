package knowledge

import (
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	return db
}

func TestUsageTrackerRecord(t *testing.T) {
	db := openTestDB(t)
	ut, err := NewUsageTracker(db)
	if err != nil {
		t.Fatalf("NewUsageTracker: %v", err)
	}
	defer ut.Close()

	ut.Record("doc-1", ActionView)
	ut.Record("doc-1", ActionView)
	ut.Record("doc-2", ActionSearchHit)
	ut.Record("doc-1", ActionCite)

	// Wait for flush
	time.Sleep(usageFlushEvery + 100*time.Millisecond)

	pop := ut.GetPopularity([]string{"doc-1", "doc-2", "doc-3"})

	// doc-1: 2*view(2) + 1*cite(5) = 9
	if pop["doc-1"] != 9.0 {
		t.Errorf("doc-1 popularity: got %v, want 9.0", pop["doc-1"])
	}
	// doc-2: 1*search_hit(1) = 1
	if pop["doc-2"] != 1.0 {
		t.Errorf("doc-2 popularity: got %v, want 1.0", pop["doc-2"])
	}
	// doc-3: not tracked
	if pop["doc-3"] != 0 {
		t.Errorf("doc-3 popularity: got %v, want 0", pop["doc-3"])
	}
}

func TestUsageTrackerClose(t *testing.T) {
	db := openTestDB(t)
	ut, err := NewUsageTracker(db)
	if err != nil {
		t.Fatal(err)
	}

	ut.Record("doc-x", ActionView)
	ut.Close() // should flush

	// Verify data was flushed
	var count int
	db.QueryRow("SELECT COUNT(*) FROM doc_usage WHERE doc_id='doc-x'").Scan(&count)
	if count != 1 {
		t.Errorf("expected 1 flushed record, got %d", count)
	}
}

func TestUsageTrackerEmptyPopularity(t *testing.T) {
	db := openTestDB(t)
	ut, err := NewUsageTracker(db)
	if err != nil {
		t.Fatal(err)
	}
	defer ut.Close()

	pop := ut.GetPopularity(nil)
	if pop != nil {
		t.Errorf("nil input should return nil, got %v", pop)
	}

	pop = ut.GetPopularity([]string{})
	if pop != nil {
		t.Errorf("empty input should return nil, got %v", pop)
	}
}

func TestBoostRRFWithPopularity(t *testing.T) {
	results := []SearchResult{
		{ID: "doc-1", Title: "First", RRFScore: 0.030},
		{ID: "doc-2", Title: "Second", RRFScore: 0.025},
		{ID: "doc-3", Title: "Third", RRFScore: 0.020},
	}

	// doc-3 is most popular → should get boosted
	popularity := map[string]float64{
		"doc-3": 100.0,
		"doc-1": 5.0,
	}

	boostRRFWithPopularity(results, popularity)

	// doc-3 should now be boosted (rank 0 in popularity → 1/61 ≈ 0.016 boost)
	// doc-1 should get smaller boost (rank 1 → 1/62 ≈ 0.016)
	if results[0].RRFScore <= results[1].RRFScore {
		t.Errorf("expected top result to have highest score after boost")
	}

	// All scores should be positive
	for _, r := range results {
		if r.RRFScore <= 0 {
			t.Errorf("%s has non-positive score: %v", r.ID, r.RRFScore)
		}
	}
}

func TestBoostRRFWithPopularityEmpty(t *testing.T) {
	results := []SearchResult{
		{ID: "doc-1", RRFScore: 0.030},
	}
	originalScore := results[0].RRFScore

	// Empty popularity should not change scores
	boostRRFWithPopularity(results, nil)
	if results[0].RRFScore != originalScore {
		t.Error("nil popularity should not change scores")
	}

	boostRRFWithPopularity(results, map[string]float64{})
	if results[0].RRFScore != originalScore {
		t.Error("empty popularity should not change scores")
	}
}
