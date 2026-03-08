package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/changmin/c4-core/internal/hub"
	"github.com/changmin/c4-core/internal/knowledge"
)

// mustNewHypothesisStore creates a temp knowledge store.
func mustNewHypothesisStore(t *testing.T) *knowledge.Store {
	t.Helper()
	dir := t.TempDir()
	s, err := knowledge.NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// createHypDoc creates a hypothesis document and optionally injects expires_at/yaml_draft
// into the markdown frontmatter directly (since the Store doesn't map these fields).
func createHypDoc(t *testing.T, store *knowledge.Store, status, expiresAt, yamlDraft string) string {
	t.Helper()
	docID, err := store.Create(knowledge.TypeHypothesis, map[string]any{
		"title":  "Test hypothesis",
		"status": status,
	}, "insight body text")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Inject expires_at and yaml_draft into frontmatter.
	if expiresAt != "" || yamlDraft != "" {
		filePath := filepath.Join(store.DocsDir(), docID+".md")
		data, err := os.ReadFile(filePath)
		if err != nil {
			t.Fatalf("read doc file: %v", err)
		}
		content := string(data)
		// Insert after the opening "---\n"
		injection := ""
		if expiresAt != "" {
			injection += "expires_at: " + expiresAt + "\n"
		}
		if yamlDraft != "" {
			injection += "yaml_draft: " + yamlDraft + "\n"
		}
		content = strings.Replace(content, "---\n", "---\n"+injection, 1)
		if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
			t.Fatalf("write doc file: %v", err)
		}
	}
	return docID
}

