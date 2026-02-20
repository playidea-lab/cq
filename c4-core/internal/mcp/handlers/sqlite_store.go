package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/changmin/c4-core/internal/config"
	"github.com/changmin/c4-core/internal/eventbus"
	"github.com/changmin/c4-core/internal/knowledge"
	"github.com/changmin/c4-core/internal/mcp"
	"github.com/changmin/c4-core/internal/state"
	"github.com/changmin/c4-core/internal/task"
)

// SQLiteStore implements the handlers.Store interface backed by SQLite.
// It operates on the shared .c4/tasks.db used by both Go and Python.
type SQLiteStore struct {
	db             *sql.DB
	projectID      string
	projectRoot    string
	config         *config.Manager
	proxy          *BridgeProxy            // optional: for legacy proxy calls
	knowledgeStore *knowledge.Store        // optional: for native knowledge recording
	knowledgeSearch *knowledge.Searcher    // optional: for knowledge context injection
	eventPub       eventbus.Publisher      // optional: for C3 EventBus remote publishing
	dispatcher     *eventbus.Dispatcher    // optional: local rule-based dispatch (C1 posting, etc.)
	registry       *mcp.Registry           // optional: for lighthouse auto-promote registry cleanup

	// Implicit heartbeat (Option C): tracks the active worker for this MCP process.
	// Set by AssignTask; refreshed before every tool dispatch via Registry.OnCall.
	workerMu        sync.RWMutex
	currentWorkerID string
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

// WithProxy sets the bridge proxy for legacy proxy calls.
func WithProxy(p *BridgeProxy) StoreOption {
	return func(s *SQLiteStore) { s.proxy = p }
}

// WithKnowledge sets the native knowledge store and searcher for auto-recording and context injection.
func WithKnowledge(store *knowledge.Store, searcher *knowledge.Searcher) StoreOption {
	return func(s *SQLiteStore) {
		s.knowledgeStore = store
		s.knowledgeSearch = searcher
	}
}

// WithRegistry sets the MCP registry for lighthouse auto-promote on T-LH task completion.
func WithRegistry(reg *mcp.Registry) StoreOption {
	return func(s *SQLiteStore) { s.registry = reg }
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

// GetProjectID returns the project ID for event publishing.
func (s *SQLiteStore) GetProjectID() string {
	return s.projectID
}

// notifyEventBus publishes a task event to the remote C3 EventBus and
// dispatches locally via the Dispatcher (for c1_post rules, etc.).
// NOTE: This does NOT cause double dispatch:
//   - Remote EventBus: PublishAsync → remote server does StoreEvent + Dispatch
//   - Local Dispatcher: Dispatch only (no store) for immediate local rules (c1_post, log)
func (s *SQLiteStore) notifyEventBus(evType string, data map[string]any) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return
	}

	// 1. Remote C3 EventBus (fire-and-forget)
	if s.eventPub != nil {
		s.eventPub.PublishAsync(evType, "c4.core", jsonData, s.projectID)
	}

	// 2. Local dispatch (c1_post, log, etc.) - dispatch only, no store
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
			execution_mode TEXT DEFAULT 'worker',
			worker_id    TEXT DEFAULT '',
			branch       TEXT DEFAULT '',
			commit_sha   TEXT DEFAULT '',
			review_decision_evidence TEXT DEFAULT '',
			failure_signature TEXT DEFAULT '',
			blocked_attempts INTEGER DEFAULT 0,
			last_error TEXT DEFAULT '',
			created_at   TEXT DEFAULT CURRENT_TIMESTAMP,
			updated_at   TEXT DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS c4_checkpoints (
			checkpoint_id    TEXT PRIMARY KEY,
			decision         TEXT NOT NULL,
			notes            TEXT DEFAULT '',
			required_changes TEXT DEFAULT '[]',
			target_task_id   TEXT DEFAULT '',
			target_review_id TEXT DEFAULT '',
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
		"ALTER TABLE c4_tasks ADD COLUMN execution_mode TEXT DEFAULT 'worker'",
		"ALTER TABLE c4_tasks ADD COLUMN review_decision_evidence TEXT DEFAULT ''",
		"ALTER TABLE c4_tasks ADD COLUMN failure_signature TEXT DEFAULT ''",
		"ALTER TABLE c4_tasks ADD COLUMN blocked_attempts INTEGER DEFAULT 0",
		"ALTER TABLE c4_tasks ADD COLUMN last_error TEXT DEFAULT ''",
		"ALTER TABLE c4_lighthouses ADD COLUMN task_id TEXT DEFAULT ''",
		"ALTER TABLE c4_checkpoints ADD COLUMN target_task_id TEXT DEFAULT ''",
		"ALTER TABLE c4_checkpoints ADD COLUMN target_review_id TEXT DEFAULT ''",
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
		review_decision_evidence TEXT DEFAULT '',
		failure_signature TEXT DEFAULT '',
		blocked_attempts INTEGER DEFAULT 0,
		last_error TEXT DEFAULT '',
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
	executionMode := normalizeExecutionMode(task.ExecutionMode)

	// Apply config-based model hint if not explicitly set
	model := task.Model
	if model == "" && s.config != nil {
		model = s.config.GetModelForTask(task.ID)
	}

	_, err := s.db.Exec(`
		INSERT INTO c4_tasks (task_id, title, scope, dod, status, dependencies, domain, priority, model, execution_mode, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		task.ID, task.Title, task.Scope, task.DoD, "pending",
		deps, task.Domain, task.Priority, model, executionMode,
	)
	if err == nil {
		// C3 EventBus: publish task.created event (domain vs execution_mode separated; mode kept for backward compat)
		s.notifyEventBus("task.created", map[string]any{
			"task_id":         task.ID,
			"title":           task.Title,
			"domain":          task.Domain,
			"execution_mode":  executionMode,
			"mode":            task.Domain, // deprecated: use "domain"; kept for backward compatibility
		})
	}
	return err
}

// DeleteTask removes a task by ID (for rollback when review task creation fails).
func (s *SQLiteStore) DeleteTask(taskID string) error {
	_, err := s.db.Exec("DELETE FROM c4_tasks WHERE task_id = ?", taskID)
	return err
}

func normalizeExecutionMode(mode string) string {
	normalized, err := resolveExecutionMode(mode)
	if err != nil {
		return "worker"
	}
	return normalized
}

func isWorkerExecutionAllowed(mode string) bool {
	normalized := normalizeExecutionMode(mode)
	return normalized == "worker" || normalized == "auto"
}

func isDirectExecutionAllowed(mode string) bool {
	normalized := normalizeExecutionMode(mode)
	return normalized == "direct" || normalized == "auto"
}

// GetTask retrieves a task by ID.
func (s *SQLiteStore) GetTask(taskID string) (*Task, error) {
	var t Task
	var deps sql.NullString
	var createdAt, updatedAt sql.NullString

	var reviewEvidence sql.NullString
	var failureSig, lastErr sql.NullString
	var blockedAttempts sql.NullInt64
	err := s.db.QueryRow(`
		SELECT task_id, title, scope, dod, status, dependencies, domain, priority, model, execution_mode, worker_id, branch, commit_sha, review_decision_evidence, failure_signature, blocked_attempts, last_error, created_at, updated_at
		FROM c4_tasks WHERE task_id = ?`, taskID,
	).Scan(&t.ID, &t.Title, &t.Scope, &t.DoD, &t.Status, &deps,
		&t.Domain, &t.Priority, &t.Model, &t.ExecutionMode, &t.WorkerID, &t.Branch, &t.CommitSHA,
		&reviewEvidence, &failureSig, &blockedAttempts, &lastErr, &createdAt, &updatedAt)

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
	if reviewEvidence.Valid {
		t.ReviewDecisionEvidence = reviewEvidence.String
	}
	if failureSig.Valid {
		t.FailureSignature = failureSig.String
	}
	if blockedAttempts.Valid {
		t.Attempts = int(blockedAttempts.Int64)
	}
	if lastErr.Valid {
		t.LastError = lastErr.String
	}
	if createdAt.Valid {
		t.CreatedAt = createdAt.String
	}
	if updatedAt.Valid {
		t.UpdatedAt = updatedAt.String
	}
	t.ExecutionMode = normalizeExecutionMode(t.ExecutionMode)

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

	// 6. Enrich with knowledge context (past patterns and insights)
	s.enrichWithKnowledge(assignment)

	// C3 EventBus: publish task.started event
	s.notifyEventBus("task.started", map[string]any{
		"task_id":   taskID,
		"title":     title,
		"worker_id": workerID,
	})

	// Track active worker for implicit heartbeat (Option C).
	s.workerMu.Lock()
	s.currentWorkerID = workerID
	s.workerMu.Unlock()

	return assignment, nil
}

// reassignStaleOrFindPendingTask queries for the next task to assign.
// Returns: taskID, title, scope, dod, deps, domain, model, staleReassign, error
//
// Concurrency: uses BEGIN IMMEDIATE to acquire a write-reservation lock before
// any reads. This prevents SQLITE_BUSY_SNAPSHOT (517) that arises when two
// processes both BEGIN DEFERRED, read the same snapshot, and then race to UPDATE
// the same row — the second writer's UPDATE fails with error 517, which
// busy_timeout cannot resolve.
//
// With BEGIN IMMEDIATE the loser blocks at BEGIN (not at UPDATE), and
// busy_timeout(5000) causes it to wait up to 5 s for the winner to commit.
// After the winner commits the loser runs its SELECT and finds the task already
// in_progress, so it returns cleanly with no double-assignment.
func (s *SQLiteStore) reassignStaleOrFindPendingTask(workerID string) (
	taskID, title, scope, dod string,
	deps sql.NullString,
	domain, model string,
	staleReassign bool,
	err error,
) {
	ctx := context.Background()
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return "", "", "", "", deps, "", "", false, err
	}
	defer conn.Close()

	// BEGIN IMMEDIATE acquires a write-reservation lock immediately.
	// busy_timeout(5000) applies here, so concurrent processes serialise cleanly.
	if _, err = conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return "", "", "", "", deps, "", "", false, fmt.Errorf("begin immediate: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			conn.ExecContext(ctx, "ROLLBACK") //nolint:errcheck
		}
	}()

	var priority int
	var isStale bool

	// Anti-fragility: try to reassign stale in_progress tasks first (>10 min without update)
	err = conn.QueryRowContext(ctx, `
		SELECT t.task_id, t.title, t.scope, t.dod, t.dependencies, t.domain, t.priority, t.model
		FROM c4_tasks t
		WHERE t.status = 'in_progress'
		AND (t.execution_mode IS NULL OR t.execution_mode IN ('', 'worker', 'auto'))
		AND (julianday('now') - julianday(t.updated_at)) * 24 * 60 > 30
		AND t.worker_id != ?
		ORDER BY t.priority DESC, t.created_at ASC
		LIMIT 1`, workerID,
	).Scan(&taskID, &title, &scope, &dod, &deps, &domain, &priority, &model)

	if err == sql.ErrNoRows {
		// Normal path: find next pending task with resolved dependencies
		err = conn.QueryRowContext(ctx, `
			SELECT t.task_id, t.title, t.scope, t.dod, t.dependencies, t.domain, t.priority, t.model
			FROM c4_tasks t
			WHERE t.status = 'pending'
			AND (t.execution_mode IS NULL OR t.execution_mode IN ('', 'worker', 'auto'))
			AND NOT EXISTS (
				SELECT 1 FROM json_each(CASE WHEN t.dependencies IS NULL OR t.dependencies = '' THEN '[]' ELSE t.dependencies END) AS dep
				JOIN c4_tasks dt ON dt.task_id = dep.value
				WHERE dt.status != 'done'
			)
			ORDER BY t.priority DESC, t.created_at ASC
			LIMIT 1`,
		).Scan(&taskID, &title, &scope, &dod, &deps, &domain, &priority, &model)
		isStale = false
	} else if err == nil {
		isStale = true
	}

	if err == sql.ErrNoRows {
		conn.ExecContext(ctx, "ROLLBACK") //nolint:errcheck
		committed = true
		return "", "", "", "", deps, "", "", false, nil // No tasks available
	}
	if err != nil {
		return "", "", "", "", deps, "", "", false, fmt.Errorf("finding task: %w", err)
	}

	originalStatus := "pending"
	if isStale {
		originalStatus = "in_progress"
	}
	result, err := conn.ExecContext(ctx, `
		UPDATE c4_tasks SET status = 'in_progress', worker_id = ?, updated_at = CURRENT_TIMESTAMP
		WHERE task_id = ? AND status = ?`, workerID, taskID, originalStatus,
	)
	if err != nil {
		return "", "", "", "", deps, "", "", false, fmt.Errorf("assigning task: %w", err)
	}
	if n, _ := result.RowsAffected(); n == 0 {
		// Extra safety: task was taken between BEGIN and UPDATE (should not happen with IMMEDIATE).
		conn.ExecContext(ctx, "ROLLBACK") //nolint:errcheck
		committed = true
		return "", "", "", "", deps, "", "", false, nil
	}

	if _, err = conn.ExecContext(ctx, "COMMIT"); err != nil {
		return "", "", "", "", deps, "", "", false, err
	}
	committed = true

	s.notifyEventBus("task.updated", map[string]any{
		"task_id":         taskID,
		"status":          "in_progress",
		"previous_status": originalStatus,
		"worker_id":       workerID,
	})
	return taskID, title, scope, dod, deps, domain, model, isStale, nil
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
	var commitSHA, legacyFilesChanged, handoff string
	if err := s.db.QueryRow("SELECT commit_sha, branch, handoff FROM c4_tasks WHERE task_id=?", parentID).Scan(&commitSHA, &legacyFilesChanged, &handoff); err != nil {
		fmt.Fprintf(os.Stderr, "c4: assign-task: review context for %s: %v\n", parentID, err)
	}
	filesChanged := extractFilesChangedFromHandoff(handoff)
	if filesChanged == "" {
		// Backward compatibility for legacy rows that stored files in branch.
		filesChanged = legacyFilesChanged
	}
	if commitSHA != "" || filesChanged != "" {
		assignment.ReviewContext = &ReviewContext{
			ParentTaskID: parentID,
			CommitSHA:    commitSHA,
			FilesChanged: filesChanged,
		}
	}
}

// enrichWithKnowledge injects relevant knowledge context (past patterns, insights, experiments).
func (s *SQLiteStore) enrichWithKnowledge(assignment *TaskAssignment) {
	if s.knowledgeSearch == nil {
		return
	}

	query := assignment.Title
	if assignment.Domain != "" {
		query = assignment.Domain + " " + query
	}

	results, err := s.knowledgeSearch.Search(query, 3, nil)
	if err != nil || len(results) == 0 {
		return
	}

	var b strings.Builder
	b.WriteString("## Relevant Knowledge (auto-injected)\n\n")
	for i, r := range results {
		fmt.Fprintf(&b, "### %d. [%s] %s\n", i+1, r.Type, r.Title)
		if r.Domain != "" {
			fmt.Fprintf(&b, "- Domain: %s\n", r.Domain)
		}
		// Fetch body summary (first 200 chars) for actionable context
		if s.knowledgeStore != nil {
			if doc, err := s.knowledgeStore.Get(r.ID); err == nil && doc.Body != "" {
				body := doc.Body
				if len(body) > 200 {
					body = body[:200] + "..."
				}
				fmt.Fprintf(&b, "- Summary: %s\n", body)
			}
		}
		b.WriteString("\n")
	}
	assignment.KnowledgeContext = b.String()
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
	if !isWorkerExecutionAllowed(task.ExecutionMode) {
		return &SubmitResult{
			Success:    false,
			NextAction: "get_next_task",
			Message:    fmt.Sprintf("Task %s execution_mode is %q (worker submit allowed: worker/auto)", taskID, task.ExecutionMode),
		}, nil
	}
	if task.WorkerID == "direct" {
		return &SubmitResult{
			Success:    false,
			NextAction: "get_next_task",
			Message:    fmt.Sprintf("Task %s is claimed by direct mode — use c4_report", taskID),
		}, nil
	}
	if task.WorkerID != "" && task.WorkerID != workerID {
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

	// Auto-record knowledge (best-effort) — pass handoff for structured extraction
	s.autoRecordKnowledge(task, "submitted via worker", nil, handoff)

	// Auto-learn: update soul from persona patterns (best-effort)
	s.autoLearn(workerID)

	// Record growth snapshot (best-effort)
	s.recordGrowthOnCompletion()

	// Trace logging
	s.logTrace("task_submitted", workerID, taskID, commitSHA)

	// Auto-promote lighthouse if T-LH- task (best-effort)
	s.autoPromoteLighthouse(taskID, workerID)

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

	// Auto-cleanup worktree if enabled (best-effort)
	if s.config != nil && s.config.GetConfig().Worktree.AutoCleanup && s.projectRoot != "" {
		wtPath := filepath.Join(s.projectRoot, ".c4", "worktrees", workerID)
		if _, statErr := os.Stat(wtPath); statErr == nil {
			if _, rmErr := runGit(s.projectRoot, "worktree", "remove", "--force", wtPath); rmErr != nil {
				fmt.Fprintf(os.Stderr, "c4: warning: failed to remove worktree %s: %v\n", wtPath, rmErr)
			}
		}
	}

	// Auto-complete paired review task (best-effort cascade)
	cascadedReview := s.completeReviewTask(taskID)

	return &SubmitResult{
		Success:           true,
		NextAction:        nextAction,
		Message:           fmt.Sprintf("Task %s submitted successfully", taskID),
		PendingReview:     cascadedReview,
		ValidationSkipped: len(results) == 0,
	}, nil
}

// MarkBlocked marks a task as blocked and persists failure_signature, attempts, last_error.
func (s *SQLiteStore) MarkBlocked(taskID, workerID, failureSignature string, attempts int, lastError string) error {
	_, err := s.db.Exec(`
		UPDATE c4_tasks SET status = 'blocked', worker_id = '', failure_signature = ?, blocked_attempts = ?, last_error = ?, updated_at = CURRENT_TIMESTAMP
		WHERE task_id = ?`, failureSignature, attempts, lastError, taskID,
	)
	if err != nil {
		return err
	}

	// C3 EventBus: publish task.blocked event (dispatched to C1 via rules)
	s.notifyEventBus("task.blocked", map[string]any{
		"task_id":            taskID,
		"worker_id":          workerID,
		"failure_signature":  failureSignature,
		"attempts":           attempts,
		"last_error":         lastError,
	})
	return nil
}

// ClaimTask claims a task for direct execution and writes .c4/active_claim.json.
// Uses an atomic UPDATE WHERE status='pending' + RowsAffected check to prevent
// two concurrent Direct-mode sessions from claiming the same task.
func (s *SQLiteStore) ClaimTask(taskID string) (*Task, error) {
	// Pre-check execution_mode before taking the write lock.
	task, err := s.GetTask(taskID)
	if err != nil {
		return nil, err
	}
	if !isDirectExecutionAllowed(task.ExecutionMode) {
		return nil, fmt.Errorf("task %s execution_mode is %q (expected direct or auto)", taskID, task.ExecutionMode)
	}

	// Atomic claim: only succeeds if task is still pending.
	result, err := s.db.Exec(`
		UPDATE c4_tasks SET status = 'in_progress', worker_id = 'direct', updated_at = CURRENT_TIMESTAMP
		WHERE task_id = ? AND status = 'pending'`, taskID,
	)
	if err != nil {
		return nil, err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		// Either already in_progress/done, or another process claimed it first.
		return nil, fmt.Errorf("task %s is not pending (already claimed or completed)", taskID)
	}

	task.Status = "in_progress"
	task.WorkerID = "direct"

	s.notifyEventBus("task.updated", map[string]any{
		"task_id":         taskID,
		"status":          "in_progress",
		"previous_status": "pending",
		"worker_id":       "direct",
	})
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
	if !isDirectExecutionAllowed(task.ExecutionMode) {
		return fmt.Errorf("task %s execution_mode is %q (expected direct or auto)", taskID, task.ExecutionMode)
	}

	handoff := buildDirectReportHandoff(summary, filesChanged)

	_, err = s.db.Exec(`
		UPDATE c4_tasks SET status = 'done', handoff = ?, updated_at = CURRENT_TIMESTAMP
		WHERE task_id = ?`, handoff, taskID,
	)
	if err != nil {
		return err
	}

	// Remove .c4/active_claim.json
	s.removeClaimFile()

	// Record persona stats (best-effort)
	s.recordPersonaStat("direct", taskID, "approved")

	// Auto-record knowledge (best-effort) — pass handoff for structured extraction
	s.autoRecordKnowledge(task, summary, filesChanged, handoff)

	// Auto-learn: update soul from persona patterns (best-effort)
	s.autoLearn("direct")

	// Record growth snapshot (best-effort)
	s.recordGrowthOnCompletion()

	// Auto-complete paired review task (best-effort cascade)
	s.completeReviewTask(taskID)

	// Auto-promote lighthouse if T-LH- task (best-effort)
	s.autoPromoteLighthouse(taskID, "direct")

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

func buildDirectReportHandoff(summary string, filesChanged []string) string {
	payload := map[string]any{
		"type":    "direct_report",
		"summary": summary,
	}
	if len(filesChanged) > 0 {
		payload["files_changed"] = filesChanged
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return summary
	}
	return string(b)
}

func extractFilesChangedFromHandoff(handoff string) string {
	if strings.TrimSpace(handoff) == "" {
		return ""
	}
	var payload struct {
		Type         string   `json:"type"`
		FilesChanged []string `json:"files_changed"`
	}
	if err := json.Unmarshal([]byte(handoff), &payload); err != nil {
		return ""
	}
	if payload.Type != "" && payload.Type != "direct_report" {
		return ""
	}
	if len(payload.FilesChanged) == 0 {
		return ""
	}
	return strings.Join(payload.FilesChanged, ",")
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

// --- Lighthouse Auto-Promote ---

// autoPromoteLighthouse checks if a completed task is a T-LH- task
// and auto-promotes the linked lighthouse from stub to implemented.
// Best-effort: failures are logged but don't block task completion.
func (s *SQLiteStore) autoPromoteLighthouse(taskID, workerID string) {
	if !strings.HasPrefix(taskID, "T-LH-") {
		return
	}
	// Extract lighthouse name: T-LH-{name}-{ver}
	parts := strings.TrimPrefix(taskID, "T-LH-")
	idx := strings.LastIndex(parts, "-")
	if idx <= 0 {
		return
	}
	lhName := parts[:idx]

	lh, err := s.getLighthouse(lhName)
	if err != nil || lh == nil || lh.Status != "stub" {
		return
	}

	// Promote lighthouse in DB
	if err := s.promoteLighthouse(lhName, workerID); err != nil {
		fmt.Fprintf(os.Stderr, "c4: warning: auto-promote lighthouse '%s' failed: %v\n", lhName, err)
		return
	}

	// Remove stub handler from MCP registry (real handler registered on next restart)
	if s.registry != nil {
		s.registry.Unregister(lhName)
	}

	s.logTrace("lighthouse_auto_promote", workerID, lhName,
		fmt.Sprintf("auto-promoted via task %s completion", taskID))

	// Notify EventBus
	s.notifyEventBus("lighthouse.promoted", map[string]any{
		"lighthouse": lhName,
		"task_id":    taskID,
		"worker_id":  workerID,
	})
}

// --- Knowledge Auto-Record ---

// autoRecordKnowledge records task completion as a knowledge experiment (best-effort).
// Uses native knowledge store if available, falls back to proxy.
func (s *SQLiteStore) autoRecordKnowledge(task *Task, summary string, filesChanged []string, handoff string) {
	if task == nil {
		return
	}
	if s.knowledgeStore == nil && s.proxy == nil {
		return
	}

	// Parse handoff to extract structured data
	ho := parseHandoff(handoff)
	if ho.Summary != "" && summary == "submitted via worker" {
		summary = ho.Summary
	}
	if len(ho.FilesChanged) > 0 && len(filesChanged) == 0 {
		filesChanged = ho.FilesChanged
	}

	// Build rich content with rationale and discoveries
	var b strings.Builder
	fmt.Fprintf(&b, "## Task: %s\n\n**Summary**: %s\n\n**Status**: done\n", task.Title, summary)
	if len(filesChanged) > 0 {
		fmt.Fprintf(&b, "\n**Files changed**: %s\n", strings.Join(filesChanged, ", "))
	}
	if len(ho.Discoveries) > 0 {
		b.WriteString("\n## Discoveries\n")
		for _, d := range ho.Discoveries {
			fmt.Fprintf(&b, "- %s\n", d)
		}
	}
	if len(ho.Concerns) > 0 {
		b.WriteString("\n## Concerns\n")
		for _, c := range ho.Concerns {
			fmt.Fprintf(&b, "- %s\n", c)
		}
	}
	if ho.Rationale != "" {
		fmt.Fprintf(&b, "\n## Rationale\n%s\n", ho.Rationale)
	}
	content := b.String()

	tags := []string{}
	if task.Domain != "" {
		tags = append(tags, task.Domain)
	}
	if task.WorkerID != "" {
		tags = append(tags, task.WorkerID)
	}
	tags = append(tags, "auto-recorded")

	title := fmt.Sprintf("Task %s: %s", task.ID, task.Title)

	// Prefer native store over proxy
	if s.knowledgeStore != nil {
		go func() {
			metadata := map[string]any{
				"title":   title,
				"domain":  task.Domain,
				"tags":    tags,
				"task_id": task.ID,
			}
			if _, err := s.knowledgeStore.Create(knowledge.TypeExperiment, metadata, content); err != nil {
				fmt.Fprintf(os.Stderr, "c4: auto-record knowledge failed for %s: %v\n", task.ID, err)
			}
		}()
		return
	}

	// Fallback to proxy
	params := map[string]any{
		"doc_type": "experiment",
		"title":    title,
		"content":  content,
		"tags":     tags,
	}
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

// StaleTasks returns in_progress tasks whose updated_at is older than minMinutes.
// Used by the admin c4_stale_tasks tool to surface stuck workers.
func (s *SQLiteStore) StaleTasks(minMinutes int) ([]Task, error) {
	rows, err := s.db.Query(`
		SELECT task_id, title, worker_id, updated_at, domain
		FROM c4_tasks
		WHERE status = 'in_progress'
		  AND (julianday('now') - julianday(updated_at)) * 24 * 60 > ?
		ORDER BY updated_at ASC`,
		minMinutes)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []Task
	for rows.Next() {
		var t Task
		var workerID sql.NullString
		var domain sql.NullString
		if err := rows.Scan(&t.ID, &t.Title, &workerID, &t.UpdatedAt, &domain); err != nil {
			return nil, err
		}
		t.WorkerID = workerID.String
		t.Domain = domain.String
		t.Status = "in_progress"
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}

// ResetTask resets an in_progress task back to pending so a new worker can pick it up.
// Only works on in_progress tasks to avoid accidentally resetting completed work.
func (s *SQLiteStore) ResetTask(taskID string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := s.db.Exec(`
		UPDATE c4_tasks
		SET status = 'pending', worker_id = '', updated_at = ?
		WHERE task_id = ? AND status = 'in_progress'`,
		now, taskID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("task %s not found or not in_progress", taskID)
	}
	return nil
}

// TouchCurrentWorkerHeartbeat refreshes updated_at for the currently active worker's task.
// Intended to be called by Registry.OnCall (implicit heartbeat — Option C).
// No-op if no worker is currently active.
func (s *SQLiteStore) TouchCurrentWorkerHeartbeat() {
	s.workerMu.RLock()
	workerID := s.currentWorkerID
	s.workerMu.RUnlock()

	if workerID == "" {
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)
	_, _ = s.db.Exec(`
		UPDATE c4_tasks
		SET updated_at = ?
		WHERE worker_id = ? AND status = 'in_progress'`,
		now, workerID)
}

// handoffData holds structured data parsed from a handoff JSON string.
type handoffData struct {
	Summary      string   `json:"summary"`
	FilesChanged []string `json:"files_changed"`
	Discoveries  []string `json:"discoveries"`
	Concerns     []string `json:"concerns"`
	Rationale    string   `json:"rationale"`
}

// parseHandoff extracts structured fields from a handoff JSON string.
func parseHandoff(handoff string) handoffData {
	if strings.TrimSpace(handoff) == "" {
		return handoffData{}
	}
	var ho handoffData
	if err := json.Unmarshal([]byte(handoff), &ho); err != nil {
		return handoffData{Summary: handoff}
	}
	return ho
}
