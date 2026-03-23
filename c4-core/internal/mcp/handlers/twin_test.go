package handlers

import (
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/changmin/c4-core/internal/knowledge"
	"github.com/changmin/c4-core/internal/mcp"
	_ "modernc.org/sqlite"
)

// newTestDB creates an in-memory SQLite database with schema initialized.
func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("opening test db: %v", err)
	}
	return db
}

func newTestStore(t *testing.T) *SQLiteStore {
	t.Helper()
	db := newTestDB(t)
	store, err := NewSQLiteStore(db)
	if err != nil {
		t.Fatalf("creating test store: %v", err)
	}
	return store
}

// --- Schema tests ---

func TestInitSchemaCreatesTwinGrowth(t *testing.T) {
	store := newTestStore(t)

	// Verify twin_growth table exists
	var count int
	err := store.db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='twin_growth'").Scan(&count)
	if err != nil {
		t.Fatalf("querying sqlite_master: %v", err)
	}
	if count != 1 {
		t.Errorf("twin_growth table not created")
	}
}

// --- DetectPatterns tests ---

func TestDetectPatternsEmpty(t *testing.T) {
	store := newTestStore(t)

	patterns := store.DetectPatterns("direct")
	if len(patterns) != 0 {
		t.Errorf("expected 0 patterns on empty db, got %d", len(patterns))
	}
}

func TestDetectTrendShiftImproving(t *testing.T) {
	store := newTestStore(t)

	// Insert 20 tasks: first 10 rejected, last 10 approved
	now := time.Now()
	for i := 0; i < 20; i++ {
		taskID := taskIDForTest(i)
		outcome := "rejected"
		if i >= 10 {
			outcome = "approved"
		}
		ts := now.Add(time.Duration(i) * time.Hour).Format(time.RFC3339)
		_, _ = store.db.Exec(
			"INSERT INTO persona_stats (persona_id, task_id, outcome, created_at) VALUES (?, ?, ?, ?)",
			"direct", taskID, outcome, ts)
	}

	patterns := store.detectTrendShift("direct")
	if len(patterns) != 1 {
		t.Fatalf("expected 1 trend pattern, got %d", len(patterns))
	}
	if patterns[0].Type != "growth" {
		t.Errorf("pattern type = %q, want 'growth'", patterns[0].Type)
	}
}

func TestDetectTrendShiftDeclining(t *testing.T) {
	store := newTestStore(t)

	// Insert 20 tasks: first 10 approved, last 10 rejected
	now := time.Now()
	for i := 0; i < 20; i++ {
		taskID := taskIDForTest(i)
		outcome := "approved"
		if i >= 10 {
			outcome = "rejected"
		}
		ts := now.Add(time.Duration(i) * time.Hour).Format(time.RFC3339)
		_, _ = store.db.Exec(
			"INSERT INTO persona_stats (persona_id, task_id, outcome, created_at) VALUES (?, ?, ?, ?)",
			"direct", taskID, outcome, ts)
	}

	patterns := store.detectTrendShift("direct")
	if len(patterns) != 1 {
		t.Fatalf("expected 1 trend pattern, got %d", len(patterns))
	}
	if patterns[0].Type != "performance" {
		t.Errorf("pattern type = %q, want 'performance'", patterns[0].Type)
	}
	if patterns[0].Severity != "warning" {
		t.Errorf("pattern severity = %q, want 'warning'", patterns[0].Severity)
	}
}

func TestDetectRepeatedFailures(t *testing.T) {
	store := newTestStore(t)

	// Insert 5 consecutive rejections (most recent first)
	now := time.Now()
	for i := 0; i < 5; i++ {
		taskID := taskIDForTest(i)
		ts := now.Add(time.Duration(5-i) * time.Hour).Format(time.RFC3339)
		_, _ = store.db.Exec(
			"INSERT INTO persona_stats (persona_id, task_id, outcome, created_at) VALUES (?, ?, ?, ?)",
			"direct", taskID, "rejected", ts)
	}

	patterns := store.detectRepeatedFailures("direct")
	if len(patterns) != 1 {
		t.Fatalf("expected 1 failure pattern, got %d", len(patterns))
	}
	if patterns[0].Severity != "warning" {
		t.Errorf("severity = %q, want 'warning'", patterns[0].Severity)
	}
}

