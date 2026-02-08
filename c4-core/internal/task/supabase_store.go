package task

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

// SupabaseConfig holds Supabase connection settings.
type SupabaseConfig struct {
	URL       string // Supabase project URL
	AnonKey   string // Supabase anon/public key
	ProjectID string // C4 project ID (for RLS filtering)
}

// supabaseTaskRow maps to the c4_tasks Supabase table.
// Includes version field for optimistic locking.
type supabaseTaskRow struct {
	ID           string `json:"id"`
	ProjectID    string `json:"project_id"`
	Title        string `json:"title"`
	Scope        string `json:"scope,omitempty"`
	Priority     int    `json:"priority"`
	DoD          string `json:"dod"`
	Dependencies string `json:"dependencies,omitempty"` // JSON array string
	Status       string `json:"status"`
	AssignedTo   string `json:"assigned_to,omitempty"`
	Branch       string `json:"branch,omitempty"`
	CommitSHA    string `json:"commit_sha,omitempty"`
	Domain       string `json:"domain,omitempty"`
	Model        string `json:"model"`
	TaskType     string `json:"task_type"`
	BaseID       string `json:"base_id,omitempty"`
	Version      int    `json:"version"`
	ParentID     string `json:"parent_id,omitempty"`
	CompletedBy  string `json:"completed_by,omitempty"`
	CreatedAt    string `json:"created_at,omitempty"`
	CompletedAt  string `json:"completed_at,omitempty"`
	RowVersion   int    `json:"row_version"` // optimistic lock
}

// SupabaseTaskStore implements TaskStore using Supabase PostgREST.
//
// Uses RLS (Row-Level Security) with project_id for automatic filtering.
// Uses row_version for optimistic locking on updates.
type SupabaseTaskStore struct {
	baseURL    string
	apiKey     string
	projectID  string
	httpClient *http.Client
}

// NewSupabaseTaskStore creates a Supabase-backed task store.
func NewSupabaseTaskStore(cfg *SupabaseConfig) *SupabaseTaskStore {
	return &SupabaseTaskStore{
		baseURL:   strings.TrimRight(cfg.URL, "/") + "/rest/v1",
		apiKey:    cfg.AnonKey,
		projectID: cfg.ProjectID,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Compile-time interface check.
var _ TaskStore = (*SupabaseTaskStore)(nil)

// CreateTask inserts a new task into Supabase.
func (s *SupabaseTaskStore) CreateTask(task *Task) error {
	row := taskToRow(task, s.projectID)
	row.RowVersion = 1 // initial version
	return s.post("c4_tasks", row)
}

// GetTask retrieves a task by ID (filtered by project_id via RLS).
func (s *SupabaseTaskStore) GetTask(id string) (*Task, error) {
	filter := fmt.Sprintf("id=eq.%s&project_id=eq.%s", id, s.projectID)
	rows, err := s.getRows(filter)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, ErrTaskNotFound
	}
	return rowToTask(&rows[0]), nil
}

// UpdateTask updates a task with optimistic locking.
// The row_version must match; on conflict ErrVersionConflict is returned.
func (s *SupabaseTaskStore) UpdateTask(task *Task) error {
	row := taskToRow(task, s.projectID)
	// Increment version for optimistic lock
	row.RowVersion = task.Version + 1

	// PATCH with version check
	filter := fmt.Sprintf(
		"id=eq.%s&project_id=eq.%s&row_version=eq.%d",
		task.ID, s.projectID, task.Version,
	)
	return s.patch("c4_tasks", filter, row)
}

// ListTasks returns all tasks for the project.
func (s *SupabaseTaskStore) ListTasks(projectID string) ([]*Task, error) {
	pid := projectID
	if pid == "" {
		pid = s.projectID
	}
	filter := fmt.Sprintf("project_id=eq.%s&order=priority.desc", pid)
	rows, err := s.getRows(filter)
	if err != nil {
		return nil, err
	}
	tasks := make([]*Task, len(rows))
	for i := range rows {
		tasks[i] = rowToTask(&rows[i])
	}
	return tasks, nil
}

// DeleteTask removes a task by ID.
func (s *SupabaseTaskStore) DeleteTask(id string) error {
	filter := fmt.Sprintf("id=eq.%s&project_id=eq.%s", id, s.projectID)
	return s.delete("c4_tasks", filter)
}

// GetNextTask finds the highest-priority pending task with
// dependencies met and scope available, then assigns it.
//
// This performs two operations:
//  1. GET all tasks to evaluate dependencies
//  2. PATCH to assign the selected task (with optimistic lock)
func (s *SupabaseTaskStore) GetNextTask(workerID string) (*Task, error) {
	// Fetch all tasks to resolve dependencies
	filter := fmt.Sprintf("project_id=eq.%s", s.projectID)
	rows, err := s.getRows(filter)
	if err != nil {
		return nil, err
	}

	// Build lookup maps
	taskMap := make(map[string]*supabaseTaskRow, len(rows))
	scopeLocks := make(map[string]string) // scope -> assigned_to
	for i := range rows {
		taskMap[rows[i].ID] = &rows[i]
		if rows[i].Status == string(StatusInProgress) && rows[i].Scope != "" {
			scopeLocks[rows[i].Scope] = rows[i].AssignedTo
		}
	}

	// Find eligible candidates
	var candidates []*supabaseTaskRow
	for i := range rows {
		row := &rows[i]
		if row.Status != string(StatusPending) {
			continue
		}
		// Check dependencies
		if !dependenciesMet(row, taskMap) {
			continue
		}
		// Check scope lock
		if row.Scope != "" {
			owner, locked := scopeLocks[row.Scope]
			if locked && owner != workerID {
				continue
			}
		}
		candidates = append(candidates, row)
	}

	if len(candidates) == 0 {
		return nil, ErrNoAvailableTask
	}

	// Sort by priority descending, then created_at ascending
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Priority != candidates[j].Priority {
			return candidates[i].Priority > candidates[j].Priority
		}
		return candidates[i].CreatedAt < candidates[j].CreatedAt
	})

	selected := candidates[0]

	// Assign via PATCH (optimistic lock on row_version)
	update := &supabaseTaskRow{
		Status:     string(StatusInProgress),
		AssignedTo: workerID,
		RowVersion: selected.RowVersion + 1,
	}
	patchFilter := fmt.Sprintf(
		"id=eq.%s&project_id=eq.%s&row_version=eq.%d",
		selected.ID, s.projectID, selected.RowVersion,
	)
	if err := s.patch("c4_tasks", patchFilter, update); err != nil {
		return nil, fmt.Errorf("assign task %s: %w", selected.ID, err)
	}

	selected.Status = string(StatusInProgress)
	selected.AssignedTo = workerID
	return rowToTask(selected), nil
}

