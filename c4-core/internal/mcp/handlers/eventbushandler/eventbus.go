//go:build c3_eventbus

package eventbushandler

import (
	"encoding/json"
	"fmt"

	"github.com/changmin/c4-core/internal/eventbus"
	"github.com/changmin/c4-core/internal/mcp"
)

// RegisterEventBusHandlers registers c4_event_* and c4_rule_* MCP tools.
func RegisterEventBusHandlers(reg *mcp.Registry, client *eventbus.Client) {
	// c4_event_publish — Publish an event to the EventBus
	reg.Register(mcp.ToolSchema{
		Name:        "c4_event_publish",
		Description: "Publish an event to the C3 EventBus for event-driven pipelines",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"type":       map[string]any{"type": "string", "description": "Event type (e.g. drive.uploaded, task.completed)"},
				"source":     map[string]any{"type": "string", "description": "Event source (e.g. c4.drive, c4.core)"},
				"data":       map[string]any{"type": "object", "description": "JSON payload data"},
				"project_id": map[string]any{"type": "string", "description": "Optional project ID filter"},
			},
			"required": []string{"type", "source"},
		},
	}, func(raw json.RawMessage) (any, error) {
		var args struct {
			Type      string          `json:"type"`
			Source    string          `json:"source"`
			Data      json.RawMessage `json:"data,omitempty"`
			ProjectID string          `json:"project_id"`
		}
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("parse args: %w", err)
		}
		if args.Type == "" {
			return nil, fmt.Errorf("type is required")
		}
		if args.Source == "" {
			args.Source = "c4.mcp"
		}

		eventID, err := client.Publish(args.Type, args.Source, args.Data, args.ProjectID)
		if err != nil {
			return nil, fmt.Errorf("publish event: %w", err)
		}

		return map[string]any{
			"status":   "published",
			"event_id": eventID,
			"type":     args.Type,
		}, nil
	})

	// c4_event_list — List recent events
	reg.Register(mcp.ToolSchema{
		Name:        "c4_event_list",
		Description: "List recent events from the C3 EventBus",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"type":     map[string]any{"type": "string", "description": "Filter by event type (e.g. drive.uploaded)"},
				"limit":    map[string]any{"type": "integer", "description": "Max events to return (default: 20)"},
				"since_ms": map[string]any{"type": "integer", "description": "Only events after this timestamp (epoch ms)"},
			},
		},
	}, func(raw json.RawMessage) (any, error) {
		var args struct {
			Type    string `json:"type"`
			Limit   int    `json:"limit"`
			SinceMs int64  `json:"since_ms"`
		}
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("parse args: %w", err)
		}
		if args.Limit <= 0 {
			args.Limit = 20
		}

		events, err := client.ListEvents(args.Type, args.Limit, args.SinceMs)
		if err != nil {
			return nil, fmt.Errorf("list events: %w", err)
		}

		items := make([]map[string]any, 0, len(events))
		for _, ev := range events {
			var data any
			json.Unmarshal(ev.Data, &data)
			items = append(items, map[string]any{
				"id":           ev.Id,
				"type":         ev.Type,
				"source":       ev.Source,
				"data":         data,
				"project_id":   ev.ProjectId,
				"timestamp_ms": ev.TimestampMs,
			})
		}

		return map[string]any{
			"count":  len(items),
			"events": items,
		}, nil
	})

	// c4_rule_add — Add an event routing rule
	reg.Register(mcp.ToolSchema{
		Name:        "c4_rule_add",
		Description: "Add an event routing rule to the C3 EventBus",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":          map[string]any{"type": "string", "description": "Unique rule name"},
				"event_pattern": map[string]any{"type": "string", "description": "Event pattern to match (e.g. drive.*, task.completed)"},
				"filter_json":   map[string]any{"type": "string", "description": "Optional JSON filter (e.g. {\"content_type\":\"application/pdf\"})"},
				"action_type":   map[string]any{"type": "string", "description": "Action type: rpc, webhook, or log"},
				"action_config": map[string]any{"type": "string", "description": "Action config as JSON string"},
				"enabled":       map[string]any{"type": "boolean", "description": "Whether rule is enabled (default: true)"},
				"priority":      map[string]any{"type": "integer", "description": "Rule priority (higher = runs first)"},
			},
			"required": []string{"name", "event_pattern", "action_type"},
		},
	}, func(raw json.RawMessage) (any, error) {
		var args struct {
			Name         string `json:"name"`
			EventPattern string `json:"event_pattern"`
			FilterJSON   string `json:"filter_json"`
			ActionType   string `json:"action_type"`
			ActionConfig string `json:"action_config"`
			Enabled      *bool  `json:"enabled"`
			Priority     int    `json:"priority"`
		}
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("parse args: %w", err)
		}
		if args.Name == "" || args.EventPattern == "" || args.ActionType == "" {
			return nil, fmt.Errorf("name, event_pattern, and action_type are required")
		}

		enabled := true
		if args.Enabled != nil {
			enabled = *args.Enabled
		}

		ruleID, err := client.AddRule(args.Name, args.EventPattern, args.FilterJSON, args.ActionType, args.ActionConfig, enabled, args.Priority)
		if err != nil {
			return nil, fmt.Errorf("add rule: %w", err)
		}

		return map[string]any{
			"status":  "added",
			"rule_id": ruleID,
			"name":    args.Name,
		}, nil
	})

	// c4_rule_list — List all rules
	reg.Register(mcp.ToolSchema{
		Name:        "c4_rule_list",
		Description: "List all event routing rules in the C3 EventBus",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}, func(raw json.RawMessage) (any, error) {
		rules, err := client.ListRules()
		if err != nil {
			return nil, fmt.Errorf("list rules: %w", err)
		}

		items := make([]map[string]any, 0, len(rules))
		for _, r := range rules {
			items = append(items, map[string]any{
				"id":            r.Id,
				"name":          r.Name,
				"event_pattern": r.EventPattern,
				"filter_json":   r.FilterJson,
				"action_type":   r.ActionType,
				"action_config": r.ActionConfig,
				"enabled":       r.Enabled,
				"priority":      r.Priority,
			})
		}

		return map[string]any{
			"count": len(items),
			"rules": items,
		}, nil
	})

	// c4_rule_remove — Remove a rule
	reg.Register(mcp.ToolSchema{
		Name:        "c4_rule_remove",
		Description: "Remove an event routing rule from the C3 EventBus",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{"type": "string", "description": "Rule name to remove"},
			},
			"required": []string{"name"},
		},
	}, func(raw json.RawMessage) (any, error) {
		var args struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("parse args: %w", err)
		}
		if args.Name == "" {
			return nil, fmt.Errorf("name is required")
		}

		if err := client.RemoveRule("", args.Name); err != nil {
			return nil, fmt.Errorf("remove rule: %w", err)
		}

		return map[string]any{
			"status": "removed",
			"name":   args.Name,
		}, nil
	})

	// c4_rule_toggle — Enable or disable a rule
	reg.Register(mcp.ToolSchema{
		Name:        "c4_rule_toggle",
		Description: "Enable or disable an event routing rule in the C3 EventBus",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":    map[string]any{"type": "string", "description": "Rule name"},
				"enabled": map[string]any{"type": "boolean", "description": "true to enable, false to disable"},
			},
			"required": []string{"name", "enabled"},
		},
	}, func(raw json.RawMessage) (any, error) {
		var args struct {
			Name    string `json:"name"`
			Enabled bool   `json:"enabled"`
		}
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("parse args: %w", err)
		}
		if args.Name == "" {
			return nil, fmt.Errorf("name is required")
		}

		if err := client.ToggleRule(args.Name, args.Enabled); err != nil {
			return nil, fmt.Errorf("toggle rule: %w", err)
		}

		state := "enabled"
		if !args.Enabled {
			state = "disabled"
		}

		return map[string]any{
			"status": state,
			"name":   args.Name,
		}, nil
	})
}
