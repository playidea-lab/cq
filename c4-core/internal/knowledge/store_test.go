package knowledge

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setupTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	basePath := filepath.Join(dir, "knowledge")
	store, err := NewStore(basePath)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func TestCreateAndGet(t *testing.T) {
	s := setupTestStore(t)

	docID, err := s.Create(TypeExperiment, map[string]any{
		"title":  "RF Baseline",
		"domain": "ml",
		"tags":   []string{"rf", "baseline"},
	}, "# RF Baseline\n\nAccuracy: 0.87")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !strings.HasPrefix(docID, "exp-") {
		t.Errorf("docID prefix: got %s, want exp-*", docID)
	}

	doc, err := s.Get(docID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if doc == nil {
		t.Fatal("Get returned nil")
	}
	if doc.Title != "RF Baseline" {
		t.Errorf("title: got %q, want %q", doc.Title, "RF Baseline")
	}
	if doc.Domain != "ml" {
		t.Errorf("domain: got %q, want %q", doc.Domain, "ml")
	}
	if len(doc.Tags) != 2 || doc.Tags[0] != "rf" {
		t.Errorf("tags: got %v, want [rf baseline]", doc.Tags)
	}
	if doc.Body != "# RF Baseline\n\nAccuracy: 0.87" {
		t.Errorf("body: got %q", doc.Body)
	}
	if doc.Version != 1 {
		t.Errorf("version: got %d, want 1", doc.Version)
	}
}

func TestCreateWithCustomID(t *testing.T) {
	s := setupTestStore(t)

	docID, err := s.Create(TypeInsight, map[string]any{
		"id":    "ins-custom01",
		"title": "Custom ID test",
	}, "body")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if docID != "ins-custom01" {
		t.Errorf("docID: got %q, want ins-custom01", docID)
	}

	doc, err := s.Get("ins-custom01")
	if err != nil || doc == nil {
		t.Fatal("Get custom ID failed")
	}
	if doc.Title != "Custom ID test" {
		t.Errorf("title: got %q", doc.Title)
	}
}

func TestGetNotFound(t *testing.T) {
	s := setupTestStore(t)

	doc, err := s.Get("nonexistent")
	if err != nil {
		t.Fatalf("Get error: %v", err)
	}
	if doc != nil {
		t.Error("expected nil for nonexistent doc")
	}
}

func TestUpdate(t *testing.T) {
	s := setupTestStore(t)

	docID, _ := s.Create(TypeExperiment, map[string]any{
		"title": "Original",
	}, "original body")

	newBody := "updated body"
	updated, err := s.Update(docID, map[string]any{
		"title":  "Updated Title",
		"domain": "updated-domain",
	}, &newBody)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if !updated {
		t.Error("Update returned false")
	}

	doc, _ := s.Get(docID)
	if doc.Title != "Updated Title" {
		t.Errorf("title: got %q", doc.Title)
	}
	if doc.Domain != "updated-domain" {
		t.Errorf("domain: got %q", doc.Domain)
	}
	if doc.Body != "updated body" {
		t.Errorf("body: got %q", doc.Body)
	}
	if doc.Version != 2 {
		t.Errorf("version: got %d, want 2", doc.Version)
	}
}

func TestUpdateNotFound(t *testing.T) {
	s := setupTestStore(t)

	updated, err := s.Update("nonexistent", map[string]any{"title": "x"}, nil)
	if err != nil {
		t.Fatalf("Update error: %v", err)
	}
	if updated {
		t.Error("Update returned true for nonexistent")
	}
}

func TestDelete(t *testing.T) {
	s := setupTestStore(t)

	docID, _ := s.Create(TypePattern, map[string]any{
		"title": "To Delete",
	}, "body")

	deleted, err := s.Delete(docID)
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if !deleted {
		t.Error("Delete returned false")
	}

	// Verify gone
	doc, _ := s.Get(docID)
	if doc != nil {
		t.Error("document still exists after delete")
	}

	// Verify file removed
	path := filepath.Join(s.DocsDir(), docID+".md")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("markdown file still exists after delete")
	}
}

