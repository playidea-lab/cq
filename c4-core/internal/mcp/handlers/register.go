package handlers

import "github.com/changmin/c4-core/internal/mcp"

// RegisterAll registers all 10 core MCP tool handlers on the registry.
//
// Tools registered:
//   - c4_status: Get project status
//   - c4_start: Transition to EXECUTE state
//   - c4_clear: Clear C4 state
//   - c4_get_task: Request task assignment
//   - c4_submit: Report task completion
//   - c4_add_todo: Add new task to queue
//   - c4_mark_blocked: Mark task as blocked
//   - c4_claim: Claim task for direct mode
//   - c4_report: Report completion for direct mode
//   - c4_checkpoint: Record supervisor checkpoint
func RegisterAll(reg *mcp.Registry, store Store) {
	RegisterStateHandlers(reg, store)
	RegisterTaskHandlers(reg, store)
	RegisterTrackingHandlers(reg, store)
}
