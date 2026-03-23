package handlers

import (
	"strings"
	"testing"
	"time"

	cqstore "github.com/changmin/c4-core/internal/store"
)

// learnLoopKW captures calls to CreateExperiment for learn loop tests.
type learnLoopKW struct {
	entries []learnLoopEntry
}

type learnLoopEntry struct {
	Metadata map[string]any
	Body     string
}

func (m *learnLoopKW) CreateExperiment(metadata map[string]any, body string) (string, error) {
	m.entries = append(m.entries, learnLoopEntry{Metadata: metadata, Body: body})
	return "mock-id", nil
}

// scopeAwareSearcher returns scope-warning results only when query contains "scope-warning".
type scopeAwareSearcher struct {
	warnings []KnowledgeSearchResult
	bodies   map[string]string
}

func (s *scopeAwareSearcher) Search(query string, topK int, filters map[string]string) ([]KnowledgeSearchResult, error) {
	if strings.Contains(query, "scope-warning") {
		if len(s.warnings) > topK {
			return s.warnings[:topK], nil
		}
		return s.warnings, nil
	}
	return nil, nil
}

func (s *scopeAwareSearcher) GetBody(docID string) (string, error) {
	return s.bodies[docID], nil
}

// --- Wire 2 Test: request_changes → scope-warning recording ---

