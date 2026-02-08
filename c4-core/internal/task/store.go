package task

import (
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"
)

var (
	// ErrTaskNotFound is returned when a task doesn't exist.
	ErrTaskNotFound = errors.New("task not found")
	// ErrNotPending is returned when trying to assign a non-pending task.
	ErrNotPending = errors.New("task is not pending")
	// ErrNotInProgress is returned when trying to complete a non-in-progress task.
	ErrNotInProgress = errors.New("task is not in progress")
	// ErrDependenciesNotMet is returned when a task's dependencies aren't done.
	ErrDependenciesNotMet = errors.New("dependencies not met")
	// ErrScopeLocked is returned when a task's scope is locked by another worker.
	ErrScopeLocked = errors.New("scope is locked by another worker")
	// ErrNoAvailableTask is returned when no assignable task exists.
	ErrNoAvailableTask = errors.New("no available task")
	// ErrCircularDependency is returned when a circular dependency is detected.
	ErrCircularDependency = errors.New("circular dependency detected")
	// ErrWorkerMismatch is returned when the worker doesn't own the task.
	ErrWorkerMismatch = errors.New("worker does not own this task")
	// ErrVersionConflict is returned when an optimistic lock fails.
	ErrVersionConflict = errors.New("version conflict")
)

// Store manages tasks with dependency resolution and Review-as-Task workflow.
//
// Thread-safe: all methods can be called concurrently.
// This is the in-memory implementation (also aliases as "sqlite" backend).
type Store struct {
	mu         sync.RWMutex
	tasks      map[string]*Task
	scopeLocks map[string]string // scope -> worker_id
}

// NewStore creates an empty task store.
func NewStore() *Store {
	return &Store{
		tasks:      make(map[string]*Task),
		scopeLocks: make(map[string]string),
	}
}

// Add adds a task to the store.
func (s *Store) Add(task *Task) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.checkCircularLocked(task.ID, task.Dependencies); err != nil {
		return err
	}

	if task.CreatedAt.IsZero() {
		task.CreatedAt = time.Now()
	}
	s.tasks[task.ID] = task
	return nil
}

// Get returns a task by ID.
func (s *Store) Get(id string) (*Task, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	task, ok := s.tasks[id]
	if !ok {
		return nil, ErrTaskNotFound
	}
	return task, nil
}

// All returns all tasks.
func (s *Store) All() []*Task {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*Task, 0, len(s.tasks))
	for _, t := range s.tasks {
		result = append(result, t)
	}
	return result
}

// GetNextTask finds the highest-priority pending task whose dependencies are
// all done and whose scope is not locked by another worker.
func (s *Store) GetNextTask(workerID string) (*Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var candidates []*Task
	for _, t := range s.tasks {
		if t.Status != StatusPending {
			continue
		}
		if !s.dependenciesMetLocked(t) {
			continue
		}
		if t.Scope != "" {
			owner, locked := s.scopeLocks[t.Scope]
			if locked && owner != workerID {
				continue
			}
		}
		candidates = append(candidates, t)
	}

	if len(candidates) == 0 {
		return nil, ErrNoAvailableTask
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Priority != candidates[j].Priority {
			return candidates[i].Priority > candidates[j].Priority
		}
		return candidates[i].CreatedAt.Before(candidates[j].CreatedAt)
	})

	task := candidates[0]
	task.Status = StatusInProgress
	task.AssignedTo = workerID
	if task.Scope != "" {
		s.scopeLocks[task.Scope] = workerID
	}

	return task, nil
}

// CompleteTask marks a task as done and auto-generates a review task.
func (s *Store) CompleteTask(taskID string, workerID string, commitSHA string) (*Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	task, ok := s.tasks[taskID]
	if !ok {
		return nil, ErrTaskNotFound
	}
	if task.Status != StatusInProgress {
		return nil, ErrNotInProgress
	}
	if task.AssignedTo != workerID {
		return nil, ErrWorkerMismatch
	}

	task.Status = StatusDone
	task.CommitSHA = commitSHA
	task.CompletedAt = time.Now()

	if task.Scope != "" {
		delete(s.scopeLocks, task.Scope)
	}

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
			CreatedAt:    time.Now(),
		}
		s.tasks[reviewID] = reviewTask
		return reviewTask, nil
	}

	return nil, nil
}

// RequestChanges handles a review decision of REQUEST_CHANGES.
func (s *Store) RequestChanges(reviewTaskID string, comments string) (*Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	reviewTask, ok := s.tasks[reviewTaskID]
	if !ok {
		return nil, ErrTaskNotFound
	}
	if reviewTask.Type != TypeReview {
		return nil, fmt.Errorf("task %s is not a review task", reviewTaskID)
	}

	parentTask, ok := s.tasks[reviewTask.ParentID]
	if !ok {
		return nil, fmt.Errorf("parent task %s not found", reviewTask.ParentID)
	}

	nextID := NextVersionID("T", parentTask.BaseID, parentTask.Version)
	nextTask := &Task{
		ID:           nextID,
		Title:        parentTask.Title,
		Scope:        parentTask.Scope,
		Priority:     parentTask.Priority,
		DoD:          fmt.Sprintf("Changes requested:\n%s\n\nOriginal DoD:\n%s", comments, parentTask.DoD),
		Dependencies: []string{reviewTaskID},
		Status:       StatusPending,
		Domain:       parentTask.Domain,
		Model:        parentTask.Model,
		Type:         TypeImplementation,
		BaseID:       parentTask.BaseID,
		Version:      parentTask.Version + 1,
		ParentID:     reviewTaskID,
		CreatedAt:    time.Now(),
	}

	s.tasks[nextID] = nextTask
	return nextTask, nil
}

func (s *Store) dependenciesMetLocked(task *Task) bool {
	for _, depID := range task.Dependencies {
		dep, ok := s.tasks[depID]
		if !ok || dep.Status != StatusDone {
			return false
		}
	}
	return true
}

func (s *Store) checkCircularLocked(taskID string, deps []string) error {
	visited := make(map[string]bool)
	return s.dfsCircular(taskID, deps, visited)
}

func (s *Store) dfsCircular(targetID string, deps []string, visited map[string]bool) error {
	for _, depID := range deps {
		if depID == targetID {
			return fmt.Errorf("%w: %s -> %s", ErrCircularDependency, targetID, depID)
		}
		if visited[depID] {
			continue
		}
		visited[depID] = true
		if dep, ok := s.tasks[depID]; ok {
			if err := s.dfsCircular(targetID, dep.Dependencies, visited); err != nil {
				return err
			}
		}
	}
	return nil
}

// PendingCount returns the number of pending tasks.
func (s *Store) PendingCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	count := 0
	for _, t := range s.tasks {
		if t.Status == StatusPending {
			count++
		}
	}
	return count
}

// DoneCount returns the number of done tasks.
func (s *Store) DoneCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	count := 0
	for _, t := range s.tasks {
		if t.Status == StatusDone {
			count++
		}
	}
	return count
}
