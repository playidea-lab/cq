package knowledge

import (
	"path/filepath"
	"testing"
)

func setupTestSearcher(t *testing.T) (*Store, *Searcher) {
	t.Helper()
	dir := t.TempDir()
	basePath := filepath.Join(dir, "knowledge")
	store, err := NewStore(basePath)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	vs, err := NewVectorStore(store.DB(), 384, nil)
	if err != nil {
		t.Fatalf("NewVectorStore: %v", err)
	}

	searcher := NewSearcher(store, vs)
	return store, searcher
}

func TestSearchHybrid(t *testing.T) {
	store, searcher := setupTestSearcher(t)

	// Create and index documents
	id1, _ := store.Create(TypeExperiment, map[string]any{
		"title":  "Random Forest Accuracy",
		"domain": "ml",
	}, "Random forest achieved 87% accuracy on the test set.")
	doc1, _ := store.Get(id1)
	searcher.IndexDocument(id1, doc1)

	id2, _ := store.Create(TypeInsight, map[string]any{
		"title":  "Neural Network Performance",
		"domain": "dl",
	}, "Transformer models outperform CNNs on NLP tasks.")
	doc2, _ := store.Get(id2)
	searcher.IndexDocument(id2, doc2)

	results, err := searcher.Search("random forest", 10, nil)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("Search returned 0 results")
	}
	// RF doc should be ranked first (both FTS and vector favor it)
	if results[0].ID != id1 {
		t.Errorf("first result: got %s, want %s", results[0].ID, id1)
	}
	if results[0].RRFScore <= 0 {
		t.Error("RRF score should be positive")
	}
}

func TestSearchFTSOnly(t *testing.T) {
	dir := t.TempDir()
	basePath := filepath.Join(dir, "knowledge")
	store, _ := NewStore(basePath)
	defer store.Close()

	// No vector store — FTS only
	searcher := NewSearcher(store, nil)

	store.Create(TypeExperiment, map[string]any{
		"title": "FTS Only Test",
	}, "This tests FTS-only mode without vectors.")

	results, err := searcher.Search("FTS", 10, nil)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) == 0 {
		t.Error("FTS-only search should return results")
	}
}

func TestSearchWithTypeFilter(t *testing.T) {
	store, searcher := setupTestSearcher(t)

	store.Create(TypeExperiment, map[string]any{
		"title": "Exp Doc",
	}, "experiment body")
	store.Create(TypeInsight, map[string]any{
		"title": "Insight Doc",
	}, "insight body")

	results, err := searcher.SearchByType("doc", "experiment", 10)
	if err != nil {
		t.Fatalf("SearchByType: %v", err)
	}
	for _, r := range results {
		if r.Type != "experiment" {
			t.Errorf("unexpected type: %s", r.Type)
		}
	}
}

func TestSearchWithDomainFilter(t *testing.T) {
	store, searcher := setupTestSearcher(t)

	store.Create(TypeExperiment, map[string]any{
		"title":  "ML Doc",
		"domain": "ml",
	}, "machine learning experiment")
	store.Create(TypeExperiment, map[string]any{
		"title":  "Ops Doc",
		"domain": "ops",
	}, "operations experiment")

	results, err := searcher.Search("experiment", 10, map[string]string{"domain": "ml"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	for _, r := range results {
		if r.Domain != "ml" {
			t.Errorf("unexpected domain: %s", r.Domain)
		}
	}
}

func TestSearchEmpty(t *testing.T) {
	_, searcher := setupTestSearcher(t)

	results, err := searcher.Search("anything", 10, nil)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("empty search: got %d results", len(results))
	}
}

func TestSearchTopK(t *testing.T) {
	store, searcher := setupTestSearcher(t)

	for i := 0; i < 10; i++ {
		store.Create(TypeExperiment, map[string]any{
			"title": "Test Document",
		}, "test body content")
	}

	results, err := searcher.Search("test", 3, nil)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) > 3 {
		t.Errorf("topK=3: got %d results", len(results))
	}
}

func TestRRFMerge(t *testing.T) {
	fts := []map[string]any{
		{"id": "doc1", "title": "First", "type": "experiment", "domain": "ml", "score": 1.0},
		{"id": "doc2", "title": "Second", "type": "insight", "domain": "dl", "score": 0.5},
	}
	vec := []VectorResult{
		{DocID: "doc2", Score: 0.9, Distance: 0.1},
		{DocID: "doc3", Score: 0.8, Distance: 0.2},
	}

	results := rrfMerge(fts, vec, 60)

	// doc2 appears in both → should have highest RRF score
	if len(results) != 3 {
		t.Fatalf("results: got %d, want 3", len(results))
	}
	if results[0].ID != "doc2" {
		t.Errorf("top result: got %s, want doc2 (appears in both lists)", results[0].ID)
	}
	if results[0].RRFScore <= 0 {
		t.Error("RRF score should be positive")
	}
}

func TestRRFMergeEmpty(t *testing.T) {
	results := rrfMerge(nil, nil, 60)
	if len(results) != 0 {
		t.Errorf("empty merge: got %d results", len(results))
	}
}

func TestDocumentToText(t *testing.T) {
	doc := &Document{
		Title:      "Test Doc",
		Domain:     "ml",
		Tags:       []string{"rf", "baseline"},
		Hypothesis: "RF is best",
		Body:       "Long body content here.",
	}
	text := documentToText(doc)
	if text == "" {
		t.Error("text should not be empty")
	}
	if !contains(text, "Test Doc") {
		t.Error("should contain title")
	}
	if !contains(text, "domain: ml") {
		t.Error("should contain domain")
	}
	if !contains(text, "RF is best") {
		t.Error("should contain hypothesis")
	}
}

func TestDocumentToTextEmpty(t *testing.T) {
	doc := &Document{}
	text := documentToText(doc)
	if text != "" {
		t.Errorf("empty doc text: got %q", text)
	}
}

func TestDocumentToTextBodyTruncation(t *testing.T) {
	longBody := ""
	for i := 0; i < 600; i++ {
		longBody += "x"
	}
	doc := &Document{Title: "T", Body: longBody}
	text := documentToText(doc)
	// Body should be truncated to 500 chars
	if len(text) > 510 { // title + separator + 500 body
		t.Errorf("body not truncated: text len=%d", len(text))
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && searchString(s, sub)
}

func searchString(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
