package task

// This file adds TaskStore interface methods to the in-memory Store,
// bridging the existing API (Add/Get/All) with the TaskStore contract
// (CreateTask/GetTask/UpdateTask/ListTasks/DeleteTask).

// Compile-time interface check.
var _ TaskStore = (*Store)(nil)

// CreateTask adds a task to the in-memory store.
func (s *Store) CreateTask(task *Task) error {
	return s.Add(task)
}

// GetTask retrieves a task by ID from the in-memory store.
func (s *Store) GetTask(id string) (*Task, error) {
	return s.Get(id)
}

// UpdateTask updates a task in the in-memory store.
// The in-memory store always succeeds (no optimistic locking).
func (s *Store) UpdateTask(task *Task) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.tasks[task.ID]; !ok {
		return ErrTaskNotFound
	}
	s.tasks[task.ID] = task
	return nil
}

// ListTasks returns all tasks. The projectID filter is ignored
// since the in-memory store is single-project.
func (s *Store) ListTasks(_ string) ([]*Task, error) {
	return s.All(), nil
}

// DeleteTask removes a task by ID.
func (s *Store) DeleteTask(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.tasks[id]; !ok {
		return ErrTaskNotFound
	}
	delete(s.tasks, id)
	return nil
}
