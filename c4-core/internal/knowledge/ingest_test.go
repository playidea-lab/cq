package knowledge

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIngestFile(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(filepath.Join(dir, "knowledge"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	vs, _ := NewVectorStore(store.DB(), 384, nil)
	searcher := NewSearcher(store, vs)

	// Create a test file
	testFile := filepath.Join(dir, "test.txt")
	content := "Machine learning is a subset of artificial intelligence.\n\nDeep learning uses neural networks with many layers."
	os.WriteFile(testFile, []byte(content), 0644)

	result, err := Ingest(store, searcher, testFile, IngestOpts{
		Title: "ML Overview",
		Tags:  []string{"ml", "intro"},
	})
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if result.DocID == "" {
		t.Error("doc_id should not be empty")
	}
	if result.Title != "ML Overview" {
		t.Errorf("title: got %q, want ML Overview", result.Title)
	}
	if result.ChunkCount < 1 {
		t.Error("should have at least 1 chunk")
	}
	if result.BodyLen != len(content) {
		t.Errorf("body_length: got %d, want %d", result.BodyLen, len(content))
	}

	// Verify document exists in store
	doc, _ := store.Get(result.DocID)
	if doc == nil {
		t.Fatal("ingested document not found in store")
	}
	if doc.Title != "ML Overview" {
		t.Errorf("stored title: got %q", doc.Title)
	}
}

func TestIngestFileDefaultTitle(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewStore(filepath.Join(dir, "knowledge"))
	defer store.Close()

	testFile := filepath.Join(dir, "my-paper.txt")
	os.WriteFile(testFile, []byte("Some content here."), 0644)

	result, err := Ingest(store, nil, testFile, IngestOpts{})
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	// Title should default to filename without extension
	if result.Title != "my-paper" {
		t.Errorf("default title: got %q, want my-paper", result.Title)
	}
}

func TestIngestFileNotFound(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewStore(filepath.Join(dir, "knowledge"))
	defer store.Close()

	_, err := Ingest(store, nil, "/nonexistent/file.txt", IngestOpts{})
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestIngestEmptyFile(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewStore(filepath.Join(dir, "knowledge"))
	defer store.Close()

	testFile := filepath.Join(dir, "empty.txt")
	os.WriteFile(testFile, []byte(""), 0644)

	_, err := Ingest(store, nil, testFile, IngestOpts{})
	if err == nil {
		t.Error("expected error for empty file")
	}
}

func TestIngestText(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewStore(filepath.Join(dir, "knowledge"))
	defer store.Close()

	result, err := IngestText(store, nil, "This is direct text content.", IngestOpts{
		Title:      "Direct Text",
		Visibility: "private",
	})
	if err != nil {
		t.Fatalf("IngestText: %v", err)
	}
	if result.DocID == "" {
		t.Error("doc_id should not be empty")
	}
	if result.Title != "Direct Text" {
		t.Errorf("title: got %q", result.Title)
	}

	doc, _ := store.Get(result.DocID)
	if doc == nil {
		t.Fatal("document not found")
	}
	if doc.Visibility != "private" {
		t.Errorf("visibility: got %q, want private", doc.Visibility)
	}
}

func TestIngestTextEmpty(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewStore(filepath.Join(dir, "knowledge"))
	defer store.Close()

	_, err := IngestText(store, nil, "", IngestOpts{})
	if err == nil {
		t.Error("expected error for empty content")
	}
}