func TestLearnLoop_RequestChanges_RecordsScopeWarning(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()

	kw := &learnLoopKW{}
	store.knowledgeWriter = kw

	// Setup: create T-HRC-001-0 (impl) and R-HRC-001-0 (review)
	store.AddTask(&cqstore.Task{ID: "T-HRC-001-0", Title: "Handler test impl", DoD: "handler dod"})
	store.db.Exec(`UPDATE c4_tasks SET scope='c4-core/internal/llm/', status='done', commit_sha='abc123' WHERE task_id='T-HRC-001-0'`)

	store.AddTask(&cqstore.Task{ID: "R-HRC-001-0", Title: "Review: Handler test impl", DoD: "review", Dependencies: []string{"T-HRC-001-0"}})
	store.db.Exec(`UPDATE c4_tasks SET status='in_progress', worker_id='reviewer-1' WHERE task_id='R-HRC-001-0'`)

	// Call handler directly
	result, err := handleRequestChanges(store, []byte(`{
		"review_task_id": "R-HRC-001-0",
		"comments": "typed-nil interface risk in CQProxyProvider",
		"required_changes": ["use var i Interface guard pattern", "add nil check in IsAvailable"]
	}`))
	if err != nil {
		t.Fatalf("handleRequestChanges: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	// Wait for async goroutine
	time.Sleep(500 * time.Millisecond)

	if len(kw.entries) == 0 {
		t.Fatal("expected scope-warning to be recorded, got 0 entries")
	}

	entry := kw.entries[0]
	meta := entry.Metadata
	if meta["doc_type"] != "scope-warning" {
		t.Errorf("doc_type = %v, want scope-warning", meta["doc_type"])
	}
	tags, _ := meta["tags"].([]string)
	foundScope := false
	for _, tag := range tags {
		if tag == "c4-core/internal/llm/" {
			foundScope = true
		}
	}
	if !foundScope {
		t.Errorf("tags %v should contain scope 'c4-core/internal/llm/'", tags)
	}
	if !strings.Contains(entry.Body, "typed-nil") {
		t.Errorf("body should contain rejection reason, got: %s", entry.Body)
	}
	t.Logf("Wire 2 OK: scope-warning recorded, doc_type=%v, tags=%v", meta["doc_type"], tags)
}

// --- Wire 3 Test: get_task → scope-warning injection (via enrichUnified) ---

func TestLearnLoop_EnrichUnified_InjectsScopeWarnings(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()

	searcher := &scopeAwareSearcher{
		warnings: []KnowledgeSearchResult{
			{ID: "sw-1", Title: "Review rejection: R-HRC-001-0", Type: "scope-warning", Domain: "go-backend"},
			{ID: "sw-2", Title: "Review rejection: R-OLD-005-0", Type: "scope-warning", Domain: "go-backend"},
		},
		bodies: map[string]string{
			"sw-1": "## Rejection Reason\ntyped-nil interface risk\n\n## Required Changes\n- use var i Interface guard",
			"sw-2": "## Rejection Reason\nmissing timeout on http call\n\n## Required Changes\n- add context.WithTimeout",
		},
	}
	store.knowledgeSearch = searcher
	store.knowledgeReader = searcher

	assignment := &TaskAssignment{
		TaskID: "T-NEW-001-0",
		Title:  "New task in same scope",
		Scope:  "c4-core/internal/llm/",
		Domain: "go-backend",
	}

	store.enrichUnified(assignment)

	ctx := assignment.KnowledgeContext
	if ctx == "" {
		t.Fatal("expected non-empty KnowledgeContext")
	}
	if !strings.Contains(ctx, "Past Review Warnings") {
		t.Errorf("KnowledgeContext should contain 'Past Review Warnings', got:\n%s", ctx)
	}
	if !strings.Contains(ctx, "typed-nil") {
		t.Errorf("KnowledgeContext should contain 'typed-nil', got:\n%s", ctx)
	}
	if !strings.Contains(ctx, "missing timeout") {
		t.Errorf("KnowledgeContext should contain 'missing timeout', got:\n%s", ctx)
	}
	t.Logf("Wire 3 OK: scope-warnings injected:\n%s", ctx)
}

// --- Wire 3b Test: 3+ warnings → repeated pattern detection ---

func TestLearnLoop_EnrichUnified_RepeatedPattern_Detected(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()

	kw := &learnLoopKW{}
	store.knowledgeWriter = kw

	searcher := &scopeAwareSearcher{
		warnings: []KnowledgeSearchResult{
			{ID: "sw-1", Title: "Review rejection: R-001-0", Type: "scope-warning"},
			{ID: "sw-2", Title: "Review rejection: R-002-0", Type: "scope-warning"},
			{ID: "sw-3", Title: "Review rejection: R-003-0", Type: "scope-warning"},
		},
		bodies: map[string]string{
			"sw-1": "typed-nil interface risk",
			"sw-2": "missing timeout",
			"sw-3": "no error wrapping",
		},
	}
	store.knowledgeSearch = searcher
	store.knowledgeReader = searcher

	assignment := &TaskAssignment{
		TaskID: "T-NEW-002-0",
		Title:  "New task with repeated warnings",
		Scope:  "c4-core/internal/llm/",
		Domain: "go-backend",
	}

	store.enrichUnified(assignment)

	ctx := assignment.KnowledgeContext
	if ctx == "" {
		t.Fatal("expected non-empty KnowledgeContext")
	}
	if !strings.Contains(ctx, "Repeated rejection pattern") {
		t.Errorf("KnowledgeContext should contain 'Repeated rejection pattern', got:\n%s", ctx)
	}

	// Wait for async goroutine
	time.Sleep(200 * time.Millisecond)

	if len(kw.entries) == 0 {
		t.Fatal("expected pattern to be recorded in knowledge, got 0 entries")
	}
	entry := kw.entries[0]
	if entry.Metadata["doc_type"] != "pattern" {
		t.Errorf("doc_type = %v, want pattern", entry.Metadata["doc_type"])
	}
	if !strings.Contains(entry.Body, "Repeated Rejection Pattern") {
		t.Errorf("body should contain 'Repeated Rejection Pattern', got: %s", entry.Body)
	}
	t.Logf("Wire 3b OK: repeated pattern detected and recorded:\n%s", ctx)
}

// --- Wire 3 Negative: no scope → no injection (via enrichUnified) ---

func TestLearnLoop_EnrichUnified_NoScope_NoWarnings(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()

	searcher := &scopeAwareSearcher{
		warnings: []KnowledgeSearchResult{
			{ID: "sw-1", Title: "Should not appear", Type: "scope-warning"},
		},
	}
	store.knowledgeSearch = searcher

	assignment := &TaskAssignment{
		TaskID: "T-NOSCOPE-001-0",
		Title:  "Task without scope",
		Scope:  "",
	}

	store.enrichUnified(assignment)

	if strings.Contains(assignment.KnowledgeContext, "Past Review Warnings") {
		t.Error("should not inject scope-warnings when scope is empty")
	}
}
