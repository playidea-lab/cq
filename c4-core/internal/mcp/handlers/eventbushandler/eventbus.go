//go:build c3_eventbus

package eventbushandler

import (
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/changmin/c4-core/internal/config"
	"github.com/changmin/c4-core/internal/eventbus"
	"github.com/changmin/c4-core/internal/mcp"
)

// RegisterEventBusHandlers registers c4_event_* and c4_rule_* MCP tools.
func RegisterEventBusHandlers(reg *mcp.Registry, client *eventbus.Client, cfgMgr *config.Manager) {
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
				"action_config": map[string]any{"type": "string", "description": "Action config as JSON string. For webhook, use {\"url\":\"...\"}. Shortcut: {\"channel\":\"name\"} to use a configured notification channel."},
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

		// Channel shortcut: resolve notification channel from config
		if args.ActionConfig != "" {
			resolved, err := resolveChannelConfig(args.ActionConfig, cfgMgr)
			if err != nil {
				return nil, err
			}
			args.ActionConfig = resolved
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

	// c4_notification_channels — List configured notification channels
	reg.Register(mcp.ToolSchema{
		Name:        "c4_notification_channels",
		Description: "List configured notification channels from config (notifications.channels). URL is masked for security.",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}, func(raw json.RawMessage) (any, error) {
		if cfgMgr == nil {
			return []map[string]any{}, nil
		}
		channels := cfgMgr.GetConfig().Notifications.Channels
		items := make([]map[string]any, 0, len(channels))
		for _, ch := range channels {
			template, _ := config.BuildPayloadTemplate(ch)
			items = append(items, map[string]any{
				"name":         ch.Name,
				"type":         ch.Type,
				"url_masked":   maskURL(ch.URL),
				"has_template": template != "",
			})
		}
		return items, nil
	})
}

// resolveChannelConfig checks action_config for a "channel" key.
// If found, looks up the channel in cfgMgr and injects url + payload_template + payload_content_type.
// If no "channel" key, returns action_config unchanged.
func resolveChannelConfig(actionConfig string, cfgMgr *config.Manager) (string, error) {
	var obj map[string]any
	if err := json.Unmarshal([]byte(actionConfig), &obj); err != nil {
		// Not a JSON object — pass through unchanged
		return actionConfig, nil
	}

	channelName, ok := obj["channel"].(string)
	if !ok || channelName == "" {
		// No "channel" key — pass through unchanged
		return actionConfig, nil
	}

	if cfgMgr == nil {
		return "", fmt.Errorf("channel %q specified but config manager not available", channelName)
	}

	ch := cfgMgr.GetNotificationChannel(channelName)
	if ch == nil {
		return "", fmt.Errorf("notification channel %q not found in config", channelName)
	}

	payloadTemplate, contentType := config.BuildPayloadTemplate(*ch)

	// Inject resolved fields; remove the "channel" shortcut key
	delete(obj, "channel")
	obj["url"] = ch.URL
	if payloadTemplate != "" {
		obj["payload_template"] = payloadTemplate
		obj["payload_content_type"] = contentType
		// Validate JSON template for application/json channels
		if contentType == "application/json" && !json.Valid([]byte(payloadTemplate)) {
			return "", fmt.Errorf("channel %q: payload_template is not valid JSON", channelName)
		}
	}

	result, err := json.Marshal(obj)
	if err != nil {
		return "", fmt.Errorf("marshal resolved action_config: %w", err)
	}
	return string(result), nil
}

// maskURL returns a URL with the path component replaced by "****".
// Example: "https://hook.dooray.com/services/123/456" → "https://hook.dooray.com/****"
func maskURL(rawURL string) string {
	if rawURL == "" {
		return ""
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return "****"
	}
	// Build scheme://host/****  without URL-encoding the asterisks.
	scheme := u.Scheme
	host := u.Host
	if scheme == "" || host == "" {
		return "****"
	}
	return scheme + "://" + host + "/****"
}
