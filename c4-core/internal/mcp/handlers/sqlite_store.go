package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/changmin/c4-core/internal/config"
	"github.com/changmin/c4-core/internal/state"
)

// SQLiteStore implements the handlers.Store interface backed by SQLite.
// It operates on the shared .c4/tasks.db used by both Go and Python.
type SQLiteStore struct {
	db          *sql.DB
	projectID   string
	projectRoot string
	config      *config.Manager
	proxy       *BridgeProxy // optional: for knowledge auto-record
}

// StoreOption configures a SQLiteStore.
type StoreOption func(*SQLiteStore)

// WithProjectRoot sets the project root directory (for .c4/active_claim.json).
func WithProjectRoot(root string) StoreOption {
	return func(s *SQLiteStore) { s.projectRoot = root }
}

// WithConfig sets the config manager for economic mode and settings.
func WithConfig(cfg *config.Manager) StoreOption {
	return func(s *SQLiteStore) { s.config = cfg }
}

// WithProxy sets the bridge proxy for knowledge auto-record on task completion.
func WithProxy(p *BridgeProxy) StoreOption {
	return func(s *SQLiteStore) { s.proxy = p }
}

// NewSQLiteStore creates a new SQLite-backed Store.
func NewSQLiteStore(db *sql.DB, opts ...StoreOption) (*SQLiteStore, error) {
	s := &SQLiteStore{db: db}

	for _, opt := range opts {
		opt(s)
	}

	// Ensure schema exists
	if err := s.initSchema(); err != nil {
		return nil, fmt.Errorf("init schema: %w", err)
	}

	// Read project ID from state (table is now guaranteed to exist)
	var stateJSON string
	err := db.QueryRow("SELECT state_json FROM c4_state LIMIT 1").Scan(&stateJSON)
	if err == nil {
		var m map[string]any
		if jsonErr := json.Unmarshal([]byte(stateJSON), &m); jsonErr == nil {
			if pid, ok := m["project_id"].(string); ok {
				s.projectID = pid
			}
		}
	}

	return s, nil
}

// initSchema creates all required tables if they don't exist.
// It also detects and migrates legacy Python c4_tasks schema.
func (s *SQLiteStore) initSchema() error {
	// Migrate legacy c4_tasks if needed (before CREATE TABLE IF NOT EXISTS)
	if err := s.migrateTasksIfNeeded(); err != nil {
		fmt.Fprintf(os.Stderr, "c4: tasks migration failed (non-fatal): %v\n", err)
	}

	statements := []string{
		`CREATE TABLE IF NOT EXISTS c4_state (
			project_id TEXT PRIMARY KEY,
			state_json TEXT NOT NULL,
			updated_at TEXT DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS c4_tasks (
			task_id      TEXT PRIMARY KEY,
			title        TEXT NOT NULL,
			scope        TEXT DEFAULT '',
			dod          TEXT DEFAULT '',
			status       TEXT DEFAULT 'pending',
			dependencies TEXT DEFAULT '[]',
			domain       TEXT DEFAULT '',
			priority     INTEGER DEFAULT 0,
			model        TEXT DEFAULT '',
			worker_id    TEXT DEFAULT '',
			branch       TEXT DEFAULT '',
			commit_sha   TEXT DEFAULT '',
			created_at   TEXT DEFAULT CURRENT_TIMESTAMP,
			updated_at   TEXT DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS c4_checkpoints (
			checkpoint_id    TEXT PRIMARY KEY,
			decision         TEXT NOT NULL,
			notes            TEXT DEFAULT '',
			required_changes TEXT DEFAULT '[]',
			created_at       TEXT DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS persona_stats (
			persona_id   TEXT NOT NULL,
			task_id      TEXT NOT NULL,
			outcome      TEXT NOT NULL,
			review_score REAL DEFAULT 0.0,
			feedback     TEXT DEFAULT '',
			created_at   TEXT NOT NULL,
			UNIQUE(persona_id, task_id)
		)`,
	}

	for _, stmt := range statements {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("executing %q: %w", stmt[:40], err)
		}
	}
	return nil
}

