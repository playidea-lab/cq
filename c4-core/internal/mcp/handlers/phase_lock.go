package handlers

import (
	"encoding/json"
	"fmt"

	"github.com/changmin/c4-core/internal/mcp"
	"github.com/changmin/c4-core/internal/state"
)

// RegisterPhaseLockHandlers registers advisory phase lock MCP tools.
//
// Tools registered:
//   - c4_phase_lock_acquire: attempt to acquire a phase lock
//   - c4_phase_lock_release: release a held phase lock
func RegisterPhaseLockHandlers(reg *mcp.Registry, rootDir string) {
	locker := state.NewPhaseLocker(rootDir)

	// c4_phase_lock_acquire
	reg.Register(mcp.ToolSchema{
		Name:        "c4_phase_lock_acquire",
		Description: "Acquire an advisory phase lock to prevent concurrent polish/finish operations. Returns {acquired: true} on success or {acquired: false, error: {code, message, details}} if lock is held.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"phase": map[string]any{
					"type":        "string",
					"description": "Phase name to lock (e.g. 'polish', 'finish')",
					"enum":        []string{"polish", "finish"},
				},
			},
			"required": []string{"phase"},
		},
	}, func(args json.RawMessage) (any, error) {
		return handlePhaseLockAcquire(locker, args)
	})

	// c4_phase_lock_release
	reg.Register(mcp.ToolSchema{
		Name:        "c4_phase_lock_release",
		Description: "Release a previously acquired advisory phase lock. Returns {released: true} on success.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"phase": map[string]any{
					"type":        "string",
					"description": "Phase name to unlock (e.g. 'polish', 'finish')",
					"enum":        []string{"polish", "finish"},
				},
			},
			"required": []string{"phase"},
		},
	}, func(args json.RawMessage) (any, error) {
		return handlePhaseLockRelease(locker, args)
	})
}

type phaseLockArgs struct {
	Phase string `json:"phase"`
}

func handlePhaseLockAcquire(locker *state.PhaseLocker, rawArgs json.RawMessage) (any, error) {
	var args phaseLockArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return nil, fmt.Errorf("parsing arguments: %w", err)
	}
	if args.Phase == "" {
		return nil, fmt.Errorf("phase is required")
	}
	result := locker.Acquire(args.Phase)
	return result, nil
}

func handlePhaseLockRelease(locker *state.PhaseLocker, rawArgs json.RawMessage) (any, error) {
	var args phaseLockArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return nil, fmt.Errorf("parsing arguments: %w", err)
	}
	if args.Phase == "" {
		return nil, fmt.Errorf("phase is required")
	}
	released := locker.Release(args.Phase)
	return map[string]any{
		"released": released,
		"phase":    args.Phase,
	}, nil
}
