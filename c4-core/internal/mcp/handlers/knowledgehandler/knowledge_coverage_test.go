package knowledgehandler

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/changmin/c4-core/internal/knowledge"
	"github.com/changmin/c4-core/internal/mcp"
)

// setupKnowledgeWithCloud creates a registry with a mock cloud attached.
func setupKnowledgeWithCloud(t *testing.T, cloud knowledge.CloudSyncer) (*mcp.Registry, *KnowledgeNativeOpts) {
	t.Helper()
	dir := t.TempDir()
	basePath := filepath.Join(dir, "knowledge")

	store, err := knowledge.NewStore(basePath)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	vs, err := knowledge.NewVectorStore(store.DB(), 384, nil)
	if err != nil {
		t.Fatalf("NewVectorStore: %v", err)
	}
	searcher := knowledge.NewSearcher(store, vs)

	opts := &KnowledgeNativeOpts{
		Store:    store,
		Searcher: searcher,
		Cloud:    cloud,
	}

	reg := mcp.NewRegistry()
	RegisterKnowledgeNativeHandlers(reg, opts)
	return reg, opts
}

// mockCloudList extends mockCloudSyncer with ListDocuments support for Pull tests.
type mockCloudList struct {
	mockCloudSyncer
	listDocs []map[string]any
}