func TestDetectRepeatedFailuresNone(t *testing.T) {
	store := newTestStore(t)

	// Insert alternating approved/rejected
	now := time.Now()
	for i := 0; i < 10; i++ {
		taskID := taskIDForTest(i)
		outcome := "approved"
		if i%2 == 0 {
			outcome = "rejected"
		}
		ts := now.Add(time.Duration(i) * time.Hour).Format(time.RFC3339)
		_, _ = store.db.Exec(
			"INSERT INTO persona_stats (persona_id, task_id, outcome, created_at) VALUES (?, ?, ?, ?)",
			"direct", taskID, outcome, ts)
	}

	patterns := store.detectRepeatedFailures("direct")
	if len(patterns) != 0 {
		t.Errorf("expected 0 patterns for alternating outcomes, got %d", len(patterns))
	}
}

func TestDetectCheckpointPatterns(t *testing.T) {
	store := newTestStore(t)

	// Insert 5 checkpoints: 4 REQUEST_CHANGES, 1 APPROVE
	for i := 0; i < 5; i++ {
		decision := "REQUEST_CHANGES"
		if i == 0 {
			decision = "APPROVE"
		}
		ts := time.Now().Add(time.Duration(i) * time.Hour).Format(time.RFC3339)
		_, _ = store.db.Exec(
			"INSERT INTO c4_checkpoints (checkpoint_id, decision, notes, required_changes, created_at) VALUES (?, ?, ?, ?, ?)",
			taskIDForTest(i), decision, "some notes", "[]", ts)
	}

	patterns := store.detectCheckpointPatterns()
	if len(patterns) != 1 {
		t.Fatalf("expected 1 checkpoint pattern, got %d", len(patterns))
	}
	if patterns[0].Severity != "challenge" {
		t.Errorf("severity = %q, want 'challenge'", patterns[0].Severity)
	}
}

func TestDetectFeedbackKeywords(t *testing.T) {
	store := newTestStore(t)

	// Insert 5 rejections all mentioning "test"
	for i := 0; i < 5; i++ {
		ts := time.Now().Add(time.Duration(i) * time.Hour).Format(time.RFC3339)
		_, _ = store.db.Exec(
			"INSERT INTO c4_checkpoints (checkpoint_id, decision, notes, required_changes, created_at) VALUES (?, ?, ?, ?, ?)",
			taskIDForTest(i), "REQUEST_CHANGES", "Missing test coverage for this change", "[]", ts)
	}

	patterns := store.detectFeedbackKeywords()
	if len(patterns) == 0 {
		t.Fatal("expected feedback keyword pattern")
	}

	found := false
	for _, p := range patterns {
		if p.Type == "behavioral" && p.Severity == "challenge" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected behavioral/challenge pattern for repeated 'test' keyword")
	}
}

// --- Speed pattern edge case tests ---

func TestDetectSpeedChangeZeroDays(t *testing.T) {
	store := newTestStore(t)

	// Insert tasks with same created_at and updated_at (0 days duration)
	ts := time.Now().Format(time.RFC3339)
	for i := 0; i < 10; i++ {
		taskID := taskIDForTest(100 + i)
		_, _ = store.db.Exec(
			"INSERT INTO c4_tasks (task_id, title, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?)",
			taskID, "Same-day task", "done", ts, ts)
	}

	patterns := store.detectSpeedChange()
	if len(patterns) > 0 {
		t.Errorf("expected no speed patterns for zero-day tasks, got: %+v", patterns)
	}
}

// --- Growth tracking tests ---

