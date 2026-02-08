// Package state implements the C4 project state machine.
//
// The state machine manages transitions between project phases:
// INIT -> DISCOVERY -> DESIGN -> PLAN -> EXECUTE <-> CHECKPOINT -> COMPLETE
//
// Each state has defined allowed transitions and invariant checks
// that are validated before any transition occurs.
package state
