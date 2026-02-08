package task

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// setupSQLiteStore creates a temporary SQLite database and returns a SQLiteTaskStore.
func setupSQLiteStore(t *testing.T) *SQLiteTaskStore {
	t.Helper()

	tmpDir := t.TempDir()
	dbFile := filepath.Join(tmpDir, "test.db")
	db, err := sql.Open("sqlite", dbFile)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	store, err := NewSQLiteTaskStore(db, "test-project")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	return store
}

// TestSQLiteStoreCreateAndGet verifies basic task creation and retrieval.
func TestSQLiteStoreCreateAndGet(t *testing.T) {
	store := setupSQLiteStore(t)

	task := &Task{
		ID:       "T-001-0",
		Title:    "Build feature",
		Scope:    "src/",
		Priority: 5,
		DoD:      "Feature works",
		Status:   StatusPending,
		Type:     TypeImplementation,
		BaseID:   "001",
		Model:    "opus",
	}

	if err := store.CreateTask(task); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := store.GetTask("T-001-0")
	if err != nil {
		t.Fatalf("get: %v", err)
	}

	if got.ID != "T-001-0" {
		t.Errorf("ID = %q, want T-001-0", got.ID)
	}
	if got.Title != "Build feature" {
		t.Errorf("Title = %q, want Build feature", got.Title)
	}
	if got.Status != StatusPending {
		t.Errorf("Status = %q, want pending", got.Status)
	}
	if got.Priority != 5 {
		t.Errorf("Priority = %d, want 5", got.Priority)
	}
}

// TestSQLiteStoreGetNotFound verifies error for missing task.
func TestSQLiteStoreGetNotFound(t *testing.T) {
	store := setupSQLiteStore(t)

	_, err := store.GetTask("nonexistent")
	if err != ErrTaskNotFound {
		t.Errorf("expected ErrTaskNotFound, got %v", err)
	}
}

// TestSQLiteStoreUpdateStatus verifies task status updates.
func TestSQLiteStoreUpdateStatus(t *testing.T) {
	store := setupSQLiteStore(t)

	task := &Task{
		ID:     "T-001-0",
		Title:  "Task",
		DoD:    "Done",
		Status: StatusPending,
		Type:   TypeImplementation,
		Model:  "opus",
	}
	store.CreateTask(task)

	// Update status
	task.Status = StatusInProgress
	task.AssignedTo = "worker-abc"
	if err := store.UpdateTask(task); err != nil {
		t.Fatalf("update: %v", err)
	}

	got, _ := store.GetTask("T-001-0")
	if got.Status != StatusInProgress {
		t.Errorf("Status = %q, want in_progress", got.Status)
	}
	if got.AssignedTo != "worker-abc" {
		t.Errorf("AssignedTo = %q, want worker-abc", got.AssignedTo)
	}
}

// TestSQLiteStoreUpdateNotFound verifies error for updating missing task.
func TestSQLiteStoreUpdateNotFound(t *testing.T) {
	store := setupSQLiteStore(t)

	task := &Task{ID: "nonexistent", Title: "X", DoD: "Y", Status: StatusPending}
	err := store.UpdateTask(task)
	if err != ErrTaskNotFound {
		t.Errorf("expected ErrTaskNotFound, got %v", err)
	}
}

// TestSQLiteStoreListTasks verifies listing all tasks.
func TestSQLiteStoreListTasks(t *testing.T) {
	store := setupSQLiteStore(t)

	for i, title := range []string{"Task A", "Task B", "Task C"} {
		store.CreateTask(&Task{
			ID:     fmt.Sprintf("T-%03d-0", i+1),
			Title:  title,
			DoD:    "Done",
			Status: StatusPending,
			Type:   TypeImplementation,
			Model:  "opus",
		})
	}

	tasks, err := store.ListTasks("")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(tasks) != 3 {
		t.Errorf("expected 3 tasks, got %d", len(tasks))
	}
}