func (m *mockCloudList) ListDocuments(docType string, limit int) ([]map[string]any, error) {
	if docType == "" {
		return m.listDocs, nil
	}
	var out []map[string]any
	for _, d := range m.listDocs {
		if tp, _ := d["type"].(string); tp == docType {
			out = append(out, d)
		}
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Utility function tests (0% → covered)
// ---------------------------------------------------------------------------

func TestUtilStringFromAny(t *testing.T) {
	if got := stringFromAny("hello"); got != "hello" {
		t.Errorf("got %q, want 'hello'", got)
	}
	if got := stringFromAny(nil); got != "" {
		t.Errorf("nil: got %q", got)
	}
	if got := stringFromAny(42); got != "" {
		t.Errorf("int: got %q", got)
	}
}

func TestUtilToStringSliceAny(t *testing.T) {
	if got := toStringSliceAny(nil); got != nil {
		t.Errorf("nil: got %v", got)
	}
	ss := []string{"a", "b"}
	if got := toStringSliceAny(ss); len(got) != 2 || got[0] != "a" {
		t.Errorf("[]string: got %v", got)
	}
	ai := []any{"x", "y", 99}
	if got := toStringSliceAny(ai); len(got) != 2 || got[0] != "x" {
		t.Errorf("[]any: got %v", got)
	}
	if got := toStringSliceAny(123); got != nil {
		t.Errorf("int: got %v", got)
	}
}

func TestUtilTruncate(t *testing.T) {
	if got := truncate("hello", 10); got != "hello" {
		t.Errorf("short: got %q", got)
	}
	if got := truncate("hello world", 5); got != "hello..." {
		t.Errorf("long: got %q", got)
	}
	if got := truncate("abcde", 5); got != "abcde" {
		t.Errorf("exact: got %q", got)
	}
}

func TestSetKnowledgeEventBus(t *testing.T) {
	// Must not panic; restore nil afterward
	SetKnowledgeEventBus(nil, "")
	SetKnowledgeEventBus(nil, "proj-123")
	SetKnowledgeEventBus(nil, "")
}

// ---------------------------------------------------------------------------
// knowledgeDeleteNativeHandler (6.2% → covered)
// ---------------------------------------------------------------------------

func TestKnowledgeDeleteMissingID(t *testing.T) {
	reg, _ := setupKnowledgeNativeTest(t)
	result := callHandler(t, reg, "c4_knowledge_delete", map[string]any{})
	if _, hasErr := result["error"]; !hasErr {
		t.Error("expected error for missing doc_id")
	}
}

func TestKnowledgeDeleteNotFound(t *testing.T) {
	reg, _ := setupKnowledgeNativeTest(t)
	result := callHandler(t, reg, "c4_knowledge_delete", map[string]any{
		"doc_id": "nonexistent-xyz",
	})
	if _, hasErr := result["error"]; !hasErr {
		t.Error("expected error for nonexistent document")
	}
}

func TestKnowledgeDeleteSuccess(t *testing.T) {
	reg, _ := setupKnowledgeNativeTest(t)

	created := callHandler(t, reg, "c4_knowledge_record", map[string]any{
		"doc_type": "insight",
		"title":    "Delete Me",
		"content":  "body to delete",
	})
	docID, _ := created["doc_id"].(string)
	if docID == "" {
		t.Fatal("expected doc_id in created result")
	}

	result := callHandler(t, reg, "c4_knowledge_delete", map[string]any{
		"doc_id": docID,
	})
	if result["deleted"] != true {
		t.Errorf("expected deleted=true, got %v", result)
	}
	if result["doc_id"] != docID {
		t.Errorf("doc_id mismatch: got %v", result["doc_id"])
	}

	// Verify gone
	got := callHandler(t, reg, "c4_knowledge_get", map[string]any{"doc_id": docID})
	if _, hasErr := got["error"]; !hasErr {
		t.Error("expected error fetching deleted document")
	}
}

// ---------------------------------------------------------------------------
// knowledgeDiscoverNativeHandler (6.7% → covered)
// ---------------------------------------------------------------------------

func TestKnowledgeDiscoverNoCloud(t *testing.T) {
	reg, _ := setupKnowledgeNativeTest(t)
	// No cloud → error about cloud not configured
	result := callHandler(t, reg, "c4_knowledge_discover", map[string]any{
		"query": "anything",
	})
	if _, hasErr := result["error"]; !hasErr {
		t.Error("expected error without cloud configured")
	}
}

func TestKnowledgeDiscoverMissingQuery(t *testing.T) {
	cloud := &mockCloudSyncer{}
	reg, _ := setupKnowledgeWithCloud(t, cloud)
	result := callHandler(t, reg, "c4_knowledge_discover", map[string]any{})
	if _, hasErr := result["error"]; !hasErr {
		t.Error("expected error for missing query")
	}
}

func TestKnowledgeDiscoverWithResults(t *testing.T) {
	cloud := &mockCloudSyncer{
		discoverResults: []map[string]any{
			{"doc_id": "pub-1", "title": "Public Insight", "type": "insight"},
			{"doc_id": "pub-2", "title": "Public Pattern", "type": "pattern"},
		},
	}
	reg, _ := setupKnowledgeWithCloud(t, cloud)
	result := callHandler(t, reg, "c4_knowledge_discover", map[string]any{
		"query": "machine learning",
		"limit": float64(5),
	})
	if _, hasErr := result["error"]; hasErr {
		t.Fatalf("unexpected error: %v", result["error"])
	}
	if result["count"] == nil {
		t.Error("expected count field")
	}
}

// ---------------------------------------------------------------------------
// knowledgeReindexNativeHandler (4.2% → covered)
// ---------------------------------------------------------------------------

func TestKnowledgeReindexBasic(t *testing.T) {
	reg, _ := setupKnowledgeNativeTest(t)

	callHandler(t, reg, "c4_knowledge_record", map[string]any{
		"doc_type": "insight",
		"title":    "Doc for Reindex",
		"content":  "some content for reindex test",
	})

	result := callHandler(t, reg, "c4_knowledge_reindex", map[string]any{})
	if result["success"] != true {
		t.Errorf("expected success=true, got %v", result)
	}
	if _, ok := result["indexed"]; !ok {
		t.Error("expected indexed field")
	}
	if _, ok := result["docs_dir"]; !ok {
		t.Error("expected docs_dir field")
	}
}

// ---------------------------------------------------------------------------
// knowledgeIngestNativeHandler (3.1% → covered)
// ---------------------------------------------------------------------------

func TestKnowledgeIngestBothParams(t *testing.T) {
	reg, _ := setupKnowledgeNativeTest(t)
	result := callHandler(t, reg, "c4_knowledge_ingest", map[string]any{
		"file_path": "/some/file.md",
		"url":       "https://example.com",
	})
	if _, hasErr := result["error"]; !hasErr {
		t.Error("expected error when both file_path and url provided")
	}
}

func TestKnowledgeIngestNeitherParam(t *testing.T) {
	reg, _ := setupKnowledgeNativeTest(t)
	result := callHandler(t, reg, "c4_knowledge_ingest", map[string]any{})
	if _, hasErr := result["error"]; !hasErr {
		t.Error("expected error when neither file_path nor url provided")
	}
}

func TestKnowledgeIngestFileMissing(t *testing.T) {
	reg, _ := setupKnowledgeNativeTest(t)
	result := callHandler(t, reg, "c4_knowledge_ingest", map[string]any{
		"file_path": "/totally/nonexistent/file.md",
	})
	if _, hasErr := result["error"]; !hasErr {
		t.Error("expected error for nonexistent file")
	}
}

func TestKnowledgeIngestFileSuccess(t *testing.T) {
	reg, _ := setupKnowledgeNativeTest(t)

	tmpDir := t.TempDir()
	mdPath := filepath.Join(tmpDir, "ingest_test.md")
	mdContent := "# Ingest Test\n\nThis document is ingested from a file.\n\n## Section\n\nContent here."
	if err := os.WriteFile(mdPath, []byte(mdContent), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	result := callHandler(t, reg, "c4_knowledge_ingest", map[string]any{
		"file_path": mdPath,
		"title":     "Ingested File Doc",
	})
	if _, hasErr := result["error"]; hasErr {
		t.Fatalf("unexpected error: %v", result["error"])
	}
	if result["success"] != true {
		t.Errorf("expected success=true, got %v", result)
	}
	if _, ok := result["doc_id"]; !ok {
		t.Error("expected doc_id in result")
	}
}

// ---------------------------------------------------------------------------
// knowledgePullNativeHandler (23.1% → covered)
// ---------------------------------------------------------------------------

func TestKnowledgePullWithCloudDocs(t *testing.T) {
	cloud := &mockCloudList{
		listDocs: []map[string]any{
			{
				"id":      "cloud-1",
				"type":    "insight",
				"title":   "Cloud Doc 1",
				"content": "cloud document body content",
				"version": float64(1),
			},
		},
	}
	reg, _ := setupKnowledgeWithCloud(t, cloud)

	result := callHandler(t, reg, "c4_knowledge_pull", map[string]any{})
	if _, hasErr := result["error"]; hasErr {
		t.Fatalf("unexpected error in pull: %v", result["error"])
	}
	for _, key := range []string{"pulled", "updated", "skipped", "errors"} {
		if _, ok := result[key]; !ok {
			t.Errorf("expected key %q in pull result", key)
		}
	}
}

func TestKnowledgePullWithDocType(t *testing.T) {
	cloud := &mockCloudList{
		listDocs: []map[string]any{
			{"id": "c-1", "type": "insight", "title": "Insight", "content": "body", "version": float64(1)},
			{"id": "c-2", "type": "pattern", "title": "Pattern", "content": "body", "version": float64(1)},
		},
	}
	reg, _ := setupKnowledgeWithCloud(t, cloud)

	result := callHandler(t, reg, "c4_knowledge_pull", map[string]any{
		"doc_type": "insight",
		"limit":    float64(10),
	})
	if _, hasErr := result["error"]; hasErr {
		t.Fatalf("unexpected error: %v", result["error"])
	}
}

// ---------------------------------------------------------------------------
// knowledgePublishNativeHandler (7.1% → covered)
// ---------------------------------------------------------------------------

func TestKnowledgePublishMissingDocID(t *testing.T) {
	reg, _ := setupKnowledgeNativeTest(t)
	result := callHandler(t, reg, "c4_knowledge_publish", map[string]any{})
	if _, hasErr := result["error"]; !hasErr {
		t.Error("expected error for missing doc_id")
	}
}

func TestKnowledgePublishNoCloudErr(t *testing.T) {
	reg, _ := setupKnowledgeNativeTest(t)
	result := callHandler(t, reg, "c4_knowledge_publish", map[string]any{
		"doc_id": "some-id",
	})
	if _, hasErr := result["error"]; !hasErr {
		t.Error("expected error when cloud not configured")
	}
}