func TestRecordGrowthSnapshot(t *testing.T) {
	store := newTestStore(t)

	// Insert some persona_stats
	ts := time.Now().Format(time.RFC3339)
	_, _ = store.db.Exec("INSERT INTO persona_stats (persona_id, task_id, outcome, created_at) VALUES (?, ?, ?, ?)",
		"direct", "T-001", "approved", ts)
	_, _ = store.db.Exec("INSERT INTO persona_stats (persona_id, task_id, outcome, created_at) VALUES (?, ?, ?, ?)",
		"direct", "T-002", "approved", ts)
	_, _ = store.db.Exec("INSERT INTO persona_stats (persona_id, task_id, outcome, created_at) VALUES (?, ?, ?, ?)",
		"direct", "T-003", "rejected", ts)

	store.RecordGrowthSnapshot("testuser")

	// Verify growth records exist
	var count int
	_ = store.db.QueryRow("SELECT COUNT(*) FROM twin_growth WHERE username='testuser'").Scan(&count)
	if count == 0 {
		t.Error("expected growth records to be created")
	}

	// Should be idempotent (same period)
	store.RecordGrowthSnapshot("testuser")
	var count2 int
	_ = store.db.QueryRow("SELECT COUNT(*) FROM twin_growth WHERE username='testuser'").Scan(&count2)
	if count2 != count {
		t.Errorf("idempotency failed: first=%d, second=%d", count, count2)
	}
}

func TestGetGrowthTrend(t *testing.T) {
	store := newTestStore(t)

	// Manually insert some growth metrics
	_, _ = store.db.Exec("INSERT INTO twin_growth (username, metric, value, period) VALUES (?, ?, ?, ?)",
		"testuser", "approval_rate", 0.85, "2026-W06")
	_, _ = store.db.Exec("INSERT INTO twin_growth (username, metric, value, period) VALUES (?, ?, ?, ?)",
		"testuser", "approval_rate", 0.72, "2026-W02")

	trend := store.GetGrowthTrend("testuser")

	ar, ok := trend["approval_rate"]
	if !ok {
		t.Fatal("missing approval_rate metric")
	}
	if ar.Current != 0.85 {
		t.Errorf("current = %f, want 0.85", ar.Current)
	}
}

// --- BuildTwinContext tests ---

func TestBuildTwinContextNil(t *testing.T) {
	store := newTestStore(t)

	task := &Task{ID: "T-001", Title: "Test", Domain: "web"}
	ctx := store.BuildTwinContext(task)

	// With empty DB, should return nil (no patterns, no soul)
	if ctx != nil && len(ctx.Patterns) > 0 {
		t.Errorf("expected nil or empty context on empty db, got %+v", ctx)
	}
}

// --- BuildTwinReview tests ---

func TestBuildTwinReviewWithCheckpoints(t *testing.T) {
	store := newTestStore(t)

	// Insert checkpoints
	for i := 0; i < 5; i++ {
		decision := "APPROVE"
		if i >= 3 {
			decision = "REQUEST_CHANGES"
		}
		ts := time.Now().Format(time.RFC3339)
		_, _ = store.db.Exec(
			"INSERT INTO c4_checkpoints (checkpoint_id, decision, notes, required_changes, created_at) VALUES (?, ?, ?, ?, ?)",
			taskIDForTest(i), decision, "notes", "[]", ts)
	}

	review := store.BuildTwinReview()
	if review == nil {
		t.Fatal("expected non-nil review")
	}
	if review.HistoricalPattern == "" {
		t.Error("expected historical pattern to be set")
	}
}

// --- c4_reflect MCP tool tests ---

func TestReflectTool(t *testing.T) {
	store := newTestStore(t)
	reg := mcp.NewRegistry()
	RegisterTwinHandlers(reg, store)

	// Insert some data
	ts := time.Now().Format(time.RFC3339)
	_, _ = store.db.Exec("INSERT INTO persona_stats (persona_id, task_id, outcome, created_at) VALUES (?, ?, ?, ?)",
		"direct", "T-001", "approved", ts)
	_, _ = store.db.Exec("INSERT INTO c4_tasks (task_id, title, status) VALUES (?, ?, ?)",
		"T-001", "Test task", "done")

	result, err := reg.Call("c4_reflect", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("result type = %T, want map[string]any", result)
	}

	// Check identity
	identity, ok := m["identity"].(map[string]any)
	if !ok {
		t.Fatal("missing identity in reflect result")
	}
	if identity["total_tasks"] != 1 {
		t.Errorf("total_tasks = %v, want 1", identity["total_tasks"])
	}

	// Check patterns exists (even if empty)
	if _, ok := m["patterns"]; !ok {
		t.Error("missing patterns in reflect result")
	}

	// Check growth exists
	if _, ok := m["growth"]; !ok {
		t.Error("missing growth in reflect result")
	}

	// Check challenges exists
	if _, ok := m["challenges"]; !ok {
		t.Error("missing challenges in reflect result")
	}
}

