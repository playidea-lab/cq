// Package cloud implements a CloudStore that satisfies the store.Store
// interface using Supabase PostgREST as the backend.
//
// All data operations are performed via HTTP against a Supabase REST API,
// using RLS (Row-Level Security) with project_id for tenant isolation.
// This allows C4 to operate in a multi-tenant cloud environment where
// multiple projects share the same database.
package cloud

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/changmin/c4-core/internal/store"
)

// Compile-time interface check.
var _ store.Store = (*CloudStore)(nil)

// CloudStore implements store.Store using Supabase PostgREST REST API.
type CloudStore struct {
	baseURL       string         // Supabase PostgREST URL (e.g., https://xxx.supabase.co/rest/v1)
	apiKey        string         // anon key
	tokenProvider *TokenProvider // manages JWT with auto-refresh
	projectID     string         // cloud project ID for RLS
	httpClient    *http.Client
}

// NewCloudStore creates a new Supabase-backed Store.
func NewCloudStore(baseURL, apiKey string, tp *TokenProvider, projectID string) *CloudStore {
	return &CloudStore{
		baseURL:       strings.TrimRight(baseURL, "/"),
		apiKey:        apiKey,
		tokenProvider: tp,
		projectID:     projectID,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

// cloudTaskRow maps to the c4_tasks Supabase table.
type cloudTaskRow struct {
	TaskID        string            `json:"task_id"`
	ProjectID     string            `json:"project_id,omitempty"`
	Title         string            `json:"title"`
	Scope         string            `json:"scope,omitempty"`
	DoD           string            `json:"dod,omitempty"`
	Status        string            `json:"status"`
	Dependencies  cloudDependencies `json:"dependencies,omitempty"` // JSON array string (or array payload from Supabase)
	Domain        string            `json:"domain,omitempty"`
	Priority      int               `json:"priority"`
	Model         string            `json:"model,omitempty"`
	ExecutionMode string            `json:"execution_mode,omitempty"` // worker, direct, auto
	WorkerID      string            `json:"worker_id,omitempty"`
	Branch        string            `json:"branch,omitempty"`
	CommitSHA     string            `json:"commit_sha,omitempty"`
	Handoff       string            `json:"handoff,omitempty"`
	CreatedAt     string            `json:"created_at,omitempty"`
	UpdatedAt     string            `json:"updated_at,omitempty"`
}

type cloudDependencies string

func (d *cloudDependencies) UnmarshalJSON(data []byte) error {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" || trimmed == "null" {
		*d = cloudDependencies("[]")
		return nil
	}
	if strings.HasPrefix(trimmed, "\"") {
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			return err
		}
		if strings.TrimSpace(s) == "" {
			s = "[]"
		}
		*d = cloudDependencies(s)
		return nil
	}

	var deps []string
	if err := json.Unmarshal(data, &deps); err != nil {
		return err
	}
	b, err := json.Marshal(deps)
	if err != nil {
		return err
	}
	*d = cloudDependencies(string(b))
	return nil
}

func (d cloudDependencies) MarshalJSON() ([]byte, error) {
	v := strings.TrimSpace(string(d))
	if v == "" {
		v = "[]"
	}
	return json.Marshal(v)
}