// TestSQLiteStoreDeleteTask verifies task deletion.
func TestSQLiteStoreDeleteTask(t *testing.T) {
	store := setupSQLiteStore(t)

	store.CreateTask(&Task{
		ID: "T-001-0", Title: "Delete me", DoD: "Gone", Status: StatusPending, Type: TypeImplementation, Model: "opus",
	})

	if err := store.DeleteTask("T-001-0"); err != nil {
		t.Fatalf("delete: %v", err)
	}

	_, err := store.GetTask("T-001-0")
	if err != ErrTaskNotFound {
		t.Errorf("expected ErrTaskNotFound after delete, got %v", err)
	}
}

// TestSQLiteStoreDeleteNotFound verifies error for deleting missing task.
func TestSQLiteStoreDeleteNotFound(t *testing.T) {
	store := setupSQLiteStore(t)

	err := store.DeleteTask("nonexistent")
	if err != ErrTaskNotFound {
		t.Errorf("expected ErrTaskNotFound, got %v", err)
	}
}

// TestSQLiteStoreGetNextTask verifies task assignment with dependency resolution.
func TestSQLiteStoreGetNextTask(t *testing.T) {
	store := setupSQLiteStore(t)

	// Task 1: no deps, priority 5
	store.CreateTask(&Task{
		ID: "T-001-0", Title: "Low", DoD: "Done", Priority: 5,
		Status: StatusPending, Type: TypeImplementation, Model: "opus",
	})
	// Task 2: no deps, priority 8 (higher)
	store.CreateTask(&Task{
		ID: "T-002-0", Title: "High", DoD: "Done", Priority: 8,
		Status: StatusPending, Type: TypeImplementation, Model: "opus",
	})

	task, err := store.GetNextTask("worker-1")
	if err != nil {
		t.Fatalf("get next: %v", err)
	}
	if task.ID != "T-002-0" {
		t.Errorf("expected highest priority T-002-0, got %s", task.ID)
	}
	if task.Status != StatusInProgress {
		t.Errorf("expected in_progress, got %s", task.Status)
	}
	if task.AssignedTo != "worker-1" {
		t.Errorf("expected worker-1, got %s", task.AssignedTo)
	}
}

// TestSQLiteStoreGetNextTaskWithDeps verifies dependency checking.
func TestSQLiteStoreGetNextTaskWithDeps(t *testing.T) {
	store := setupSQLiteStore(t)

	// Task 1: done
	store.CreateTask(&Task{
		ID: "T-001-0", Title: "Done task", DoD: "Done", Priority: 5,
		Status: StatusDone, Type: TypeImplementation, Model: "opus",
	})
	// Task 2: depends on T-001-0 (should be available)
	store.CreateTask(&Task{
		ID: "T-002-0", Title: "Dep met", DoD: "Done", Priority: 8,
		Dependencies: []string{"T-001-0"},
		Status:       StatusPending, Type: TypeImplementation, Model: "opus",
	})
	// Task 3: depends on T-999-0 (not done, should be blocked)
	store.CreateTask(&Task{
		ID: "T-003-0", Title: "Dep not met", DoD: "Done", Priority: 10,
		Dependencies: []string{"T-999-0"},
		Status:       StatusPending, Type: TypeImplementation, Model: "opus",
	})

	task, err := store.GetNextTask("worker-1")
	if err != nil {
		t.Fatalf("get next: %v", err)
	}
	// T-003-0 has higher priority but deps not met, so T-002-0 should be selected
	if task.ID != "T-002-0" {
		t.Errorf("expected T-002-0 (deps met), got %s", task.ID)
	}
}

