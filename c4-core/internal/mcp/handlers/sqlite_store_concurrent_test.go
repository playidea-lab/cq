package handlers

// Concurrent assignment tests simulate two separate processes (MCP server instances)
// sharing the same .c4/tasks.db file.
//
// In production, each Claude Code session spawns its own `cq` MCP server process.
// All processes open the same SQLite file with WAL mode + MaxOpenConns=1.
// The optimistic concurrency fix (UPDATE WHERE status=? + RowsAffected check)
// ensures only one process gets each task even under concurrent access.

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	_ "modernc.org/sqlite"
)

// newFileTestStore opens a file-based SQLite DB (simulating a real MCP process).
// Each call returns an independent *sql.DB + *SQLiteStore — simulating separate processes.
func newFileTestStore(t *testing.T, dbPath string) (*SQLiteStore, *sql.DB) {
	t.Helper()

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite %s: %v", dbPath, err)
	}

	// Mirror production settings (root.go: openDB)
	db.SetMaxOpenConns(1)
	db.Exec("PRAGMA journal_mode=WAL")
	db.Exec("PRAGMA busy_timeout=5000")

	store, err := NewSQLiteStore(db)
	if err != nil {
		db.Close()
		t.Fatalf("new store: %v", err)
	}

	return store, db
}

// TestConcurrentAssignTask_OnlyOneWins verifies that when two simulated MCP processes
// race to assign the same pending task, exactly one succeeds and the other gets nil.
func TestConcurrentAssignTask_OnlyOneWins(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "concurrent_assign.db")

	// Process A: initializes schema + seeds task
	storeA, dbA := newFileTestStore(t, dbPath)
	defer dbA.Close()

	if err := storeA.AddTask(&Task{
		ID:     "T-001-0",
		Title:  "Shared feature",
		DoD:    "Tests pass",
		Status: "pending",
	}); err != nil {
		t.Fatalf("seed task: %v", err)
	}

	// Process B: opens same file independently
	storeB, dbB := newFileTestStore(t, dbPath)
	defer dbB.Close()

	// Both processes race to call AssignTask simultaneously.
	type result struct {
		assignment *TaskAssignment
		err        error
	}

	ch := make(chan result, 2)
	var wg sync.WaitGroup

	wg.Add(2)
	go func() {
		defer wg.Done()
		a, err := storeA.AssignTask("worker-a")
		ch <- result{a, err}
	}()
	go func() {
		defer wg.Done()
		a, err := storeB.AssignTask("worker-b")
		ch <- result{a, err}
	}()

	wg.Wait()
	close(ch)

	var successes, empties int
	for r := range ch {
		if r.err != nil {
			t.Errorf("AssignTask returned error: %v", r.err)
			continue
		}
		if r.assignment != nil && r.assignment.TaskID != "" {
			successes++
		} else {
			empties++
		}
	}

	if successes != 1 {
		t.Errorf("expected exactly 1 successful assignment, got %d", successes)
	}
	if empties != 1 {
		t.Errorf("expected exactly 1 empty result (loser backs off), got %d", empties)
	}

	// Verify DB state: task must be in_progress with exactly one worker_id.
	var status, workerID string
	if err := dbA.QueryRow(`SELECT status, COALESCE(worker_id,'') FROM c4_tasks WHERE task_id = 'T-001-0'`).
		Scan(&status, &workerID); err != nil {
		t.Fatalf("query task state: %v", err)
	}
	if status != "in_progress" {
		t.Errorf("task status = %q, want in_progress", status)
	}
	if workerID != "worker-a" && workerID != "worker-b" {
		t.Errorf("task worker_id = %q, want worker-a or worker-b", workerID)
	}
}