// cloudStateRow maps to the c4_state Supabase table.
type cloudStateRow struct {
	ProjectID string `json:"project_id"`
	StateJSON string `json:"state_json"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

// cloudCheckpointRow maps to the c4_checkpoints Supabase table.
type cloudCheckpointRow struct {
	CheckpointID    string `json:"checkpoint_id"`
	ProjectID       string `json:"project_id,omitempty"`
	Decision        string `json:"decision"`
	Notes           string `json:"notes,omitempty"`
	RequiredChanges string `json:"required_changes,omitempty"` // JSON array string
	CreatedAt       string `json:"created_at,omitempty"`
}

// =========================================================================
// Store interface implementation
// =========================================================================

// GetStatus returns the current project status with task counts.
func (c *CloudStore) GetStatus() (*store.ProjectStatus, error) {
	status := &store.ProjectStatus{State: "INIT", ProjectName: c.projectID}

	// Read state
	var stateRows []cloudStateRow
	if err := c.get("c4_state", "project_id=eq."+url.QueryEscape(c.projectID), &stateRows); err != nil {
		return nil, fmt.Errorf("get state: %w", err)
	}
	if len(stateRows) > 0 {
		var m map[string]any
		if err := json.Unmarshal([]byte(stateRows[0].StateJSON), &m); err == nil {
			if st, ok := m["status"].(string); ok {
				status.State = st
			}
			if pn, ok := m["project_id"].(string); ok {
				status.ProjectName = pn
			}
		}
	}

	// Count tasks by status
	var taskRows []cloudTaskRow
	if err := c.get("c4_tasks", "project_id=eq."+url.QueryEscape(c.projectID)+"&select=task_id,status", &taskRows); err != nil {
		return status, nil // return partial status on task fetch failure
	}

	for _, row := range taskRows {
		status.TotalTasks++
		switch row.Status {
		case "pending":
			status.PendingTasks++
		case "in_progress":
			status.InProgress++
		case "done":
			status.DoneTasks++
		case "blocked":
			status.BlockedTasks++
		}
	}

	return status, nil
}

// Start transitions the project to EXECUTE state.
// For the cloud store, this reads the current state and advances it.
func (c *CloudStore) Start() error {
	currentState, err := c.readCurrentState()
	if err != nil {
		return fmt.Errorf("reading state: %w", err)
	}

	// Determine target state based on current
	var targetState string
	switch currentState {
	case "INIT":
		targetState = "PLAN"
	case "PLAN":
		targetState = "EXECUTE"
	case "DISCOVERY":
		targetState = "DESIGN"
	case "DESIGN":
		targetState = "PLAN"
	case "CHECKPOINT":
		targetState = "EXECUTE"
	default:
		targetState = "EXECUTE"
	}

	return c.writeState(targetState)
}

// Clear resets the C4 project data.
func (c *CloudStore) Clear(keepConfig bool) error {
	tables := []string{"c4_tasks", "c4_checkpoints"}
	if !keepConfig {
		tables = append(tables, "c4_state")
	}

	for _, table := range tables {
		if err := c.del(table, "project_id=eq."+url.QueryEscape(c.projectID)); err != nil {
			return fmt.Errorf("clearing %s: %w", table, err)
		}
	}

	return nil
}

// TransitionState transitions the project state, verifying the current state matches.
func (c *CloudStore) TransitionState(from, to string) error {
	currentState, err := c.readCurrentState()
	if err != nil {
		return fmt.Errorf("reading state: %w", err)
	}

	if currentState != from {
		return fmt.Errorf("cannot transition: current state is %s, expected %s", currentState, from)
	}

	return c.writeState(to)
}

// AddTask inserts a new task.
func (c *CloudStore) AddTask(task *store.Task) error {
	deps := "[]"
	if len(task.Dependencies) > 0 {
		depsJSON, _ := json.Marshal(task.Dependencies)
		deps = string(depsJSON)
	}

	row := &cloudTaskRow{
		TaskID:        task.ID,
		ProjectID:     c.projectID,
		Title:         task.Title,
		Scope:         task.Scope,
		DoD:           task.DoD,
		Status:        "pending",
		Dependencies:  cloudDependencies(deps),
		Domain:        task.Domain,
		Priority:      task.Priority,
		Model:         task.Model,
		ExecutionMode: normalizeExecutionMode(task.ExecutionMode),
		CreatedAt:     time.Now().UTC().Format(time.RFC3339),
		UpdatedAt:     time.Now().UTC().Format(time.RFC3339),
	}

	return c.post("c4_tasks", row)
}

// GetTask retrieves a task by ID.
func (c *CloudStore) GetTask(taskID string) (*store.Task, error) {
	var rows []cloudTaskRow
	filter := fmt.Sprintf("task_id=eq.%s&project_id=eq.%s", url.QueryEscape(taskID), url.QueryEscape(c.projectID))
	if err := c.get("c4_tasks", filter, &rows); err != nil {
		return nil, fmt.Errorf("get task %s: %w", taskID, err)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("task not found: %s", taskID)
	}

	return rowToTask(&rows[0]), nil
}

// AssignTask finds and assigns the next available task to a worker.
// It fetches all tasks, evaluates dependencies, picks the highest-priority
// pending task with all dependencies met, and assigns it via PATCH.
func (c *CloudStore) AssignTask(workerID string) (*store.TaskAssignment, error) {
	var rows []cloudTaskRow
	filter := "project_id=eq." + url.QueryEscape(c.projectID)
	if err := c.get("c4_tasks", filter, &rows); err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}

	// Build lookup map
	taskMap := make(map[string]*cloudTaskRow, len(rows))
	for i := range rows {
		taskMap[rows[i].TaskID] = &rows[i]
	}

	// Find eligible candidates (pending with deps met)
	var candidates []*cloudTaskRow
	for i := range rows {
		row := &rows[i]
		if row.Status != "pending" {
			continue
		}
		if !isWorkerExecutionAllowed(row.ExecutionMode) {
			continue
		}
		if !cloudDependenciesMet(row, taskMap) {
			continue
		}
		candidates = append(candidates, row)
	}

	if len(candidates) == 0 {
		return nil, nil // no tasks available
	}

	// Sort by priority descending, then created_at ascending
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Priority != candidates[j].Priority {
			return candidates[i].Priority > candidates[j].Priority
		}
		return candidates[i].CreatedAt < candidates[j].CreatedAt
	})

	selected := candidates[0]

	// Assign via PATCH
	patchFilter := fmt.Sprintf("task_id=eq.%s&project_id=eq.%s", url.QueryEscape(selected.TaskID), url.QueryEscape(c.projectID))
	update := map[string]any{
		"status":     "in_progress",
		"worker_id":  workerID,
		"updated_at": time.Now().UTC().Format(time.RFC3339),
	}
	if err := c.patch("c4_tasks", patchFilter, update); err != nil {
		return nil, fmt.Errorf("assign task %s: %w", selected.TaskID, err)
	}

	var deps []string
	if selected.Dependencies != "" && string(selected.Dependencies) != "[]" {
		_ = json.Unmarshal([]byte(selected.Dependencies), &deps)
	}

	assignment := &store.TaskAssignment{
		TaskID:       selected.TaskID,
		Title:        selected.Title,
		Scope:        selected.Scope,
		DoD:          selected.DoD,
		Dependencies: deps,
		Domain:       selected.Domain,
		WorkerID:     workerID,
		Model:        selected.Model,
	}

	return assignment, nil
}

// SubmitTask marks a task as done after validating results.
func (c *CloudStore) SubmitTask(taskID, workerID, commitSHA, handoff string, results []store.ValidationResult) (*store.SubmitResult, error) {
	// Check for validation failures first (no HTTP needed)
	for _, r := range results {
		if r.Status == "fail" {
			return &store.SubmitResult{
				Success:    false,
				NextAction: "get_next_task",
				Message:    fmt.Sprintf("Validation failed: %s -- %s", r.Name, r.Message),
			}, nil
		}
	}

	// Verify task exists
	task, err := c.GetTask(taskID)
	if err != nil {
		return nil, fmt.Errorf("getting task for submit: %w", err)
	}
	if task.Status != "in_progress" {
		return &store.SubmitResult{
			Success:    false,
			NextAction: "get_next_task",
			Message:    fmt.Sprintf("Task %s is %s (expected in_progress)", taskID, task.Status),
		}, nil
	}
	if !isWorkerExecutionAllowed(task.ExecutionMode) {
		return &store.SubmitResult{
			Success:    false,
			NextAction: "get_next_task",
			Message:    fmt.Sprintf("Task %s execution_mode is %q (worker submit allowed: worker/auto)", taskID, task.ExecutionMode),
		}, nil
	}
	if task.WorkerID == "direct" {
		return &store.SubmitResult{
			Success:    false,
			NextAction: "get_next_task",
			Message:    fmt.Sprintf("Task %s is claimed by direct mode — use c4_report", taskID),
		}, nil
	}
	if workerID != "" && task.WorkerID != "" && task.WorkerID != workerID {
		return &store.SubmitResult{
			Success:    false,
			NextAction: "get_next_task",
			Message: fmt.Sprintf(
				"Task %s is owned by worker %s (submitter: %s)",
				taskID, task.WorkerID, workerID,
			),
		}, nil
	}

	// Mark as done
	patchFilter := fmt.Sprintf("task_id=eq.%s&project_id=eq.%s", url.QueryEscape(taskID), url.QueryEscape(c.projectID))
	update := map[string]any{
		"status":     "done",
		"commit_sha": commitSHA,
		"handoff":    handoff,
		"updated_at": time.Now().UTC().Format(time.RFC3339),
	}
	if err := c.patch("c4_tasks", patchFilter, update); err != nil {
		return nil, fmt.Errorf("updating task %s: %w", taskID, err)
	}

	// Check remaining tasks to determine next action
	var remainingRows []cloudTaskRow
	remainingFilter := fmt.Sprintf(
		"project_id=eq.%s&status=in.(pending,in_progress)",
		url.QueryEscape(c.projectID),
	)
	_ = c.get("c4_tasks", remainingFilter, &remainingRows)

	nextAction := "get_next_task"
	if len(remainingRows) == 0 {
		nextAction = "complete"
	}

	// Check for pending review task (auto-judge hint)
	var pendingReview string
	if strings.HasPrefix(taskID, "T-") {
		reviewID := "R-" + strings.TrimPrefix(taskID, "T-")
		var reviewRows []cloudTaskRow
		reviewFilter := fmt.Sprintf("task_id=eq.%s&project_id=eq.%s&status=eq.pending", url.QueryEscape(reviewID), url.QueryEscape(c.projectID))
		if err := c.get("c4_tasks", reviewFilter, &reviewRows); err == nil && len(reviewRows) > 0 {
			pendingReview = reviewID
		}
	}

	return &store.SubmitResult{
		Success:       true,
		NextAction:    nextAction,
		Message:       fmt.Sprintf("Task %s submitted successfully", taskID),
		PendingReview: pendingReview,
	}, nil
}

// MarkBlocked marks a task as blocked.
func (c *CloudStore) MarkBlocked(taskID, workerID, failureSignature string, attempts int, lastError string) error {
	patchFilter := fmt.Sprintf("task_id=eq.%s&project_id=eq.%s", url.QueryEscape(taskID), url.QueryEscape(c.projectID))
	update := map[string]any{
		"status":     "blocked",
		"worker_id":  "",
		"updated_at": time.Now().UTC().Format(time.RFC3339),
	}
	return c.patch("c4_tasks", patchFilter, update)
}

// ClaimTask claims a task for direct execution.
func (c *CloudStore) ClaimTask(taskID string) (*store.Task, error) {
	task, err := c.GetTask(taskID)
	if err != nil {
		return nil, err
	}

	if task.Status != "pending" {
		return nil, fmt.Errorf("task %s is %s, not pending", taskID, task.Status)
	}
	if !isDirectExecutionAllowed(task.ExecutionMode) {
		return nil, fmt.Errorf("task %s execution_mode is %q (expected direct or auto)", taskID, task.ExecutionMode)
	}

	patchFilter := fmt.Sprintf("task_id=eq.%s&project_id=eq.%s", url.QueryEscape(taskID), url.QueryEscape(c.projectID))
	update := map[string]any{
		"status":     "in_progress",
		"worker_id":  "direct",
		"updated_at": time.Now().UTC().Format(time.RFC3339),
	}
	if err := c.patch("c4_tasks", patchFilter, update); err != nil {
		return nil, fmt.Errorf("claiming task %s: %w", taskID, err)
	}

	task.Status = "in_progress"
	task.WorkerID = "direct"
	return task, nil
}

// ReportTask marks a directly-claimed task as done.
func (c *CloudStore) ReportTask(taskID, summary string, filesChanged []string) error {
	task, err := c.GetTask(taskID)
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

	patchFilter := fmt.Sprintf("task_id=eq.%s&project_id=eq.%s", url.QueryEscape(taskID), url.QueryEscape(c.projectID))
	update := map[string]any{
		"status":     "done",
		"handoff":    handoff,
		"updated_at": time.Now().UTC().Format(time.RFC3339),
	}
	return c.patch("c4_tasks", patchFilter, update)
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

// Checkpoint records a checkpoint decision.
func (c *CloudStore) Checkpoint(checkpointID, decision, notes string, requiredChanges []string) (*store.CheckpointResult, error) {
	changesJSON := "[]"
	if len(requiredChanges) > 0 {
		b, _ := json.Marshal(requiredChanges)
		changesJSON = string(b)
	}

	row := &cloudCheckpointRow{
		CheckpointID:    checkpointID,
		ProjectID:       c.projectID,
		Decision:        decision,
		Notes:           notes,
		RequiredChanges: changesJSON,
		CreatedAt:       time.Now().UTC().Format(time.RFC3339),
	}

	if err := c.post("c4_checkpoints", row); err != nil {
		return nil, fmt.Errorf("recording checkpoint: %w", err)
	}

	result := &store.CheckpointResult{
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

// RequestChanges rejects a review task and creates the next version T+R pair.
func (c *CloudStore) RequestChanges(reviewTaskID string, comments string, requiredChanges []string) (*store.RequestChangesResult, error) {
	// Parse review task ID: R-{baseID}-{version}
	if !strings.HasPrefix(reviewTaskID, "R-") {
		return nil, fmt.Errorf("%s is not a review task", reviewTaskID)
	}

	parts := strings.SplitN(strings.TrimPrefix(reviewTaskID, "R-"), "-", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid review task ID format: %s", reviewTaskID)
	}

	baseID := parts[0]
	var version int
	if _, err := fmt.Sscanf(parts[1], "%d", &version); err != nil {
		return nil, fmt.Errorf("invalid version in task ID %s: %w", reviewTaskID, err)
	}

	nextVersion := version + 1

	// Mark current R task as done with REQUEST_CHANGES result (reason in review_decision_evidence, not commit_sha)
	patchFilter := fmt.Sprintf("task_id=eq.%s&project_id=eq.%s", url.QueryEscape(reviewTaskID), url.QueryEscape(c.projectID))
	update := map[string]any{
		"status":                     "done",
		"review_decision_evidence":   comments,
		"commit_sha":                 "",
		"updated_at": time.Now().UTC().Format(time.RFC3339),
	}
	if err := c.patch("c4_tasks", patchFilter, update); err != nil {
		return nil, fmt.Errorf("updating review task: %w", err)
	}

	// Look up parent T's DoD
	parentTaskID := fmt.Sprintf("T-%s-%d", baseID, version)
	var parentRows []cloudTaskRow
	_ = c.get("c4_tasks", fmt.Sprintf("task_id=eq.%s&project_id=eq.%s", url.QueryEscape(parentTaskID), url.QueryEscape(c.projectID)), &parentRows)
	var originalDoD string
	if len(parentRows) > 0 {
		originalDoD = parentRows[0].DoD
	}

	// Create next version T + R
	changesText := strings.Join(requiredChanges, "\n- ")
	newDoD := fmt.Sprintf("Changes requested:\n- %s\n\nOriginal DoD:\n%s", changesText, originalDoD)

	nextTaskID := fmt.Sprintf("T-%s-%d", baseID, nextVersion)
	nextReviewID := fmt.Sprintf("R-%s-%d", baseID, nextVersion)

	// T-XXX-(N+1) -- fix task
	if err := c.AddTask(&store.Task{
		ID:           nextTaskID,
		Title:        fmt.Sprintf("Fix: %s", parentTaskID),
		DoD:          newDoD,
		Status:       "pending",
		Dependencies: []string{reviewTaskID},
		Priority:     10,
	}); err != nil {
		return nil, fmt.Errorf("creating fix task %s: %w", nextTaskID, err)
	}

	// R-XXX-(N+1) -- review of fix
	if err := c.AddTask(&store.Task{
		ID:           nextReviewID,
		Title:        fmt.Sprintf("Review: %s", nextTaskID),
		DoD:          fmt.Sprintf("Review fix of %s\n\nRequired changes:\n- %s", parentTaskID, changesText),
		Status:       "pending",
		Dependencies: []string{nextTaskID},
	}); err != nil {
		return nil, fmt.Errorf("creating review task %s: %w", nextReviewID, err)
	}

	return &store.RequestChangesResult{
		Success:      true,
		NextTaskID:   nextTaskID,
		NextReviewID: nextReviewID,
		Version:      nextVersion,
		Message:      fmt.Sprintf("Created %s + %s (v%d)", nextTaskID, nextReviewID, nextVersion),
	}, nil
}

// =========================================================================
// Internal state helpers
// =========================================================================

// readCurrentState reads the current project state from c4_state.
func (c *CloudStore) readCurrentState() (string, error) {
	var rows []cloudStateRow
	if err := c.get("c4_state", "project_id=eq."+url.QueryEscape(c.projectID), &rows); err != nil {
		return "", fmt.Errorf("get state: %w", err)
	}
	if len(rows) == 0 {
		return "INIT", nil
	}

	var m map[string]any
	if err := json.Unmarshal([]byte(rows[0].StateJSON), &m); err != nil {
		return "INIT", nil
	}

	if st, ok := m["status"].(string); ok {
		return st, nil
	}
	return "INIT", nil
}

// writeState updates the c4_state row for this project.
func (c *CloudStore) writeState(newState string) error {
	stateMap := map[string]any{
		"status":     newState,
		"project_id": c.projectID,
		"updated_at": time.Now().UTC().Format(time.RFC3339),
	}
	stateJSON, err := json.Marshal(stateMap)
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}

	patchFilter := "project_id=eq." + url.QueryEscape(c.projectID)
	update := map[string]any{
		"state_json": string(stateJSON),
		"updated_at": time.Now().UTC().Format(time.RFC3339),
	}
	return c.patch("c4_state", patchFilter, update)
}

// cloudDependenciesMet checks if all of a task's dependencies are done.
func cloudDependenciesMet(row *cloudTaskRow, taskMap map[string]*cloudTaskRow) bool {
	var deps []string
	if row.Dependencies != "" && string(row.Dependencies) != "[]" {
		if err := json.Unmarshal([]byte(row.Dependencies), &deps); err != nil {
			return false
		}
	}
	for _, depID := range deps {
		dep, ok := taskMap[depID]
		if !ok || dep.Status != "done" {
			return false
		}
	}
	return true
}

// rowToTask converts a cloudTaskRow to a store.Task.
func rowToTask(row *cloudTaskRow) *store.Task {
	var deps []string
	if row.Dependencies != "" && string(row.Dependencies) != "[]" {
		_ = json.Unmarshal([]byte(row.Dependencies), &deps)
	}

	return &store.Task{
		ID:            row.TaskID,
		Title:         row.Title,
		Scope:         row.Scope,
		DoD:           row.DoD,
		Status:        row.Status,
		Dependencies:  deps,
		Domain:        row.Domain,
		Priority:      row.Priority,
		Model:         row.Model,
		ExecutionMode: normalizeExecutionMode(row.ExecutionMode),
		WorkerID:      row.WorkerID,
		Branch:        row.Branch,
		CommitSHA:     row.CommitSHA,
		CreatedAt:     row.CreatedAt,
		UpdatedAt:     row.UpdatedAt,
	}
}

func normalizeExecutionMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", "worker":
		return "worker"
	case "direct":
		return "direct"
	case "auto":
		return "auto"
	default:
		return "worker"
	}
}

func isWorkerExecutionAllowed(mode string) bool {
	normalized := normalizeExecutionMode(mode)
	return normalized == "worker" || normalized == "auto"
}

func isDirectExecutionAllowed(mode string) bool {
	normalized := normalizeExecutionMode(mode)
	return normalized == "direct" || normalized == "auto"
}

// =========================================================================
// HTTP helpers (PostgREST)
// =========================================================================

// get performs a GET request and decodes the JSON response into dest.
// Retries once on 401 Unauthorized after refreshing the token.
func (c *CloudStore) get(table, filter string, dest any) error {
	u := c.baseURL + "/" + table
	if filter != "" {
		u += "?" + filter
	}

	for attempt := 0; attempt < 2; attempt++ {
		req, err := http.NewRequest("GET", u, nil)
		if err != nil {
			return err
		}
		c.setHeaders(req)

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("GET %s: %w", table, err)
		}

		if resp.StatusCode == http.StatusUnauthorized && attempt == 0 {
			resp.Body.Close()
			if _, err := c.tokenProvider.Refresh(); err == nil {
				continue
			}
		}

		defer resp.Body.Close()

		if resp.StatusCode >= 400 {
			body, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("GET %s: %d %s", table, resp.StatusCode, string(body))
		}

		if dest != nil {
			if err := json.NewDecoder(resp.Body).Decode(dest); err != nil {
				return fmt.Errorf("decode %s: %w", table, err)
			}
		}
		return nil
	}
	return nil
}

// post performs a POST request with the given body.
// Retries once on 401 Unauthorized after refreshing the token.
func (c *CloudStore) post(table string, body any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	for attempt := 0; attempt < 2; attempt++ {
		req, err := http.NewRequest("POST", c.baseURL+"/"+table, strings.NewReader(string(data)))
		if err != nil {
			return err
		}
		c.setHeaders(req)
		req.Header.Set("Prefer", "return=minimal")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("POST %s: %w", table, err)
		}

		if resp.StatusCode == http.StatusUnauthorized && attempt == 0 {
			resp.Body.Close()
			if _, err := c.tokenProvider.Refresh(); err == nil {
				continue
			}
		}

		defer resp.Body.Close()

		if resp.StatusCode >= 400 {
			respBody, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("POST %s: %d %s", table, resp.StatusCode, string(respBody))
		}
		return nil
	}
	return nil
}

// patch performs a PATCH request with the given filter and body.
// Retries once on 401 Unauthorized after refreshing the token.
func (c *CloudStore) patch(table, filter string, body any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	u := c.baseURL + "/" + table + "?" + filter
	for attempt := 0; attempt < 2; attempt++ {
		req, err := http.NewRequest("PATCH", u, strings.NewReader(string(data)))
		if err != nil {
			return err
		}
		c.setHeaders(req)
		req.Header.Set("Prefer", "return=minimal")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("PATCH %s: %w", table, err)
		}

		if resp.StatusCode == http.StatusUnauthorized && attempt == 0 {
			resp.Body.Close()
			if _, err := c.tokenProvider.Refresh(); err == nil {
				continue
			}
		}

		defer resp.Body.Close()

		if resp.StatusCode >= 400 {
			respBody, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("PATCH %s: %d %s", table, resp.StatusCode, string(respBody))
		}
		return nil
	}
	return nil
}

// del performs a DELETE request with the given filter.
// Retries once on 401 Unauthorized after refreshing the token.
func (c *CloudStore) del(table, filter string) error {
	u := c.baseURL + "/" + table + "?" + filter
	for attempt := 0; attempt < 2; attempt++ {
		req, err := http.NewRequest("DELETE", u, nil)
		if err != nil {
			return err
		}
		c.setHeaders(req)

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("DELETE %s: %w", table, err)
		}

		if resp.StatusCode == http.StatusUnauthorized && attempt == 0 {
			resp.Body.Close()
			if _, err := c.tokenProvider.Refresh(); err == nil {
				continue
			}
		}

		defer resp.Body.Close()

		if resp.StatusCode >= 400 {
			body, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("DELETE %s: %d %s", table, resp.StatusCode, string(body))
		}
		return nil
	}
	return nil
}

// setHeaders adds standard Supabase headers to the request.
func (c *CloudStore) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("apikey", c.apiKey)
	req.Header.Set("Authorization", "Bearer "+c.tokenProvider.Token())
}
