// Package task implements the task store and lifecycle management.
//
// Tasks follow the lifecycle: pending -> in_progress -> done | blocked
//
// Task types include:
//   - Implementation tasks (T-XXX-N)
//   - Review tasks (R-XXX-N) - auto-generated after implementation
//   - Checkpoint tasks (CP-XXX) - phase gate reviews
//   - Repair tasks (RPR-XXX-N) - for blocked task recovery
package task
