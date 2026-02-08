// Package git provides Git operations for C4 task management.
//
// This includes:
//   - Branch creation and switching for worker tasks (c4/w-{task_id})
//   - Worktree management for parallel worker execution
//   - Commit and merge operations after task completion
//   - Diff generation for checkpoint bundles
package git
