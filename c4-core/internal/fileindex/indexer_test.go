package fileindex

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open in-memory sqlite: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestCreateTables(t *testing.T) {
	db := openTestDB(t)

	if err := CreateTables(db); err != nil {
		t.Fatalf("CreateTables: %v", err)
	}
	// idempotent: calling again must not error
	if err := CreateTables(db); err != nil {
		t.Fatalf("CreateTables second call: %v", err)
	}
}

func TestIndex_NewDirectory(t *testing.T) {
	db := openTestDB(t)
	if err := CreateTables(db); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	for i := 0; i < 5; i++ {
		f, err := os.CreateTemp(dir, "file*.txt")
		if err != nil {
			t.Fatal(err)
		}
		f.WriteString("content")
		f.Close()
	}

	indexed, updated, err := Index(db, dir, "test-device")
	if err != nil {
		t.Fatalf("Index: %v", err)
	}
	if indexed != 5 {
		t.Errorf("want indexed=5, got %d", indexed)
	}
	if updated != 5 {
		t.Errorf("want updated=5, got %d", updated)
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM file_index WHERE device_id='test-device'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 5 {
		t.Errorf("want 5 rows in file_index, got %d", count)
	}
}

func TestIndex_IncrementalUpdate(t *testing.T) {
	db := openTestDB(t)
	if err := CreateTables(db); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	files := make([]string, 3)
	for i := 0; i < 3; i++ {
		f, err := os.CreateTemp(dir, "file*.txt")
		if err != nil {
			t.Fatal(err)
		}
		f.WriteString("initial")
		f.Close()
		files[i] = f.Name()
	}

	// first index
	indexed, updated, err := Index(db, dir, "dev1")
	if err != nil {
		t.Fatalf("first Index: %v", err)
	}
	if indexed != 3 || updated != 3 {
		t.Fatalf("first index: want indexed=3 updated=3, got %d/%d", indexed, updated)
	}

	// touch only one file (advance mtime by 2s to be safe)
	future := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(files[0], future, future); err != nil {
		t.Fatal(err)
	}

	// second index
	indexed2, updated2, err := Index(db, dir, "dev1")
	if err != nil {
		t.Fatalf("second Index: %v", err)
	}
	if indexed2 != 3 {
		t.Errorf("second index: want indexed=3, got %d", indexed2)
	}
	if updated2 != 1 {
		t.Errorf("second index: want updated=1, got %d", updated2)
	}
}

func TestIndex_DeletedFile(t *testing.T) {
	db := openTestDB(t)
	if err := CreateTables(db); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	var toDelete string
	for i := 0; i < 4; i++ {
		f, err := os.CreateTemp(dir, "file*.txt")
		if err != nil {
			t.Fatal(err)
		}
		f.Close()
		if i == 0 {
			toDelete = f.Name()
		}
	}

	if _, _, err := Index(db, dir, "dev2"); err != nil {
		t.Fatalf("first Index: %v", err)
	}

	// delete one file
	if err := os.Remove(toDelete); err != nil {
		t.Fatal(err)
	}

	if _, _, err := Index(db, dir, "dev2"); err != nil {
		t.Fatalf("second Index: %v", err)
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM file_index WHERE device_id='dev2'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Errorf("want 3 rows after deletion, got %d", count)
	}
}

func TestIndex_SkipDirs(t *testing.T) {
	db := openTestDB(t)
	if err := CreateTables(db); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()

	// create a real file in root
	f, err := os.CreateTemp(dir, "real*.txt")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	// create files inside skip-dirs
	skipDirs := []string{".git", ".c4", "node_modules", "__pycache__", ".venv", "venv"}
	for _, sd := range skipDirs {
		subdir := filepath.Join(dir, sd)
		if err := os.MkdirAll(subdir, 0o755); err != nil {
			t.Fatal(err)
		}
		sf, err := os.CreateTemp(subdir, "hidden*.txt")
		if err != nil {
			t.Fatal(err)
		}
		sf.Close()
	}

	indexed, updated, err := Index(db, dir, "dev3")
	if err != nil {
		t.Fatalf("Index: %v", err)
	}
	if indexed != 1 {
		t.Errorf("want indexed=1 (skip dirs), got %d", indexed)
	}
	if updated != 1 {
		t.Errorf("want updated=1, got %d", updated)
	}
}
