package fileindex

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// setupSearchDB creates an in-memory DB with both file_index and FTS5 tables ready.
func setupSearchDB(t *testing.T) *sql.DB {
	t.Helper()
	db := openTestDB(t)
	if err := CreateTables(db); err != nil {
		t.Fatalf("CreateTables: %v", err)
	}
	if err := CreateSearchIndex(db); err != nil {
		t.Fatalf("CreateSearchIndex: %v", err)
	}
	return db
}

// insertFile inserts a single file entry directly into file_index.
func insertFile(t *testing.T, db *sql.DB, path, name, deviceID string, size int64, mtime int64) {
	t.Helper()
	_, err := db.Exec(
		`INSERT OR REPLACE INTO file_index (path, device_id, name, size, modified_at, indexed_at) VALUES (?, ?, ?, ?, ?, ?)`,
		path, deviceID, name, size, mtime, time.Now().Unix(),
	)
	if err != nil {
		t.Fatalf("insertFile %s: %v", path, err)
	}
}

func TestSearch_ExactName(t *testing.T) {
	db := setupSearchDB(t)

	now := time.Now().Unix()
	insertFile(t, db, "/models/best.pt", "best.pt", "dev1", 1024, now)
	insertFile(t, db, "/models/other.pt", "other.pt", "dev1", 512, now)

	if err := RebuildSearchIndex(db); err != nil {
		t.Fatalf("RebuildSearchIndex: %v", err)
	}

	results, err := Search(db, "best.pt", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("want at least 1 result, got 0")
	}
	if results[0].Name != "best.pt" {
		t.Errorf("want results[0].Name=best.pt, got %s", results[0].Name)
	}
}

func TestSearch_PartialMatch(t *testing.T) {
	db := setupSearchDB(t)

	now := time.Now().Unix()
	insertFile(t, db, "/models/best.pt", "best.pt", "dev1", 100, now)
	insertFile(t, db, "/models/best_model.pt", "best_model.pt", "dev1", 200, now)
	insertFile(t, db, "/models/other.pt", "other.pt", "dev1", 300, now)

	if err := RebuildSearchIndex(db); err != nil {
		t.Fatalf("RebuildSearchIndex: %v", err)
	}

	results, err := Search(db, "best", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) < 2 {
		t.Fatalf("want at least 2 results for 'best', got %d", len(results))
	}

	// Verify "other.pt" is NOT in results.
	for _, r := range results {
		if r.Name == "other.pt" {
			t.Errorf("unexpected result: other.pt should not match 'best'")
		}
	}
}

func TestSearch_PathMatch(t *testing.T) {
	db := setupSearchDB(t)

	now := time.Now().Unix()
	insertFile(t, db, "/project/checkpoints/epoch1.pt", "epoch1.pt", "dev1", 100, now)
	insertFile(t, db, "/project/checkpoints/epoch2.pt", "epoch2.pt", "dev1", 100, now)
	insertFile(t, db, "/project/models/final.pt", "final.pt", "dev1", 100, now)

	if err := RebuildSearchIndex(db); err != nil {
		t.Fatalf("RebuildSearchIndex: %v", err)
	}

	results, err := Search(db, "checkpoints", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) < 2 {
		t.Fatalf("want at least 2 results for path 'checkpoints', got %d", len(results))
	}

	for _, r := range results {
		if r.Name == "final.pt" {
			t.Errorf("unexpected result: final.pt path does not contain 'checkpoints'")
		}
	}
}

func TestSearch_RecencyBoost(t *testing.T) {
	db := setupSearchDB(t)

	// recent file modified 1 day ago, old file modified 365 days ago.
	recent := time.Now().Add(-24 * time.Hour).Unix()
	old := time.Now().Add(-365 * 24 * time.Hour).Unix()

	insertFile(t, db, "/models/checkpoint_recent.pt", "checkpoint_recent.pt", "dev1", 100, recent)
	insertFile(t, db, "/models/checkpoint_old.pt", "checkpoint_old.pt", "dev1", 100, old)

	if err := RebuildSearchIndex(db); err != nil {
		t.Fatalf("RebuildSearchIndex: %v", err)
	}

	results, err := Search(db, "checkpoint", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) < 2 {
		t.Fatalf("want 2 results, got %d", len(results))
	}

	// Recent file should rank first due to recency boost.
	if results[0].Name != "checkpoint_recent.pt" {
		t.Errorf("want checkpoint_recent.pt first, got %s", results[0].Name)
	}

	// Verify Score is higher for the recent result.
	if results[0].Score <= results[1].Score {
		t.Errorf("want recent score (%.4f) > old score (%.4f)", results[0].Score, results[1].Score)
	}
}

func TestSearch_NoResults(t *testing.T) {
	db := setupSearchDB(t)

	now := time.Now().Unix()
	insertFile(t, db, "/models/best.pt", "best.pt", "dev1", 100, now)

	if err := RebuildSearchIndex(db); err != nil {
		t.Fatalf("RebuildSearchIndex: %v", err)
	}

	results, err := Search(db, "nonexistent_xyz_abc", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("want 0 results, got %d", len(results))
	}
}

func TestSearch_Limit(t *testing.T) {
	db := setupSearchDB(t)

	now := time.Now().Unix()
	for i := 0; i < 10; i++ {
		name := "model_file.pt"
		path := filepath.Join("/models", string(rune('a'+i)), name)
		insertFile(t, db, path, name, "dev1", 100, now-int64(i))
	}

	if err := RebuildSearchIndex(db); err != nil {
		t.Fatalf("RebuildSearchIndex: %v", err)
	}

	results, err := Search(db, "model_file", 3)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 3 {
		t.Errorf("want 3 results (limit=3), got %d", len(results))
	}
}

func TestCreateSearchIndex_Idempotent(t *testing.T) {
	db := setupSearchDB(t)

	// Calling CreateSearchIndex again must not error.
	if err := CreateSearchIndex(db); err != nil {
		t.Fatalf("CreateSearchIndex second call: %v", err)
	}
}

func TestRebuildSearchIndex_Empty(t *testing.T) {
	db := setupSearchDB(t)

	// Rebuild on empty table must not error.
	if err := RebuildSearchIndex(db); err != nil {
		t.Fatalf("RebuildSearchIndex on empty table: %v", err)
	}

	results, err := Search(db, "anything", 10)
	if err != nil {
		t.Fatalf("Search on empty FTS: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("want 0 results on empty index, got %d", len(results))
	}
}

func TestSearch_WithRealIndex(t *testing.T) {
	db := setupSearchDB(t)

	// Use real filesystem walk to populate index, then search.
	dir := t.TempDir()

	subdir := filepath.Join(dir, "checkpoints")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatal(err)
	}

	files := []string{"best_model.pt", "epoch_10.pt", "final.pt"}
	for _, name := range files {
		p := filepath.Join(subdir, name)
		if err := os.WriteFile(p, []byte("data"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if _, _, err := Index(db, dir, "dev-test"); err != nil {
		t.Fatalf("Index: %v", err)
	}
	if err := RebuildSearchIndex(db); err != nil {
		t.Fatalf("RebuildSearchIndex: %v", err)
	}

	results, err := Search(db, "best", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("want at least 1 result for 'best'")
	}
	if results[0].Name != "best_model.pt" {
		t.Errorf("want best_model.pt, got %s", results[0].Name)
	}
}
