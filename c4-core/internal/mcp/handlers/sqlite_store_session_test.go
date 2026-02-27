package handlers

import (
	"testing"

	"github.com/changmin/c4-core/internal/store"
)

// newTestStoreWithSession creates a SQLiteStore with a specific session ID against a shared DB.
func newTestStoreWithSession(t *testing.T, sessionID string) *SQLiteStore {
	t.Helper()
	db := newTestDB(t)
	s, err := NewSQLiteStore(db, WithSessionID(sessionID))
	if err != nil {
		t.Fatalf("NewSQLiteStore(session=%q): %v", sessionID, err)
	}
	return s
}

// newTestStoreWithSharedDB creates a SQLiteStore with a given session ID sharing an existing DB.
func newTestStoreWithSharedDB(t *testing.T, s *SQLiteStore, sessionID string) *SQLiteStore {
	t.Helper()
	store2, err := NewSQLiteStore(s.db, WithSessionID(sessionID))
	if err != nil {
		t.Fatalf("NewSQLiteStore(session=%q, shared db): %v", sessionID, err)
	}
	return store2
}

// TestSessionIsolation_AddAndList verifies that session A tasks are not visible in session B's ListTasks.
func TestSessionIsolation_AddAndList(t *testing.T) {
	storeA := newTestStoreWithSession(t, "session-A")
	storeB := newTestStoreWithSharedDB(t, storeA, "session-B")

	// Add tasks in each session
	if err := storeA.AddTask(&Task{ID: "T-A-001", Title: "Task A1", ExecutionMode: "direct"}); err != nil {
		t.Fatalf("AddTask A: %v", err)
	}
	if err := storeB.AddTask(&Task{ID: "T-B-001", Title: "Task B1", ExecutionMode: "direct"}); err != nil {
		t.Fatalf("AddTask B: %v", err)
	}

	// Session A should only see T-A-001
	tasksA, _, err := storeA.ListTasks(store.TaskFilter{})
	if err != nil {
		t.Fatalf("ListTasks A: %v", err)
	}
	if len(tasksA) != 1 || tasksA[0].ID != "T-A-001" {
		t.Errorf("session A: expected [T-A-001], got %v", taskIDs(tasksA))
	}

	// Session B should only see T-B-001
	tasksB, _, err := storeB.ListTasks(store.TaskFilter{})
	if err != nil {
		t.Fatalf("ListTasks B: %v", err)
	}
	if len(tasksB) != 1 || tasksB[0].ID != "T-B-001" {
		t.Errorf("session B: expected [T-B-001], got %v", taskIDs(tasksB))
	}
}

// TestSessionIsolation_TaskAssignment verifies that a worker in session A cannot be assigned
// a task that belongs to session B.
func TestSessionIsolation_TaskAssignment(t *testing.T) {
	storeA := newTestStoreWithSession(t, "session-A")
	storeB := newTestStoreWithSharedDB(t, storeA, "session-B")

	// Add a task only in session B
	if err := storeB.AddTask(&Task{ID: "T-B-002", Title: "Task B2", ExecutionMode: "worker"}); err != nil {
		t.Fatalf("AddTask B: %v", err)
	}

	// Worker in session A should find no tasks
	assignment, err := storeA.AssignTask("worker-A-1")
	if err != nil {
		t.Fatalf("AssignTask A: %v", err)
	}
	if assignment != nil {
		t.Errorf("session A worker assigned session-B task %q — expected nil", assignment.TaskID)
	}

	// Worker in session B should find T-B-002
	assignmentB, err := storeB.AssignTask("worker-B-1")
	if err != nil {
		t.Fatalf("AssignTask B: %v", err)
	}
	if assignmentB == nil || assignmentB.TaskID != "T-B-002" {
		t.Errorf("session B worker: expected T-B-002, got %v", assignmentB)
	}
}

