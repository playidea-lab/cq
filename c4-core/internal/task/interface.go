// Package task implements the C4 task store with dependency resolution,
// Review-as-Task workflow, and version management.
//
// Two backends are provided:
//   - Store (in-memory, for testing and single-process)
//   - SupabaseTaskStore (PostgREST, for cloud/multi-node)
//
// Both implement the TaskStore interface.
package task

// TaskStore defines the contract for task persistence backends.
//
// Both SQLite (in-memory Store) and Supabase implementations
// satisfy this interface, allowing seamless backend switching.
type TaskStore interface {
	// CreateTask inserts a new task.
	CreateTask(task *Task) error

	// GetTask returns a task by ID.
	GetTask(id string) (*Task, error)

	// UpdateTask updates an existing task.
	// Returns ErrVersionConflict if optimistic lock fails.
	UpdateTask(task *Task) error

	// ListTasks returns all tasks, optionally filtered by project.
	ListTasks(projectID string) ([]*Task, error)

	// DeleteTask removes a task by ID.
	DeleteTask(id string) error

	// GetNextTask finds the highest-priority pending task whose
	// dependencies are done and scope is not locked.
	// Assigns it to the given worker and returns it.
	GetNextTask(workerID string) (*Task, error)

	// CompleteTask marks a task as done, releases scope locks,
	// and auto-generates a review task for IMPLEMENTATION tasks.
	// Returns the generated review task (nil for non-implementation).
	CompleteTask(taskID string, workerID string, commitSHA string) (*Task, error)
}
