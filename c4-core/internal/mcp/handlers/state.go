package handlers

import (
	"encoding/json"
	"fmt"

	"github.com/changmin/c4-core/internal/mcp"
)

// clearArgs is the input for c4_clear.
type clearArgs struct {
	Confirm    bool `json:"confirm"`
	KeepConfig bool `json:"keep_config"`
}

// RegisterStateHandlers registers state management tools on the registry.
func RegisterStateHandlers(reg *mcp.Registry, store Store) {
	// c4_status
	reg.Register(mcp.ToolSchema{
		Name:        "c4_status",
		Description: "Get current C4 project status including state, queue, and workers",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
			"required":   []string{},
		},
	}, func(args json.RawMessage) (any, error) {
		return handleStatus(store, args)
	})

	// c4_start
	reg.Register(mcp.ToolSchema{
		Name:        "c4_start",
		Description: "Start execution by transitioning from PLAN/HALTED to EXECUTE state",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
			"required":   []string{},
		},
	}, func(args json.RawMessage) (any, error) {
		return handleStart(store, args)
	})

	// c4_clear
	reg.Register(mcp.ToolSchema{
		Name:        "c4_clear",
		Description: "Clear C4 state completely. Deletes .c4 directory and clears daemon cache.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"confirm": map[string]any{
					"type":        "boolean",
					"description": "Must be true to confirm deletion",
				},
				"keep_config": map[string]any{
					"type":        "boolean",
					"description": "Keep config.yaml (default: false)",
					"default":     false,
				},
			},
			"required": []string{"confirm"},
		},
	}, func(args json.RawMessage) (any, error) {
		return handleClear(store, args)
	})
}

func handleStatus(store Store, _ json.RawMessage) (any, error) {
	status, err := store.GetStatus()
	if err != nil {
		return nil, fmt.Errorf("getting status: %w", err)
	}
	return status, nil
}

func handleStart(store Store, _ json.RawMessage) (any, error) {
	if err := store.Start(); err != nil {
		return map[string]any{
			"success": false,
			"error":   err.Error(),
		}, nil
	}
	return map[string]any{
		"success": true,
		"status":  "EXECUTE",
		"message": "Transitioned to EXECUTE state",
	}, nil
}

func handleClear(store Store, rawArgs json.RawMessage) (any, error) {
	var args clearArgs
	if len(rawArgs) > 0 {
		if err := json.Unmarshal(rawArgs, &args); err != nil {
			return nil, fmt.Errorf("parsing arguments: %w", err)
		}
	}

	if !args.Confirm {
		return map[string]any{
			"error": "Must set confirm=true to clear C4 state",
		}, nil
	}

	if err := store.Clear(args.KeepConfig); err != nil {
		return nil, fmt.Errorf("clearing state: %w", err)
	}

	return map[string]any{
		"success": true,
		"message": "C4 state cleared. Run /c4-init to reinitialize.",
	}, nil
}