// CompleteTask marks a task as done and auto-generates a review task.
func (s *SupabaseTaskStore) CompleteTask(taskID string, workerID string, commitSHA string) (*Task, error) {
	// Fetch the task
	task, err := s.GetTask(taskID)
	if err != nil {
		return nil, err
	}

	if task.Status != StatusInProgress {
		return nil, ErrNotInProgress
	}
	if task.AssignedTo != workerID {
		return nil, ErrWorkerMismatch
	}

	// Mark done
	now := time.Now()
	task.Status = StatusDone
	task.CommitSHA = commitSHA
	task.CompletedAt = now

	if err := s.UpdateTask(task); err != nil {
		return nil, fmt.Errorf("complete task %s: %w", taskID, err)
	}

	// Auto-generate review task for implementation tasks
	if task.Type == TypeImplementation {
		reviewID := ReviewID(task.BaseID, task.Version)
		reviewTask := &Task{
			ID:           reviewID,
			Title:        fmt.Sprintf("Review: %s", task.Title),
			Scope:        task.Scope,
			DoD:          fmt.Sprintf("Review implementation of %s (commit: %s)", taskID, commitSHA),
			Dependencies: []string{taskID},
			Status:       StatusPending,
			Domain:       task.Domain,
			Model:        task.Model,
			Type:         TypeReview,
			BaseID:       task.BaseID,
			Version:      task.Version,
			ParentID:     taskID,
			CompletedBy:  workerID,
			CreatedAt:    now,
		}
		if err := s.CreateTask(reviewTask); err != nil {
			return nil, fmt.Errorf("create review task: %w", err)
		}
		return reviewTask, nil
	}

	return nil, nil
}

// =========================================================================
// Row ↔ Task conversion
// =========================================================================

