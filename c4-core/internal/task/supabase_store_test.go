package task

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

// =========================================================================
// Mock PostgREST server for SupabaseTaskStore tests
// =========================================================================

// mockPostgREST creates an httptest.Server that simulates Supabase PostgREST
// for the c4_tasks table. Returns the server and a SupabaseTaskStore.
func mockPostgREST(t *testing.T) (*httptest.Server, *SupabaseTaskStore) {
	t.Helper()

	store := make(map[string]*supabaseTaskRow) // id -> row

	mux := http.NewServeMux()

	// POST /rest/v1/c4_tasks
	mux.HandleFunc("POST /rest/v1/c4_tasks", func(w http.ResponseWriter, r *http.Request) {
		var row supabaseTaskRow
		if err := json.NewDecoder(r.Body).Decode(&row); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		store[row.ID] = &row
		w.WriteHeader(http.StatusCreated)
	})

	// GET /rest/v1/c4_tasks
	mux.HandleFunc("GET /rest/v1/c4_tasks", func(w http.ResponseWriter, r *http.Request) {
		pid := extractEqParam(r, "project_id")
		taskID := extractEqParam(r, "id")

		var result []supabaseTaskRow
		for _, row := range store {
			if pid != "" && row.ProjectID != pid {
				continue
			}
			if taskID != "" && row.ID != taskID {
				continue
			}
			// row_version filter for optimistic locking
			rvStr := extractEqParam(r, "row_version")
			if rvStr != "" {
				// Parse and compare
				var rv int
				for _, c := range rvStr {
					rv = rv*10 + int(c-'0')
				}
				if row.RowVersion != rv {
					continue
				}
			}
			result = append(result, *row)
		}
		json.NewEncoder(w).Encode(result)
	})

	// PATCH /rest/v1/c4_tasks
	mux.HandleFunc("PATCH /rest/v1/c4_tasks", func(w http.ResponseWriter, r *http.Request) {
		taskID := extractEqParam(r, "id")
		rvStr := extractEqParam(r, "row_version")

		if taskID == "" {
			http.Error(w, "missing id", http.StatusBadRequest)
			return
		}

		existing, ok := store[taskID]
		if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		// Optimistic lock check
		if rvStr != "" {
			var rv int
			for _, c := range rvStr {
				rv = rv*10 + int(c-'0')
			}
			if existing.RowVersion != rv {
				http.Error(w, "version conflict", http.StatusConflict)
				return
			}
		}

		var update supabaseTaskRow
		if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// Apply partial update
		if update.Status != "" {
			existing.Status = update.Status
		}
		if update.AssignedTo != "" {
			existing.AssignedTo = update.AssignedTo
		}
		if update.CommitSHA != "" {
			existing.CommitSHA = update.CommitSHA
		}
		if update.CompletedAt != "" {
			existing.CompletedAt = update.CompletedAt
		}
		if update.RowVersion > 0 {
			existing.RowVersion = update.RowVersion
		}

		w.WriteHeader(http.StatusNoContent)
	})

	// DELETE /rest/v1/c4_tasks
	mux.HandleFunc("DELETE /rest/v1/c4_tasks", func(w http.ResponseWriter, r *http.Request) {
		taskID := extractEqParam(r, "id")
		if taskID == "" {
			http.Error(w, "missing id", http.StatusBadRequest)
			return
		}
		delete(store, taskID)
		w.WriteHeader(http.StatusNoContent)
	})

	server := httptest.NewServer(mux)
	ts := NewSupabaseTaskStore(&SupabaseConfig{
		URL:       server.URL,
		AnonKey:   "test-key",
		ProjectID: "test-project",
	})

	return server, ts
}

// extractEqParam extracts a "param=eq.value" from query string.
func extractEqParam(r *http.Request, key string) string {
	v := r.URL.Query().Get(key)
	if v == "" {
		return ""
	}
	if strings.HasPrefix(v, "eq.") {
		return v[3:]
	}
	return v
}

// =========================================================================
// Tests: CRUD
// =========================================================================

