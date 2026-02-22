package handlers

import (
	"strings"
	"testing"
)

func TestNormalizeRequiredChanges(t *testing.T) {
	tests := []struct {
		name  string
		input []string
		want  []string
	}{
		{
			name:  "removes duplicates",
			input: []string{"fix A", "fix B", "fix A"},
			want:  []string{"fix A", "fix B"},
		},
		{
			name:  "trims whitespace",
			input: []string{"  fix A  ", "fix B"},
			want:  []string{"fix A", "fix B"},
		},
		{
			name:  "removes empty strings",
			input: []string{"fix A", "", "  ", "fix B"},
			want:  []string{"fix A", "fix B"},
		},
		{
			name:  "nil input",
			input: nil,
			want:  []string{},
		},
		{
			name:  "all empty",
			input: []string{"", "  "},
			want:  []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeRequiredChanges(tt.input)
			if len(got) != len(tt.want) {
				t.Fatalf("len = %d, want %d; got=%v", len(got), len(tt.want), got)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestCompleteReviewTask(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()

	// Insert a T-task and corresponding R-task
	db.Exec(`INSERT INTO c4_tasks (task_id, title, status, dod) VALUES ('T-001-0', 'Impl', 'done', 'test')`)
	db.Exec(`INSERT INTO c4_tasks (task_id, title, status, dod) VALUES ('R-001-0', 'Review', 'pending', 'test')`)

	// completeReviewTask returns R-task ID when it is pending (for Worker assignment)
	reviewID := store.completeReviewTask("T-001-0")
	if reviewID != "R-001-0" {
		t.Errorf("reviewID = %q, want %q", reviewID, "R-001-0")
	}

	// R-task must stay pending — no auto-cascade; a review Worker will process it
	var status string
	db.QueryRow("SELECT status FROM c4_tasks WHERE task_id='R-001-0'").Scan(&status)
	if status != "pending" {
		t.Errorf("R-001-0 status = %q, want %q", status, "pending")
	}
}

func TestCompleteReviewTask_NonTPrefix(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()

	// R- prefix should not cascade
	result := store.completeReviewTask("R-001-0")
	if result != "" {
		t.Errorf("expected empty for R- prefix, got %q", result)
	}

	// CP- prefix should not cascade
	result = store.completeReviewTask("CP-001")
	if result != "" {
		t.Errorf("expected empty for CP- prefix, got %q", result)
	}
}

func TestCompleteReviewTask_InvalidIDFailFast(t *testing.T) {
	store, _ := newTestSQLiteStore(t)

	// Non-conforming IDs are skipped (fail-fast: no cascade)
	for _, invalid := range []string{"", "T-bad!!!", "T-", "no-prefix"} {
		result := store.completeReviewTask(invalid)
		if result != "" {
			t.Errorf("completeReviewTask(%q) = %q, want \"\" (fail-fast)", invalid, result)
		}
	}
}

func TestCompleteReviewTask_NoMatchingReview(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()

	db.Exec(`INSERT INTO c4_tasks (task_id, title, status, dod) VALUES ('T-002-0', 'Impl', 'done', 'test')`)

	// No R-002-0 exists
	result := store.completeReviewTask("T-002-0")
	if result != "" {
		t.Errorf("expected empty when no review exists, got %q", result)
	}
}

func TestResolveCheckpointTargets(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()

	// Insert checkpoint with JSON array deps
	db.Exec(`INSERT INTO c4_tasks (task_id, title, status, dod, dependencies) VALUES ('CP-001', 'Check', 'pending', 'review', '["T-001-0","R-001-0"]')`)

	taskID, reviewID, err := store.resolveCheckpointTargets("CP-001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if taskID != "T-001-0" {
		t.Errorf("targetTaskID = %q, want %q", taskID, "T-001-0")
	}
	if reviewID != "R-001-0" {
		t.Errorf("targetReviewID = %q, want %q", reviewID, "R-001-0")
	}
}

func TestResolveCheckpointTargets_InfersTaskFromReview(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()

	// Only R- dep, should infer T- from it
	db.Exec(`INSERT INTO c4_tasks (task_id, title, status, dod, dependencies) VALUES ('CP-002', 'Check', 'pending', 'review', '["R-005-0"]')`)

	taskID, reviewID, err := store.resolveCheckpointTargets("CP-002")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if taskID != "T-005-0" {
		t.Errorf("targetTaskID = %q, want %q", taskID, "T-005-0")
	}
	if reviewID != "R-005-0" {
		t.Errorf("targetReviewID = %q, want %q", reviewID, "R-005-0")
	}
}

func TestResolveCheckpointTargets_NotFound(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()

	taskID, reviewID, err := store.resolveCheckpointTargets("CP-NONEXIST")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if taskID != "" || reviewID != "" {
		t.Errorf("expected empty results for non-existent checkpoint, got task=%q review=%q", taskID, reviewID)
	}
}

// --- T-RS-01-0: Past Solutions in RPR DoD ---

// mockKnowledgeSearcher satisfies KnowledgeContextSearcher for testing.
type mockKnowledgeSearcher struct {
	results []KnowledgeSearchResult
	bodies  map[string]string
}

func (m *mockKnowledgeSearcher) Search(query string, topK int, filters map[string]string) ([]KnowledgeSearchResult, error) {
	if len(m.results) > topK {
		return m.results[:topK], nil
	}
	return m.results, nil
}

// mockBodyReader satisfies KnowledgeReader for testing.
type mockBodyReader struct {
	bodies map[string]string
}

func (m *mockBodyReader) GetBody(docID string) (string, error) {
	return m.bodies[docID], nil
}

func setupReviewTask(t *testing.T, store *SQLiteStore, db interface{ Exec(string, ...any) (interface{}, error) }, baseID string) {
	t.Helper()
}

func TestRequestChanges_RPR_AppendsPastSolutions_WhenFound(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()

	// Inject mock knowledge searcher + reader
	searcher := &mockKnowledgeSearcher{
		results: []KnowledgeSearchResult{
			{ID: "doc-001", Title: "Fix nil pointer in scope X", Type: "experiment"},
		},
	}
	reader := &mockBodyReader{
		bodies: map[string]string{
			"doc-001": "Use nil guard before accessing pointer",
		},
	}
	store.knowledgeSearch = searcher
	store.knowledgeReader = reader

	// Insert T-001-0 (done) and R-001-0 (in_progress)
	db.Exec(`INSERT INTO c4_tasks (task_id, title, status, dod, scope) VALUES ('T-001-0', 'Impl', 'done', 'done', 'infra')`)
	db.Exec(`INSERT INTO c4_tasks (task_id, title, status, dod, scope) VALUES ('R-001-0', 'Review', 'in_progress', 'review done', 'infra')`)

	result, err := store.RequestChanges("R-001-0", "nil pointer dereference in handler", []string{"add nil guard"})
	if err != nil {
		t.Fatalf("RequestChanges: %v", err)
	}
	if !result.Success {
		t.Fatal("expected success=true")
	}

	// Verify new T task DoD contains Past Solutions
	var newDoD string
	db.QueryRow(`SELECT dod FROM c4_tasks WHERE task_id=?`, result.NextTaskID).Scan(&newDoD)
	if newDoD == "" {
		t.Fatal("new task DoD is empty")
	}
	if !strings.Contains(newDoD, "## Past Solutions") {
		t.Errorf("DoD does not contain '## Past Solutions'; got: %s", newDoD)
	}
	if !strings.Contains(newDoD, "Fix nil pointer in scope X") {
		t.Errorf("DoD does not contain knowledge title; got: %s", newDoD)
	}
}

func TestRequestChanges_RPR_NoPastSolutions_WhenNotFound(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()

	// Empty searcher
	store.knowledgeSearch = &mockKnowledgeSearcher{results: nil}

	db.Exec(`INSERT INTO c4_tasks (task_id, title, status, dod, scope) VALUES ('T-002-0', 'Impl', 'done', 'done', 'infra')`)
	db.Exec(`INSERT INTO c4_tasks (task_id, title, status, dod, scope) VALUES ('R-002-0', 'Review', 'in_progress', 'review done', 'infra')`)

	result, err := store.RequestChanges("R-002-0", "logic error", []string{"fix logic"})
	if err != nil {
		t.Fatalf("RequestChanges: %v", err)
	}

	var newDoD string
	db.QueryRow(`SELECT dod FROM c4_tasks WHERE task_id=?`, result.NextTaskID).Scan(&newDoD)
	if strings.Contains(newDoD, "## Past Solutions") {
		t.Errorf("DoD should not contain Past Solutions when none found; got: %s", newDoD)
	}
}

func TestRequestChanges_RPR_PastSolutionsFormat_TruncatesLongBody(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()

	// Body with 200 runes (Korean + ASCII mix, > 150 rune limit)
	longBody := "가나다라마바사아자차카타파하 long body truncation test that goes well beyond 150 runes " + strings.Repeat("extra", 30)
	// Make sure it's > 150 runes
	for len([]rune(longBody)) < 160 {
		longBody += "extra"
	}

	searcher := &mockKnowledgeSearcher{
		results: []KnowledgeSearchResult{
			{ID: "doc-003", Title: "Korean Title 가나다", Type: "insight"},
		},
	}
	reader := &mockBodyReader{bodies: map[string]string{"doc-003": longBody}}
	store.knowledgeSearch = searcher
	store.knowledgeReader = reader

	db.Exec(`INSERT INTO c4_tasks (task_id, title, status, dod, scope) VALUES ('T-003-0', 'Impl', 'done', 'done', 'infra')`)
	db.Exec(`INSERT INTO c4_tasks (task_id, title, status, dod, scope) VALUES ('R-003-0', 'Review', 'in_progress', 'review done', 'infra')`)

	result, err := store.RequestChanges("R-003-0", "test query", []string{"fix something"})
	if err != nil {
		t.Fatalf("RequestChanges: %v", err)
	}

	var newDoD string
	db.QueryRow(`SELECT dod FROM c4_tasks WHERE task_id=?`, result.NextTaskID).Scan(&newDoD)

	if !strings.Contains(newDoD, "## Past Solutions") {
		t.Fatal("DoD should contain Past Solutions")
	}
	// Should end with "..." indicating truncation
	if !strings.Contains(newDoD, "...") {
		t.Errorf("Long body should be truncated with '...'; DoD: %s", newDoD)
	}
}
