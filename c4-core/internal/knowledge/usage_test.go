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

	// doc-1: 2*view(2) + 1*cite(5) = ~9 (with time decay ~1.0 for recent events)
	// Time decay for events just inserted: (1/(1+0/30)) ≈ 1.0
	if pop["doc-1"] < 8.5 || pop["doc-1"] > 9.5 {
		t.Errorf("doc-1 popularity: got %v, want ~9.0", pop["doc-1"])
	}
	// doc-2: 1*search_hit(1) ≈ 1
	if pop["doc-2"] < 0.9 || pop["doc-2"] > 1.1 {
		t.Errorf("doc-2 popularity: got %v, want ~1.0", pop["doc-2"])
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

func TestTimeDecayPopularity(t *testing.T) {
	db := openTestDB(t)
	ut, err := NewUsageTracker(db)
	if err != nil {
		t.Fatal(err)
	}
	defer ut.Close()

	// Insert events with different timestamps directly into DB
	// Recent event (today)
	db.Exec("INSERT INTO doc_usage (doc_id, action, created_at) VALUES (?, ?, ?)",
		"doc-recent", "cite", time.Now().UTC().Format(time.RFC3339))
	// Old event (60 days ago)
	old := time.Now().UTC().Add(-60 * 24 * time.Hour)
	db.Exec("INSERT INTO doc_usage (doc_id, action, created_at) VALUES (?, ?, ?)",
		"doc-old", "cite", old.Format(time.RFC3339))

	pop := ut.GetPopularity([]string{"doc-recent", "doc-old"})

	// Recent: cite(5) * (1/(1+0/30)) ≈ 5.0
	// Old (60d): cite(5) * (1/(1+60/30)) = 5 * 1/3 ≈ 1.67
	if pop["doc-recent"] < 4.5 {
		t.Errorf("recent doc score too low: %v", pop["doc-recent"])
	}
	if pop["doc-old"] > 2.0 {
		t.Errorf("old doc score should be decayed: %v (want < 2.0)", pop["doc-old"])
	}
	if pop["doc-recent"] <= pop["doc-old"] {
		t.Errorf("recent (%v) should score higher than old (%v)", pop["doc-recent"], pop["doc-old"])
	}
}

func TestRetentionCleanup(t *testing.T) {
	db := openTestDB(t)
	ut, err := NewUsageTracker(db)
	if err != nil {
		t.Fatal(err)
	}
	defer ut.Close()

	// Insert old record (100 days ago)
	old := time.Now().UTC().Add(-100 * 24 * time.Hour)
	db.Exec("INSERT INTO doc_usage (doc_id, action, created_at) VALUES (?, ?, ?)",
		"doc-ancient", "view", old.Format(time.RFC3339))

	// Insert recent record
	db.Exec("INSERT INTO doc_usage (doc_id, action, created_at) VALUES (?, ?, ?)",
		"doc-fresh", "view", time.Now().UTC().Format(time.RFC3339))

	// Run cleanup
	deleted, err := ut.Cleanup(90 * 24 * time.Hour)
	if err != nil {
		t.Fatalf("Cleanup error: %v", err)
	}
	if deleted != 1 {
		t.Errorf("expected 1 deleted, got %d", deleted)
	}

	// Verify ancient is gone but fresh remains
	var count int
	db.QueryRow("SELECT COUNT(*) FROM doc_usage WHERE doc_id='doc-ancient'").Scan(&count)
	if count != 0 {
		t.Error("ancient record should be deleted")
	}
	db.QueryRow("SELECT COUNT(*) FROM doc_usage WHERE doc_id='doc-fresh'").Scan(&count)
	if count != 1 {
		t.Error("fresh record should remain")
	}
}

func TestUsageGetStats(t *testing.T) {
	db := openTestDB(t)
	ut, err := NewUsageTracker(db)
	if err != nil {
		t.Fatal(err)
	}
	defer ut.Close()

	// Insert events directly
	now := time.Now().UTC().Format(time.RFC3339)
	db.Exec("INSERT INTO doc_usage (doc_id, action, created_at) VALUES (?, ?, ?)", "d1", "view", now)
	db.Exec("INSERT INTO doc_usage (doc_id, action, created_at) VALUES (?, ?, ?)", "d1", "cite", now)
	db.Exec("INSERT INTO doc_usage (doc_id, action, created_at) VALUES (?, ?, ?)", "d2", "search_hit", now)

	stats := ut.GetStats()

	if stats["total_events"] != 3 {
		t.Errorf("total_events: got %v, want 3", stats["total_events"])
	}
	if stats["last_7d"] != 3 {
		t.Errorf("last_7d: got %v, want 3", stats["last_7d"])
	}
	if stats["last_30d"] != 3 {
		t.Errorf("last_30d: got %v, want 3", stats["last_30d"])
	}

	byAction, ok := stats["by_action"].(map[string]int)
	if !ok {
		t.Fatal("by_action should be map[string]int")
	}
	if byAction["view"] != 1 || byAction["cite"] != 1 || byAction["search_hit"] != 1 {
		t.Errorf("by_action: got %v", byAction)
	}

	topCited, ok := stats["top_cited"].([]map[string]any)
	if !ok || len(topCited) == 0 {
		t.Fatal("top_cited should have entries")
	}
	// d1 has view(2)+cite(5) = 7 > d2's search_hit(1)
	if topCited[0]["id"] != "d1" {
		t.Errorf("top cited should be d1, got %v", topCited[0]["id"])
	}
}
