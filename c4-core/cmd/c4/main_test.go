package main

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/changmin/c4-core/internal/mcp"
	"github.com/changmin/c4-core/internal/mcp/apps"
	"github.com/changmin/c4-core/internal/mcp/handlers"
	_ "modernc.org/sqlite"
)

// setupTestDB creates a temporary .c4/ directory with a SQLite database
// containing the c4_state and c4_tasks tables, and sets projectDir so
// dbPath() resolves correctly.
func setupTestDB(t *testing.T) (*sql.DB, func()) {
	t.Helper()

	tmpDir := t.TempDir()
	c4DirPath := filepath.Join(tmpDir, ".c4")
	if err := os.MkdirAll(c4DirPath, 0o755); err != nil {
		t.Fatalf("failed to create .c4 dir: %v", err)
	}

	// Set the global projectDir so dbPath() resolves correctly
	oldProjectDir := projectDir
	projectDir = tmpDir

	dbFile := filepath.Join(c4DirPath, "tasks.db")
	db, err := sql.Open("sqlite", dbFile)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}

	// Create schema
	schema := `
		CREATE TABLE IF NOT EXISTS c4_state (
			project_id TEXT PRIMARY KEY,
			state_json TEXT NOT NULL,
			updated_at TEXT DEFAULT CURRENT_TIMESTAMP
		);
		CREATE TABLE IF NOT EXISTS c4_tasks (
			task_id    TEXT PRIMARY KEY,
			title      TEXT NOT NULL,
			scope      TEXT DEFAULT '',
			dod        TEXT DEFAULT '',
			priority   INTEGER DEFAULT 5,
			status     TEXT DEFAULT 'pending',
			dependencies TEXT DEFAULT '[]',
			depends_on TEXT DEFAULT '[]',
			domain     TEXT DEFAULT '',
			model      TEXT DEFAULT '',
			worker_id  TEXT DEFAULT '',
			branch     TEXT DEFAULT '',
			commit_sha TEXT DEFAULT '',
			created_at TEXT DEFAULT CURRENT_TIMESTAMP,
			updated_at TEXT DEFAULT CURRENT_TIMESTAMP
		);
		CREATE TABLE IF NOT EXISTS c4_checkpoints (
			checkpoint_id TEXT PRIMARY KEY,
			decision TEXT,
			notes TEXT,
			required_changes TEXT DEFAULT '[]',
			created_at TEXT DEFAULT CURRENT_TIMESTAMP
		);
	`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	cleanup := func() {
		db.Close()
		projectDir = oldProjectDir
	}

	return db, cleanup
}

// insertState inserts a project state row.
func insertState(t *testing.T, db *sql.DB, projectID, status string) {
	t.Helper()
	stateJSON := `{"project_id":"` + projectID + `","status":"` + status + `"}`
	_, err := db.Exec(
		"INSERT INTO c4_state (project_id, state_json) VALUES (?, ?)",
		projectID, stateJSON,
	)
	if err != nil {
		t.Fatalf("failed to insert state: %v", err)
	}
}

// insertTask inserts a task row.
func insertTask(t *testing.T, db *sql.DB, taskID, title, status string) {
	t.Helper()
	_, err := db.Exec(
		"INSERT INTO c4_tasks (task_id, title, status) VALUES (?, ?, ?)",
		taskID, title, status,
	)
	if err != nil {
		t.Fatalf("failed to insert task: %v", err)
	}
}

// newTestMCPServer creates a minimal MCP server backed by a test DB.
// It only registers core tools (no sidecar/proxy needed for tests).
func newTestMCPServer(t *testing.T, db *sql.DB) *mcpServer {
	t.Helper()

	store, err := handlers.NewSQLiteStore(db)
	if err != nil {
		t.Fatalf("creating store: %v", err)
	}

	reg := mcp.NewRegistry()
	handlers.RegisterAll(reg, store)

	return &mcpServer{registry: reg}
}