// migrateTasksIfNeeded detects legacy Python c4_tasks schema and migrates data.
// Legacy schema: (project_id, task_id, task_json, status, assigned_to, updated_at)
// Go schema: (task_id, title, scope, dod, status, dependencies, domain, priority, model, ...)
func (s *SQLiteStore) migrateTasksIfNeeded() error {
	// Check if c4_tasks exists at all
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='c4_tasks'").Scan(&count)
	if err != nil || count == 0 {
		return nil // Table doesn't exist yet — initSchema will create it
	}

	// Check if it's legacy schema (has task_json column = legacy)
	var hasTaskJSON bool
	rows, err := s.db.Query("PRAGMA table_info(c4_tasks)")
	if err != nil {
		return fmt.Errorf("checking table info: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, typ string
		var notnull int
		var dflt sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
			continue
		}
		if name == "task_json" {
			hasTaskJSON = true
		}
	}

	if !hasTaskJSON {
		return nil // Already Go schema — no migration needed
	}

	fmt.Fprintln(os.Stderr, "c4: detected legacy c4_tasks schema, migrating...")

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// 1. Rename legacy table
	if _, err := tx.Exec("ALTER TABLE c4_tasks RENAME TO c4_tasks_legacy"); err != nil {
		return fmt.Errorf("rename legacy table: %w", err)
	}

	// 2. Create new table with Go schema
	if _, err := tx.Exec(`CREATE TABLE c4_tasks (
		task_id      TEXT PRIMARY KEY,
		title        TEXT NOT NULL,
		scope        TEXT DEFAULT '',
		dod          TEXT DEFAULT '',
		status       TEXT DEFAULT 'pending',
		dependencies TEXT DEFAULT '[]',
		domain       TEXT DEFAULT '',
		priority     INTEGER DEFAULT 0,
		model        TEXT DEFAULT '',
		worker_id    TEXT DEFAULT '',
		branch       TEXT DEFAULT '',
		commit_sha   TEXT DEFAULT '',
		created_at   TEXT DEFAULT CURRENT_TIMESTAMP,
		updated_at   TEXT DEFAULT CURRENT_TIMESTAMP
	)`); err != nil {
		return fmt.Errorf("create new table: %w", err)
	}

	// 3. Migrate data from legacy task_json
	legacyRows, err := tx.Query("SELECT task_id, task_json, status, updated_at FROM c4_tasks_legacy")
	if err != nil {
		return fmt.Errorf("read legacy rows: %w", err)
	}
	defer legacyRows.Close()

	migrated := 0
	for legacyRows.Next() {
		var taskID, taskJSON, status string
		var updatedAt sql.NullString
		if err := legacyRows.Scan(&taskID, &taskJSON, &status, &updatedAt); err != nil {
			continue
		}

		var m map[string]any
		if err := json.Unmarshal([]byte(taskJSON), &m); err != nil {
			continue
		}

		title, _ := m["title"].(string)
		if title == "" {
			title = taskID
		}
		scope, _ := m["scope"].(string)
		dod, _ := m["dod"].(string)
		domain, _ := m["domain"].(string)
		model, _ := m["model"].(string)
		workerID, _ := m["assigned_to"].(string)
		branch, _ := m["branch"].(string)
		commitSHA, _ := m["commit_sha"].(string)

		priority := 0
		if p, ok := m["priority"].(float64); ok {
			priority = int(p)
		}

		deps := "[]"
		if d, ok := m["dependencies"]; ok {
			if dBytes, err := json.Marshal(d); err == nil {
				deps = string(dBytes)
			}
		}

		ts := time.Now().Format(time.RFC3339)
		if updatedAt.Valid && updatedAt.String != "" {
			ts = updatedAt.String
		}

		// Use status from task_json if richer, else from column
		if s, ok := m["status"].(string); ok && s != "" {
			status = s
		}

		_, err := tx.Exec(`INSERT OR IGNORE INTO c4_tasks
			(task_id, title, scope, dod, status, dependencies, domain, priority, model, worker_id, branch, commit_sha, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			taskID, title, scope, dod, status, deps, domain, priority, model, workerID, branch, commitSHA, ts, ts)
		if err != nil {
			fmt.Fprintf(os.Stderr, "c4: migrate task %s failed: %v\n", taskID, err)
			continue
		}
		migrated++
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration: %w", err)
	}

	fmt.Fprintf(os.Stderr, "c4: migrated %d tasks from legacy schema\n", migrated)
	return nil
}

// GetStatus returns the current project status with task counts.
func (s *SQLiteStore) GetStatus() (*ProjectStatus, error) {
	status := &ProjectStatus{State: "INIT", ProjectName: s.projectID}

	// Read state
	var stateJSON string
	err := s.db.QueryRow("SELECT state_json FROM c4_state LIMIT 1").Scan(&stateJSON)
	if err == nil {
		var m map[string]any
		if jsonErr := json.Unmarshal([]byte(stateJSON), &m); jsonErr == nil {
			if st, ok := m["status"].(string); ok {
				status.State = st
			}
			if pn, ok := m["project_id"].(string); ok {
				status.ProjectName = pn
			}
		}
	}

	// Count tasks by status
	rows, err := s.db.Query("SELECT status, COUNT(*) FROM c4_tasks GROUP BY status")
	if err != nil {
		return status, nil
	}
	defer rows.Close()

	for rows.Next() {
		var st string
		var count int
		if err := rows.Scan(&st, &count); err != nil {
			continue
		}
		status.TotalTasks += count
		switch st {
		case "pending":
			status.PendingTasks = count
		case "in_progress":
			status.InProgress = count
		case "done":
			status.DoneTasks = count
		case "blocked":
			status.BlockedTasks = count
		}
	}

	// Add economic mode info if config is available
	if s.config != nil {
		cfg := s.config.GetConfig()
		routing := cfg.EconomicMode.ModelRouting
		status.EconomicMode = &EconomicModeInfo{
			Enabled: cfg.EconomicMode.Enabled,
			Preset:  cfg.EconomicMode.Preset,
			ModelRouting: map[string]string{
				"implementation": routing.Implementation,
				"review":         routing.Review,
				"checkpoint":     routing.Checkpoint,
			},
		}
	}

	return status, nil
}

// Start transitions the project to EXECUTE state using the state machine.
func (s *SQLiteStore) Start() error {
	machine := state.NewMachine(s.db)

	pid := s.projectID
	if pid == "" {
		pid = "c4"
	}

	st, err := machine.LoadState(pid)
	if err != nil {
		return fmt.Errorf("loading state: %w", err)
	}

	// Map Start to appropriate event based on current state
	event := "c4_run"
	if st.Status == state.StatusINIT {
		event = "c4_init_legacy" // INIT → PLAN (legacy shortcut)
	}

	_, err = machine.Transition(event)
	if err != nil {
		return fmt.Errorf("transition failed: %w", err)
	}

	return nil
}

// TransitionState transitions the project using the state machine's transition table.
// The 'to' parameter is used to infer the appropriate event.
func (s *SQLiteStore) TransitionState(from, to string) error {
	machine := state.NewMachine(s.db)

	pid := s.projectID
	if pid == "" {
		pid = "c4"
	}

	st, err := machine.LoadState(pid)
	if err != nil {
		return fmt.Errorf("loading state: %w", err)
	}

	if string(st.Status) != from {
		return fmt.Errorf("cannot transition: current state is %s, expected %s", st.Status, from)
	}

	// Find the event that maps (from → to)
	event := inferEvent(state.ProjectStatus(from), state.ProjectStatus(to))
	if event == "" {
		allowed := state.AllowedEvents(state.ProjectStatus(from))
		return fmt.Errorf("no valid transition from %s to %s (allowed events from %s: %v)", from, to, from, allowed)
	}

	_, err = machine.Transition(event)
	if err != nil {
		return fmt.Errorf("transition failed: %w", err)
	}

	return nil
}

// inferEvent finds the event name that produces a transition from → to.
func inferEvent(from, to state.ProjectStatus) string {
	// Check all allowed events from the current state
	for _, event := range state.AllowedEvents(from) {
		if state.TransitionTarget(from, event) == to {
			return event
		}
	}
	return ""
}

// Clear resets the C4 state.
func (s *SQLiteStore) Clear(keepConfig bool) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	tables := []string{"c4_tasks", "c4_state", "c4_checkpoints", "persona_stats"}
	for _, table := range tables {
		if _, err := tx.Exec("DELETE FROM " + table); err != nil {
			return fmt.Errorf("clearing %s: %w", table, err)
		}
	}

	// Remove active claim file
	if s.projectRoot != "" {
		_ = os.Remove(filepath.Join(s.projectRoot, ".c4", "active_claim.json"))
	}

	return tx.Commit()
}

// AddTask inserts a new task.
func (s *SQLiteStore) AddTask(task *Task) error {
	deps := ""
	if len(task.Dependencies) > 0 {
		depsJSON, _ := json.Marshal(task.Dependencies)
		deps = string(depsJSON)
	}

	// Apply config-based model hint if not explicitly set
	model := task.Model
	if model == "" && s.config != nil {
		model = s.config.GetModelForTask(task.ID)
	}

	_, err := s.db.Exec(`
		INSERT INTO c4_tasks (task_id, title, scope, dod, status, dependencies, domain, priority, model, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		task.ID, task.Title, task.Scope, task.DoD, "pending",
		deps, task.Domain, task.Priority, model,
	)
	return err
}

// GetTask retrieves a task by ID.
func (s *SQLiteStore) GetTask(taskID string) (*Task, error) {
	var t Task
	var deps sql.NullString
	var createdAt, updatedAt sql.NullString

	err := s.db.QueryRow(`
		SELECT task_id, title, scope, dod, status, dependencies, domain, priority, model, worker_id, branch, commit_sha, created_at, updated_at
		FROM c4_tasks WHERE task_id = ?`, taskID,
	).Scan(&t.ID, &t.Title, &t.Scope, &t.DoD, &t.Status, &deps,
		&t.Domain, &t.Priority, &t.Model, &t.WorkerID, &t.Branch, &t.CommitSHA,
		&createdAt, &updatedAt)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("task not found: %s", taskID)
	}
	if err != nil {
		return nil, err
	}

	if deps.Valid && deps.String != "" {
		if err := json.Unmarshal([]byte(deps.String), &t.Dependencies); err != nil {
			fmt.Fprintf(os.Stderr, "c4: warning: failed to parse dependencies for task %s: %v\n", taskID, err)
		}
	}
	if createdAt.Valid {
		t.CreatedAt = createdAt.String
	}
	if updatedAt.Valid {
		t.UpdatedAt = updatedAt.String
	}

	return &t, nil
}

// AssignTask finds and assigns the next available task to a worker.
func (s *SQLiteStore) AssignTask(workerID string) (*TaskAssignment, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var taskID, title, scope, dod, domain, model string
	var deps sql.NullString
	var priority int

	err = tx.QueryRow(`
		SELECT t.task_id, t.title, t.scope, t.dod, t.dependencies, t.domain, t.priority, t.model
		FROM c4_tasks t
		WHERE t.status = 'pending'
		AND NOT EXISTS (
			SELECT 1 FROM json_each(COALESCE(t.dependencies, '[]')) AS dep
			JOIN c4_tasks dt ON dt.task_id = dep.value
			WHERE dt.status != 'done'
		)
		ORDER BY t.priority DESC, t.created_at ASC
		LIMIT 1`,
	).Scan(&taskID, &title, &scope, &dod, &deps, &domain, &priority, &model)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("finding task: %w", err)
	}

	_, err = tx.Exec(`
		UPDATE c4_tasks SET status = 'in_progress', worker_id = ?, updated_at = CURRENT_TIMESTAMP
		WHERE task_id = ?`, workerID, taskID,
	)
	if err != nil {
		return nil, fmt.Errorf("assigning task: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	// Apply config model hint if task has no explicit model
	if model == "" && s.config != nil {
		model = s.config.GetModelForTask(taskID)
	}

	assignment := &TaskAssignment{
		TaskID:   taskID,
		Title:    title,
		Scope:    scope,
		DoD:      dod,
		Domain:   domain,
		WorkerID: workerID,
		Model:    model,
	}

	if deps.Valid && deps.String != "" {
		if err := json.Unmarshal([]byte(deps.String), &assignment.Dependencies); err != nil {
			fmt.Fprintf(os.Stderr, "c4: warning: failed to parse dependencies for task %s: %v\n", taskID, err)
		}
	}

	return assignment, nil
}

// SubmitTask marks a task as done.
func (s *SQLiteStore) SubmitTask(taskID, workerID, commitSHA string, results []ValidationResult) (*SubmitResult, error) {
	for _, r := range results {
		if r.Status == "fail" {
			return &SubmitResult{
				Success:    false,
				NextAction: "get_next_task",
				Message:    fmt.Sprintf("Validation failed: %s — %s", r.Name, r.Message),
			}, nil
		}
	}

	task, err := s.GetTask(taskID)
	if err != nil {
		return nil, fmt.Errorf("getting task for submit: %w", err)
	}

	_, err = s.db.Exec(`
		UPDATE c4_tasks SET status = 'done', commit_sha = ?, updated_at = CURRENT_TIMESTAMP
		WHERE task_id = ?`, commitSHA, taskID,
	)
	if err != nil {
		return nil, fmt.Errorf("updating task: %w", err)
	}

	// Record persona stats (best-effort)
	s.recordPersonaStat(workerID, taskID, "approved")

	// Auto-record knowledge (best-effort)
	s.autoRecordKnowledge(task, "submitted via worker", nil)

	var pending int
	_ = s.db.QueryRow("SELECT COUNT(*) FROM c4_tasks WHERE status IN ('pending', 'in_progress')").Scan(&pending)

	nextAction := "get_next_task"
	if pending == 0 {
		nextAction = "complete"
	}

	return &SubmitResult{
		Success:    true,
		NextAction: nextAction,
		Message:    fmt.Sprintf("Task %s submitted successfully", taskID),
	}, nil
}

// MarkBlocked marks a task as blocked.
func (s *SQLiteStore) MarkBlocked(taskID, workerID, failureSignature string, attempts int, lastError string) error {
	_, err := s.db.Exec(`
		UPDATE c4_tasks SET status = 'blocked', worker_id = '', updated_at = CURRENT_TIMESTAMP
		WHERE task_id = ?`, taskID,
	)
	return err
}

// ClaimTask claims a task for direct execution and writes .c4/active_claim.json.
func (s *SQLiteStore) ClaimTask(taskID string) (*Task, error) {
	task, err := s.GetTask(taskID)
	if err != nil {
		return nil, err
	}

	if task.Status != "pending" {
		return nil, fmt.Errorf("task %s is %s, not pending", taskID, task.Status)
	}

	_, err = s.db.Exec(`
		UPDATE c4_tasks SET status = 'in_progress', worker_id = 'direct', updated_at = CURRENT_TIMESTAMP
		WHERE task_id = ?`, taskID,
	)
	if err != nil {
		return nil, err
	}

	task.Status = "in_progress"
	task.WorkerID = "direct"

	// Write .c4/active_claim.json for hook validation
	s.writeClaimFile(taskID)

	return task, nil
}

// ReportTask marks a directly-claimed task as done and removes .c4/active_claim.json.
func (s *SQLiteStore) ReportTask(taskID, summary string, filesChanged []string) error {
	task, err := s.GetTask(taskID)
	if err != nil {
		return fmt.Errorf("getting task for report: %w", err)
	}

	files := ""
	if len(filesChanged) > 0 {
		files = strings.Join(filesChanged, ",")
	}

	_, err = s.db.Exec(`
		UPDATE c4_tasks SET status = 'done', commit_sha = ?, branch = ?, updated_at = CURRENT_TIMESTAMP
		WHERE task_id = ?`, summary, files, taskID,
	)
	if err != nil {
		return err
	}

	// Remove .c4/active_claim.json
	s.removeClaimFile()

	// Record persona stats (best-effort)
	s.recordPersonaStat("direct", taskID, "approved")

	// Auto-record knowledge (best-effort)
	s.autoRecordKnowledge(task, summary, filesChanged)

	return nil
}

// Checkpoint records a checkpoint decision.
func (s *SQLiteStore) Checkpoint(checkpointID, decision, notes string, requiredChanges []string) (*CheckpointResult, error) {
	changesJSON := "[]"
	if len(requiredChanges) > 0 {
		b, _ := json.Marshal(requiredChanges)
		changesJSON = string(b)
	}

	_, _ = s.db.Exec(`
		INSERT OR REPLACE INTO c4_checkpoints (checkpoint_id, decision, notes, required_changes, created_at)
		VALUES (?, ?, ?, ?, ?)`,
		checkpointID, decision, notes, changesJSON, time.Now().Format(time.RFC3339),
	)

	result := &CheckpointResult{
		Success: true,
		Message: fmt.Sprintf("Checkpoint %s: %s", checkpointID, decision),
	}

	switch decision {
	case "APPROVE":
		result.NextAction = "continue"
	case "REQUEST_CHANGES":
		result.NextAction = "apply_changes"
	case "REPLAN":
		result.NextAction = "replan"
	}

	return result, nil
}

// --- Active Claim File Management ---

// writeClaimFile creates .c4/active_claim.json for hook validation.
func (s *SQLiteStore) writeClaimFile(taskID string) {
	if s.projectRoot == "" {
		return
	}
	claim := map[string]any{
		"task_id":    taskID,
		"claimed_at": time.Now().UTC().Format(time.RFC3339),
		"worker_id":  "direct",
	}
	data, err := json.MarshalIndent(claim, "", "  ")
	if err != nil {
		return
	}
	claimPath := filepath.Join(s.projectRoot, ".c4", "active_claim.json")
	_ = os.MkdirAll(filepath.Dir(claimPath), 0755)
	_ = os.WriteFile(claimPath, data, 0644)
}

// removeClaimFile deletes .c4/active_claim.json after task completion.
func (s *SQLiteStore) removeClaimFile() {
	if s.projectRoot == "" {
		return
	}
	claimPath := filepath.Join(s.projectRoot, ".c4", "active_claim.json")
	_ = os.Remove(claimPath)
}

// --- Persona Stats ---

// recordPersonaStat records a persona outcome for a task (best-effort).
func (s *SQLiteStore) recordPersonaStat(personaID, taskID, outcome string) {
	if personaID == "" {
		personaID = "direct"
	}
	_, _ = s.db.Exec(`
		INSERT OR REPLACE INTO persona_stats (persona_id, task_id, outcome, created_at)
		VALUES (?, ?, ?, ?)`,
		personaID, taskID, outcome, time.Now().UTC().Format(time.RFC3339),
	)
}

// GetPersonaStats retrieves performance stats for a persona.
func (s *SQLiteStore) GetPersonaStats(personaID string) (map[string]any, error) {
	stats := map[string]any{
		"persona_id": personaID,
	}

	// Total tasks
	var total int
	_ = s.db.QueryRow("SELECT COUNT(*) FROM persona_stats WHERE persona_id = ?", personaID).Scan(&total)
	stats["total_tasks"] = total

	if total == 0 {
		return stats, nil
	}

	// Outcome breakdown
	rows, err := s.db.Query("SELECT outcome, COUNT(*) FROM persona_stats WHERE persona_id = ? GROUP BY outcome", personaID)
	if err != nil {
		return stats, nil
	}
	defer rows.Close()

	outcomes := map[string]int{}
	for rows.Next() {
		var outcome string
		var count int
		if err := rows.Scan(&outcome, &count); err != nil {
			continue
		}
		outcomes[outcome] = count
	}
	stats["outcomes"] = outcomes

	// Average review score
	var avgScore sql.NullFloat64
	_ = s.db.QueryRow("SELECT AVG(review_score) FROM persona_stats WHERE persona_id = ? AND review_score > 0", personaID).Scan(&avgScore)
	if avgScore.Valid {
		stats["avg_review_score"] = avgScore.Float64
	}

	return stats, nil
}

// ListPersonas returns all known persona IDs with their task counts.
func (s *SQLiteStore) ListPersonas() ([]map[string]any, error) {
	rows, err := s.db.Query("SELECT persona_id, COUNT(*), AVG(review_score) FROM persona_stats GROUP BY persona_id ORDER BY COUNT(*) DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var personas []map[string]any
	for rows.Next() {
		var pid string
		var count int
		var avgScore sql.NullFloat64
		if err := rows.Scan(&pid, &count, &avgScore); err != nil {
			continue
		}
		p := map[string]any{
			"persona_id":  pid,
			"total_tasks": count,
		}
		if avgScore.Valid {
			p["avg_review_score"] = avgScore.Float64
		}
		personas = append(personas, p)
	}
	return personas, nil
}

// --- Knowledge Auto-Record ---

// autoRecordKnowledge sends an experiment knowledge record via the sidecar proxy (best-effort).
func (s *SQLiteStore) autoRecordKnowledge(task *Task, summary string, filesChanged []string) {
	if s.proxy == nil || task == nil {
		return
	}

	content := fmt.Sprintf("## Task: %s\n\n**Summary**: %s\n\n**Status**: done\n", task.Title, summary)
	if len(filesChanged) > 0 {
		content += fmt.Sprintf("\n**Files changed**: %s\n", strings.Join(filesChanged, ", "))
	}

	tags := []any{}
	if task.Domain != "" {
		tags = append(tags, task.Domain)
	}
	if task.WorkerID != "" {
		tags = append(tags, task.WorkerID)
	}
	tags = append(tags, "auto-recorded")

	params := map[string]any{
		"doc_type": "experiment",
		"title":    fmt.Sprintf("Task %s: %s", task.ID, task.Title),
		"content":  content,
		"tags":     tags,
	}

	// Best-effort: don't block on failure, with timeout to prevent goroutine leak
	go func() {
		done := make(chan struct{})
		go func() {
			defer close(done)
			if _, err := s.proxy.Call("KnowledgeRecord", params); err != nil {
				fmt.Fprintf(os.Stderr, "c4: auto-record knowledge failed for %s: %v\n", task.ID, err)
			}
		}()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			fmt.Fprintf(os.Stderr, "c4: auto-record knowledge timed out for %s\n", task.ID)
		}
	}()
}