func taskToRow(task *Task, projectID string) *supabaseTaskRow {
	deps := "[]"
	if len(task.Dependencies) > 0 {
		b, _ := json.Marshal(task.Dependencies)
		deps = string(b)
	}

	row := &supabaseTaskRow{
		ID:          task.ID,
		ProjectID:   projectID,
		Title:       task.Title,
		Scope:       task.Scope,
		Priority:    task.Priority,
		DoD:         task.DoD,
		Dependencies: deps,
		Status:      string(task.Status),
		AssignedTo:  task.AssignedTo,
		Branch:      task.Branch,
		CommitSHA:   task.CommitSHA,
		Domain:      task.Domain,
		Model:       task.Model,
		TaskType:    string(task.Type),
		BaseID:      task.BaseID,
		Version:     task.Version,
		ParentID:    task.ParentID,
		CompletedBy: task.CompletedBy,
		RowVersion:  task.Version, // map to optimistic lock
	}

	if !task.CreatedAt.IsZero() {
		row.CreatedAt = task.CreatedAt.Format(time.RFC3339)
	}
	if !task.CompletedAt.IsZero() {
		row.CompletedAt = task.CompletedAt.Format(time.RFC3339)
	}

	return row
}

func rowToTask(row *supabaseTaskRow) *Task {
	var deps []string
	if row.Dependencies != "" && row.Dependencies != "[]" {
		_ = json.Unmarshal([]byte(row.Dependencies), &deps)
	}

	task := &Task{
		ID:           row.ID,
		Title:        row.Title,
		Scope:        row.Scope,
		Priority:     row.Priority,
		DoD:          row.DoD,
		Dependencies: deps,
		Status:       Status(row.Status),
		AssignedTo:   row.AssignedTo,
		Branch:       row.Branch,
		CommitSHA:    row.CommitSHA,
		Domain:       row.Domain,
		Model:        row.Model,
		Type:         Type(row.TaskType),
		BaseID:       row.BaseID,
		Version:      row.Version,
		ParentID:     row.ParentID,
		CompletedBy:  row.CompletedBy,
	}

	if row.CreatedAt != "" {
		task.CreatedAt, _ = time.Parse(time.RFC3339, row.CreatedAt)
	}
	if row.CompletedAt != "" {
		task.CompletedAt, _ = time.Parse(time.RFC3339, row.CompletedAt)
	}

	return task
}

// dependenciesMet checks if all of a task's dependencies are done.
func dependenciesMet(row *supabaseTaskRow, taskMap map[string]*supabaseTaskRow) bool {
	var deps []string
	if row.Dependencies != "" && row.Dependencies != "[]" {
		if err := json.Unmarshal([]byte(row.Dependencies), &deps); err != nil {
			return false
		}
	}
	for _, depID := range deps {
		dep, ok := taskMap[depID]
		if !ok || dep.Status != string(StatusDone) {
			return false
		}
	}
	return true
}

// =========================================================================
// HTTP helpers (PostgREST)
// =========================================================================

func (s *SupabaseTaskStore) post(table string, body any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	req, err := http.NewRequest("POST", s.baseURL+"/"+table, strings.NewReader(string(data)))
	if err != nil {
		return err
	}
	s.setHeaders(req)
	req.Header.Set("Prefer", "return=minimal")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("POST %s: %w", table, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("POST %s: %d %s", table, resp.StatusCode, string(body))
	}
	return nil
}

func (s *SupabaseTaskStore) getRows(filter string) ([]supabaseTaskRow, error) {
	url := s.baseURL + "/c4_tasks"
	if filter != "" {
		url += "?" + filter
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	s.setHeaders(req)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET c4_tasks: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GET c4_tasks: %d %s", resp.StatusCode, string(body))
	}

	var rows []supabaseTaskRow
	if err := json.NewDecoder(resp.Body).Decode(&rows); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	return rows, nil
}

func (s *SupabaseTaskStore) patch(table, filter string, body any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	url := s.baseURL + "/" + table + "?" + filter
	req, err := http.NewRequest("PATCH", url, strings.NewReader(string(data)))
	if err != nil {
		return err
	}
	s.setHeaders(req)
	req.Header.Set("Prefer", "return=minimal")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("PATCH %s: %w", table, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("PATCH %s: %d %s", table, resp.StatusCode, string(body))
	}
	return nil
}

func (s *SupabaseTaskStore) delete(table, filter string) error {
	url := s.baseURL + "/" + table + "?" + filter
	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return err
	}
	s.setHeaders(req)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("DELETE %s: %w", table, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("DELETE %s: %d %s", table, resp.StatusCode, string(body))
	}
	return nil
}

func (s *SupabaseTaskStore) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("apikey", s.apiKey)
	req.Header.Set("Authorization", "Bearer "+s.apiKey)
}
