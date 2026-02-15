package knowledge

import (
	"fmt"
	"path/filepath"
	"testing"
)

// mockCloud implements CloudSyncer for testing.
type mockCloud struct {
	docs    map[string]map[string]any
	synced  []string
	failGet map[string]bool
}

func newMockCloud() *mockCloud {
	return &mockCloud{
		docs:    make(map[string]map[string]any),
		failGet: make(map[string]bool),
	}
}

func (m *mockCloud) addDoc(docID, title, docType, body string, version float64) {
	m.docs[docID] = map[string]any{
		"doc_id":   docID,
		"title":    title,
		"doc_type": docType,
		"body":     body,
		"domain":   "test",
		"tags":     []string{"cloud"},
		"version":  version,
	}
}

func (m *mockCloud) SyncDocument(params map[string]any, docID string) error {
	m.synced = append(m.synced, docID)
	return nil
}

func (m *mockCloud) SearchDocuments(query, docType string, limit int) ([]map[string]any, error) {
	var results []map[string]any
	for _, d := range m.docs {
		if docType != "" {
			dt, _ := d["doc_type"].(string)
			if dt != docType {
				continue
			}
		}
		results = append(results, d)
	}
	return results, nil
}

func (m *mockCloud) ListDocuments(docType string, limit int) ([]map[string]any, error) {
	var results []map[string]any
	for _, d := range m.docs {
		if docType != "" {
			dt, _ := d["doc_type"].(string)
			if dt != docType {
				continue
			}
		}
		results = append(results, d)
		if len(results) >= limit {
			break
		}
	}
	return results, nil
}

func (m *mockCloud) GetDocument(docID string) (map[string]any, error) {
	if m.failGet[docID] {
		return nil, fmt.Errorf("get failed for %s", docID)
	}
	d, ok := m.docs[docID]
	if !ok {
		return nil, fmt.Errorf("not found: %s", docID)
	}
	return d, nil
}

func TestPullNewDocs(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewStore(filepath.Join(dir, "knowledge"))
	defer store.Close()

	cloud := newMockCloud()
	cloud.addDoc("exp-cloud01", "Cloud Exp 1", "experiment", "body1", 1)
	cloud.addDoc("ins-cloud02", "Cloud Ins 2", "insight", "body2", 1)

	result, err := Pull(store, cloud, "", 50, false)
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if result.Pulled != 2 {
		t.Errorf("pulled: got %d, want 2", result.Pulled)
	}
	if result.Skipped != 0 {
		t.Errorf("skipped: got %d, want 0", result.Skipped)
	}

	// Verify local
	doc, _ := store.Get("exp-cloud01")
	if doc == nil || doc.Title != "Cloud Exp 1" {
		t.Error("pulled doc not found locally")
	}
}

func TestPullSkipsOlderVersion(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewStore(filepath.Join(dir, "knowledge"))
	defer store.Close()

	// Create local doc with version 2
	store.Create(TypeExperiment, map[string]any{
		"id":    "exp-local01",
		"title": "Local Doc",
	}, "local body")
	body := "updated"
	store.Update("exp-local01", nil, &body) // version 2

	cloud := newMockCloud()
	cloud.addDoc("exp-local01", "Cloud Doc", "experiment", "cloud body", 1) // version 1

	result, _ := Pull(store, cloud, "", 50, false)
	if result.Skipped != 1 {
		t.Errorf("skipped: got %d, want 1", result.Skipped)
	}
	if result.Pulled != 0 {
		t.Errorf("pulled: got %d, want 0", result.Pulled)
	}
}

func TestPullForceOverwrite(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewStore(filepath.Join(dir, "knowledge"))
	defer store.Close()

	store.Create(TypeExperiment, map[string]any{
		"id":    "exp-local01",
		"title": "Old Title",
	}, "old body")

	cloud := newMockCloud()
	cloud.addDoc("exp-local01", "New Title", "experiment", "new body", 1)

	result, _ := Pull(store, cloud, "", 50, true) // force=true
	if result.Updated != 1 {
		t.Errorf("updated: got %d, want 1", result.Updated)
	}

	doc, _ := store.Get("exp-local01")
	if doc.Title != "New Title" {
		t.Errorf("title: got %q, want New Title", doc.Title)
	}
}

func TestPullHandlesGetError(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewStore(filepath.Join(dir, "knowledge"))
	defer store.Close()

	cloud := newMockCloud()
	cloud.addDoc("exp-err01", "Error Doc", "experiment", "body", 1)
	cloud.failGet["exp-err01"] = true

	result, _ := Pull(store, cloud, "", 50, false)
	if len(result.Errors) != 1 {
		t.Errorf("errors: got %d, want 1", len(result.Errors))
	}
	if result.Pulled != 0 {
		t.Errorf("pulled: got %d, want 0", result.Pulled)
	}
}

func TestPullNilCloud(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewStore(filepath.Join(dir, "knowledge"))
	defer store.Close()

	_, err := Pull(store, nil, "", 50, false)
	if err == nil {
		t.Error("expected error for nil cloud")
	}
}

func TestSyncAfterRecord(t *testing.T) {
	cloud := newMockCloud()
	err := SyncAfterRecord(cloud, map[string]any{"title": "test"}, "exp-123")
	if err != nil {
		t.Fatalf("SyncAfterRecord: %v", err)
	}
	if len(cloud.synced) != 1 || cloud.synced[0] != "exp-123" {
		t.Errorf("synced: %v", cloud.synced)
	}
}

func TestSyncAfterRecordNilCloud(t *testing.T) {
	err := SyncAfterRecord(nil, nil, "")
	if err != nil {
		t.Error("nil cloud should return nil")
	}
}

func TestExtractTags(t *testing.T) {
	// []string
	tags := extractTags([]string{"a", "b"})
	if len(tags) != 2 {
		t.Errorf("[]string: got %v", tags)
	}

	// []any
	tags = extractTags([]any{"c", "d"})
	if len(tags) != 2 {
		t.Errorf("[]any: got %v", tags)
	}

	// JSON string
	tags = extractTags(`["e","f"]`)
	if len(tags) != 2 {
		t.Errorf("JSON string: got %v", tags)
	}

	// nil
	tags = extractTags(nil)
	if tags != nil {
		t.Errorf("nil: got %v", tags)
	}
}
