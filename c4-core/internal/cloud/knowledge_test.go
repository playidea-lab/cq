package cloud

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newTestKnowledgeClient(serverURL string) *KnowledgeCloudClient {
	return NewKnowledgeCloudClient(serverURL, "test-key", "test-token", "proj-1")
}

func TestSyncDocument(t *testing.T) {
	var receivedBody string
	var receivedPrefer string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPrefer = r.Header.Get("Prefer")
		b, _ := json.Marshal(map[string]string{}) // read body
		_ = b
		buf := make([]byte, 4096)
		n, _ := r.Body.Read(buf)
		receivedBody = string(buf[:n])
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	kc := newTestKnowledgeClient(srv.URL)
	params := map[string]any{
		"doc_type": "experiment",
		"title":    "Test Experiment",
		"content":  "Results of test",
		"tags":     []string{"ml", "test"},
		"domain":   "testing",
	}

	err := kc.SyncDocument(params, "exp-abc123")
	if err != nil {
		t.Fatalf("SyncDocument failed: %v", err)
	}

	// Verify upsert header
	if !strings.Contains(receivedPrefer, "resolution=merge-duplicates") {
		t.Errorf("expected merge-duplicates in Prefer header, got: %s", receivedPrefer)
	}

	// Verify body fields
	var row cloudDocRow
	if err := json.Unmarshal([]byte(receivedBody), &row); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if row.DocID != "exp-abc123" {
		t.Errorf("doc_id = %q, want %q", row.DocID, "exp-abc123")
	}
	if row.DocType != "experiment" {
		t.Errorf("doc_type = %q, want %q", row.DocType, "experiment")
	}
	if row.Title != "Test Experiment" {
		t.Errorf("title = %q, want %q", row.Title, "Test Experiment")
	}
	if row.Body != "Results of test" {
		t.Errorf("body = %q, want %q", row.Body, "Results of test")
	}
	if row.ContentHash == "" {
		t.Error("content_hash should not be empty")
	}
}

func TestSyncDocument_EmptyDocID(t *testing.T) {
	kc := newTestKnowledgeClient("http://localhost")
	err := kc.SyncDocument(map[string]any{}, "")
	if err == nil {
		t.Error("expected error for empty doc_id")
	}
}

func TestSyncDocument_UsesBodyField(t *testing.T) {
	var receivedBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 4096)
		n, _ := r.Body.Read(buf)
		receivedBody = string(buf[:n])
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	kc := newTestKnowledgeClient(srv.URL)
	// Use "body" instead of "content" (Python-style)
	params := map[string]any{
		"doc_type": "insight",
		"title":    "Test",
		"body":     "Body content here",
	}

	err := kc.SyncDocument(params, "ins-xyz")
	if err != nil {
		t.Fatalf("SyncDocument failed: %v", err)
	}

	var row cloudDocRow
	if err := json.Unmarshal([]byte(receivedBody), &row); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if row.Body != "Body content here" {
		t.Errorf("body = %q, want %q", row.Body, "Body content here")
	}
}

func TestSearchDocuments(t *testing.T) {
	var receivedURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedURL = r.URL.String()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]map[string]any{
			{"doc_id": "exp-001", "doc_type": "experiment", "title": "ML Test"},
		})
	}))
	defer srv.Close()

	kc := newTestKnowledgeClient(srv.URL)
	results, err := kc.SearchDocuments("machine learning", "experiment", 5)
	if err != nil {
		t.Fatalf("SearchDocuments failed: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0]["doc_id"] != "exp-001" {
		t.Errorf("doc_id = %v, want exp-001", results[0]["doc_id"])
	}

	// Verify FTS filter in URL
	if !strings.Contains(receivedURL, "tsv=fts.english.") {
		t.Errorf("URL should contain FTS filter, got: %s", receivedURL)
	}
	if !strings.Contains(receivedURL, "doc_type=eq.experiment") {
		t.Errorf("URL should contain doc_type filter, got: %s", receivedURL)
	}
	if !strings.Contains(receivedURL, "limit=5") {
		t.Errorf("URL should contain limit=5, got: %s", receivedURL)
	}
}