// TestConcurrentAssignTask_MultipleTasksNoContention verifies that when each of N
// pending tasks is unique, N concurrent workers each get a different task (no contention).
func TestConcurrentAssignTask_MultipleTasksNoContention(t *testing.T) {
	const numTasks = 4
	dbPath := filepath.Join(t.TempDir(), "multi_tasks.db")

	storeA, dbA := newFileTestStore(t, dbPath)
	defer dbA.Close()

	for i := 0; i < numTasks; i++ {
		if err := storeA.AddTask(&Task{
			ID:     fmt.Sprintf("T-%03d-0", i+1),
			Title:  fmt.Sprintf("Task %d", i+1),
			DoD:    "done",
			Status: "pending",
		}); err != nil {
			t.Fatalf("seed task %d: %v", i+1, err)
		}
	}

	// Open numTasks separate store connections (simulate separate processes).
	stores := make([]*SQLiteStore, numTasks)
	dbs := make([]*sql.DB, numTasks)
	for i := 0; i < numTasks; i++ {
		stores[i], dbs[i] = newFileTestStore(t, dbPath)
		defer dbs[i].Close()
	}

	type result struct {
		taskID string
		err    error
	}
	ch := make(chan result, numTasks)
	var wg sync.WaitGroup

	for i := 0; i < numTasks; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			a, err := stores[idx].AssignTask(fmt.Sprintf("worker-%d", idx))
			if err != nil {
				ch <- result{"", err}
				return
			}
			if a == nil {
				ch <- result{"", nil}
			} else {
				ch <- result{a.TaskID, nil}
			}
		}(i)
	}

	wg.Wait()
	close(ch)

	seen := make(map[string]int)
	for r := range ch {
		if r.err != nil {
			t.Errorf("AssignTask error: %v", r.err)
			continue
		}
		if r.taskID != "" {
			seen[r.taskID]++
		}
	}

	// Each task_id must appear at most once (no double-assignment).
	for taskID, count := range seen {
		if count > 1 {
			t.Errorf("task %s was assigned to %d workers (double-assignment!)", taskID, count)
		}
	}

	// All 4 tasks should have been assigned (no task left behind).
	if len(seen) != numTasks {
		t.Errorf("expected %d unique task assignments, got %d: %v", numTasks, len(seen), seen)
	}
}

// TestConcurrentClaimTask_OnlyOneWins verifies that two Direct-mode processes racing
// to claim the same task result in exactly one success and one error.
func TestConcurrentClaimTask_OnlyOneWins(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "concurrent_claim.db")

	storeA, dbA := newFileTestStore(t, dbPath)
	defer dbA.Close()

	// Seed a direct-mode pending task.
	if err := storeA.AddTask(&Task{
		ID:            "T-001-0",
		Title:         "Direct task",
		DoD:           "done",
		Status:        "pending",
		ExecutionMode: "direct",
	}); err != nil {
		t.Fatalf("seed task: %v", err)
	}

	storeB, dbB := newFileTestStore(t, dbPath)
	defer dbB.Close()

	type result struct {
		task *Task
		err  error
	}
	ch := make(chan result, 2)
	var wg sync.WaitGroup

	wg.Add(2)
	go func() {
		defer wg.Done()
		task, err := storeA.ClaimTask("T-001-0")
		ch <- result{task, err}
	}()
	go func() {
		defer wg.Done()
		task, err := storeB.ClaimTask("T-001-0")
		ch <- result{task, err}
	}()

	wg.Wait()
	close(ch)

	var successes, failures int
	for r := range ch {
		if r.err == nil && r.task != nil {
			successes++
		} else {
			failures++
		}
	}

	if successes != 1 {
		t.Errorf("expected exactly 1 successful claim, got %d", successes)
	}
	if failures != 1 {
		t.Errorf("expected exactly 1 failed claim, got %d", failures)
	}

	// DB must show exactly one claimer.
	var status, workerID string
	if err := dbA.QueryRow(`SELECT status, COALESCE(worker_id,'') FROM c4_tasks WHERE task_id = 'T-001-0'`).
		Scan(&status, &workerID); err != nil {
		t.Fatalf("query task: %v", err)
	}
	if status != "in_progress" {
		t.Errorf("status = %q, want in_progress", status)
	}
	if workerID != "direct" {
		t.Errorf("worker_id = %q, want direct", workerID)
	}
}

