// Package worker implements worker registration, assignment, and lifecycle.
//
// Workers request tasks via c4_get_task, execute them, and submit
// results via c4_submit. The worker manager handles:
//   - Worker registration and heartbeat tracking
//   - Task assignment based on priority and dependencies
//   - Scope locking to prevent concurrent file access
//   - Zombie worker detection and cleanup
package worker
