package handlers

import (
	"testing"
)

func TestGetStatus_Empty(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()

	status, err := store.GetStatus()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.State != "INIT" {
		t.Errorf("State = %q, want %q", status.State, "INIT")
	}
	if status.TotalTasks != 0 {
		t.Errorf("TotalTasks = %d, want 0", status.TotalTasks)
	}
}

func TestGetStatus_WithTasks(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()

	// Insert tasks in various states
	db.Exec(`INSERT INTO c4_tasks (task_id, title, status, dod) VALUES ('T-001-0', 'A', 'pending', 'test')`)
	db.Exec(`INSERT INTO c4_tasks (task_id, title, status, dod) VALUES ('T-002-0', 'B', 'in_progress', 'test')`)
	db.Exec(`INSERT INTO c4_tasks (task_id, title, status, dod) VALUES ('T-003-0', 'C', 'done', 'test')`)
	db.Exec(`INSERT INTO c4_tasks (task_id, title, status, dod) VALUES ('T-004-0', 'D', 'blocked', 'test')`)

	status, err := store.GetStatus()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.TotalTasks != 4 {
		t.Errorf("TotalTasks = %d, want 4", status.TotalTasks)
	}
	if status.PendingTasks != 1 {
		t.Errorf("PendingTasks = %d, want 1", status.PendingTasks)
	}
	if status.InProgress != 1 {
		t.Errorf("InProgress = %d, want 1", status.InProgress)
	}
	if status.DoneTasks != 1 {
		t.Errorf("DoneTasks = %d, want 1", status.DoneTasks)
	}
	if status.BlockedTasks != 1 {
		t.Errorf("BlockedTasks = %d, want 1", status.BlockedTasks)
	}
}

func TestGetStatus_ReadyTasks(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()

	// T-001: pending, no deps -> ready
	db.Exec(`INSERT INTO c4_tasks (task_id, title, status, dod, dependencies) VALUES ('T-001-0', 'Ready', 'pending', 'test', '[]')`)
	// T-002: pending, depends on T-001 (not done) -> not ready
	db.Exec(`INSERT INTO c4_tasks (task_id, title, status, dod, dependencies) VALUES ('T-002-0', 'Blocked', 'pending', 'test', '["T-001-0"]')`)

	status, err := store.GetStatus()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.ReadyTasks != 1 {
		t.Errorf("ReadyTasks = %d, want 1", status.ReadyTasks)
	}
	if len(status.ReadyTaskIDs) != 1 || status.ReadyTaskIDs[0] != "T-001-0" {
		t.Errorf("ReadyTaskIDs = %v, want [T-001-0]", status.ReadyTaskIDs)
	}
}

func TestListTasks_Basic(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()

	db.Exec(`INSERT INTO c4_tasks (task_id, title, status, dod, priority, domain) VALUES ('T-001-0', 'High', 'pending', 'test', 10, 'backend')`)
	db.Exec(`INSERT INTO c4_tasks (task_id, title, status, dod, priority, domain) VALUES ('T-002-0', 'Low', 'pending', 'test', 1, 'frontend')`)
	db.Exec(`INSERT INTO c4_tasks (task_id, title, status, dod, priority) VALUES ('T-003-0', 'Done', 'done', 'test', 5)`)

	tasks, total, err := store.ListTasks(TaskFilter{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 3 {
		t.Errorf("total = %d, want 3", total)
	}
	if len(tasks) != 3 {
		t.Fatalf("len(tasks) = %d, want 3", len(tasks))
	}
	// First should be highest priority
	if tasks[0].ID != "T-001-0" {
		t.Errorf("first task = %q, want T-001-0 (highest priority)", tasks[0].ID)
	}
}

func TestListTasks_FilterByStatus(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()

	db.Exec(`INSERT INTO c4_tasks (task_id, title, status, dod) VALUES ('T-001-0', 'A', 'pending', 'test')`)
	db.Exec(`INSERT INTO c4_tasks (task_id, title, status, dod) VALUES ('T-002-0', 'B', 'done', 'test')`)

	tasks, _, err := store.ListTasks(TaskFilter{Status: "pending"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("len(tasks) = %d, want 1", len(tasks))
	}
	if tasks[0].ID != "T-001-0" {
		t.Errorf("task ID = %q, want T-001-0", tasks[0].ID)
	}
}

func TestListTasks_FilterByDomain(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()

	db.Exec(`INSERT INTO c4_tasks (task_id, title, status, dod, domain) VALUES ('T-001-0', 'A', 'pending', 'test', 'backend')`)
	db.Exec(`INSERT INTO c4_tasks (task_id, title, status, dod, domain) VALUES ('T-002-0', 'B', 'pending', 'test', 'frontend')`)

	tasks, _, err := store.ListTasks(TaskFilter{Domain: "backend"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tasks) != 1 || tasks[0].ID != "T-001-0" {
		t.Errorf("got %v, want [T-001-0]", tasks)
	}
}

func TestGetPersonaStats_Empty(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()

	stats, err := store.GetPersonaStats("worker-nonexist")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stats["total_tasks"] != 0 {
		t.Errorf("total_tasks = %v, want 0", stats["total_tasks"])
	}
}

func TestListPersonas_Empty(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()

	personas, err := store.ListPersonas()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(personas) != 0 {
		t.Errorf("len(personas) = %d, want 0", len(personas))
	}
}
