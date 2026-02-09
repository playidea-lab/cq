package cloud

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/changmin/c4-core/internal/mcp/handlers"
)

// newTestServer creates a PostgREST-like test server with a handler map.
// The routeMap keys are "METHOD /path" and values return (statusCode, responseBody).
func newTestServer(t *testing.T, routeMap map[string]func(r *http.Request) (int, string)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Method + " " + r.URL.Path
		if handler, ok := routeMap[key]; ok {
			code, body := handler(r)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(code)
			_, _ = w.Write([]byte(body))
			return
		}
		t.Logf("unhandled route: %s %s?%s", r.Method, r.URL.Path, r.URL.RawQuery)
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"route not found"}`))
	}))
}

func newTestStore(serverURL string) *CloudStore {
	return NewCloudStore(
		serverURL+"/rest/v1",
		"test-anon-key",
		"test-jwt-token",
		"proj-123",
	)
}

// --- Compile-time interface check ---

func TestCloudStoreImplementsStoreInterface(t *testing.T) {
	var _ handlers.Store = (*CloudStore)(nil)
}

// --- GetStatus ---

func TestGetStatus(t *testing.T) {
	srv := newTestServer(t, map[string]func(r *http.Request) (int, string){
		"GET /rest/v1/c4_state": func(r *http.Request) (int, string) {
			assertHeaders(t, r)
			return 200, `[{"project_id":"proj-123","state_json":"{\"status\":\"EXECUTE\",\"project_id\":\"proj-123\"}"}]`
		},
		"GET /rest/v1/c4_tasks": func(r *http.Request) (int, string) {
			assertHeaders(t, r)
			return 200, `[
				{"task_id":"T-001-0","status":"done"},
				{"task_id":"T-002-0","status":"pending"},
				{"task_id":"T-003-0","status":"in_progress"},
				{"task_id":"T-004-0","status":"blocked"}
			]`
		},
	})
	defer srv.Close()

	store := newTestStore(srv.URL)
	status, err := store.GetStatus()
	if err != nil {
		t.Fatalf("GetStatus() error: %v", err)
	}

	if status.State != "EXECUTE" {
		t.Errorf("State = %q, want %q", status.State, "EXECUTE")
	}
	if status.ProjectName != "proj-123" {
		t.Errorf("ProjectName = %q, want %q", status.ProjectName, "proj-123")
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

func TestGetStatus_EmptyProject(t *testing.T) {
	srv := newTestServer(t, map[string]func(r *http.Request) (int, string){
		"GET /rest/v1/c4_state": func(r *http.Request) (int, string) {
			return 200, `[]`
		},
		"GET /rest/v1/c4_tasks": func(r *http.Request) (int, string) {
			return 200, `[]`
		},
	})
	defer srv.Close()

	store := newTestStore(srv.URL)
	status, err := store.GetStatus()
	if err != nil {
		t.Fatalf("GetStatus() error: %v", err)
	}
	if status.State != "INIT" {
		t.Errorf("State = %q, want %q", status.State, "INIT")
	}
	if status.TotalTasks != 0 {
		t.Errorf("TotalTasks = %d, want 0", status.TotalTasks)
	}
}

// --- AddTask ---

func TestAddTask(t *testing.T) {
	var capturedBody string
	srv := newTestServer(t, map[string]func(r *http.Request) (int, string){
		"POST /rest/v1/c4_tasks": func(r *http.Request) (int, string) {
			assertHeaders(t, r)
			b, _ := io.ReadAll(r.Body)
			capturedBody = string(b)
			return 201, ""
		},
	})
	defer srv.Close()

	store := newTestStore(srv.URL)
	err := store.AddTask(&handlers.Task{
		ID:           "T-001-0",
		Title:        "Implement feature X",
		Scope:        "backend",
		DoD:          "Tests pass",
		Status:       "pending",
		Dependencies: []string{"T-000-0"},
		Domain:       "go",
		Priority:     5,
	})
	if err != nil {
		t.Fatalf("AddTask() error: %v", err)
	}

	// Verify the sent payload
	var payload cloudTaskRow
	if err := json.Unmarshal([]byte(capturedBody), &payload); err != nil {
		t.Fatalf("unmarshal captured body: %v", err)
	}
	if payload.TaskID != "T-001-0" {
		t.Errorf("task_id = %q, want %q", payload.TaskID, "T-001-0")
	}
	if payload.ProjectID != "proj-123" {
		t.Errorf("project_id = %q, want %q", payload.ProjectID, "proj-123")
	}
	if payload.Title != "Implement feature X" {
		t.Errorf("title = %q, want %q", payload.Title, "Implement feature X")
	}
	if payload.Dependencies != `["T-000-0"]` {
		t.Errorf("dependencies = %q, want %q", payload.Dependencies, `["T-000-0"]`)
	}
}

// --- GetTask ---

func TestGetTask(t *testing.T) {
	srv := newTestServer(t, map[string]func(r *http.Request) (int, string){
		"GET /rest/v1/c4_tasks": func(r *http.Request) (int, string) {
			assertHeaders(t, r)
			q := r.URL.RawQuery
			if !strings.Contains(q, "task_id=eq.T-001-0") {
				t.Errorf("query missing task_id filter: %s", q)
			}
			if !strings.Contains(q, "project_id=eq.proj-123") {
				t.Errorf("query missing project_id filter: %s", q)
			}
			return 200, `[{
				"task_id":"T-001-0",
				"title":"Implement feature X",
				"scope":"backend",
				"dod":"Tests pass",
				"status":"pending",
				"dependencies":"[\"T-000-0\"]",
				"domain":"go",
				"priority":5,
				"worker_id":"",
				"branch":"",
				"commit_sha":"",
				"project_id":"proj-123"
			}]`
		},
	})
	defer srv.Close()

	store := newTestStore(srv.URL)
	task, err := store.GetTask("T-001-0")
	if err != nil {
		t.Fatalf("GetTask() error: %v", err)
	}

	if task.ID != "T-001-0" {
		t.Errorf("ID = %q, want %q", task.ID, "T-001-0")
	}
	if task.Title != "Implement feature X" {
		t.Errorf("Title = %q, want %q", task.Title, "Implement feature X")
	}
	if task.Priority != 5 {
		t.Errorf("Priority = %d, want 5", task.Priority)
	}
	if len(task.Dependencies) != 1 || task.Dependencies[0] != "T-000-0" {
		t.Errorf("Dependencies = %v, want [T-000-0]", task.Dependencies)
	}
}

func TestGetTask_NotFound(t *testing.T) {
	srv := newTestServer(t, map[string]func(r *http.Request) (int, string){
		"GET /rest/v1/c4_tasks": func(r *http.Request) (int, string) {
			return 200, `[]`
		},
	})
	defer srv.Close()

	store := newTestStore(srv.URL)
	_, err := store.GetTask("T-999-0")
	if err == nil {
		t.Fatal("GetTask() expected error for missing task, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want it to contain 'not found'", err.Error())
	}
}

// --- SubmitTask ---

func TestSubmitTask(t *testing.T) {
	taskFetched := false
	taskUpdated := false

	srv := newTestServer(t, map[string]func(r *http.Request) (int, string){
		"GET /rest/v1/c4_tasks": func(r *http.Request) (int, string) {
			q := r.URL.RawQuery
			if strings.Contains(q, "task_id=eq.T-001-0") {
				taskFetched = true
				return 200, `[{
					"task_id":"T-001-0",
					"title":"Implement feature X",
					"status":"in_progress",
					"dod":"Tests pass",
					"dependencies":"[]",
					"domain":"go",
					"priority":5,
					"worker_id":"worker-1",
					"branch":"",
					"commit_sha":"",
					"scope":"backend",
					"project_id":"proj-123"
				}]`
			}
			// For checking pending count after submit
			if strings.Contains(q, "status=in.") {
				return 200, `[]`
			}
			return 200, `[]`
		},
		"PATCH /rest/v1/c4_tasks": func(r *http.Request) (int, string) {
			taskUpdated = true
			assertHeaders(t, r)
			b, _ := io.ReadAll(r.Body)
			var payload map[string]any
			_ = json.Unmarshal(b, &payload)
			if payload["status"] != "done" {
				t.Errorf("status = %v, want 'done'", payload["status"])
			}
			return 200, ""
		},
	})
	defer srv.Close()

	store := newTestStore(srv.URL)
	result, err := store.SubmitTask("T-001-0", "worker-1", "abc123", "", nil)
	if err != nil {
		t.Fatalf("SubmitTask() error: %v", err)
	}

	if !taskFetched {
		t.Error("expected task to be fetched")
	}
	if !taskUpdated {
		t.Error("expected task to be updated via PATCH")
	}
	if !result.Success {
		t.Error("expected Success = true")
	}
	if result.NextAction != "complete" {
		t.Errorf("NextAction = %q, want %q", result.NextAction, "complete")
	}
}

func TestSubmitTask_ValidationFailure(t *testing.T) {
	srv := newTestServer(t, map[string]func(r *http.Request) (int, string){})
	defer srv.Close()

	store := newTestStore(srv.URL)
	result, err := store.SubmitTask("T-001-0", "worker-1", "", "", []handlers.ValidationResult{
		{Name: "go_vet", Status: "fail", Message: "unused variable"},
	})
	if err != nil {
		t.Fatalf("SubmitTask() error: %v", err)
	}
	if result.Success {
		t.Error("expected Success = false for validation failure")
	}
	if !strings.Contains(result.Message, "go_vet") {
		t.Errorf("Message = %q, want it to contain 'go_vet'", result.Message)
	}
}

// --- ClaimTask ---

func TestClaimTask(t *testing.T) {
	patchCalled := false

	srv := newTestServer(t, map[string]func(r *http.Request) (int, string){
		"GET /rest/v1/c4_tasks": func(r *http.Request) (int, string) {
			return 200, `[{
				"task_id":"T-001-0",
				"title":"Implement feature X",
				"status":"pending",
				"dod":"Tests pass",
				"dependencies":"[]",
				"domain":"go",
				"priority":5,
				"worker_id":"",
				"branch":"",
				"commit_sha":"",
				"scope":"",
				"project_id":"proj-123"
			}]`
		},
		"PATCH /rest/v1/c4_tasks": func(r *http.Request) (int, string) {
			patchCalled = true
			b, _ := io.ReadAll(r.Body)
			var payload map[string]any
			_ = json.Unmarshal(b, &payload)
			if payload["status"] != "in_progress" {
				t.Errorf("status = %v, want 'in_progress'", payload["status"])
			}
			if payload["worker_id"] != "direct" {
				t.Errorf("worker_id = %v, want 'direct'", payload["worker_id"])
			}
			return 200, ""
		},
	})
	defer srv.Close()

	store := newTestStore(srv.URL)
	task, err := store.ClaimTask("T-001-0")
	if err != nil {
		t.Fatalf("ClaimTask() error: %v", err)
	}

	if !patchCalled {
		t.Error("expected PATCH to be called")
	}
	if task.ID != "T-001-0" {
		t.Errorf("ID = %q, want %q", task.ID, "T-001-0")
	}
	if task.Status != "in_progress" {
		t.Errorf("Status = %q, want %q", task.Status, "in_progress")
	}
	if task.WorkerID != "direct" {
		t.Errorf("WorkerID = %q, want %q", task.WorkerID, "direct")
	}
}

func TestClaimTask_NotPending(t *testing.T) {
	srv := newTestServer(t, map[string]func(r *http.Request) (int, string){
		"GET /rest/v1/c4_tasks": func(r *http.Request) (int, string) {
			return 200, `[{
				"task_id":"T-001-0",
				"title":"Implement feature X",
				"status":"in_progress",
				"dod":"",
				"dependencies":"[]",
				"domain":"",
				"priority":0,
				"worker_id":"other-worker",
				"branch":"",
				"commit_sha":"",
				"scope":"",
				"project_id":"proj-123"
			}]`
		},
	})
	defer srv.Close()

	store := newTestStore(srv.URL)
	_, err := store.ClaimTask("T-001-0")
	if err == nil {
		t.Fatal("ClaimTask() expected error for non-pending task, got nil")
	}
	if !strings.Contains(err.Error(), "not pending") {
		t.Errorf("error = %q, want it to contain 'not pending'", err.Error())
	}
}

// --- ReportTask ---

func TestReportTask(t *testing.T) {
	patchCalled := false

	srv := newTestServer(t, map[string]func(r *http.Request) (int, string){
		"GET /rest/v1/c4_tasks": func(r *http.Request) (int, string) {
			return 200, `[{
				"task_id":"T-001-0",
				"title":"Implement feature X",
				"status":"in_progress",
				"dod":"Tests pass",
				"dependencies":"[]",
				"domain":"go",
				"priority":5,
				"worker_id":"direct",
				"branch":"",
				"commit_sha":"",
				"scope":"",
				"project_id":"proj-123"
			}]`
		},
		"PATCH /rest/v1/c4_tasks": func(r *http.Request) (int, string) {
			patchCalled = true
			b, _ := io.ReadAll(r.Body)
			var payload map[string]any
			_ = json.Unmarshal(b, &payload)
			if payload["status"] != "done" {
				t.Errorf("status = %v, want 'done'", payload["status"])
			}
			return 200, ""
		},
	})
	defer srv.Close()

	store := newTestStore(srv.URL)
	err := store.ReportTask("T-001-0", "Completed implementation", []string{"main.go", "handler.go"})
	if err != nil {
		t.Fatalf("ReportTask() error: %v", err)
	}

	if !patchCalled {
		t.Error("expected PATCH to be called")
	}
}

// --- AssignTask ---

func TestAssignTask(t *testing.T) {
	patchCalled := false

	srv := newTestServer(t, map[string]func(r *http.Request) (int, string){
		"GET /rest/v1/c4_tasks": func(r *http.Request) (int, string) {
			return 200, `[
				{"task_id":"T-001-0","title":"Task 1","status":"done","dod":"","dependencies":"[]","domain":"","priority":1,"worker_id":"","branch":"","commit_sha":"","scope":"","project_id":"proj-123"},
				{"task_id":"T-002-0","title":"Task 2","status":"pending","dod":"Do stuff","dependencies":"[\"T-001-0\"]","domain":"go","priority":5,"worker_id":"","branch":"","commit_sha":"","scope":"","project_id":"proj-123"},
				{"task_id":"T-003-0","title":"Task 3","status":"pending","dod":"Other stuff","dependencies":"[\"T-999-0\"]","domain":"go","priority":10,"worker_id":"","branch":"","commit_sha":"","scope":"","project_id":"proj-123"}
			]`
		},
		"PATCH /rest/v1/c4_tasks": func(r *http.Request) (int, string) {
			patchCalled = true
			q := r.URL.RawQuery
			if !strings.Contains(q, "task_id=eq.T-002-0") {
				t.Errorf("expected PATCH for T-002-0, got query: %s", q)
			}
			return 200, ""
		},
	})
	defer srv.Close()

	store := newTestStore(srv.URL)
	assignment, err := store.AssignTask("worker-1")
	if err != nil {
		t.Fatalf("AssignTask() error: %v", err)
	}
	if assignment == nil {
		t.Fatal("AssignTask() returned nil assignment")
	}

	if !patchCalled {
		t.Error("expected PATCH to be called")
	}
	// T-002-0 is pending with deps met (T-001-0 is done). T-003-0 depends on T-999-0 (not done).
	if assignment.TaskID != "T-002-0" {
		t.Errorf("TaskID = %q, want %q", assignment.TaskID, "T-002-0")
	}
	if assignment.WorkerID != "worker-1" {
		t.Errorf("WorkerID = %q, want %q", assignment.WorkerID, "worker-1")
	}
}

func TestAssignTask_NoneAvailable(t *testing.T) {
	srv := newTestServer(t, map[string]func(r *http.Request) (int, string){
		"GET /rest/v1/c4_tasks": func(r *http.Request) (int, string) {
			return 200, `[
				{"task_id":"T-001-0","title":"Task 1","status":"done","dod":"","dependencies":"[]","domain":"","priority":1,"worker_id":"","branch":"","commit_sha":"","scope":"","project_id":"proj-123"}
			]`
		},
	})
	defer srv.Close()

	store := newTestStore(srv.URL)
	assignment, err := store.AssignTask("worker-1")
	if err != nil {
		t.Fatalf("AssignTask() error: %v", err)
	}
	if assignment != nil {
		t.Errorf("expected nil assignment when no tasks available, got %+v", assignment)
	}
}

// --- Checkpoint ---

func TestCheckpoint(t *testing.T) {
	postCalled := false

	srv := newTestServer(t, map[string]func(r *http.Request) (int, string){
		"POST /rest/v1/c4_checkpoints": func(r *http.Request) (int, string) {
			postCalled = true
			assertHeaders(t, r)
			b, _ := io.ReadAll(r.Body)
			var payload map[string]any
			_ = json.Unmarshal(b, &payload)
			if payload["checkpoint_id"] != "CP-001" {
				t.Errorf("checkpoint_id = %v, want 'CP-001'", payload["checkpoint_id"])
			}
			if payload["decision"] != "APPROVE" {
				t.Errorf("decision = %v, want 'APPROVE'", payload["decision"])
			}
			return 201, ""
		},
	})
	defer srv.Close()

	store := newTestStore(srv.URL)
	result, err := store.Checkpoint("CP-001", "APPROVE", "Looks good", nil)
	if err != nil {
		t.Fatalf("Checkpoint() error: %v", err)
	}

	if !postCalled {
		t.Error("expected POST to be called")
	}
	if !result.Success {
		t.Error("expected Success = true")
	}
	if result.NextAction != "continue" {
		t.Errorf("NextAction = %q, want %q", result.NextAction, "continue")
	}
}

// --- HTTP header assertions ---

func assertHeaders(t *testing.T, r *http.Request) {
	t.Helper()
	if r.Header.Get("apikey") != "test-anon-key" {
		t.Errorf("apikey header = %q, want %q", r.Header.Get("apikey"), "test-anon-key")
	}
	if r.Header.Get("Authorization") != "Bearer test-jwt-token" {
		t.Errorf("Authorization header = %q, want %q", r.Header.Get("Authorization"), "Bearer test-jwt-token")
	}
	if ct := r.Header.Get("Content-Type"); ct != "" && ct != "application/json" {
		t.Errorf("Content-Type header = %q, want %q", ct, "application/json")
	}
}

// --- TransitionState ---

func TestTransitionState(t *testing.T) {
	getCalled := false
	patchCalled := false

	srv := newTestServer(t, map[string]func(r *http.Request) (int, string){
		"GET /rest/v1/c4_state": func(r *http.Request) (int, string) {
			getCalled = true
			return 200, `[{"project_id":"proj-123","state_json":"{\"status\":\"PLAN\",\"project_id\":\"proj-123\"}"}]`
		},
		"PATCH /rest/v1/c4_state": func(r *http.Request) (int, string) {
			patchCalled = true
			b, _ := io.ReadAll(r.Body)
			var payload map[string]any
			_ = json.Unmarshal(b, &payload)
			stateJSON, _ := payload["state_json"].(string)
			var stateMap map[string]any
			_ = json.Unmarshal([]byte(stateJSON), &stateMap)
			if stateMap["status"] != "EXECUTE" {
				t.Errorf("new status = %v, want 'EXECUTE'", stateMap["status"])
			}
			return 200, ""
		},
	})
	defer srv.Close()

	store := newTestStore(srv.URL)
	err := store.TransitionState("PLAN", "EXECUTE")
	if err != nil {
		t.Fatalf("TransitionState() error: %v", err)
	}

	if !getCalled {
		t.Error("expected GET to be called for reading current state")
	}
	if !patchCalled {
		t.Error("expected PATCH to be called for updating state")
	}
}

func TestTransitionState_Mismatch(t *testing.T) {
	srv := newTestServer(t, map[string]func(r *http.Request) (int, string){
		"GET /rest/v1/c4_state": func(r *http.Request) (int, string) {
			return 200, `[{"project_id":"proj-123","state_json":"{\"status\":\"INIT\",\"project_id\":\"proj-123\"}"}]`
		},
	})
	defer srv.Close()

	store := newTestStore(srv.URL)
	err := store.TransitionState("PLAN", "EXECUTE")
	if err == nil {
		t.Fatal("TransitionState() expected error for state mismatch, got nil")
	}
	if !strings.Contains(err.Error(), "current state is INIT") {
		t.Errorf("error = %q, want it to contain 'current state is INIT'", err.Error())
	}
}

// --- RequestChanges ---

func TestRequestChanges(t *testing.T) {
	postsReceived := 0
	patchCalled := false

	srv := newTestServer(t, map[string]func(r *http.Request) (int, string){
		"GET /rest/v1/c4_tasks": func(r *http.Request) (int, string) {
			q := r.URL.RawQuery
			if strings.Contains(q, "task_id=eq.T-001-0") {
				return 200, `[{"task_id":"T-001-0","title":"Impl feature","status":"done","dod":"original dod","dependencies":"[]","domain":"go","priority":5,"worker_id":"","branch":"","commit_sha":"","scope":"","project_id":"proj-123"}]`
			}
			return 200, `[]`
		},
		"PATCH /rest/v1/c4_tasks": func(r *http.Request) (int, string) {
			patchCalled = true
			return 200, ""
		},
		"POST /rest/v1/c4_tasks": func(r *http.Request) (int, string) {
			postsReceived++
			return 201, ""
		},
	})
	defer srv.Close()

	store := newTestStore(srv.URL)
	result, err := store.RequestChanges("R-001-0", "Needs more tests", []string{"Add unit tests", "Fix error handling"})
	if err != nil {
		t.Fatalf("RequestChanges() error: %v", err)
	}

	if !result.Success {
		t.Error("expected Success = true")
	}
	if result.NextTaskID != "T-001-1" {
		t.Errorf("NextTaskID = %q, want %q", result.NextTaskID, "T-001-1")
	}
	if result.NextReviewID != "R-001-1" {
		t.Errorf("NextReviewID = %q, want %q", result.NextReviewID, "R-001-1")
	}
	if result.Version != 1 {
		t.Errorf("Version = %d, want 1", result.Version)
	}
	if !patchCalled {
		t.Error("expected PATCH to be called (mark R-001-0 done)")
	}
	if postsReceived != 2 {
		t.Errorf("POST count = %d, want 2 (T-001-1 + R-001-1)", postsReceived)
	}
}

// --- MarkBlocked ---

func TestMarkBlocked(t *testing.T) {
	patchCalled := false

	srv := newTestServer(t, map[string]func(r *http.Request) (int, string){
		"PATCH /rest/v1/c4_tasks": func(r *http.Request) (int, string) {
			patchCalled = true
			b, _ := io.ReadAll(r.Body)
			var payload map[string]any
			_ = json.Unmarshal(b, &payload)
			if payload["status"] != "blocked" {
				t.Errorf("status = %v, want 'blocked'", payload["status"])
			}
			return 200, ""
		},
	})
	defer srv.Close()

	store := newTestStore(srv.URL)
	err := store.MarkBlocked("T-001-0", "worker-1", "build_fail", 3, "go vet failed")
	if err != nil {
		t.Fatalf("MarkBlocked() error: %v", err)
	}

	if !patchCalled {
		t.Error("expected PATCH to be called")
	}
}

// --- Start ---

func TestStart(t *testing.T) {
	patchCalled := false

	srv := newTestServer(t, map[string]func(r *http.Request) (int, string){
		"GET /rest/v1/c4_state": func(r *http.Request) (int, string) {
			return 200, `[{"project_id":"proj-123","state_json":"{\"status\":\"PLAN\",\"project_id\":\"proj-123\"}"}]`
		},
		"PATCH /rest/v1/c4_state": func(r *http.Request) (int, string) {
			patchCalled = true
			return 200, ""
		},
	})
	defer srv.Close()

	store := newTestStore(srv.URL)
	err := store.Start()
	if err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	if !patchCalled {
		t.Error("expected PATCH to be called to update state")
	}
}

// --- Clear ---

func TestClear(t *testing.T) {
	deletedTables := map[string]bool{}

	srv := newTestServer(t, map[string]func(r *http.Request) (int, string){
		"DELETE /rest/v1/c4_tasks": func(r *http.Request) (int, string) {
			deletedTables["c4_tasks"] = true
			return 200, ""
		},
		"DELETE /rest/v1/c4_state": func(r *http.Request) (int, string) {
			deletedTables["c4_state"] = true
			return 200, ""
		},
		"DELETE /rest/v1/c4_checkpoints": func(r *http.Request) (int, string) {
			deletedTables["c4_checkpoints"] = true
			return 200, ""
		},
	})
	defer srv.Close()

	store := newTestStore(srv.URL)
	err := store.Clear(false)
	if err != nil {
		t.Fatalf("Clear() error: %v", err)
	}

	for _, table := range []string{"c4_tasks", "c4_state", "c4_checkpoints"} {
		if !deletedTables[table] {
			t.Errorf("expected DELETE for table %s", table)
		}
	}
}

// --- HTTP error handling ---

func TestHTTPErrorPropagation(t *testing.T) {
	srv := newTestServer(t, map[string]func(r *http.Request) (int, string){
		"GET /rest/v1/c4_state": func(r *http.Request) (int, string) {
			return 500, `{"message":"internal error"}`
		},
	})
	defer srv.Close()

	store := newTestStore(srv.URL)
	_, err := store.GetStatus()
	if err == nil {
		t.Fatal("expected error on 500 response, got nil")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error = %q, want it to contain '500'", err.Error())
	}
}