func TestSupabaseCreateAndGetTask(t *testing.T) {
	server, store := mockPostgREST(t)
	defer server.Close()

	task := &Task{
		ID:       "T-001-0",
		Title:    "Build feature",
		Priority: 5,
		DoD:      "Implement the feature",
		Status:   StatusPending,
		Type:     TypeImplementation,
		BaseID:   "001",
		Model:    "sonnet",
	}

	if err := store.CreateTask(task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	got, err := store.GetTask("T-001-0")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.ID != "T-001-0" {
		t.Errorf("ID = %q, want T-001-0", got.ID)
	}
	if got.Title != "Build feature" {
		t.Errorf("Title = %q, want %q", got.Title, "Build feature")
	}
	if got.Priority != 5 {
		t.Errorf("Priority = %d, want 5", got.Priority)
	}
}

func TestSupabaseGetTaskNotFound(t *testing.T) {
	server, store := mockPostgREST(t)
	defer server.Close()

	_, err := store.GetTask("T-NONEXISTENT-0")
	if err == nil {
		t.Error("expected error for nonexistent task")
	}
	if err != ErrTaskNotFound {
		t.Errorf("err = %v, want ErrTaskNotFound", err)
	}
}

func TestSupabaseListTasks(t *testing.T) {
	server, store := mockPostgREST(t)
	defer server.Close()

	for i := 1; i <= 3; i++ {
		store.CreateTask(&Task{
			ID:       "T-00" + string(rune('0'+i)) + "-0",
			Title:    "Task " + string(rune('0'+i)),
			Status:   StatusPending,
			Type:     TypeImplementation,
			Model:    "sonnet",
			Priority: i,
		})
	}

	tasks, err := store.ListTasks("test-project")
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	if len(tasks) != 3 {
		t.Errorf("got %d tasks, want 3", len(tasks))
	}
}

func TestSupabaseDeleteTask(t *testing.T) {
	server, store := mockPostgREST(t)
	defer server.Close()

	store.CreateTask(&Task{
		ID:     "T-001-0",
		Title:  "To delete",
		Status: StatusPending,
		Type:   TypeImplementation,
		Model:  "sonnet",
	})

	if err := store.DeleteTask("T-001-0"); err != nil {
		t.Fatalf("DeleteTask: %v", err)
	}

	_, err := store.GetTask("T-001-0")
	if err != ErrTaskNotFound {
		t.Errorf("after delete: err = %v, want ErrTaskNotFound", err)
	}
}

// =========================================================================
// Tests: GetNextTask (dependency resolution + scope locking)
// =========================================================================

func TestSupabaseGetNextTaskPriority(t *testing.T) {
	server, store := mockPostgREST(t)
	defer server.Close()

	// Low priority
	store.CreateTask(&Task{
		ID:       "T-001-0",
		Title:    "Low priority",
		Priority: 1,
		Status:   StatusPending,
		Type:     TypeImplementation,
		Model:    "sonnet",
	})
	// High priority
	store.CreateTask(&Task{
		ID:       "T-002-0",
		Title:    "High priority",
		Priority: 10,
		Status:   StatusPending,
		Type:     TypeImplementation,
		Model:    "sonnet",
	})

	task, err := store.GetNextTask("worker-aabbccdd")
	if err != nil {
		t.Fatalf("GetNextTask: %v", err)
	}
	if task.ID != "T-002-0" {
		t.Errorf("got %q, want T-002-0 (highest priority)", task.ID)
	}
	if task.Status != StatusInProgress {
		t.Errorf("status = %q, want in_progress", task.Status)
	}
}

func TestSupabaseGetNextTaskDependencies(t *testing.T) {
	server, store := mockPostgREST(t)
	defer server.Close()

	// T-001-0: done (dependency)
	store.CreateTask(&Task{
		ID:     "T-001-0",
		Title:  "Prereq",
		Status: StatusDone,
		Type:   TypeImplementation,
		Model:  "sonnet",
	})

	// T-002-0: depends on T-001-0 (should be assignable)
	store.CreateTask(&Task{
		ID:           "T-002-0",
		Title:        "Has met dep",
		Status:       StatusPending,
		Dependencies: []string{"T-001-0"},
		Priority:     5,
		Type:         TypeImplementation,
		Model:        "sonnet",
	})

	// T-003-0: depends on T-999-0 (not done, should be skipped)
	store.CreateTask(&Task{
		ID:           "T-003-0",
		Title:        "Unmet dep",
		Status:       StatusPending,
		Dependencies: []string{"T-999-0"},
		Priority:     10,
		Type:         TypeImplementation,
		Model:        "sonnet",
	})

	task, err := store.GetNextTask("worker-aabbccdd")
	if err != nil {
		t.Fatalf("GetNextTask: %v", err)
	}
	// Should pick T-002-0 despite T-003-0 having higher priority (unmet dep)
	if task.ID != "T-002-0" {
		t.Errorf("got %q, want T-002-0 (T-003-0 has unmet dep)", task.ID)
	}
}

func TestSupabaseGetNextTaskNoAvailable(t *testing.T) {
	server, store := mockPostgREST(t)
	defer server.Close()

	// Only done tasks
	store.CreateTask(&Task{
		ID:     "T-001-0",
		Title:  "Done",
		Status: StatusDone,
		Type:   TypeImplementation,
		Model:  "sonnet",
	})

	_, err := store.GetNextTask("worker-aabbccdd")
	if err != ErrNoAvailableTask {
		t.Errorf("err = %v, want ErrNoAvailableTask", err)
	}
}

// =========================================================================
// Tests: CompleteTask (Review-as-Task)
// =========================================================================

func TestSupabaseCompleteTaskGeneratesReview(t *testing.T) {
	server, store := mockPostgREST(t)
	defer server.Close()

	// Create and "assign" a task
	store.CreateTask(&Task{
		ID:         "T-001-0",
		Title:      "Build feature",
		Status:     StatusInProgress,
		AssignedTo: "worker-aabbccdd",
		Type:       TypeImplementation,
		BaseID:     "001",
		Version:    0,
		Model:      "sonnet",
	})

	review, err := store.CompleteTask("T-001-0", "worker-aabbccdd", "abc123")
	if err != nil {
		t.Fatalf("CompleteTask: %v", err)
	}

	// Check original task is done
	original, _ := store.GetTask("T-001-0")
	if original.Status != StatusDone {
		t.Errorf("original status = %q, want done", original.Status)
	}

	// Check review task was created
	if review == nil {
		t.Fatal("expected review task, got nil")
	}
	if review.ID != "R-001-0" {
		t.Errorf("review ID = %q, want R-001-0", review.ID)
	}
	if review.Type != TypeReview {
		t.Errorf("review type = %q, want REVIEW", review.Type)
	}
	if review.Status != StatusPending {
		t.Errorf("review status = %q, want pending", review.Status)
	}

	// Verify review exists in store
	fetched, err := store.GetTask("R-001-0")
	if err != nil {
		t.Fatalf("GetTask review: %v", err)
	}
	if fetched.ParentID != "T-001-0" {
		t.Errorf("review parent = %q, want T-001-0", fetched.ParentID)
	}
}

func TestSupabaseCompleteTaskWrongWorker(t *testing.T) {
	server, store := mockPostgREST(t)
	defer server.Close()

	store.CreateTask(&Task{
		ID:         "T-001-0",
		Title:      "Build feature",
		Status:     StatusInProgress,
		AssignedTo: "worker-aabbccdd",
		Type:       TypeImplementation,
		Model:      "sonnet",
	})

	_, err := store.CompleteTask("T-001-0", "worker-other000", "abc123")
	if err == nil {
		t.Error("expected error for wrong worker")
	}
}

func TestSupabaseCompleteTaskNotInProgress(t *testing.T) {
	server, store := mockPostgREST(t)
	defer server.Close()

	store.CreateTask(&Task{
		ID:     "T-001-0",
		Title:  "Pending task",
		Status: StatusPending,
		Type:   TypeImplementation,
		Model:  "sonnet",
	})

	_, err := store.CompleteTask("T-001-0", "worker-aabbccdd", "abc123")
	if err == nil {
		t.Error("expected error for pending task")
	}
}

func TestSupabaseCompleteReviewTaskNoAutoReview(t *testing.T) {
	server, store := mockPostgREST(t)
	defer server.Close()

	store.CreateTask(&Task{
		ID:         "R-001-0",
		Title:      "Review task",
		Status:     StatusInProgress,
		AssignedTo: "worker-aabbccdd",
		Type:       TypeReview,
		BaseID:     "001",
		Model:      "opus",
	})

	review, err := store.CompleteTask("R-001-0", "worker-aabbccdd", "def456")
	if err != nil {
		t.Fatalf("CompleteTask review: %v", err)
	}
	if review != nil {
		t.Errorf("expected nil review for REVIEW type, got %+v", review)
	}
}

// =========================================================================
// Tests: Factory
// =========================================================================

func TestFactoryMemory(t *testing.T) {
	store, err := NewTaskStore(&StoreConfig{Backend: BackendMemory})
	if err != nil {
		t.Fatalf("NewTaskStore memory: %v", err)
	}
	if store == nil {
		t.Fatal("store is nil")
	}
}

func TestFactorySQLite(t *testing.T) {
	tmpDir := t.TempDir()
	dbFile := filepath.Join(tmpDir, "test.db")
	db, err := sql.Open("sqlite", dbFile)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	store, err := NewTaskStore(&StoreConfig{
		Backend: BackendSQLite,
		SQLite:  &SQLiteConfig{DB: db, ProjectID: "test"},
	})
	if err != nil {
		t.Fatalf("NewTaskStore sqlite: %v", err)
	}
	if store == nil {
		t.Fatal("store is nil")
	}
}

func TestFactorySupabase(t *testing.T) {
	store, err := NewTaskStore(&StoreConfig{
		Backend: BackendSupabase,
		Supabase: &SupabaseConfig{
			URL:       "https://test.supabase.co",
			AnonKey:   "test-key",
			ProjectID: "test-project",
		},
	})
	if err != nil {
		t.Fatalf("NewTaskStore supabase: %v", err)
	}
	if store == nil {
		t.Fatal("store is nil")
	}
}

func TestFactorySupabaseMissingConfig(t *testing.T) {
	_, err := NewTaskStore(&StoreConfig{Backend: BackendSupabase})
	if err == nil {
		t.Error("expected error when supabase config is missing")
	}
}

func TestFactoryDefault(t *testing.T) {
	store, err := NewTaskStore(nil)
	if err != nil {
		t.Fatalf("NewTaskStore nil: %v", err)
	}
	if store == nil {
		t.Fatal("store is nil")
	}
}

func TestFactoryUnknownBackend(t *testing.T) {
	_, err := NewTaskStore(&StoreConfig{Backend: "postgres"})
	if err == nil {
		t.Error("expected error for unknown backend")
	}
}

// =========================================================================
// Tests: Row conversion
// =========================================================================

func TestTaskToRowAndBack(t *testing.T) {
	original := &Task{
		ID:           "T-001-0",
		Title:        "Build feature",
		Scope:        "c4/daemon/",
		Priority:     5,
		DoD:          "Do the thing",
		Dependencies: []string{"T-000-0"},
		Status:       StatusPending,
		Domain:       "library",
		Model:        "sonnet",
		Type:         TypeImplementation,
		BaseID:       "001",
		Version:      0,
	}

	row := taskToRow(original, "test-project")
	if row.ProjectID != "test-project" {
		t.Errorf("ProjectID = %q", row.ProjectID)
	}

	roundTrip := rowToTask(row)
	if roundTrip.ID != original.ID {
		t.Errorf("ID = %q, want %q", roundTrip.ID, original.ID)
	}
	if roundTrip.Title != original.Title {
		t.Errorf("Title = %q, want %q", roundTrip.Title, original.Title)
	}
	if len(roundTrip.Dependencies) != 1 || roundTrip.Dependencies[0] != "T-000-0" {
		t.Errorf("Dependencies = %v, want [T-000-0]", roundTrip.Dependencies)
	}
	if roundTrip.Type != TypeImplementation {
		t.Errorf("Type = %q, want IMPLEMENTATION", roundTrip.Type)
	}
}

func TestRowToTaskEmptyDeps(t *testing.T) {
	row := &supabaseTaskRow{
		ID:           "T-001-0",
		Dependencies: "[]",
		Status:       "pending",
		TaskType:     "IMPLEMENTATION",
	}
	task := rowToTask(row)
	if len(task.Dependencies) != 0 {
		t.Errorf("expected empty deps, got %v", task.Dependencies)
	}
}

// =========================================================================
// Tests: Interface compliance
// =========================================================================

func TestStoreImplementsTaskStore(t *testing.T) {
	// Compile-time check that Store (in-memory) implements TaskStore.
	var _ TaskStore = (*Store)(nil)
}

func TestSupabaseStoreImplementsTaskStore(t *testing.T) {
	// Compile-time check (also in supabase_store.go).
	var _ TaskStore = (*SupabaseTaskStore)(nil)
}