// TestSQLiteStoreGetNextTaskScopeLock verifies scope locking.
func TestSQLiteStoreGetNextTaskScopeLock(t *testing.T) {
	store := setupSQLiteStore(t)

	// Task 1: in_progress, locks "src/" scope for worker-1
	store.CreateTask(&Task{
		ID: "T-001-0", Title: "Running", DoD: "Done", Scope: "src/",
		Status: StatusInProgress, AssignedTo: "worker-1", Type: TypeImplementation, Model: "opus",
	})
	// Task 2: pending, same scope, should be blocked for worker-2
	store.CreateTask(&Task{
		ID: "T-002-0", Title: "Same scope", DoD: "Done", Priority: 10, Scope: "src/",
		Status: StatusPending, Type: TypeImplementation, Model: "opus",
	})
	// Task 3: pending, different scope
	store.CreateTask(&Task{
		ID: "T-003-0", Title: "Other scope", DoD: "Done", Priority: 5, Scope: "tests/",
		Status: StatusPending, Type: TypeImplementation, Model: "opus",
	})

	// Worker-2 should get T-003-0 (T-002-0 is scope-locked)
	task, err := store.GetNextTask("worker-2")
	if err != nil {
		t.Fatalf("get next: %v", err)
	}
	if task.ID != "T-003-0" {
		t.Errorf("expected T-003-0 (no scope conflict), got %s", task.ID)
	}
}

// TestSQLiteStoreGetNextTaskNoAvailable verifies error when no tasks available.
func TestSQLiteStoreGetNextTaskNoAvailable(t *testing.T) {
	store := setupSQLiteStore(t)

	_, err := store.GetNextTask("worker-1")
	if err != ErrNoAvailableTask {
		t.Errorf("expected ErrNoAvailableTask, got %v", err)
	}
}

// TestSQLiteStoreCompleteTask verifies task completion with review generation.
func TestSQLiteStoreCompleteTask(t *testing.T) {
	store := setupSQLiteStore(t)

	store.CreateTask(&Task{
		ID: "T-001-0", Title: "Build feature", DoD: "Done",
		Status: StatusInProgress, AssignedTo: "worker-1",
		Type: TypeImplementation, BaseID: "001", Version: 0, Model: "opus",
	})

	reviewTask, err := store.CompleteTask("T-001-0", "worker-1", "abc123")
	if err != nil {
		t.Fatalf("complete: %v", err)
	}

	// Verify task is done
	task, _ := store.GetTask("T-001-0")
	if task.Status != StatusDone {
		t.Errorf("expected done, got %s", task.Status)
	}
	if task.CommitSHA != "abc123" {
		t.Errorf("expected commit abc123, got %s", task.CommitSHA)
	}

	// Verify review task was created
	if reviewTask == nil {
		t.Fatal("expected review task, got nil")
	}
	if reviewTask.ID != "R-001-0" {
		t.Errorf("review ID = %q, want R-001-0", reviewTask.ID)
	}
	if reviewTask.Type != TypeReview {
		t.Errorf("review type = %q, want REVIEW", reviewTask.Type)
	}

	// Verify review task is in the database
	got, err := store.GetTask("R-001-0")
	if err != nil {
		t.Fatalf("get review: %v", err)
	}
	if got.Status != StatusPending {
		t.Errorf("review status = %q, want pending", got.Status)
	}
}

// TestSQLiteStoreCompleteTaskNotInProgress verifies error for wrong status.
func TestSQLiteStoreCompleteTaskNotInProgress(t *testing.T) {
	store := setupSQLiteStore(t)

	store.CreateTask(&Task{
		ID: "T-001-0", Title: "Pending", DoD: "Done",
		Status: StatusPending, Type: TypeImplementation, Model: "opus",
	})

	_, err := store.CompleteTask("T-001-0", "worker-1", "abc")
	if err != ErrNotInProgress {
		t.Errorf("expected ErrNotInProgress, got %v", err)
	}
}

// TestSQLiteStoreCompleteTaskWorkerMismatch verifies worker ownership check.
func TestSQLiteStoreCompleteTaskWorkerMismatch(t *testing.T) {
	store := setupSQLiteStore(t)

	store.CreateTask(&Task{
		ID: "T-001-0", Title: "Task", DoD: "Done",
		Status: StatusInProgress, AssignedTo: "worker-1",
		Type: TypeImplementation, Model: "opus",
	})

	_, err := store.CompleteTask("T-001-0", "worker-2", "abc")
	if err != ErrWorkerMismatch {
		t.Errorf("expected ErrWorkerMismatch, got %v", err)
	}
}

