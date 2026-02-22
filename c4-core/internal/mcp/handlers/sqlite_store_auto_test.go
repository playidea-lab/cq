package handlers

import (
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// mockKnowledgeWriter tracks calls to CreateExperiment for testing.
type mockKnowledgeWriter struct {
	mu       sync.Mutex
	called   atomic.Int32
	lastMeta map[string]any
	lastBody string
}

func (m *mockKnowledgeWriter) CreateExperiment(metadata map[string]any, body string) (string, error) {
	m.mu.Lock()
	m.lastMeta = metadata
	m.lastBody = body
	m.mu.Unlock()
	m.called.Add(1)
	return "doc-mock-001", nil
}

func TestMarkBlocked_AutoRecordsFailurePattern_WhenSignatureNonEmpty(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()

	mock := &mockKnowledgeWriter{}
	store.knowledgeWriter = mock

	if err := store.AddTask(&Task{
		ID:    "T-FP-001-0",
		Title: "test task",
		Scope: "infra/supabase",
	}); err != nil {
		t.Fatalf("add task: %v", err)
	}
	_, err := store.AssignTask("worker-fp")
	if err != nil {
		t.Fatalf("assign: %v", err)
	}

	if err := store.MarkBlocked("T-FP-001-0", "worker-fp", "nil pointer dereference", 1, "panic"); err != nil {
		t.Fatalf("MarkBlocked: %v", err)
	}

	// Give goroutine time to complete
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if mock.called.Load() > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if mock.called.Load() == 0 {
		t.Fatal("CreateExperiment was not called; expected autoRecordFailurePattern to call it")
	}
}

func TestMarkBlocked_NoKnowledgeRecord_WhenSignatureEmpty(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()

	mock := &mockKnowledgeWriter{}
	store.knowledgeWriter = mock

	if err := store.AddTask(&Task{
		ID:    "T-FP-002-0",
		Title: "test task 2",
		Scope: "infra/supabase",
	}); err != nil {
		t.Fatalf("add task: %v", err)
	}
	_, err := store.AssignTask("worker-fp2")
	if err != nil {
		t.Fatalf("assign: %v", err)
	}

	// Empty signature — no knowledge record
	if err := store.MarkBlocked("T-FP-002-0", "worker-fp2", "", 1, "some error"); err != nil {
		t.Fatalf("MarkBlocked: %v", err)
	}

	time.Sleep(100 * time.Millisecond)
	if mock.called.Load() != 0 {
		t.Errorf("CreateExperiment called %d times; want 0 (empty signature)", mock.called.Load())
	}
}

func TestAutoRecordFailurePattern_ContentContainsScope(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()

	mock := &mockKnowledgeWriter{}
	store.knowledgeWriter = mock

	task := &Task{
		ID:    "T-FP-003-0",
		Title: "scope test",
		Scope: "infra/supabase",
	}
	store.autoRecordFailurePattern(task, "nil pointer", "panic: nil")

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if mock.called.Load() > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if mock.called.Load() == 0 {
		t.Fatal("CreateExperiment was not called")
	}
	mock.mu.Lock()
	defer mock.mu.Unlock()
	if mock.lastBody == "" {
		t.Fatal("body is empty")
	}
	if !strings.Contains(mock.lastBody, "infra/supabase") {
		t.Errorf("body %q does not contain scope", mock.lastBody)
	}
	if !strings.Contains(mock.lastBody, "nil pointer") {
		t.Errorf("body %q does not contain signature", mock.lastBody)
	}
}