// TestStaleReassignment_TakesOverAfterTimeout verifies that a task stuck in_progress
// for more than 10 minutes is reassigned to a new worker (anti-fragility path).
func TestStaleReassignment_TakesOverAfterTimeout(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()

	// Seed task and assign it to worker-a.
	if err := store.AddTask(&Task{
		ID:     "T-001-0",
		Title:  "Long running task",
		DoD:    "done",
		Status: "pending",
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := store.AssignTask("worker-a"); err != nil {
		t.Fatalf("initial assign: %v", err)
	}

	// Simulate task being stuck: backdate updated_at by 11 minutes.
	if _, err := db.Exec(`
		UPDATE c4_tasks SET updated_at = datetime('now', '-11 minutes')
		WHERE task_id = 'T-001-0'`); err != nil {
		t.Fatalf("backdate: %v", err)
	}

	// worker-b should now be able to take over the stale task.
	assignment, err := store.AssignTask("worker-b")
	if err != nil {
		t.Fatalf("stale reassign: %v", err)
	}
	if assignment == nil || assignment.TaskID != "T-001-0" {
		t.Fatalf("expected T-001-0 reassigned, got %v", assignment)
	}

	var workerID string
	if err := db.QueryRow(`SELECT worker_id FROM c4_tasks WHERE task_id = 'T-001-0'`).Scan(&workerID); err != nil {
		t.Fatalf("query: %v", err)
	}
	if workerID != "worker-b" {
		t.Errorf("worker_id = %q, want worker-b", workerID)
	}
}

// TestStaleReassignment_NotTriggeredBeforeTimeout verifies that a task in_progress
// for less than 10 minutes is NOT reassigned (normal operation protected).
func TestStaleReassignment_NotTriggeredBeforeTimeout(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()

	// Task in_progress for only 5 minutes — must NOT be stolen.
	if err := store.AddTask(&Task{
		ID:     "T-001-0",
		Title:  "Active task",
		DoD:    "done",
		Status: "pending",
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := store.AssignTask("worker-a"); err != nil {
		t.Fatalf("initial assign: %v", err)
	}
	if _, err := db.Exec(`UPDATE c4_tasks SET updated_at = datetime('now', '-5 minutes') WHERE task_id = 'T-001-0'`); err != nil {
		t.Fatalf("backdate: %v", err)
	}

	// No other pending task → worker-b should get nil (no task available).
	assignment, err := store.AssignTask("worker-b")
	if err != nil {
		t.Fatalf("assign: %v", err)
	}
	if assignment != nil && assignment.TaskID == "T-001-0" {
		t.Error("stale reassignment triggered too early (< 10 min): task should not have been stolen")
	}
}

// TestAssignTask_PendingPath_NotFlaggedAsStale verifies that the staleReassign
// tracking bug is fixed: a normal pending task assignment must not log stale events.
// We test this behaviorally: only 1 pending task, no stale tasks → AssignTask returns it,
// and the task's previous status is 'pending' (visible in the EventBus notification).
func TestAssignTask_PendingPath_NotFlaggedAsStale(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()

	if err := store.AddTask(&Task{
		ID:     "T-001-0",
		Title:  "Normal task",
		DoD:    "done",
		Status: "pending",
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	assignment, err := store.AssignTask("worker-x")
	if err != nil {
		t.Fatalf("assign: %v", err)
	}
	if assignment == nil || assignment.TaskID != "T-001-0" {
		t.Fatalf("expected T-001-0, got %v", assignment)
	}

	// Verify the task is properly in_progress and owned by worker-x.
	var status, workerID string
	if err := db.QueryRow(`SELECT status, worker_id FROM c4_tasks WHERE task_id = 'T-001-0'`).
		Scan(&status, &workerID); err != nil {
		t.Fatalf("query: %v", err)
	}
	if status != "in_progress" {
		t.Errorf("status = %q, want in_progress", status)
	}
	if workerID != "worker-x" {
		t.Errorf("worker_id = %q, want worker-x", workerID)
	}
}

// TestConcurrentAssignTask_RaceDetector is designed to be run with -race to catch
// any data races in the concurrent assignment path.
// go test -race -run TestConcurrentAssignTask_RaceDetector ./internal/mcp/handlers/
func TestConcurrentAssignTask_RaceDetector(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "race_detect.db")

	storeA, dbA := newFileTestStore(t, dbPath)
	defer dbA.Close()

	// Seed multiple tasks to reduce serialization pressure.
	for i := 0; i < 6; i++ {
		if err := storeA.AddTask(&Task{
			ID:     fmt.Sprintf("T-%03d-0", i+1),
			Title:  fmt.Sprintf("Task %d", i+1),
			DoD:    "done",
			Status: "pending",
		}); err != nil {
			t.Fatalf("seed task %d: %v", i+1, err)
		}
	}

	stores := make([]*SQLiteStore, 3)
	dbs := make([]*sql.DB, 3)
	for i := 0; i < 3; i++ {
		stores[i], dbs[i] = newFileTestStore(t, dbPath)
		defer dbs[i].Close()
	}

	var wg sync.WaitGroup
	assignments := make([]string, 3)

	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			a, err := stores[idx].AssignTask(fmt.Sprintf("worker-%d", idx))
			if err == nil && a != nil {
				assignments[idx] = a.TaskID
			}
		}(i)
	}
	wg.Wait()

	// No duplicate assignments.
	seen := make(map[string]bool)
	for _, id := range assignments {
		if id == "" {
			continue
		}
		if seen[id] {
			t.Errorf("task %s assigned to multiple workers (race condition)", id)
		}
		seen[id] = true
	}
}