// TestSQLiteStoreCompleteReviewTaskNoAutoReview verifies no review for review tasks.
func TestSQLiteStoreCompleteReviewTaskNoAutoReview(t *testing.T) {
	store := setupSQLiteStore(t)

	store.CreateTask(&Task{
		ID: "R-001-0", Title: "Review", DoD: "Done",
		Status: StatusInProgress, AssignedTo: "worker-1",
		Type: TypeReview, Model: "opus",
	})

	reviewTask, err := store.CompleteTask("R-001-0", "worker-1", "abc")
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if reviewTask != nil {
		t.Errorf("expected nil review for review task, got %v", reviewTask)
	}
}

// TestSQLiteStorePythonCompatibility verifies Go can read Python-written task_json.
func TestSQLiteStorePythonCompatibility(t *testing.T) {
	tmpDir := t.TempDir()
	dbFile := filepath.Join(tmpDir, "c4.db")
	db, err := sql.Open("sqlite", dbFile)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	store, err := NewSQLiteTaskStore(db, "my-project")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	// Insert a task in the Python format (task_json contains full Pydantic-like JSON)
	pythonTaskJSON := `{
		"id": "T-001-0",
		"title": "Build API",
		"scope": "src/api/",
		"priority": 7,
		"dod": "API endpoint works",
		"validations": ["lint", "unit"],
		"dependencies": ["T-000-0"],
		"status": "pending",
		"assigned_to": null,
		"type": "IMPLEMENTATION",
		"base_id": "001",
		"version": 0,
		"model": "opus"
	}`

	_, err = db.Exec(
		`INSERT INTO c4_tasks (project_id, task_id, task_json, status, assigned_to)
		 VALUES (?, ?, ?, ?, ?)`,
		"my-project", "T-001-0", pythonTaskJSON, "pending", nil,
	)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	// Read it back through Go store
	task, err := store.GetTask("T-001-0")
	if err != nil {
		t.Fatalf("get: %v", err)
	}

	if task.ID != "T-001-0" {
		t.Errorf("ID = %q, want T-001-0", task.ID)
	}
	if task.Title != "Build API" {
		t.Errorf("Title = %q, want Build API", task.Title)
	}
	if task.Priority != 7 {
		t.Errorf("Priority = %d, want 7", task.Priority)
	}
	if task.Scope != "src/api/" {
		t.Errorf("Scope = %q, want src/api/", task.Scope)
	}
	if task.Type != TypeImplementation {
		t.Errorf("Type = %q, want IMPLEMENTATION", task.Type)
	}
	if len(task.Dependencies) != 1 || task.Dependencies[0] != "T-000-0" {
		t.Errorf("Dependencies = %v, want [T-000-0]", task.Dependencies)
	}
}

// TestSQLiteStoreGoWrittenReadByQuery verifies that Go-written tasks
// have correct denormalized status/assigned_to columns for Python queries.
func TestSQLiteStoreGoWrittenReadByQuery(t *testing.T) {
	tmpDir := t.TempDir()
	dbFile := filepath.Join(tmpDir, "c4.db")
	db, err := sql.Open("sqlite", dbFile)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	store, err := NewSQLiteTaskStore(db, "proj")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	store.CreateTask(&Task{
		ID: "T-001-0", Title: "Test", DoD: "Done", Priority: 5,
		Status: StatusPending, Type: TypeImplementation, Model: "opus",
	})

	// Query the denormalized columns directly (as Python would)
	var status, taskJSON string
	var assignedTo sql.NullString
	err = db.QueryRow(
		"SELECT status, assigned_to, task_json FROM c4_tasks WHERE task_id = ?",
		"T-001-0",
	).Scan(&status, &assignedTo, &taskJSON)
	if err != nil {
		t.Fatalf("query: %v", err)
	}

	if status != "pending" {
		t.Errorf("denormalized status = %q, want pending", status)
	}

	// Verify task_json is valid JSON
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(taskJSON), &parsed); err != nil {
		t.Fatalf("task_json is not valid JSON: %v", err)
	}
	if parsed["title"] != "Test" {
		t.Errorf("task_json.title = %v, want Test", parsed["title"])
	}
}