// newTestHubServer starts a test HTTP server that accepts or rejects job submissions.
func newTestHubServer(t *testing.T, fail bool) (*httptest.Server, *string) {
	t.Helper()
	var lastJobID string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health", "/v1/health":
			w.WriteHeader(http.StatusOK)
		case "/jobs/submit", "/v1/jobs/submit":
			if fail {
				http.Error(w, "hub error", http.StatusInternalServerError)
				return
			}
			var req hub.JobSubmitRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			lastJobID = "job-hyp-001"
			json.NewEncoder(w).Encode(hub.JobSubmitResponse{
				JobID:  lastJobID,
				Status: "QUEUED",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &lastJobID
}

// setupProjectDir creates a minimal .c4/config.yaml pointing to the hub server.
func setupProjectDir(t *testing.T, hubURL string) string {
	t.Helper()
	tmpDir := t.TempDir()
	c4Dir := filepath.Join(tmpDir, ".c4")
	if err := os.MkdirAll(c4Dir, 0755); err != nil {
		t.Fatal(err)
	}
	cfg := "hub:\n  enabled: true\n  url: " + hubURL + "\n"
	if err := os.WriteFile(filepath.Join(c4Dir, "config.yaml"), []byte(cfg), 0644); err != nil {
		t.Fatal(err)
	}
	return tmpDir
}

// TestSuggestApprove_HappyPath: pending hyp → hub submit → approved.
func TestSuggestApprove_HappyPath(t *testing.T) {
	srv, _ := newTestHubServer(t, false)
	tmpDir := setupProjectDir(t, srv.URL)

	// Use a knowledge store in .c4/knowledge under tmpDir.
	store, err := knowledge.NewStore(filepath.Join(tmpDir, ".c4", "knowledge"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	future := time.Now().Add(24 * time.Hour).Format(time.RFC3339)
	docID := createHypDoc(t, store, "pending", future, "python train.py")

	// Temporarily override projectDir and restore after.
	origDir := projectDir
	projectDir = tmpDir
	defer func() { projectDir = origDir }()

	cmd := suggestApproveCmd
	if err := cmd.RunE(cmd, []string{docID}); err != nil {
		t.Fatalf("RunE: %v", err)
	}

	// Verify status updated to approved.
	doc, err := store.Get(docID)
	if err != nil || doc == nil {
		t.Fatalf("Get: %v", err)
	}
	if doc.Status != "approved" {
		t.Errorf("Status = %q, want 'approved'", doc.Status)
	}
}

// TestSuggestApprove_HubFailure: hub error → status stays pending.
func TestSuggestApprove_HubFailure(t *testing.T) {
	srv, _ := newTestHubServer(t, true) // fail=true
	tmpDir := setupProjectDir(t, srv.URL)

	store, err := knowledge.NewStore(filepath.Join(tmpDir, ".c4", "knowledge"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	future := time.Now().Add(24 * time.Hour).Format(time.RFC3339)
	docID := createHypDoc(t, store, "pending", future, "python train.py")

	origDir := projectDir
	projectDir = tmpDir
	defer func() { projectDir = origDir }()

	err = suggestApproveCmd.RunE(suggestApproveCmd, []string{docID})
	if err == nil {
		t.Fatal("expected error on hub failure, got nil")
	}
	if !strings.Contains(err.Error(), "Hub 제출 실패") {
		t.Errorf("error = %q, want to contain 'Hub 제출 실패'", err.Error())
	}

	// Status must still be pending.
	doc, err2 := store.Get(docID)
	if err2 != nil || doc == nil {
		t.Fatalf("Get: %v", err2)
	}
	if doc.Status != "pending" {
		t.Errorf("Status = %q, want 'pending' after hub failure", doc.Status)
	}
	if doc.HypothesisStatus == "approved" {
		t.Errorf("HypothesisStatus should not be 'approved' after hub failure")
	}
}

// TestSuggestApprove_Expired: expires_at<now → error, submit not called.
func TestSuggestApprove_Expired(t *testing.T) {
	srv, jobID := newTestHubServer(t, false)
	tmpDir := setupProjectDir(t, srv.URL)

	store, err := knowledge.NewStore(filepath.Join(tmpDir, ".c4", "knowledge"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	past := time.Now().Add(-1 * time.Hour).Format(time.RFC3339)
	docID := createHypDoc(t, store, "pending", past, "python train.py")

	origDir := projectDir
	projectDir = tmpDir
	defer func() { projectDir = origDir }()

	err = suggestApproveCmd.RunE(suggestApproveCmd, []string{docID})
	if err == nil {
		t.Fatal("expected error for expired hypothesis, got nil")
	}
	if !strings.Contains(err.Error(), "만료된 제안") {
		t.Errorf("error = %q, want to contain '만료된 제안'", err.Error())
	}
	if *jobID != "" {
		t.Errorf("hub should not have been called for expired hypothesis")
	}
}

// TestSuggestApprove_AlreadyApproved: approved hyp → error.
func TestSuggestApprove_AlreadyApproved(t *testing.T) {
	srv, _ := newTestHubServer(t, false)
	tmpDir := setupProjectDir(t, srv.URL)

	store, err := knowledge.NewStore(filepath.Join(tmpDir, ".c4", "knowledge"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// Create with status=approved.
	docID, err := store.Create(knowledge.TypeHypothesis, map[string]any{
		"title":  "Already approved",
		"status": "approved",
	}, "insight")
	if err != nil {
		t.Fatal(err)
	}

	origDir := projectDir
	projectDir = tmpDir
	defer func() { projectDir = origDir }()

	err = suggestApproveCmd.RunE(suggestApproveCmd, []string{docID})
	if err == nil {
		t.Fatal("expected error for already-approved hypothesis, got nil")
	}
	if !strings.Contains(err.Error(), "이미 처리된 제안") {
		t.Errorf("error = %q, want to contain '이미 처리된 제안'", err.Error())
	}
}

// TestSuggestList_ShowsPending: pending 2건 → 목록 출력.
func TestSuggestList_ShowsPending(t *testing.T) {
	tmpDir := t.TempDir()
	c4Dir := filepath.Join(tmpDir, ".c4")
	os.MkdirAll(c4Dir, 0755)
	// No hub config needed for list.
	os.WriteFile(filepath.Join(c4Dir, "config.yaml"), []byte(""), 0644)

	store, err := knowledge.NewStore(filepath.Join(c4Dir, "knowledge"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// Create 2 pending and 1 approved.
	createHypDoc(t, store, "pending", "", "")
	createHypDoc(t, store, "pending", "", "")
	createHypDoc(t, store, "approved", "", "")

	origDir := projectDir
	projectDir = tmpDir
	defer func() { projectDir = origDir }()

	// Capture output.
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err = suggestListCmd.RunE(suggestListCmd, nil)
	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("RunE: %v", err)
	}

	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	// Should contain the 2 pending IDs (IDs start with "hyp-").
	count := strings.Count(output, "hyp-")
	// Header line contains no hyp- prefix, so count should be 2.
	if count < 2 {
		t.Errorf("output contains %d 'hyp-' entries, want >= 2\noutput:\n%s", count, output)
	}
}