func TestDeleteNotFound(t *testing.T) {
	s := setupTestStore(t)

	deleted, err := s.Delete("nonexistent")
	if err != nil {
		t.Fatalf("Delete error: %v", err)
	}
	if deleted {
		t.Error("Delete returned true for nonexistent")
	}
}

func TestList(t *testing.T) {
	s := setupTestStore(t)

	s.Create(TypeExperiment, map[string]any{"title": "Exp1", "domain": "ml"}, "body1")
	s.Create(TypeExperiment, map[string]any{"title": "Exp2", "domain": "ml"}, "body2")
	s.Create(TypeInsight, map[string]any{"title": "Ins1", "domain": "ops"}, "body3")

	// List all
	all, err := s.List("", "", 50)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("List all: got %d, want 3", len(all))
	}

	// Filter by type
	exps, err := s.List("experiment", "", 50)
	if err != nil {
		t.Fatalf("List type: %v", err)
	}
	if len(exps) != 2 {
		t.Errorf("List experiment: got %d, want 2", len(exps))
	}

	// Filter by domain
	ops, err := s.List("", "ops", 50)
	if err != nil {
		t.Fatalf("List domain: %v", err)
	}
	if len(ops) != 1 {
		t.Errorf("List ops domain: got %d, want 1", len(ops))
	}
}

