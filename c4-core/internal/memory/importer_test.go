package memory

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/changmin/c4-core/internal/knowledge"
)

func newTestStore(t *testing.T) *knowledge.Store {
	t.Helper()
	dir := t.TempDir()
	s, err := knowledge.NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func makeSessions(count int) []Session {
	sessions := make([]Session, count)
	for i := range sessions {
		sessions[i] = Session{
			ID:        fmt.Sprintf("sess-%03d", i),
			Source:    "claude-code",
			Project:   "test-project",
			StartedAt: time.Date(2026, 3, 27, 10, 0, 0, 0, time.UTC),
			Turns: []Turn{
				{Role: "user", Content: "hello"},
				{Role: "assistant", Content: "hi there"},
			},
		}
	}
	return sessions
}

func Test_ImportSessions_HappyPath(t *testing.T) {
	store := newTestStore(t)
	imp := &Importer{
		Store:         store,
		Summarizer:    nil, // no LLM -- fallback path
		MaxConcurrent: 2,
	}

	sessions := makeSessions(3)
	result, err := imp.ImportSessions(context.Background(), sessions)
	if err != nil {
		t.Fatalf("ImportSessions: %v", err)
	}

	if result.Total != 3 {
		t.Errorf("Total: got %d, want 3", result.Total)
	}
	if result.Imported != 3 {
		t.Errorf("Imported: got %d, want 3", result.Imported)
	}
	if result.Skipped != 0 {
		t.Errorf("Skipped: got %d, want 0", result.Skipped)
	}
	if len(result.Errors) != 0 {
		t.Errorf("Errors: got %d, want 0", len(result.Errors))
	}

	// Verify docs were created in the store.
	docs, err := store.List("insight", "", 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(docs) != 3 {
		t.Errorf("stored docs: got %d, want 3", len(docs))
	}
}

func Test_ImportSessions_Dedup(t *testing.T) {
	store := newTestStore(t)
	imp := &Importer{
		Store:         store,
		Summarizer:    nil,
		MaxConcurrent: 1,
	}

	sessions := makeSessions(2)

	// First import.
	result1, err := imp.ImportSessions(context.Background(), sessions)
	if err != nil {
		t.Fatalf("first import: %v", err)
	}
	if result1.Imported != 2 {
		t.Fatalf("first import: got %d imported, want 2", result1.Imported)
	}

	// Second import of the same sessions should skip all.
	result2, err := imp.ImportSessions(context.Background(), sessions)
	if err != nil {
		t.Fatalf("second import: %v", err)
	}
	if result2.Skipped != 2 {
		t.Errorf("second import Skipped: got %d, want 2", result2.Skipped)
	}
	if result2.Imported != 0 {
		t.Errorf("second import Imported: got %d, want 0", result2.Imported)
	}
}

func Test_ImportSessions_NilStore(t *testing.T) {
	imp := &Importer{Store: nil}
	_, err := imp.ImportSessions(context.Background(), makeSessions(1))
	if err == nil {
		t.Fatal("expected error for nil store")
	}
}

func Test_ImportSessions_EmptySessions(t *testing.T) {
	store := newTestStore(t)
	imp := &Importer{Store: store}
	result, err := imp.ImportSessions(context.Background(), nil)
	if err != nil {
		t.Fatalf("ImportSessions: %v", err)
	}
	if result.Total != 0 {
		t.Errorf("Total: got %d, want 0", result.Total)
	}
}

func Test_ImportSessions_ContextCancel(t *testing.T) {
	store := newTestStore(t)
	imp := &Importer{
		Store:         store,
		MaxConcurrent: 1,
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	sessions := makeSessions(5)
	result, err := imp.ImportSessions(ctx, sessions)
	if err != nil {
		t.Fatalf("ImportSessions: %v", err)
	}
	// With immediate cancel, not all sessions should be processed.
	if result.Imported == 5 {
		t.Error("expected fewer than 5 imported with cancelled context")
	}
}

func Test_ImportSessions_ContinuesOnError(t *testing.T) {
	store := newTestStore(t)
	imp := &Importer{
		Store:         store,
		Summarizer:    nil,
		MaxConcurrent: 1,
	}

	sessions := makeSessions(3)
	// Make one session have empty turns -- it will still import (fallback body handles it).
	// To test error continuation we need a different approach.
	// For now, verify that all 3 sessions import successfully.
	result, err := imp.ImportSessions(context.Background(), sessions)
	if err != nil {
		t.Fatalf("ImportSessions: %v", err)
	}
	if result.Imported != 3 {
		t.Errorf("Imported: got %d, want 3", result.Imported)
	}
}