// TestStatusCommandOutput verifies that runStatus reads project state and
// task counts from the database and produces output without error.
func TestStatusCommandOutput(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	insertState(t, db, "test-project", "EXECUTE")
	insertTask(t, db, "T-001-0", "Task one", "done")
	insertTask(t, db, "T-002-0", "Task two", "pending")
	insertTask(t, db, "T-003-0", "Task three", "in_progress")

	// Run the status command (it writes to stdout)
	err := runStatus(nil, nil)
	if err != nil {
		t.Fatalf("runStatus returned error: %v", err)
	}

	// Verify state was read correctly
	state, err := loadProjectState(db)
	if err != nil {
		t.Fatalf("loadProjectState failed: %v", err)
	}
	if state.Status != "EXECUTE" {
		t.Errorf("expected status EXECUTE, got %s", state.Status)
	}
	if state.ProjectID != "test-project" {
		t.Errorf("expected project_id test-project, got %s", state.ProjectID)
	}

	// Verify task counts
	counts, err := countTasks(db)
	if err != nil {
		t.Fatalf("countTasks failed: %v", err)
	}
	if counts.Total != 3 {
		t.Errorf("expected 3 total tasks, got %d", counts.Total)
	}
	if counts.Done != 1 {
		t.Errorf("expected 1 done, got %d", counts.Done)
	}
	if counts.Pending != 1 {
		t.Errorf("expected 1 pending, got %d", counts.Pending)
	}
	if counts.InProgress != 1 {
		t.Errorf("expected 1 in_progress, got %d", counts.InProgress)
	}
}

// TestRunStartsWorkers verifies that the run command transitions the state
// from PLAN to EXECUTE when tasks are present.
func TestRunStartsWorkers(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	insertState(t, db, "test-project", "PLAN")
	insertTask(t, db, "T-001-0", "Task one", "pending")
	insertTask(t, db, "T-002-0", "Task two", "pending")

	// Set global workers flag
	oldWorkers := runWorkers
	runWorkers = 2
	defer func() { runWorkers = oldWorkers }()

	err := runRun(nil, nil)
	if err != nil {
		t.Fatalf("runRun returned error: %v", err)
	}

	// Verify state was transitioned to EXECUTE
	state, err := loadProjectState(db)
	if err != nil {
		t.Fatalf("loadProjectState failed: %v", err)
	}
	if state.Status != "EXECUTE" {
		t.Errorf("expected status EXECUTE after run, got %s", state.Status)
	}
}

// TestRunWithoutTasksErrors verifies that the run command returns an error
// when there are no tasks in the database.
func TestRunWithoutTasksErrors(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	insertState(t, db, "test-project", "PLAN")
	// No tasks inserted

	err := runRun(nil, nil)
	if err == nil {
		t.Fatal("expected error when running with no tasks, got nil")
	}

	expected := "no tasks found"
	if !containsString(err.Error(), expected) {
		t.Errorf("expected error containing %q, got %q", expected, err.Error())
	}
}

// TestStopTransitionsState verifies the stop command transitions EXECUTE -> HALTED.
func TestStopTransitionsState(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	insertState(t, db, "test-project", "EXECUTE")

	err := runTaskStop(nil, nil)
	if err != nil {
		t.Fatalf("runTaskStop returned error: %v", err)
	}

	state, err := loadProjectState(db)
	if err != nil {
		t.Fatalf("loadProjectState failed: %v", err)
	}
	if state.Status != "HALTED" {
		t.Errorf("expected status HALTED after stop, got %s", state.Status)
	}
}

// TestAddTaskCreatesTask verifies that add-task inserts a task row.
func TestAddTaskCreatesTask(t *testing.T) {
	_, cleanup := setupTestDB(t)
	defer cleanup()

	// Set global flags for add-task
	oldTitle := taskTitle
	oldDoD := taskDoD
	oldScope := taskScope
	oldPriority := taskPriority
	taskTitle = "Test Task"
	taskDoD = ""
	taskScope = "src/"
	taskPriority = 3
	taskDepends = nil
	defer func() {
		taskTitle = oldTitle
		taskDoD = oldDoD
		taskScope = oldScope
		taskPriority = oldPriority
	}()

	err := runAddTask(nil, nil)
	if err != nil {
		t.Fatalf("runAddTask returned error: %v", err)
	}

	// Verify task was inserted by opening the DB and checking
	db, err := sql.Open("sqlite", dbPath())
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	var title, status string
	err = db.QueryRow("SELECT title, status FROM c4_tasks WHERE task_id = 'T-001-0'").Scan(&title, &status)
	if err != nil {
		t.Fatalf("failed to query inserted task: %v", err)
	}
	if title != "Test Task" {
		t.Errorf("expected title 'Test Task', got %q", title)
	}
	if status != "pending" {
		t.Errorf("expected status 'pending', got %q", status)
	}
}

// TestMCPInitialize verifies the MCP server handles the initialize method.
func TestMCPInitialize(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	srv := newTestMCPServer(t, db)

	req := &mcpRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "initialize",
	}

	resp := srv.handleRequest(req)
	if resp == nil {
		t.Fatal("expected response, got nil")
	}
	if resp.Error != nil {
		t.Fatalf("expected no error, got %v", resp.Error)
	}

	result, ok := resp.Result.(initializeResult)
	if !ok {
		t.Fatalf("expected initializeResult, got %T", resp.Result)
	}
	if result.ServerInfo.Name != "c4" {
		t.Errorf("expected server name 'c4', got %q", result.ServerInfo.Name)
	}
}