func TestSearchDocuments_NoQuery(t *testing.T) {
	var receivedURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedURL = r.URL.String()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]map[string]any{})
	}))
	defer srv.Close()

	kc := newTestKnowledgeClient(srv.URL)
	_, err := kc.SearchDocuments("", "", 0)
	if err != nil {
		t.Fatalf("SearchDocuments failed: %v", err)
	}
	// Should not contain tsv filter
	if strings.Contains(receivedURL, "tsv=") {
		t.Errorf("empty query should not have tsv filter, got: %s", receivedURL)
	}
}

func TestGetDocument(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]map[string]any{
			{"doc_id": "exp-001", "title": "Found Doc", "body": "Content here"},
		})
	}))
	defer srv.Close()

	kc := newTestKnowledgeClient(srv.URL)
	doc, err := kc.GetDocument("exp-001")
	if err != nil {
		t.Fatalf("GetDocument failed: %v", err)
	}
	if doc["title"] != "Found Doc" {
		t.Errorf("title = %v, want Found Doc", doc["title"])
	}
}

func TestGetDocument_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]map[string]any{})
	}))
	defer srv.Close()

	kc := newTestKnowledgeClient(srv.URL)
	_, err := kc.GetDocument("nonexistent")
	if err == nil {
		t.Error("expected error for not found document")
	}
}

func TestListDocuments(t *testing.T) {
	var receivedURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedURL = r.URL.String()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]map[string]any{
			{"doc_id": "pat-001", "doc_type": "pattern"},
			{"doc_id": "pat-002", "doc_type": "pattern"},
		})
	}))
	defer srv.Close()

	kc := newTestKnowledgeClient(srv.URL)
	results, err := kc.ListDocuments("pattern", 20)
	if err != nil {
		t.Fatalf("ListDocuments failed: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}
	if !strings.Contains(receivedURL, "doc_type=eq.pattern") {
		t.Errorf("URL should contain doc_type filter, got: %s", receivedURL)
	}
}

func TestListDocuments_IncludesContentHash(t *testing.T) {
	var receivedURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedURL = r.URL.String()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]map[string]any{
			{"doc_id": "exp-001", "content_hash": "abc123"},
		})
	}))
	defer srv.Close()

	kc := newTestKnowledgeClient(srv.URL)
	results, err := kc.ListDocuments("", 10)
	if err != nil {
		t.Fatalf("ListDocuments failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0]["content_hash"] != "abc123" {
		t.Errorf("content_hash = %v, want abc123", results[0]["content_hash"])
	}
	// Verify select includes content_hash
	if !strings.Contains(receivedURL, "content_hash") {
		t.Errorf("URL should request content_hash in select, got: %s", receivedURL)
	}
}

func TestDeleteDocument(t *testing.T) {
	var receivedMethod string
	var receivedURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		receivedURL = r.URL.String()
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	kc := newTestKnowledgeClient(srv.URL)
	err := kc.DeleteDocument("exp-001")
	if err != nil {
		t.Fatalf("DeleteDocument failed: %v", err)
	}
	if receivedMethod != "DELETE" {
		t.Errorf("method = %s, want DELETE", receivedMethod)
	}
	if !strings.Contains(receivedURL, "doc_id=eq.exp-001") {
		t.Errorf("URL should contain doc_id filter, got: %s", receivedURL)
	}
}

func TestSyncDocument_CloudFailureNonFatal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"internal"}`))
	}))
	defer srv.Close()

	kc := newTestKnowledgeClient(srv.URL)
	err := kc.SyncDocument(map[string]any{
		"doc_type": "experiment",
		"title":    "Test",
		"content":  "Body",
	}, "exp-fail")

	if err == nil {
		t.Error("expected error on 500 response")
	}
	// The caller (proxy interceptor) should handle this non-fatally
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error should mention 500, got: %v", err)
	}
}