// TestSessionIsolation_LegacyVisible verifies that tasks with session_id='' are visible in all sessions.
func TestSessionIsolation_LegacyVisible(t *testing.T) {
	storeA := newTestStoreWithSession(t, "session-A")
	storeB := newTestStoreWithSharedDB(t, storeA, "session-B")

	// Insert a legacy task (session_id='') directly via SQL
	_, err := storeA.db.Exec(`
		INSERT INTO c4_tasks (task_id, title, scope, dod, status, dependencies, domain, priority, model, execution_mode, session_id, created_at, updated_at)
		VALUES ('T-LEGACY-001', 'Legacy Task', '', '', 'pending', '[]', '', 0, '', 'direct', '', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)
	if err != nil {
		t.Fatalf("insert legacy task: %v", err)
	}

	// Both sessions should see the legacy task
	tasksA, _, err := storeA.ListTasks(store.TaskFilter{})
	if err != nil {
		t.Fatalf("ListTasks A: %v", err)
	}
	if !containsTaskID(tasksA, "T-LEGACY-001") {
		t.Errorf("session A: expected legacy task T-LEGACY-001, got %v", taskIDs(tasksA))
	}

	tasksB, _, err := storeB.ListTasks(store.TaskFilter{})
	if err != nil {
		t.Fatalf("ListTasks B: %v", err)
	}
	if !containsTaskID(tasksB, "T-LEGACY-001") {
		t.Errorf("session B: expected legacy task T-LEGACY-001, got %v", taskIDs(tasksB))
	}
}

// TestSessionIsolation_StatusScoped verifies that GetStatus aggregates only the session's own tasks.
func TestSessionIsolation_StatusScoped(t *testing.T) {
	storeA := newTestStoreWithSession(t, "session-A")
	storeB := newTestStoreWithSharedDB(t, storeA, "session-B")

	// 2 tasks in A, 3 tasks in B
	for i, id := range []string{"T-A-S1", "T-A-S2"} {
		_ = i
		if err := storeA.AddTask(&Task{ID: id, Title: id, ExecutionMode: "direct"}); err != nil {
			t.Fatalf("AddTask A %s: %v", id, err)
		}
	}
	for _, id := range []string{"T-B-S1", "T-B-S2", "T-B-S3"} {
		if err := storeB.AddTask(&Task{ID: id, Title: id, ExecutionMode: "direct"}); err != nil {
			t.Fatalf("AddTask B %s: %v", id, err)
		}
	}

	statusA, err := storeA.GetStatus()
	if err != nil {
		t.Fatalf("GetStatus A: %v", err)
	}
	if statusA.TotalTasks != 2 {
		t.Errorf("session A: expected TotalTasks=2, got %d", statusA.TotalTasks)
	}

	statusB, err := storeB.GetStatus()
	if err != nil {
		t.Fatalf("GetStatus B: %v", err)
	}
	if statusB.TotalTasks != 3 {
		t.Errorf("session B: expected TotalTasks=3, got %d", statusB.TotalTasks)
	}
}

// TestSessionIsolation_EmptySessionSeesAll verifies that a store with no session ID sees all tasks.
func TestSessionIsolation_EmptySessionSeesAll(t *testing.T) {
	storeA := newTestStoreWithSession(t, "session-A")
	storeB := newTestStoreWithSharedDB(t, storeA, "session-B")
	storeAll := newTestStoreWithSharedDB(t, storeA, "") // no session = legacy mode

	if err := storeA.AddTask(&Task{ID: "T-ALL-A", Title: "A", ExecutionMode: "direct"}); err != nil {
		t.Fatalf("AddTask A: %v", err)
	}
	if err := storeB.AddTask(&Task{ID: "T-ALL-B", Title: "B", ExecutionMode: "direct"}); err != nil {
		t.Fatalf("AddTask B: %v", err)
	}

	tasks, _, err := storeAll.ListTasks(store.TaskFilter{})
	if err != nil {
		t.Fatalf("ListTasks all: %v", err)
	}
	if len(tasks) != 2 {
		t.Errorf("no-session store: expected 2 tasks, got %d (%v)", len(tasks), taskIDs(tasks))
	}
}

// --- helpers ---

func taskIDs(tasks []store.Task) []string {
	ids := make([]string, len(tasks))
	for i, t := range tasks {
		ids[i] = t.ID
	}
	return ids
}

func containsTaskID(tasks []store.Task, id string) bool {
	for _, t := range tasks {
		if t.ID == id {
			return true
		}
	}
	return false
}
