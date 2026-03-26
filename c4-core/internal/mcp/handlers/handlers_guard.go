//go:build c6_guard

package handlers

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/changmin/c4-core/internal/guard"
	"github.com/changmin/c4-core/internal/mcp"
)

// RegisterGuardHandlers registers c4_guard_* MCP tools backed by the guard Engine.
func RegisterGuardHandlers(reg *mcp.Registry, eng *guard.Engine) {
	// c4_guard_check — check actor + tool permission
	reg.Register(mcp.ToolSchema{
		Name:        "cq_guard_check",
		Description: "Check whether an actor is permitted to call a tool. Returns Allow, Deny, or AuditOnly.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"actor": map[string]any{"type": "string", "description": "Actor identifier (e.g. worker-abc123, user:alice)"},
				"tool":  map[string]any{"type": "string", "description": "Tool name to check (e.g. c4_add_todo)"},
			},
			"required": []string{"actor", "tool"},
		},
	}, func(raw json.RawMessage) (any, error) {
		var args struct {
			Actor string `json:"actor"`
			Tool  string `json:"tool"`
		}
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("parse args: %w", err)
		}
		if args.Actor == "" {
			return nil, fmt.Errorf("actor is required")
		}
		if args.Tool == "" {
			return nil, fmt.Errorf("tool is required")
		}

		action := eng.Check(context.Background(), args.Actor, args.Tool, nil)
		return map[string]any{
			"actor":  args.Actor,
			"tool":   args.Tool,
			"action": action.String(),
		}, nil
	})

	// c4_guard_audit — query audit log
	reg.Register(mcp.ToolSchema{
		Name:        "cq_guard_audit",
		Description: "Query the guard audit log. Returns recent access-control decisions.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"limit":        map[string]any{"type": "integer", "description": "Max entries to return (default: 20)"},
				"tool_filter":  map[string]any{"type": "string", "description": "Filter by tool name (optional)"},
				"actor_filter": map[string]any{"type": "string", "description": "Filter by actor (optional)"},
			},
		},
	}, func(raw json.RawMessage) (any, error) {
		var args struct {
			Limit       int    `json:"limit"`
			ToolFilter  string `json:"tool_filter"`
			ActorFilter string `json:"actor_filter"`
		}
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("parse args: %w", err)
		}
		if args.Limit <= 0 {
			args.Limit = 20
		}

		entries, err := eng.AuditEntries(context.Background(), args.Limit, args.ToolFilter, args.ActorFilter)
		if err != nil {
			return nil, fmt.Errorf("query audit log: %w", err)
		}

		items := make([]map[string]any, 0, len(entries))
		for _, e := range entries {
			items = append(items, map[string]any{
				"id":         e.ID,
				"actor":      e.Actor,
				"tool":       e.Tool,
				"action":     e.Action.String(),
				"reason":     e.Reason,
				"created_at": e.CreatedAt.Format("2006-01-02T15:04:05Z"),
			})
		}

		return map[string]any{
			"count":   len(items),
			"entries": items,
		}, nil
	})

	// c4_guard_policy_set — add or modify a policy rule
	reg.Register(mcp.ToolSchema{
		Name:        "cq_guard_policy_set",
		Description: "Add or update a guard policy rule for a tool. action: allow | deny | audit_only.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"tool":     map[string]any{"type": "string", "description": "Tool name (use '*' to match all tools)"},
				"action":   map[string]any{"type": "string", "description": "Action: allow, deny, or audit_only"},
				"reason":   map[string]any{"type": "string", "description": "Human-readable justification (optional)"},
				"priority": map[string]any{"type": "integer", "description": "Evaluation priority (higher = checked first, default: 0)"},
			},
			"required": []string{"tool", "action"},
		},
	}, func(raw json.RawMessage) (any, error) {
		var args struct {
			Tool     string `json:"tool"`
			Action   string `json:"action"`
			Reason   string `json:"reason"`
			Priority int    `json:"priority"`
		}
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("parse args: %w", err)
		}
		if args.Tool == "" {
			return nil, fmt.Errorf("tool is required")
		}

		var action guard.Action
		switch args.Action {
		case "allow", "":
			action = guard.ActionAllow
		case "deny":
			action = guard.ActionDeny
		case "audit_only":
			action = guard.ActionAuditOnly
		default:
			return nil, fmt.Errorf("invalid action %q: must be allow, deny, or audit_only", args.Action)
		}

		rule := guard.PolicyRule{
			Tool:     args.Tool,
			Action:   action,
			Reason:   args.Reason,
			Priority: args.Priority,
		}
		if err := eng.SavePolicy(context.Background(), rule); err != nil {
			return nil, fmt.Errorf("save policy: %w", err)
		}

		return map[string]any{
			"status":   "ok",
			"tool":     args.Tool,
			"action":   action.String(),
			"priority": args.Priority,
		}, nil
	})

	// c4_guard_policy_list — list current DB-stored policies
	reg.Register(mcp.ToolSchema{
		Name:        "cq_guard_policy_list",
		Description: "List all guard policy rules currently stored in the database.",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}, func(raw json.RawMessage) (any, error) {
		policies, err := eng.ListPolicies(context.Background())
		if err != nil {
			return nil, fmt.Errorf("list policies: %w", err)
		}

		items := make([]map[string]any, 0, len(policies))
		for _, p := range policies {
			items = append(items, map[string]any{
				"tool":     p.Tool,
				"action":   p.Action.String(),
				"reason":   p.Reason,
				"priority": p.Priority,
			})
		}

		return map[string]any{
			"count":    len(items),
			"policies": items,
		}, nil
	})

	// c4_guard_role_assign — assign role to actor
	reg.Register(mcp.ToolSchema{
		Name:        "cq_guard_role_assign",
		Description: "Assign a role to an actor for RBAC enforcement.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"actor": map[string]any{"type": "string", "description": "Actor identifier (e.g. worker-abc123, user:alice)"},
				"role":  map[string]any{"type": "string", "description": "Role to assign (e.g. admin, readonly)"},
			},
			"required": []string{"actor", "role"},
		},
	}, func(raw json.RawMessage) (any, error) {
		var args struct {
			Actor string `json:"actor"`
			Role  string `json:"role"`
		}
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("parse args: %w", err)
		}
		if args.Actor == "" {
			return nil, fmt.Errorf("actor is required")
		}
		if args.Role == "" {
			return nil, fmt.Errorf("role is required")
		}

		if err := eng.AssignRole(context.Background(), args.Actor, args.Role); err != nil {
			return nil, fmt.Errorf("assign role: %w", err)
		}

		return map[string]any{
			"status": "ok",
			"actor":  args.Actor,
			"role":   args.Role,
		}, nil
	})
}
