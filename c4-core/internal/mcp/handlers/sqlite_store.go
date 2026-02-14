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
	"github.com/changmin/c4-core/internal/eventbus"
	"github.com/changmin/c4-core/internal/state"
	"github.com/changmin/c4-core/internal/task"
)

// SQLiteStore implements the handlers.Store interface backed by SQLite.
// It operates on the shared .c4/tasks.db used by both Go and Python.
type SQLiteStore struct {
	db          *sql.DB
	projectID   string
	projectRoot string
	config      *config.Manager
	proxy       *BridgeProxy          // optional: for knowledge auto-record
	eventPub    eventbus.Publisher    // optional: for C3 EventBus remote publishing
	dispatcher  *eventbus.Dispatcher  // optional: local rule-based dispatch (C1 posting, etc.)
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

// SetEventBus sets the EventBus publisher after construction (for cases where
// the eventbus client depends on components created after the store).
func (s *SQLiteStore) SetEventBus(pub eventbus.Publisher) {
	s.eventPub = pub
}

// SetDispatcher sets the local Dispatcher for in-process rule-based dispatch
// (e.g. c1_post action to post task events to C1 channels).
func (s *SQLiteStore) SetDispatcher(d *eventbus.Dispatcher) {
	s.dispatcher = d
}

// notifyEventBus publishes a task event to the remote C3 EventBus and
// dispatches locally via the Dispatcher (for c1_post rules, etc.).
func (s *SQLiteStore) notifyEventBus(evType string, data map[string]any) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return
	}

	// 1. Remote C3 EventBus (fire-and-forget)
	if s.eventPub != nil {
		s.eventPub.PublishAsync(evType, "c4.core", jsonData, s.projectID)
	}

	// 2. Local dispatch (c1_post, log, etc.)
	if s.dispatcher != nil {
		go s.dispatcher.Dispatch("local-"+evType, evType, jsonData)
	}
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
		`CREATE TABLE IF NOT EXISTS twin_growth (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			username    TEXT NOT NULL,
			metric      TEXT NOT NULL,
			value       REAL NOT NULL,
			period      TEXT NOT NULL,
			recorded_at TEXT DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(username, metric, period)
		)`,
		`CREATE TABLE IF NOT EXISTS c4_agent_traces (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			event_type TEXT NOT NULL,
			agent_id   TEXT DEFAULT '',
			task_id    TEXT DEFAULT '',
			detail     TEXT DEFAULT '',
			created_at TEXT DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS c4_lighthouses (
			name         TEXT PRIMARY KEY,
			description  TEXT NOT NULL DEFAULT '',
			input_schema TEXT NOT NULL DEFAULT '{}',
			spec         TEXT NOT NULL DEFAULT '',
			status       TEXT NOT NULL DEFAULT 'stub',
			version      INTEGER NOT NULL DEFAULT 1,
			created_by   TEXT DEFAULT '',
			promoted_by  TEXT DEFAULT '',
			task_id      TEXT DEFAULT '',
			created_at   TEXT DEFAULT CURRENT_TIMESTAMP,
			updated_at   TEXT DEFAULT CURRENT_TIMESTAMP
		)`,
	}

	// Best-effort migrations for existing tables
	migrations := []string{
		"ALTER TABLE c4_tasks ADD COLUMN handoff TEXT DEFAULT ''",
		"ALTER TABLE c4_lighthouses ADD COLUMN task_id TEXT DEFAULT ''",
	}

	for _, stmt := range statements {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("executing %q: %w", stmt[:40], err)
		}
	}

	// Best-effort: apply migrations (ignore only "duplicate column" errors)
	for _, m := range migrations {
		if _, err := s.db.Exec(m); err != nil {
			if !strings.Contains(err.Error(), "duplicate column") {
				fmt.Fprintf(os.Stderr, "c4: warning: migration failed: %v\n", err)
			}
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

	tables := []string{"c4_tasks", "c4_state", "c4_checkpoints", "persona_stats", "twin_growth", "c4_lighthouses"}
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
	deps := "[]"
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
	if err == nil {
		// C3 EventBus: publish task.created event
		s.notifyEventBus("task.created", map[string]any{
			"task_id": task.ID,
			"title":   task.Title,
			"mode":    task.Domain,
		})
	}
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
	// 1. Find and assign task (within transaction)
	taskID, title, scope, dod, deps, domain, model, staleReassign, err := s.reassignStaleOrFindPendingTask(workerID)
	if err != nil {
		return nil, err
	}
	if taskID == "" {
		return nil, nil // No tasks available
	}

	// Log stale reassignment AFTER commit (avoids deadlock with MaxOpenConns=1)
	if staleReassign {
		s.logTrace("stale_reassign", workerID, taskID, "Reassigned stale in_progress task")
	}

	// Apply config model hint if task has no explicit model
	if model == "" && s.config != nil {
		model = s.config.GetModelForTask(taskID)
	}

	// Build base assignment
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

	// 2. Enrich with config-based branch and worktree
	s.enrichWithConfig(assignment, workerID)

	// 3. Enrich with soul context
	s.enrichWithSoulContext(assignment)

	// 4. Enrich with lighthouse spec (if T-LH- task)
	s.enrichWithLighthouse(assignment)

	// 5. Enrich with review context (if R- task)
	s.enrichWithReviewContext(assignment)

	// C3 EventBus: publish task.started event
	s.notifyEventBus("task.started", map[string]any{
		"task_id":   taskID,
		"title":     title,
		"worker_id": workerID,
	})

	return assignment, nil
}

// reassignStaleOrFindPendingTask queries for the next task to assign.
// Returns: taskID, title, scope, dod, deps, domain, model, staleReassign, error
func (s *SQLiteStore) reassignStaleOrFindPendingTask(workerID string) (
	taskID, title, scope, dod string,
	deps sql.NullString,
	domain, model string,
	staleReassign bool,
	err error,
) {
	tx, err := s.db.Begin()
	if err != nil {
		return "", "", "", "", deps, "", "", false, err
	}
	defer tx.Rollback()

	var priority int

	// Anti-fragility: try to reassign stale in_progress tasks first (>10 min without update)
	err = tx.QueryRow(`
		SELECT t.task_id, t.title, t.scope, t.dod, t.dependencies, t.domain, t.priority, t.model
		FROM c4_tasks t
		WHERE t.status = 'in_progress'
		AND (julianday('now') - julianday(t.updated_at)) * 24 * 60 > 10
		AND t.worker_id != ?
		ORDER BY t.priority DESC, t.created_at ASC
		LIMIT 1`, workerID,
	).Scan(&taskID, &title, &scope, &dod, &deps, &domain, &priority, &model)

	if err == sql.ErrNoRows {
		// Normal path: find next pending task with resolved dependencies
		err = tx.QueryRow(`
			SELECT t.task_id, t.title, t.scope, t.dod, t.dependencies, t.domain, t.priority, t.model
			FROM c4_tasks t
			WHERE t.status = 'pending'
			AND NOT EXISTS (
				SELECT 1 FROM json_each(CASE WHEN t.dependencies IS NULL OR t.dependencies = '' THEN '[]' ELSE t.dependencies END) AS dep
				JOIN c4_tasks dt ON dt.task_id = dep.value
				WHERE dt.status != 'done'
			)
			ORDER BY t.priority DESC, t.created_at ASC
			LIMIT 1`,
		).Scan(&taskID, &title, &scope, &dod, &deps, &domain, &priority, &model)
	}
	staleReassign = err == nil && taskID != "" // track for post-commit logging

	if err == sql.ErrNoRows {
		return "", "", "", "", deps, "", "", false, nil // No tasks available
	}
	if err != nil {
		return "", "", "", "", deps, "", "", false, fmt.Errorf("finding task: %w", err)
	}

	_, err = tx.Exec(`
		UPDATE c4_tasks SET status = 'in_progress', worker_id = ?, updated_at = CURRENT_TIMESTAMP
		WHERE task_id = ?`, workerID, taskID,
	)
	if err != nil {
		return "", "", "", "", deps, "", "", false, fmt.Errorf("assigning task: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return "", "", "", "", deps, "", "", false, err
	}

	return taskID, title, scope, dod, deps, domain, model, staleReassign, nil
}

// enrichWithConfig applies config-based branch and worktree assignment.
func (s *SQLiteStore) enrichWithConfig(assignment *TaskAssignment, workerID string) {
	if s.config == nil {
		return
	}

	cfg := s.config.GetConfig()
	if cfg.WorkBranchPrefix != "" {
		assignment.Branch = cfg.WorkBranchPrefix + assignment.TaskID
	}

	if cfg.Worktree.Enabled && s.projectRoot != "" {
		wtPath := filepath.Join(s.projectRoot, ".c4", "worktrees", workerID)
		assignment.WorktreePath = wtPath
		// Auto-create worktree (best-effort, skip if already exists)
		if _, statErr := os.Stat(wtPath); os.IsNotExist(statErr) {
			branch := assignment.Branch
			if branch == "" {
				branch = "c4-" + assignment.TaskID
			}
			if _, wtErr := runGit(s.projectRoot, "worktree", "add", wtPath, "-b", branch); wtErr != nil {
				fmt.Fprintf(os.Stderr, "c4: warning: failed to create worktree %s: %v\n", wtPath, wtErr)
			}
		}
	}
}

// enrichWithSoulContext injects soul context (best-effort).
func (s *SQLiteStore) enrichWithSoulContext(assignment *TaskAssignment) {
	if s.projectRoot != "" {
		s.injectSoulContext(assignment)
	}
}

// enrichWithLighthouse injects lighthouse spec for T-LH- tasks.
func (s *SQLiteStore) enrichWithLighthouse(assignment *TaskAssignment) {
	if !strings.HasPrefix(assignment.TaskID, "T-LH-") {
		return
	}

	// Extract lighthouse name: T-LH-{name}-{ver}
	parts := strings.TrimPrefix(assignment.TaskID, "T-LH-")
	if idx := strings.LastIndex(parts, "-"); idx > 0 {
		lhName := parts[:idx]
		lh, lhErr := s.getLighthouse(lhName)
		if lhErr == nil {
			assignment.LighthouseSpec = &LighthouseContext{
				Name:        lh.Name,
				Spec:        lh.Spec,
				InputSchema: lh.InputSchema,
				Description: lh.Description,
			}
		} else {
			fmt.Fprintf(os.Stderr, "c4: warning: task %s has T-LH- prefix but lighthouse '%s' not found\n", assignment.TaskID, lhName)
		}
	}
}

// enrichWithReviewContext injects parent T's review context for R- tasks.
func (s *SQLiteStore) enrichWithReviewContext(assignment *TaskAssignment) {
	if !strings.HasPrefix(assignment.TaskID, "R-") {
		return
	}

	_, baseID, ver, _ := task.ParseTaskID(assignment.TaskID)
	parentID := fmt.Sprintf("T-%s-%d", baseID, ver)
	var commitSHA, filesChanged string
	if err := s.db.QueryRow("SELECT commit_sha, branch FROM c4_tasks WHERE task_id=?", parentID).Scan(&commitSHA, &filesChanged); err != nil {
		fmt.Fprintf(os.Stderr, "c4: assign-task: review context for %s: %v\n", parentID, err)
	}
	if commitSHA != "" || filesChanged != "" {
		assignment.ReviewContext = &ReviewContext{
			ParentTaskID: parentID,
			CommitSHA:    commitSHA,
			FilesChanged: filesChanged,
		}
	}
}

// SubmitTask marks a task as done.
func (s *SQLiteStore) SubmitTask(taskID, workerID, commitSHA, handoff string, results []ValidationResult) (*SubmitResult, error) {
	for _, r := range results {
		if r.Status == "fail" {
			s.recordPersonaStat(workerID, taskID, "validation_failed")
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
	if task.Status != "in_progress" {
		return &SubmitResult{
			Success:    false,
			NextAction: "get_next_task",
			Message:    fmt.Sprintf("Task %s is %s (expected in_progress)", taskID, task.Status),
		}, nil
	}
	if task.WorkerID == "direct" {
		return &SubmitResult{
			Success:    false,
			NextAction: "get_next_task",
			Message:    fmt.Sprintf("Task %s is claimed by direct mode — use c4_report", taskID),
		}, nil
	}
	if workerID != "" && task.WorkerID != "" && task.WorkerID != workerID {
		return &SubmitResult{
			Success:    false,
			NextAction: "get_next_task",
			Message: fmt.Sprintf(
				"Task %s is owned by worker %s (submitter: %s)",
				taskID, task.WorkerID, workerID,
			),
		}, nil
	}

	// Store task completion + handoff
	_, err = s.db.Exec(`
		UPDATE c4_tasks SET status = 'done', commit_sha = ?, handoff = ?, updated_at = CURRENT_TIMESTAMP
		WHERE task_id = ?`, commitSHA, handoff, taskID,
	)
	if err != nil {
		return nil, fmt.Errorf("updating task: %w", err)
	}

	// Record persona stats (best-effort)
	s.recordPersonaStat(workerID, taskID, "approved")

	// Auto-record knowledge (best-effort)
	s.autoRecordKnowledge(task, "submitted via worker", nil)

	// Auto-learn: update soul from persona patterns (best-effort)
	s.autoLearn(workerID)

	// Record growth snapshot (best-effort)
	s.recordGrowthOnCompletion()

	// Trace logging
	s.logTrace("task_submitted", workerID, taskID, commitSHA)

	// C3 EventBus: publish task.completed event (dispatched to C1 via rules)
	s.notifyEventBus("task.completed", map[string]any{
		"task_id":    taskID,
		"title":      task.Title,
		"worker_id":  workerID,
		"commit_sha": commitSHA,
	})

	var pending int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM c4_tasks WHERE status IN ('pending', 'in_progress')").Scan(&pending); err != nil {
		fmt.Fprintf(os.Stderr, "c4: submit-task: pending count: %v\n", err)
	}

	nextAction := "get_next_task"
	if pending == 0 {
		nextAction = "complete"
	}

	// Auto-complete paired review task (best-effort cascade)
	cascadedReview := s.completeReviewTask(taskID)

	return &SubmitResult{
		Success:       true,
		NextAction:    nextAction,
		Message:       fmt.Sprintf("Task %s submitted successfully", taskID),
		PendingReview: cascadedReview,
	}, nil
}


// MarkBlocked marks a task as blocked.
func (s *SQLiteStore) MarkBlocked(taskID, workerID, failureSignature string, attempts int, lastError string) error {
	_, err := s.db.Exec(`
		UPDATE c4_tasks SET status = 'blocked', worker_id = '', updated_at = CURRENT_TIMESTAMP
		WHERE task_id = ?`, taskID,
	)
	if err != nil {
		return err
	}

	// C3 EventBus: publish task.blocked event (dispatched to C1 via rules)
	s.notifyEventBus("task.blocked", map[string]any{
		"task_id":   taskID,
		"worker_id": workerID,
	})

	return nil
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

	// Trace logging
	s.logTrace("task_claimed", "direct", taskID, task.Title)

	return task, nil
}

// ReportTask marks a directly-claimed task as done and removes .c4/active_claim.json.
func (s *SQLiteStore) ReportTask(taskID, summary string, filesChanged []string) error {
	task, err := s.GetTask(taskID)
	if err != nil {
		return fmt.Errorf("getting task for report: %w", err)
	}
	if task.Status != "in_progress" {
		return fmt.Errorf("task %s is %s (expected in_progress)", taskID, task.Status)
	}
	if task.WorkerID != "direct" {
		return fmt.Errorf("task %s is owned by %q (expected direct)", taskID, task.WorkerID)
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

	// Auto-learn: update soul from persona patterns (best-effort)
	s.autoLearn("direct")

	// Record growth snapshot (best-effort)
	s.recordGrowthOnCompletion()

	// Auto-complete paired review task (best-effort cascade)
	s.completeReviewTask(taskID)

	// C3 EventBus: publish task.completed event (dispatched to C1 via rules)
	s.notifyEventBus("task.completed", map[string]any{
		"task_id":       taskID,
		"title":         task.Title,
		"worker_id":     "direct",
		"summary":       summary,
		"files_changed": filesChanged,
	})

	return nil
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
// Note: Functions moved to store_soul.go and store_status.go

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

