package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/changmin/c4-core/internal/config"
	"github.com/changmin/c4-core/internal/mcp"
	"github.com/changmin/c4-core/internal/state"
	"github.com/changmin/c4-core/internal/store"
)

var _ store.Store = (*SQLiteStore)(nil)

// SQLiteStore implements the handlers.Store interface backed by SQLite.
// It operates on the shared .c4/tasks.db used by both Go and Python.
type SQLiteStore struct {
	db             *sql.DB
	projectID      string
	projectRoot    string
	config         *config.Manager
	proxy           *BridgeProxy              // optional: for legacy proxy calls
	knowledgeWriter     KnowledgeWriter          // optional: for native knowledge recording
	knowledgeReader     KnowledgeReader          // optional: for knowledge body lookup
	knowledgeSearch     KnowledgeContextSearcher // optional: for knowledge context injection
	knowledgeHitTracker *KnowledgeHitTracker     // optional: tracks search hit/miss in memory
	eventPub            EventPublisher            // optional: for C3 EventBus remote publishing
	dispatcher      EventDispatcher           // optional: local rule-based dispatch (C1 posting, etc.)
	registry        *mcp.Registry             // optional: for lighthouse auto-promote registry cleanup

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

// WithKnowledge sets the knowledge interfaces for auto-recording and context injection.
// Use AdaptKnowledge() to wrap concrete *knowledge.Store and *knowledge.Searcher.
func WithKnowledge(writer KnowledgeWriter, reader KnowledgeReader, searcher KnowledgeContextSearcher) StoreOption {
	return func(s *SQLiteStore) {
		s.knowledgeWriter = writer
		s.knowledgeReader = reader
		s.knowledgeSearch = searcher
	}
}

// WithKnowledgeHitTracker sets the knowledge hit tracker for monitoring search hit/miss rates.
func WithKnowledgeHitTracker(t *KnowledgeHitTracker) StoreOption {
	return func(s *SQLiteStore) { s.knowledgeHitTracker = t }
}

// WithRegistry sets the MCP registry for lighthouse auto-promote on T-LH task completion.
func WithRegistry(reg *mcp.Registry) StoreOption {
	return func(s *SQLiteStore) { s.registry = reg }
}

// SetEventBus sets the EventBus publisher after construction (for cases where
// the eventbus client depends on components created after the store).
// Accepts any type satisfying the EventPublisher interface (e.g. eventbus.Client).
func (s *SQLiteStore) SetEventBus(pub EventPublisher) {
	s.eventPub = pub
}

// SetDispatcher sets the local Dispatcher for in-process rule-based dispatch
// (e.g. c1_post action to post task events to C1 channels).
// Accepts any type satisfying the EventDispatcher interface (e.g. *eventbus.Dispatcher).
func (s *SQLiteStore) SetDispatcher(d EventDispatcher) {
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
	var filesChangedNull sql.NullString
	var supersededByNull sql.NullString
	err := s.db.QueryRow(`
		SELECT task_id, title, scope, dod, status, dependencies, domain, priority, model, execution_mode, worker_id, branch, commit_sha, files_changed, review_decision_evidence, failure_signature, blocked_attempts, last_error, superseded_by, created_at, updated_at
		FROM c4_tasks WHERE task_id = ?`, taskID,
	).Scan(&t.ID, &t.Title, &t.Scope, &t.DoD, &t.Status, &deps,
		&t.Domain, &t.Priority, &t.Model, &t.ExecutionMode, &t.WorkerID, &t.Branch, &t.CommitSHA,
		&filesChangedNull, &reviewEvidence, &failureSig, &blockedAttempts, &lastErr, &supersededByNull, &createdAt, &updatedAt)

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
	if filesChangedNull.Valid {
		t.FilesChanged = filesChangedNull.String
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
	if supersededByNull.Valid {
		t.SupersededBy = supersededByNull.String
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
	taskRowModel := model
	if model == "" && s.config != nil {
		model = s.config.GetModelForTask(taskID)
	}

	// Risk routing: override model for R- tasks based on scope
	if strings.HasPrefix(taskID, "R-") && s.config != nil && taskRowModel == "" {
		riskCfg := s.config.GetRiskRouting()
		if riskCfg.Enabled {
			if riskCfg.Models.High != "" && riskCfg.Models.Low != "" {
				risk := classifyTaskRisk(scope, riskCfg)
				switch risk {
				case "high":
					model = riskCfg.Models.High
				case "low":
					model = riskCfg.Models.Low
				// default: model stays as-is (GetModelForTask result)
				}
			} else {
				slog.Warn("risk_routing: Models.High or Models.Low is empty, skipping override")
			}
		}
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

// classifyTaskRisk determines risk level ("high", "low", or "default") for a
// scope path based on the configured risk routing path patterns.
//
// Matching rules (priority order per pattern):
//  1. Suffix '/' → strings.HasPrefix (directory match)
//  2. Contains '*' → filepath.Match against filepath.Base (extension match)
//  3. Otherwise → strings.Contains (substring match)
//
// Multi-scope (comma-separated): each part is classified independently;
// the highest risk wins (high > low > default).
func classifyTaskRisk(scope string, cfg config.RiskRoutingConfig) string {
	if scope == "" {
		return "default"
	}

	// Split comma-separated scopes and classify each independently.
	parts := strings.Split(scope, ",")
	hasHigh := false
	hasLow := false

	for _, part := range parts {
		scopePath := filepath.ToSlash(strings.TrimSpace(part))
		if scopePath == "" {
			continue
		}

		// Check high-risk patterns first.
		for _, pattern := range cfg.Paths.High {
			if pattern == "" {
				continue
			}
			if matchScopePattern(scopePath, pattern) {
				hasHigh = true
				break
			}
		}
		if hasHigh {
			break // high > low > default; no need to continue
		}

		// Check low-risk patterns.
		for _, pattern := range cfg.Paths.Low {
			if pattern == "" {
				continue
			}
			if matchScopePattern(scopePath, pattern) {
				hasLow = true
				break
			}
		}
	}

	if hasHigh {
		return "high"
	}
	if hasLow {
		return "low"
	}
	return "default"
}

// matchScopePattern checks if scopePath matches a single pattern.
// Three match types, evaluated in order:
//   - Directory prefix: pattern ends with "/" → strings.HasPrefix match (e.g. "internal/auth/")
//   - Glob: pattern contains "*" → filepath.Match on basename only (e.g. "*.md")
//   - Substring: fallback → strings.Contains (intentionally broad; use directory-prefix patterns
//     to avoid false positives like "auth" matching "oauth/handler.go")
func matchScopePattern(scopePath, pattern string) bool {
	if strings.HasSuffix(pattern, "/") {
		return strings.HasPrefix(scopePath, pattern)
	}
	if strings.Contains(pattern, "*") {
		matched, _ := filepath.Match(pattern, filepath.Base(scopePath))
		return matched
	}
	return strings.Contains(scopePath, pattern)
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

	// Anti-fragility: try to reassign stale in_progress tasks first (>30 min without update)
	err = conn.QueryRowContext(ctx, `
		SELECT t.task_id, t.title, t.scope, t.dod, t.dependencies, t.domain, t.priority, t.model
		FROM c4_tasks t
		WHERE t.status = 'in_progress'
		AND (t.execution_mode IS NULL OR t.execution_mode IN ('', 'worker', 'auto'))
		AND (julianday('now') - julianday(t.updated_at)) * 24 * 60 > 30
		AND t.worker_id != ?
		AND (t.superseded_by IS NULL OR t.superseded_by = '')
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
			AND (t.superseded_by IS NULL OR t.superseded_by = '')
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

// SubmitTask marks a task as done.
func (s *SQLiteStore) SubmitTask(taskID, workerID, commitSHA, handoff string, results []ValidationResult) (*SubmitResult, error) {
	for _, r := range results {
		switch r.Status {
		case "pass":
			// ok
		case "fail":
			s.recordPersonaStat(workerID, taskID, "validation_failed")
			return &SubmitResult{
				Success:    false,
				NextAction: "get_next_task",
				Message:    fmt.Sprintf("Validation failed: %s — %s", r.Name, r.Message),
			}, nil
		default:
			return nil, fmt.Errorf("validation_results[%s].status %q is invalid: must be \"pass\" or \"fail\"", r.Name, r.Status)
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
	if task.WorkerID != workerID {
		return &SubmitResult{
			Success:    false,
			NextAction: "get_next_task",
			Message: fmt.Sprintf(
				"Task %s is owned by worker %s (submitter: %s)",
				taskID, task.WorkerID, workerID,
			),
		}, nil
	}

	// Validate evidence types in handoff (if any)
	ho := parseHandoff(handoff)
	for _, e := range ho.Evidence {
		if !isValidEvidenceType(e.Type) {
			return nil, fmt.Errorf("invalid evidence type %q: allowed values are screenshot|log|test_result", e.Type)
		}
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
			} else {
				// Also delete the associated branch (git worktree remove does not delete the branch)
				cfg := s.config.GetConfig()
				if cfg.WorkBranchPrefix != "" {
					branch := cfg.WorkBranchPrefix + taskID
					if _, branchErr := runGit(s.projectRoot, "branch", "-D", branch); branchErr != nil {
						fmt.Fprintf(os.Stderr, "c4: warning: failed to delete branch %s: %v\n", branch, branchErr)
					}
				}
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
// Returns an error if the task_id does not exist (RowsAffected == 0).
func (s *SQLiteStore) MarkBlocked(taskID, workerID, failureSignature string, attempts int, lastError string) error {
	result, err := s.db.Exec(`
		UPDATE c4_tasks SET status = 'blocked', worker_id = '', failure_signature = ?, blocked_attempts = ?, last_error = ?, updated_at = CURRENT_TIMESTAMP
		WHERE task_id = ?`, failureSignature, attempts, lastError, taskID,
	)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("task %s not found", taskID)
	}

	// C3 EventBus: publish task.blocked event (dispatched to C1 via rules)
	s.notifyEventBus("task.blocked", map[string]any{
		"task_id":            taskID,
		"worker_id":          workerID,
		"failure_signature":  failureSignature,
		"attempts":           attempts,
		"last_error":         lastError,
	})

	// Auto-record failure pattern for knowledge feedback loop (best-effort)
	if failureSignature != "" {
		var t Task
		if err2 := s.db.QueryRow(
			"SELECT task_id, title, scope, domain, worker_id FROM c4_tasks WHERE task_id=?", taskID).
			Scan(&t.ID, &t.Title, &t.Scope, &t.Domain, &t.WorkerID); err2 == nil {
			s.autoRecordFailurePattern(&t, failureSignature, lastError)
		}
	}
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
	filesChangedCSV := strings.Join(filesChanged, ",")

	_, err = s.db.Exec(`
		UPDATE c4_tasks SET status = 'done', handoff = ?, files_changed = ?, updated_at = CURRENT_TIMESTAMP
		WHERE task_id = ?`, handoff, filesChangedCSV, taskID,
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

