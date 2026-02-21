package handlers

import (
	"database/sql"
	"encoding/json"
	"testing"

	_ "modernc.org/sqlite"
)

func setupTaskListTest(t *testing.T) *SQLiteStore {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	store, err := NewSQLiteStore(db)
	if err != nil {
		t.Fatal(err)
	}

	// Insert test tasks
	for _, q := range []string{
		`INSERT INTO c4_tasks (task_id, title, status, priority, domain, worker_id, created_at) VALUES ('T-001-0', 'Backend API', 'done', 2, 'backend', 'w1', '2026-02-15T01:00:00Z')`,
		`INSERT INTO c4_tasks (task_id, title, status, priority, domain, worker_id, created_at) VALUES ('T-002-0', 'Frontend UI', 'pending', 1, 'frontend', '', '2026-02-15T02:00:00Z')`,
		`INSERT INTO c4_tasks (task_id, title, status, priority, domain, worker_id, created_at) VALUES ('T-003-0', 'DB Migration', 'in_progress', 3, 'backend', 'w2', '2026-02-15T03:00:00Z')`,
		`INSERT INTO c4_tasks (task_id, title, status, priority, domain, worker_id, created_at) VALUES ('T-004-0', 'Tests', 'pending', 1, 'backend', '', '2026-02-15T04:00:00Z')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}
	return store
}

func TestListTasksNoFilter(t *testing.T) {
	store := setupTaskListTest(t)

	tasks, total, err := store.ListTasks(TaskFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if total != 4 {
		t.Errorf("total = %d, want 4", total)
	}
	if len(tasks) != 4 {
		t.Errorf("filtered = %d, want 4", len(tasks))
	}
	// First task should be highest priority (3 = DB Migration)
	if tasks[0].ID != "T-003-0" {
		t.Errorf("first task = %s, want T-003-0 (highest priority)", tasks[0].ID)
	}
}

func TestListTasksFilterByStatus(t *testing.T) {
	store := setupTaskListTest(t)

	tasks, total, err := store.ListTasks(TaskFilter{Status: "pending"})
	if err != nil {
		t.Fatal(err)
	}
	if total != 4 {
		t.Errorf("total = %d, want 4 (unfiltered count)", total)
	}
	if len(tasks) != 2 {
		t.Errorf("filtered = %d, want 2 pending tasks", len(tasks))
	}
	for _, task := range tasks {
		if task.Status != "pending" {
			t.Errorf("task %s status = %q, want pending", task.ID, task.Status)
		}
	}
}

func TestListTasksFilterByDomain(t *testing.T) {
	store := setupTaskListTest(t)

	tasks, _, err := store.ListTasks(TaskFilter{Domain: "backend"})
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 3 {
		t.Errorf("filtered = %d, want 3 backend tasks", len(tasks))
	}
}

func TestListTasksFilterByWorker(t *testing.T) {
	store := setupTaskListTest(t)

	tasks, _, err := store.ListTasks(TaskFilter{WorkerID: "w2"})
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 {
		t.Errorf("filtered = %d, want 1 task for w2", len(tasks))
	}
	if len(tasks) > 0 && tasks[0].ID != "T-003-0" {
		t.Errorf("task = %s, want T-003-0", tasks[0].ID)
	}
}

func TestListTasksCombinedFilter(t *testing.T) {
	store := setupTaskListTest(t)

	tasks, _, err := store.ListTasks(TaskFilter{Status: "pending", Domain: "backend"})
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 {
		t.Errorf("filtered = %d, want 1 (pending+backend)", len(tasks))
	}
	if len(tasks) > 0 && tasks[0].ID != "T-004-0" {
		t.Errorf("task = %s, want T-004-0", tasks[0].ID)
	}
}

func TestListTasksLimit(t *testing.T) {
	store := setupTaskListTest(t)

	tasks, total, err := store.ListTasks(TaskFilter{Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if total != 4 {
		t.Errorf("total = %d, want 4", total)
	}
	if len(tasks) != 2 {
		t.Errorf("filtered = %d, want 2 (limited)", len(tasks))
	}
}

func TestListTasksEmpty(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	store, err := NewSQLiteStore(db)
	if err != nil {
		t.Fatal(err)
	}

	tasks, total, err := store.ListTasks(TaskFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if total != 0 {
		t.Errorf("total = %d, want 0", total)
	}
	if len(tasks) != 0 {
		t.Errorf("filtered = %d, want 0", len(tasks))
	}
}

func TestHandleTaskListIncludeDod(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	s, err := NewSQLiteStore(db)
	if err != nil {
		t.Fatal(err)
	}

	dod := "Goal: Implement feature\n\nTests: unit tests pass"
	_, err = db.Exec(
		`INSERT INTO c4_tasks (task_id, title, status, priority, dod, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		"T-001-0", "Backend API", "pending", 2, dod, "2026-02-15T01:00:00Z",
	)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("include_dod=false omits dod field", func(t *testing.T) {
		result, err := handleTaskList(s, json.RawMessage(`{"include_dod":false}`))
		if err != nil {
			t.Fatal(err)
		}
		resp := result.(map[string]any)
		tasks, ok := resp["tasks"].([]map[string]any)
		if !ok {
			t.Fatalf("tasks type = %T, want []map[string]any", resp["tasks"])
		}
		if len(tasks) != 1 {
			t.Fatalf("len(tasks) = %d, want 1", len(tasks))
		}
		if _, hasDod := tasks[0]["dod"]; hasDod {
			t.Error("dod field present, want omitted when include_dod=false")
		}
		if tasks[0]["task_id"] != "T-001-0" {
			t.Errorf("task_id = %v, want T-001-0", tasks[0]["task_id"])
		}
	})

	t.Run("include_dod=true preserves dod field", func(t *testing.T) {
		result, err := handleTaskList(s, json.RawMessage(`{"include_dod":true}`))
		if err != nil {
			t.Fatal(err)
		}
		resp := result.(map[string]any)
		tasks, ok := resp["tasks"].([]Task)
		if !ok {
			t.Fatalf("tasks type = %T, want []Task", resp["tasks"])
		}
		if len(tasks) != 1 {
			t.Fatalf("len(tasks) = %d, want 1", len(tasks))
		}
		if tasks[0].DoD != dod {
			t.Errorf("dod = %q, want %q", tasks[0].DoD, dod)
		}
	})

	t.Run("default omits dod field", func(t *testing.T) {
		result, err := handleTaskList(s, json.RawMessage(`{}`))
		if err != nil {
			t.Fatal(err)
		}
		resp := result.(map[string]any)
		tasks, ok := resp["tasks"].([]map[string]any)
		if !ok {
			t.Fatalf("tasks type = %T, want []map[string]any", resp["tasks"])
		}
		if len(tasks) != 1 {
			t.Fatalf("len(tasks) = %d, want 1", len(tasks))
		}
		if _, hasDod := tasks[0]["dod"]; hasDod {
			t.Error("dod field present, want omitted by default")
		}
	})
}
