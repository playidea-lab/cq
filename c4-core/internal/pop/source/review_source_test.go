package source

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/changmin/c4-core/internal/store"
)

// stubStore is a minimal store.Store that returns a fixed task list.
type stubStore struct {
	tasks []store.Task
	err   error
}

func (s *stubStore) ListTasks(filter store.TaskFilter) ([]store.Task, int, error) {
	if s.err != nil {
		return nil, 0, s.err
	}
	var out []store.Task
	for _, t := range s.tasks {
		if filter.Status != "" && t.Status != filter.Status {
			continue
		}
		out = append(out, t)
	}
	return out, len(out), nil
}

// Unused Store methods — satisfy interface.
func (s *stubStore) GetStatus() (*store.ProjectStatus, error)  { return nil, nil }
func (s *stubStore) Start() error                              { return nil }
func (s *stubStore) Clear(bool) error                          { return nil }
func (s *stubStore) TransitionState(from, to string) error     { return nil }
func (s *stubStore) AddTask(task *store.Task) error            { return nil }
func (s *stubStore) DeleteTask(taskID string) error            { return nil }
func (s *stubStore) GetTask(taskID string) (*store.Task, error) {
	for i := range s.tasks {
		if s.tasks[i].ID == taskID {
			return &s.tasks[i], nil
		}
	}
	return nil, nil
}
func (s *stubStore) AssignTask(workerID string) (*store.TaskAssignment, error) { return nil, nil }
func (s *stubStore) SubmitTask(taskID, workerID, commitSHA, handoff string, results []store.ValidationResult) (*store.SubmitResult, error) {
	return nil, nil
}
func (s *stubStore) MarkBlocked(taskID, workerID, failureSignature string, attempts int, lastError string) error {
	return nil
}
func (s *stubStore) ClaimTask(taskID string) (*store.Task, error)                    { return nil, nil }
func (s *stubStore) ReportTask(taskID, summary string, filesChanged []string) error  { return nil }
func (s *stubStore) Checkpoint(checkpointID, decision, notes string, requiredChanges []string, targetTaskID, targetReviewID string) (*store.CheckpointResult, error) {
	return nil, nil
}
func (s *stubStore) RequestChanges(reviewTaskID string, comments string, requiredChanges []string) (*store.RequestChangesResult, error) {
	return nil, nil
}

// --- helpers ---

func makeTask(id, title, status, evidence, updatedAt string) store.Task {
	return store.Task{
		ID:                     id,
		Title:                  title,
		Status:                 status,
		ReviewDecisionEvidence: evidence,
		UpdatedAt:              updatedAt,
	}
}

const ts2024 = "2024-01-15T10:00:00Z"
const ts2024after = "2024-01-15T11:00:00Z"
const ts2023 = "2023-12-01T00:00:00Z"

// --- tests ---

func TestReviewSource_Approve(t *testing.T) {
	s := &stubStore{tasks: []store.Task{
		makeTask("R-001-0", "Review impl", "done", "", ts2024),
	}}
	src := NewReviewSource(s)
	msgs, err := src.RecentMessages(context.Background(), time.Time{}, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if !strings.Contains(msgs[0].Content, "APPROVE") {
		t.Errorf("expected APPROVE in content, got %q", msgs[0].Content)
	}
	if !strings.Contains(msgs[0].Content, "R-001-0") {
		t.Errorf("expected task ID in content, got %q", msgs[0].Content)
	}
}

func TestReviewSource_Reject(t *testing.T) {
	reason := "missing test coverage"
	s := &stubStore{tasks: []store.Task{
		makeTask("R-002-0", "Review feat", "done", reason, ts2024),
	}}
	src := NewReviewSource(s)
	msgs, err := src.RecentMessages(context.Background(), time.Time{}, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if !strings.Contains(msgs[0].Content, "REJECT") {
		t.Errorf("expected REJECT in content, got %q", msgs[0].Content)
	}
	if !strings.Contains(msgs[0].Content, reason) {
		t.Errorf("expected reason %q in content, got %q", reason, msgs[0].Content)
	}
}

func TestReviewSource_SkipsNonReviewTasks(t *testing.T) {
	s := &stubStore{tasks: []store.Task{
		makeTask("T-001-0", "Implementation task", "done", "", ts2024),
		makeTask("R-003-0", "Review task", "done", "", ts2024),
	}}
	src := NewReviewSource(s)
	msgs, err := src.RecentMessages(context.Background(), time.Time{}, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected only 1 message (R- tasks only), got %d", len(msgs))
	}
	if !strings.Contains(msgs[0].ID, "R-003-0") {
		t.Errorf("expected R-003-0 message, got %q", msgs[0].ID)
	}
}

func TestReviewSource_SkipsNonDoneTasks(t *testing.T) {
	// stubStore filters by status; pending review task should not appear.
	s := &stubStore{tasks: []store.Task{
		makeTask("R-004-0", "Pending review", "pending", "", ts2024),
		makeTask("R-005-0", "Done review", "done", "", ts2024),
	}}
	src := NewReviewSource(s)
	msgs, err := src.RecentMessages(context.Background(), time.Time{}, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message (done only), got %d", len(msgs))
	}
}

func TestReviewSource_AfterFilter(t *testing.T) {
	afterTime, _ := time.Parse(time.RFC3339, ts2024after)
	s := &stubStore{tasks: []store.Task{
		// This task's updated_at is before the after threshold.
		makeTask("R-006-0", "Old review", "done", "", ts2023),
		// This task's updated_at is after.
		makeTask("R-007-0", "New review", "done", "", ts2024after),
	}}
	src := NewReviewSource(s)
	msgs, err := src.RecentMessages(context.Background(), afterTime, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Only the task after the threshold should appear.
	if len(msgs) != 0 {
		// ts2024after is equal to afterTime so !ts.After(after) excludes it too
		t.Errorf("expected 0 messages (equal is not after), got %d: %v", len(msgs), msgs)
	}
}

func TestReviewSource_AfterFilter_IncludesNewer(t *testing.T) {
	cutoff, _ := time.Parse(time.RFC3339, ts2023)
	s := &stubStore{tasks: []store.Task{
		makeTask("R-008-0", "Recent review", "done", "", ts2024),
	}}
	src := NewReviewSource(s)
	msgs, err := src.RecentMessages(context.Background(), cutoff, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message for task after cutoff, got %d", len(msgs))
	}
}

func TestReviewSource_LimitRespected(t *testing.T) {
	tasks := []store.Task{
		makeTask("R-010-0", "Review A", "done", "", ts2024),
		makeTask("R-011-0", "Review B", "done", "", ts2024),
		makeTask("R-012-0", "Review C", "done", "", ts2024),
	}
	s := &stubStore{tasks: tasks}
	src := NewReviewSource(s)
	msgs, err := src.RecentMessages(context.Background(), time.Time{}, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) != 2 {
		t.Errorf("expected 2 messages (limit=2), got %d", len(msgs))
	}
}

func TestReviewSource_StoreError(t *testing.T) {
	s := &stubStore{err: fmt.Errorf("db error")}
	src := NewReviewSource(s)
	_, err := src.RecentMessages(context.Background(), time.Time{}, 0)
	if err == nil {
		t.Error("expected error from store, got nil")
	}
}
