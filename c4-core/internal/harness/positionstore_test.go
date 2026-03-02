package harness

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPositionStore_GetSetOffset(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "positions.db")

	store, err := NewPositionStore(dbPath)
	if err != nil {
		t.Fatalf("NewPositionStore: %v", err)
	}
	defer store.Close()

	// Default offset is 0.
	if got := store.GetOffset("/some/file.jsonl"); got != 0 {
		t.Errorf("GetOffset = %d, want 0", got)
	}

	// Set an offset.
	if err := store.SetOffset("/some/file.jsonl", 1234); err != nil {
		t.Fatalf("SetOffset: %v", err)
	}
	if got := store.GetOffset("/some/file.jsonl"); got != 1234 {
		t.Errorf("GetOffset = %d, want 1234", got)
	}

	// Update (upsert).
	if err := store.SetOffset("/some/file.jsonl", 5678); err != nil {
		t.Fatalf("SetOffset update: %v", err)
	}
	if got := store.GetOffset("/some/file.jsonl"); got != 5678 {
		t.Errorf("GetOffset after update = %d, want 5678", got)
	}
}

func TestPositionStore_MultipleFiles(t *testing.T) {
	dir := t.TempDir()
	store, err := NewPositionStore(filepath.Join(dir, "pos.db"))
	if err != nil {
		t.Fatalf("NewPositionStore: %v", err)
	}
	defer store.Close()

	_ = store.SetOffset("/path/a.jsonl", 100)
	_ = store.SetOffset("/path/b.jsonl", 200)

	if got := store.GetOffset("/path/a.jsonl"); got != 100 {
		t.Errorf("a = %d, want 100", got)
	}
	if got := store.GetOffset("/path/b.jsonl"); got != 200 {
		t.Errorf("b = %d, want 200", got)
	}
}

func TestPositionStore_DBFileCreated(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "sub", "positions.db")

	store, err := NewPositionStore(dbPath)
	if err != nil {
		t.Fatalf("NewPositionStore: %v", err)
	}
	store.Close()

	if _, err := os.Stat(dbPath); err != nil {
		t.Errorf("db file not created: %v", err)
	}
}