func TestReflectToolFocusPatterns(t *testing.T) {
	store := newTestStore(t)
	reg := mcp.NewRegistry()
	RegisterTwinHandlers(reg, store)

	result, err := reg.Call("c4_reflect", json.RawMessage(`{"focus":"patterns"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	if _, ok := m["patterns"]; !ok {
		t.Error("missing patterns when focus=patterns")
	}
	if _, ok := m["growth"]; ok {
		t.Error("growth should not be present when focus=patterns")
	}
}

func TestReflectToolFocusGrowth(t *testing.T) {
	store := newTestStore(t)
	reg := mcp.NewRegistry()
	RegisterTwinHandlers(reg, store)

	result, err := reg.Call("c4_reflect", json.RawMessage(`{"focus":"growth"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	if _, ok := m["growth"]; !ok {
		t.Error("missing growth when focus=growth")
	}
	if _, ok := m["patterns"]; ok {
		t.Error("patterns should not be present when focus=growth")
	}
}

// --- Milestones tests ---

func TestDetectMilestones(t *testing.T) {
	store := newTestStore(t)

	// Insert 15 approved tasks
	ts := time.Now().Format(time.RFC3339)
	for i := 0; i < 15; i++ {
		taskID := taskIDForTest(i)
		_, _ = store.db.Exec("INSERT INTO persona_stats (persona_id, task_id, outcome, created_at) VALUES (?, ?, ?, ?)",
			"direct", taskID, "approved", ts)
		_, _ = store.db.Exec("INSERT INTO c4_tasks (task_id, title, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?)",
			taskID, "Task", "done", ts, ts)
	}

	milestones := store.detectMilestones()
	if len(milestones) == 0 {
		t.Error("expected at least one milestone")
	}
}

// --- Project role tests ---

func TestSetProjectRoleForStage(t *testing.T) {
	// Save and restore
	old := projectRoleForStage
	defer func() { projectRoleForStage = old }()

	SetProjectRoleForStage("project-c4")

	roles := GetActiveRolesForStage("EXECUTE")
	found := false
	for _, r := range roles {
		if r == "project-c4" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'project-c4' in roles, got %v", roles)
	}

	// Should still have developer
	found = false
	for _, r := range roles {
		if r == "developer" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'developer' in roles, got %v", roles)
	}
}

// --- Tool count test ---

func TestRegisterAllToolCountWithTwin(t *testing.T) {
	store := newMockStore()
	reg := mcp.NewRegistry()
	RegisterAll(reg, store)

	tools := reg.ListTools()
	if len(tools) != 14 {
		names := make([]string, 0, len(tools))
		for _, tool := range tools {
			names = append(names, tool.Name)
		}
		t.Errorf("registered %d tools, want 14: %v", len(tools), names)
	}
}

// --- c4_pop_reflect tests ---

func newTestKnowledgeStore(t *testing.T) *knowledge.Store {
	t.Helper()
	dir := t.TempDir()
	ks, err := knowledge.NewStore(filepath.Join(dir, "knowledge"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { ks.Close() })
	return ks
}

func TestPopReflectNilStore(t *testing.T) {
	reg := mcp.NewRegistry()
	RegisterPopReflectHandlers(reg, nil)

	result, err := reg.Call("c4_pop_reflect", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]any)
	pending := m["pending"].([]map[string]any)
	if len(pending) != 0 {
		t.Errorf("expected empty pending list, got %d", len(pending))
	}
	if m["total_pending"] != 0 {
		t.Errorf("total_pending = %v, want 0", m["total_pending"])
	}
}

func TestPopReflectEmptyStore(t *testing.T) {
	ks := newTestKnowledgeStore(t)
	reg := mcp.NewRegistry()
	RegisterPopReflectHandlers(reg, ks)

	result, err := reg.Call("c4_pop_reflect", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]any)
	if m["total_pending"] != 0 {
		t.Errorf("total_pending = %v, want 0", m["total_pending"])
	}
	if m["hint"] == "" {
		t.Error("expected hint in result")
	}
}

func TestPopReflectWithPendingItem(t *testing.T) {
	ks := newTestKnowledgeStore(t)

	// Create a pending document with HIGH confidence
	_, err := ks.Create(knowledge.TypeInsight, map[string]any{
		"title":      "선호: 작은 PR 지향",
		"confidence": 0.9,
		"status":     "pending",
	}, "사용자는 PR 크기를 500줄 미만으로 유지하는 것을 선호한다")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	reg := mcp.NewRegistry()
	RegisterPopReflectHandlers(reg, ks)

	result, err := reg.Call("c4_pop_reflect", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]any)
	if m["total_pending"] != 1 {
		t.Errorf("total_pending = %v, want 1", m["total_pending"])
	}
	pending := m["pending"].([]map[string]any)
	if len(pending) != 1 {
		t.Fatalf("pending len = %d, want 1", len(pending))
	}
	item := pending[0]
	if item["confidence"] != "HIGH" {
		t.Errorf("confidence = %v, want HIGH", item["confidence"])
	}
	if item["title"] != "선호: 작은 PR 지향" {
		t.Errorf("title = %v, want '선호: 작은 PR 지향'", item["title"])
	}
}

func TestPopReflectConfidenceFilter(t *testing.T) {
	ks := newTestKnowledgeStore(t)

	// HIGH confidence pending
	_, _ = ks.Create(knowledge.TypeInsight, map[string]any{
		"title":      "HIGH item",
		"confidence": 0.9,
		"status":     "pending",
	}, "high body")
	// MEDIUM confidence pending
	_, _ = ks.Create(knowledge.TypeInsight, map[string]any{
		"title":      "MEDIUM item",
		"confidence": 0.6,
		"status":     "pending",
	}, "medium body")
	// Not pending
	_, _ = ks.Create(knowledge.TypeInsight, map[string]any{
		"title":      "Active item",
		"confidence": 0.9,
		"status":     "active",
	}, "active body")

	reg := mcp.NewRegistry()
	RegisterPopReflectHandlers(reg, ks)

	// HIGH filter: only HIGH item
	result, err := reg.Call("c4_pop_reflect", json.RawMessage(`{"confidence":"HIGH"}`))
	if err != nil {
		t.Fatalf("HIGH filter error: %v", err)
	}
	m := result.(map[string]any)
	if m["total_pending"] != 1 {
		t.Errorf("HIGH filter: total_pending = %v, want 1", m["total_pending"])
	}

	// ALL filter: both pending items
	result2, err := reg.Call("c4_pop_reflect", json.RawMessage(`{"confidence":"ALL"}`))
	if err != nil {
		t.Fatalf("ALL filter error: %v", err)
	}
	m2 := result2.(map[string]any)
	if m2["total_pending"] != 2 {
		t.Errorf("ALL filter: total_pending = %v, want 2", m2["total_pending"])
	}
}

// --- LLM Usage Pattern tests ---

// createTraceStepsTable creates the trace_steps table in the given db (for testing only).
func createTraceStepsTable(t *testing.T, db *sql.DB) {
	t.Helper()
	_, err := db.Exec(`
CREATE TABLE IF NOT EXISTS trace_steps (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    trace_id   TEXT    NOT NULL DEFAULT '',
    step_type  TEXT    NOT NULL,
    ts         TEXT    NOT NULL,
    model      TEXT    NOT NULL DEFAULT ''
)`)
	if err != nil {
		t.Fatalf("create trace_steps: %v", err)
	}
}

func TestDetectLLMUsagePattern_NoTable(t *testing.T) {
	store := newTestStore(t)
	// trace_steps table does not exist — should return empty, no panic
	patterns := store.detectLLMUsagePattern()
	if len(patterns) != 0 {
		t.Errorf("expected 0 patterns without trace_steps table, got %d", len(patterns))
	}
}

func TestDetectLLMUsagePattern_OpusOveruse(t *testing.T) {
	store := newTestStore(t)
	createTraceStepsTable(t, store.db)

	// Insert 7 opus + 3 sonnet calls (opus ratio 70%)
	now := time.Now()
	for i := 0; i < 7; i++ {
		ts := now.Add(-time.Duration(i) * time.Hour).Format(time.RFC3339)
		_, _ = store.db.Exec(
			`INSERT INTO trace_steps (trace_id, step_type, ts, model) VALUES ('t1', 'llm', ?, 'claude-opus-3')`,
			ts)
	}
	for i := 0; i < 3; i++ {
		ts := now.Add(-time.Duration(i) * time.Hour).Format(time.RFC3339)
		_, _ = store.db.Exec(
			`INSERT INTO trace_steps (trace_id, step_type, ts, model) VALUES ('t1', 'llm', ?, 'claude-sonnet-4')`,
			ts)
	}

	patterns := store.detectLLMUsagePattern()
	found := false
	for _, p := range patterns {
		if p.Type == "behavioral" && p.Severity == "warning" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected behavioral/warning pattern for opus overuse, got %+v", patterns)
	}
}

func TestDetectLLMUsagePattern_OpusUnderThreshold(t *testing.T) {
	store := newTestStore(t)
	createTraceStepsTable(t, store.db)

	// Insert 3 opus + 7 sonnet (opus ratio 30% — below threshold)
	now := time.Now()
	for i := 0; i < 3; i++ {
		ts := now.Add(-time.Duration(i) * time.Hour).Format(time.RFC3339)
		_, _ = store.db.Exec(
			`INSERT INTO trace_steps (trace_id, step_type, ts, model) VALUES ('t1', 'llm', ?, 'claude-opus-3')`,
			ts)
	}
	for i := 0; i < 7; i++ {
		ts := now.Add(-time.Duration(i) * time.Hour).Format(time.RFC3339)
		_, _ = store.db.Exec(
			`INSERT INTO trace_steps (trace_id, step_type, ts, model) VALUES ('t1', 'llm', ?, 'claude-sonnet-4')`,
			ts)
	}

	patterns := store.detectLLMUsagePattern()
	for _, p := range patterns {
		if p.Type == "behavioral" && p.Severity == "warning" {
			t.Errorf("unexpected behavioral/warning pattern below opus threshold: %+v", p)
		}
	}
}

func TestDetectLLMUsagePattern_GrowthIncrease(t *testing.T) {
	store := newTestStore(t)
	createTraceStepsTable(t, store.db)

	now := time.Now()
	// Recent 7 days: 10 calls
	for i := 0; i < 10; i++ {
		ts := now.Add(-time.Duration(i) * time.Hour).Format(time.RFC3339)
		_, _ = store.db.Exec(
			`INSERT INTO trace_steps (trace_id, step_type, ts, model) VALUES ('t1', 'llm', ?, 'claude-sonnet-4')`,
			ts)
	}
	// Previous 7 days: 5 calls (100% increase → above 20% threshold)
	for i := 0; i < 5; i++ {
		ts := now.Add(-time.Duration(8+i) * 24 * time.Hour).Format(time.RFC3339)
		_, _ = store.db.Exec(
			`INSERT INTO trace_steps (trace_id, step_type, ts, model) VALUES ('t1', 'llm', ?, 'claude-sonnet-4')`,
			ts)
	}

	patterns := store.detectLLMUsagePattern()
	found := false
	for _, p := range patterns {
		if p.Type == "growth" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected growth pattern for LLM usage increase, got %+v", patterns)
	}
}

// --- Helpers ---

func taskIDForTest(i int) string {
	return "T-TEST-" + padInt(i)
}

func padInt(i int) string {
	if i < 10 {
		return "00" + intToStr(i)
	}
	if i < 100 {
		return "0" + intToStr(i)
	}
	return intToStr(i)
}

func intToStr(i int) string {
	s := ""
	if i == 0 {
		return "0"
	}
	for i > 0 {
		s = string(rune('0'+i%10)) + s
		i /= 10
	}
	return s
}