// TestMCPToolsList verifies the MCP server returns tool definitions.
func TestMCPToolsList(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	srv := newTestMCPServer(t, db)

	req := &mcpRequest{
		JSONRPC: "2.0",
		ID:      2,
		Method:  "tools/list",
	}

	resp := srv.handleRequest(req)
	if resp == nil {
		t.Fatal("expected response, got nil")
	}
	if resp.Error != nil {
		t.Fatalf("expected no error, got %v", resp.Error)
	}

	resultMap, ok := resp.Result.(map[string]any)
	if !ok {
		t.Fatalf("expected map result, got %T", resp.Result)
	}

	tools, ok := resultMap["tools"].([]map[string]any)
	if !ok {
		t.Fatalf("expected []map[string]any, got %T", resultMap["tools"])
	}

	// Core tools: c4_status, c4_start, c4_clear, c4_get_task, c4_submit,
	// c4_add_todo, c4_mark_blocked, c4_claim, c4_report, c4_checkpoint
	if len(tools) < 10 {
		t.Errorf("expected at least 10 core tools, got %d", len(tools))
	}

	// Verify key tools exist
	names := make(map[string]bool)
	for _, tool := range tools {
		if name, ok := tool["name"].(string); ok {
			names[name] = true
		}
	}
	for _, expected := range []string{"c4_status", "c4_start", "c4_get_task", "c4_submit", "c4_claim"} {
		if !names[expected] {
			t.Errorf("expected tool %q not found", expected)
		}
	}
}

// TestMCPUnknownMethod verifies the MCP server returns a method-not-found error.
func TestMCPUnknownMethod(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	srv := newTestMCPServer(t, db)

	req := &mcpRequest{
		JSONRPC: "2.0",
		ID:      3,
		Method:  "unknown/method",
	}

	resp := srv.handleRequest(req)
	if resp == nil {
		t.Fatal("expected response, got nil")
	}
	if resp.Error == nil {
		t.Fatal("expected error, got nil")
	}
	if resp.Error.Code != -32601 {
		t.Errorf("expected error code -32601, got %d", resp.Error.Code)
	}
}

// TestLoadProjectStateNoRows verifies default state when no rows exist.
func TestLoadProjectStateNoRows(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	state, err := loadProjectState(db)
	if err != nil {
		t.Fatalf("loadProjectState failed: %v", err)
	}
	if state.Status != "INIT" {
		t.Errorf("expected status INIT for empty db, got %s", state.Status)
	}
}

// TestCountTasksEmptyTable verifies zero counts when no tasks exist.
func TestCountTasksEmptyTable(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	counts, err := countTasks(db)
	if err != nil {
		t.Fatalf("countTasks failed: %v", err)
	}
	if counts.Total != 0 {
		t.Errorf("expected 0 total, got %d", counts.Total)
	}
}

// TestRunFromCompleteState verifies that run returns early for COMPLETE state.
func TestRunFromCompleteState(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	insertState(t, db, "test-project", "COMPLETE")
	insertTask(t, db, "T-001-0", "Task one", "done")

	err := runRun(nil, nil)
	if err != nil {
		t.Fatalf("runRun returned error for COMPLETE state: %v", err)
	}
}

// TestMCPStdioIntegration verifies the full JSON-RPC round-trip:
// send initialize request, receive response with server info.
func TestMCPStdioIntegration(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	srv := newTestMCPServer(t, db)

	reqJSON := `{"jsonrpc":"2.0","id":1,"method":"initialize"}`
	var req mcpRequest
	if err := json.Unmarshal([]byte(reqJSON), &req); err != nil {
		t.Fatalf("unmarshal request: %v", err)
	}

	resp := srv.handleRequest(&req)
	if resp == nil {
		t.Fatal("expected non-nil response")
	}

	respBytes, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}

	var parsed struct {
		JSONRPC string      `json:"jsonrpc"`
		ID      interface{} `json:"id"`
		Result  interface{} `json:"result"`
		Error   interface{} `json:"error"`
	}
	if err := json.Unmarshal(respBytes, &parsed); err != nil {
		t.Fatalf("response not valid JSON: %v", err)
	}

	if parsed.JSONRPC != "2.0" {
		t.Errorf("jsonrpc = %q, want '2.0'", parsed.JSONRPC)
	}
	if parsed.ID != float64(1) {
		t.Errorf("id = %v, want 1", parsed.ID)
	}
	if parsed.Error != nil {
		t.Errorf("unexpected error: %v", parsed.Error)
	}
}

