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

func (m *mockCloud) DeleteDocument(docID string) error {
	delete(m.docs, docID)
	return nil
}

func (m *mockCloud) DiscoverPublic(query string, docType string, limit int) ([]map[string]any, error) {
	return m.SearchDocuments(query, docType, limit)
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

func TestPushChanges(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewStore(filepath.Join(dir, "knowledge"))
	defer store.Close()

	store.Create(TypeExperiment, map[string]any{
		"title":  "Local Doc 1",
		"domain": "test",
	}, "body 1")
	store.Create(TypeInsight, map[string]any{
		"title": "Local Doc 2",
	}, "body 2")

	cloud := newMockCloud()

	result, err := PushChanges(store, cloud, 100)
	if err != nil {
		t.Fatalf("PushChanges: %v", err)
	}
	if result.Pushed != 2 {
		t.Errorf("pushed: got %d, want 2", result.Pushed)
	}
	if len(cloud.synced) != 2 {
		t.Errorf("cloud synced: got %d, want 2", len(cloud.synced))
	}
}

func TestPushChangesNilCloud(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewStore(filepath.Join(dir, "knowledge"))
	defer store.Close()

	_, err := PushChanges(store, nil, 50)
	if err == nil {
		t.Error("expected error for nil cloud")
	}
}

func TestSyncAfterUpdate(t *testing.T) {
	cloud := newMockCloud()
	doc := &Document{
		Type:  TypeExperiment,
		Title: "Updated Doc",
		Body:  "new body",
	}
	err := SyncAfterUpdate(cloud, "exp-123", doc)
	if err != nil {
		t.Fatalf("SyncAfterUpdate: %v", err)
	}
	if len(cloud.synced) != 1 || cloud.synced[0] != "exp-123" {
		t.Errorf("synced: %v", cloud.synced)
	}
}

func TestSyncAfterDelete(t *testing.T) {
	cloud := newMockCloud()
	cloud.addDoc("exp-del", "To Delete", "experiment", "body", 1)

	err := SyncAfterDelete(cloud, "exp-del")
	if err != nil {
		t.Fatalf("SyncAfterDelete: %v", err)
	}
	// Should be deleted from mock
	if _, ok := cloud.docs["exp-del"]; ok {
		t.Error("document should be deleted from cloud")
	}
}

func TestPullDeletesLocalDocsRemovedFromCloud(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewStore(filepath.Join(dir, "knowledge"))
	defer store.Close()

	// Create local doc that doesn't exist in cloud
	store.Create(TypeExperiment, map[string]any{
		"id":    "exp-orphan",
		"title": "Orphan Doc",
	}, "orphan body")

	// Cloud has a different doc
	cloud := newMockCloud()
	cloud.addDoc("exp-cloud01", "Cloud Doc", "experiment", "body1", 1)

	result, err := Pull(store, cloud, "", 50, false)
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if result.Pulled != 1 {
		t.Errorf("pulled: got %d, want 1", result.Pulled)
	}
	if result.Deleted != 1 {
		t.Errorf("deleted: got %d, want 1", result.Deleted)
	}
	if len(result.DeletedIDs) != 1 || result.DeletedIDs[0] != "exp-orphan" {
		t.Errorf("deleted_ids: got %v, want [exp-orphan]", result.DeletedIDs)
	}

	// Verify orphan was deleted
	doc, _ := store.Get("exp-orphan")
	if doc != nil {
		t.Error("orphan doc should have been deleted")
	}
}

func TestPullDeleteSkippedWhenFilteringByDocType(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewStore(filepath.Join(dir, "knowledge"))
	defer store.Close()

	// Create local docs of different types
	store.Create(TypeExperiment, map[string]any{
		"id":    "exp-local",
		"title": "Local Experiment",
	}, "exp body")
	store.Create(TypeInsight, map[string]any{
		"id":    "ins-local",
		"title": "Local Insight",
	}, "ins body")

	// Cloud only has experiments (filter will only return experiments)
	cloud := newMockCloud()
	cloud.addDoc("exp-cloud01", "Cloud Exp", "experiment", "body1", 1)

	// Pull with docType filter — should NOT delete ins-local
	result, err := Pull(store, cloud, "experiment", 50, false)
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if result.Deleted != 0 {
		t.Errorf("deleted: got %d, want 0 (should skip deletion when filtering by docType)", result.Deleted)
	}

	// Verify insight doc still exists
	doc, _ := store.Get("ins-local")
	if doc == nil {
		t.Error("insight doc should NOT have been deleted when filtering by docType")
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

func TestStripMetadata(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"task IDs", "Fixed T-001-0 and R-042 issues", "Fixed [task] and [task] issues"},
		{"checkpoint", "See CP-003 for details", "See [task] for details"},
		{"git SHA 8 chars", "commit abcdef12 merged", "commit [commit] merged"},
		{"git SHA 40 chars", "sha 1234567890abcdef1234567890abcdef12345678 ok", "sha [commit] ok"},
		{"short hex preserved", "value abc1234 stays", "value abc1234 stays"},
		{"abs path /Users", "file at /Users/changmin/git/cq/main.go here", "file at [path] here"},
		{"abs path /home", "see /home/user/project/file.py end", "see [path] end"},
		{"no match", "plain text without identifiers", "plain text without identifiers"},
		{"mixed", "T-001 fixed in abcdef12 at /Users/dev/src/f.go", "[task] fixed in [commit] at [path]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StripMetadata(tt.input)
			if got != tt.want {
				t.Errorf("StripMetadata(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestPublishDocument(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewStore(filepath.Join(dir, "knowledge"))
	defer store.Close()

	store.Create(TypeInsight, map[string]any{
		"id":    "ins-publish",
		"title": "Test Insight",
	}, "Insight about T-001 at /Users/dev/src/main.go")

	cloud := newMockCloud()

	err := PublishDocument(store, cloud, "ins-publish")
	if err != nil {
		t.Fatalf("PublishDocument: %v", err)
	}

	// Verify cloud sync was called
	if len(cloud.synced) != 1 || cloud.synced[0] != "ins-publish" {
		t.Errorf("synced: got %v, want [ins-publish]", cloud.synced)
	}

	// Verify local visibility updated
	doc, _ := store.Get("ins-publish")
	if doc == nil {
		t.Fatal("doc should exist")
	}
	if doc.Visibility != "public" {
		t.Errorf("visibility: got %q, want public", doc.Visibility)
	}
}

func TestPublishDocumentNilCloud(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewStore(filepath.Join(dir, "knowledge"))
	defer store.Close()

	err := PublishDocument(store, nil, "any-id")
	if err == nil {
		t.Error("expected error for nil cloud")
	}
}

func TestPublishDocumentNotFound(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewStore(filepath.Join(dir, "knowledge"))
	defer store.Close()

	cloud := newMockCloud()
	err := PublishDocument(store, cloud, "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent doc")
	}
}