func TestSearchFTS(t *testing.T) {
	s := setupTestStore(t)

	s.Create(TypeExperiment, map[string]any{
		"title":  "Random Forest Accuracy",
		"domain": "ml",
	}, "# RF Results\n\nThe random forest model achieved 87% accuracy.")
	s.Create(TypeInsight, map[string]any{
		"title":  "Neural Network Architecture",
		"domain": "dl",
	}, "# NN Architecture\n\nA deep learning approach with transformers.")

	results, err := s.SearchFTS("random forest", 10)
	if err != nil {
		t.Fatalf("SearchFTS: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("SearchFTS returned 0 results")
	}
	if results[0]["title"] != "Random Forest Accuracy" {
		t.Errorf("first result title: got %v", results[0]["title"])
	}
}

func TestSearchFTSFallback(t *testing.T) {
	s := setupTestStore(t)

	s.Create(TypeExperiment, map[string]any{
		"title":  "Test Document",
		"domain": "test",
	}, "body")

	// Invalid FTS query should fallback to LIKE
	results, err := s.SearchFTS("test", 10)
	if err != nil {
		t.Fatalf("SearchFTS fallback: %v", err)
	}
	if len(results) == 0 {
		t.Error("SearchFTS fallback returned 0 results")
	}
}

func TestDocTypePrefixes(t *testing.T) {
	tests := []struct {
		docType DocumentType
		prefix  string
	}{
		{TypeExperiment, "exp-"},
		{TypePattern, "pat-"},
		{TypeInsight, "ins-"},
		{TypeHypothesis, "hyp-"},
	}
	for _, tt := range tests {
		s := setupTestStore(t)
		id, _ := s.Create(tt.docType, map[string]any{"title": "test"}, "body")
		if !strings.HasPrefix(id, tt.prefix) {
			t.Errorf("type %s: got prefix %s, want %s", tt.docType, id[:4], tt.prefix)
		}
	}
}

func TestMarkdownSSoT(t *testing.T) {
	s := setupTestStore(t)

	docID, _ := s.Create(TypeExperiment, map[string]any{
		"title":  "SSOT Test",
		"domain": "test",
		"tags":   []string{"a", "b"},
	}, "# Content\n\nParagraph here.")

	// Verify Markdown file exists and contains frontmatter
	filePath := filepath.Join(s.DocsDir(), docID+".md")
	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	content := string(data)

	if !strings.HasPrefix(content, "---\n") {
		t.Error("markdown should start with ---")
	}
	if !strings.Contains(content, "title: SSOT Test") {
		t.Error("markdown should contain title")
	}
	if !strings.Contains(content, "type: experiment") {
		t.Error("markdown should contain type")
	}
	if !strings.Contains(content, "# Content") {
		t.Error("markdown should contain body")
	}
}

func TestRebuildIndex(t *testing.T) {
	s := setupTestStore(t)

	s.Create(TypeExperiment, map[string]any{"title": "Doc1"}, "body1")
	s.Create(TypeInsight, map[string]any{"title": "Doc2"}, "body2")

	count, err := s.RebuildIndex()
	if err != nil {
		t.Fatalf("RebuildIndex: %v", err)
	}
	if count != 2 {
		t.Errorf("RebuildIndex: got %d, want 2", count)
	}

	// Verify search still works after rebuild
	results, err := s.SearchFTS("Doc1", 10)
	if err != nil || len(results) == 0 {
		t.Error("search failed after rebuild")
	}
}

func TestParseFrontmatter(t *testing.T) {
	input := "---\nid: exp-123\ntype: experiment\ntitle: Test\nversion: 1\n---\n\nBody content"
	fm, body := parseFrontmatter(input)

	if fm["id"] != "exp-123" {
		t.Errorf("id: got %v", fm["id"])
	}
	if fm["type"] != "experiment" {
		t.Errorf("type: got %v", fm["type"])
	}
	if fm["title"] != "Test" {
		t.Errorf("title: got %v", fm["title"])
	}
	if body != "Body content" {
		t.Errorf("body: got %q", body)
	}
}

func TestParseFrontmatterWithList(t *testing.T) {
	input := "---\nid: exp-123\ntags:\n- alpha\n- beta\n---\n\nBody"
	fm, _ := parseFrontmatter(input)

	tags, ok := fm["tags"].([]string)
	if !ok {
		t.Fatalf("tags type: got %T", fm["tags"])
	}
	if len(tags) != 2 || tags[0] != "alpha" || tags[1] != "beta" {
		t.Errorf("tags: got %v", tags)
	}
}

func TestGetBacklinks(t *testing.T) {
	s := setupTestStore(t)

	// Create doc A
	_, _ = s.Create(TypeExperiment, map[string]any{
		"id":    "exp-aaaaaaaa",
		"title": "Doc A",
	}, "No links here.")

	// Create doc B that references A
	_, _ = s.Create(TypeExperiment, map[string]any{
		"id":    "exp-bbbbbbbb",
		"title": "Doc B",
	}, "This references [[exp-aaaaaaaa]].")

	backlinks, err := s.GetBacklinks("exp-aaaaaaaa")
	if err != nil {
		t.Fatalf("GetBacklinks: %v", err)
	}
	if len(backlinks) != 1 || backlinks[0] != "exp-bbbbbbbb" {
		t.Errorf("backlinks: got %v", backlinks)
	}
}

func TestCreateExperimentFields(t *testing.T) {
	s := setupTestStore(t)

	docID, _ := s.Create(TypeExperiment, map[string]any{
		"title":             "Exp with fields",
		"hypothesis":        "RF beats XGBoost",
		"hypothesis_status": "confirmed",
	}, "body")

	doc, _ := s.Get(docID)
	if doc.Hypothesis != "RF beats XGBoost" {
		t.Errorf("hypothesis: got %q", doc.Hypothesis)
	}
	if doc.HypothesisStatus != "confirmed" {
		t.Errorf("hypothesis_status: got %q", doc.HypothesisStatus)
	}
}

func TestCreatePatternFields(t *testing.T) {
	s := setupTestStore(t)

	docID, _ := s.Create(TypePattern, map[string]any{
		"title":          "Pattern with fields",
		"confidence":     0.85,
		"evidence_count": 5,
		"evidence_ids":   []string{"exp-111", "exp-222"},
	}, "body")

	doc, _ := s.Get(docID)
	if doc.Confidence != 0.85 {
		t.Errorf("confidence: got %g", doc.Confidence)
	}
	if doc.EvidenceCount != 5 {
		t.Errorf("evidence_count: got %d", doc.EvidenceCount)
	}
	if len(doc.EvidenceIDs) != 2 {
		t.Errorf("evidence_ids: got %v", doc.EvidenceIDs)
	}
}

func TestListWithLimit(t *testing.T) {
	s := setupTestStore(t)

	for i := 0; i < 5; i++ {
		s.Create(TypeExperiment, map[string]any{"title": "doc"}, "body")
	}

	results, _ := s.List("", "", 3)
	if len(results) != 3 {
		t.Errorf("List with limit 3: got %d", len(results))
	}
}