// TestMCPStdioToolsCallIntegration tests a complete tools/call flow with c4_status.
func TestMCPStdioToolsCallIntegration(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	insertState(t, db, "integration-test", "EXECUTE")
	insertTask(t, db, "T-001-0", "Task one", "done")
	insertTask(t, db, "T-002-0", "Task two", "pending")

	srv := newTestMCPServer(t, db)

	reqJSON := `{"jsonrpc":"2.0","id":42,"method":"tools/call","params":{"name":"c4_status","arguments":{}}}`
	var req mcpRequest
	if err := json.Unmarshal([]byte(reqJSON), &req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	resp := srv.handleRequest(&req)
	if resp == nil {
		t.Fatal("nil response")
	}
	if resp.Error != nil {
		t.Fatalf("error: %v", resp.Error)
	}

	resultMap, ok := resp.Result.(map[string]any)
	if !ok {
		t.Fatalf("result type = %T, want map", resp.Result)
	}

	content, ok := resultMap["content"].([]map[string]any)
	if !ok {
		t.Fatalf("content type = %T, want []map", resultMap["content"])
	}

	if len(content) == 0 {
		t.Fatal("content is empty")
	}

	text, ok := content[0]["text"].(string)
	if !ok {
		t.Fatalf("text type = %T, want string", content[0]["text"])
	}

	var statusResult map[string]interface{}
	if err := json.Unmarshal([]byte(text), &statusResult); err != nil {
		t.Fatalf("parse status result: %v (text: %s)", err, text)
	}

	if statusResult["state"] != "EXECUTE" {
		t.Errorf("state = %v, want EXECUTE", statusResult["state"])
	}
}

// TestMCPResourcesRead verifies resources/read returns ui:// content.
func TestMCPResourcesRead(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	store, err := handlers.NewSQLiteStore(db)
	if err != nil {
		t.Fatalf("creating store: %v", err)
	}

	reg := mcp.NewRegistry()
	handlers.RegisterAll(reg, store)

	rsStore := apps.NewResourceStore()
	rsStore.Register("ui://cq/dashboard", "<html>dashboard</html>")

	srv := &mcpServer{
		registry:      reg,
		resourceStore: rsStore,
	}

	reqJSON := `{"jsonrpc":"2.0","id":10,"method":"resources/read","params":{"uri":"ui://cq/dashboard"}}`
	var req mcpRequest
	if err := json.Unmarshal([]byte(reqJSON), &req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	resp := srv.handleRequest(&req)
	if resp == nil {
		t.Fatal("nil response")
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}

	resultMap, ok := resp.Result.(map[string]any)
	if !ok {
		t.Fatalf("result type = %T, want map", resp.Result)
	}
	contents, ok := resultMap["contents"].([]map[string]any)
	if !ok || len(contents) == 0 {
		t.Fatalf("contents missing or empty: %v", resultMap["contents"])
	}
	if contents[0]["text"] != "<html>dashboard</html>" {
		t.Errorf("text = %q, want <html>dashboard</html>", contents[0]["text"])
	}
	if contents[0]["mimeType"] != "text/html" {
		t.Errorf("mimeType = %q, want text/html", contents[0]["mimeType"])
	}
}

// TestMCPResourcesRead_NotFound verifies resources/read returns error for missing resource.
func TestMCPResourcesRead_NotFound(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	srv := newTestMCPServer(t, db)
	srv.resourceStore = apps.NewResourceStore()

	reqJSON := `{"jsonrpc":"2.0","id":11,"method":"resources/read","params":{"uri":"ui://cq/missing"}}`
	var req mcpRequest
	if err := json.Unmarshal([]byte(reqJSON), &req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	resp := srv.handleRequest(&req)
	if resp == nil {
		t.Fatal("nil response")
	}
	if resp.Error == nil {
		t.Fatal("expected error for missing resource")
	}
}

// TestMCPResourcesRead_NoStore verifies resources/read returns error when store not configured.
func TestMCPResourcesRead_NoStore(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	srv := newTestMCPServer(t, db) // resourceStore is nil

	reqJSON := `{"jsonrpc":"2.0","id":12,"method":"resources/read","params":{"uri":"ui://cq/dashboard"}}`
	var req mcpRequest
	if err := json.Unmarshal([]byte(reqJSON), &req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	resp := srv.handleRequest(&req)
	if resp == nil {
		t.Fatal("nil response")
	}
	if resp.Error == nil {
		t.Fatal("expected error when resource store is nil")
	}
}

// containsString checks if s contains substr.
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
