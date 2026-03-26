//go:build c8_gate

package handlers

import (
	"encoding/json"
	"fmt"

	"github.com/changmin/c4-core/internal/gate"
	"github.com/changmin/c4-core/internal/mcp"
)

// RegisterGateHandlers registers the c4_gate_* MCP tools.
func RegisterGateHandlers(reg *mcp.Registry, wm *gate.WebhookManager, sched *gate.Scheduler, slack *gate.SlackConnector, github *gate.GitHubConnector) {
	// c4_gate_webhook_register — register a webhook endpoint
	reg.Register(mcp.ToolSchema{
		Name:        "cq_gate_webhook_register",
		Description: "Register an outbound webhook endpoint that receives C4 events",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":   map[string]any{"type": "string", "description": "Unique name for the endpoint"},
				"url":    map[string]any{"type": "string", "description": "Destination URL for POST requests"},
				"secret": map[string]any{"type": "string", "description": "HMAC-SHA256 secret for X-Gate-Signature header (optional)"},
				"events": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Event types to subscribe (empty = all events)",
				},
			},
			"required": []string{"name", "url"},
		},
	}, func(raw json.RawMessage) (any, error) {
		var args struct {
			Name   string   `json:"name"`
			URL    string   `json:"url"`
			Secret string   `json:"secret"`
			Events []string `json:"events"`
		}
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("parse args: %w", err)
		}
		if args.Name == "" {
			return nil, fmt.Errorf("name is required")
		}
		if args.URL == "" {
			return nil, fmt.Errorf("url is required")
		}
		ep := wm.RegisterEndpoint(args.Name, args.URL, args.Secret, args.Events)
		return map[string]any{
			"status": "registered",
			"name":   ep.Name,
			"url":    ep.URL,
			"events": ep.Events,
		}, nil
	})

	// c4_gate_webhook_list — list registered endpoints
	reg.Register(mcp.ToolSchema{
		Name:        "cq_gate_webhook_list",
		Description: "List all registered outbound webhook endpoints",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}, func(raw json.RawMessage) (any, error) {
		endpoints := wm.ListEndpoints()
		items := make([]map[string]any, 0, len(endpoints))
		for _, ep := range endpoints {
			items = append(items, map[string]any{
				"name":       ep.Name,
				"url":        ep.URL,
				"has_secret": ep.Secret != "",
				"events":     ep.Events,
			})
		}
		return map[string]any{
			"endpoints": items,
			"count":     len(items),
		}, nil
	})

	// c4_gate_webhook_test — send a test event to an endpoint
	reg.Register(mcp.ToolSchema{
		Name:        "cq_gate_webhook_test",
		Description: "Send a test event to a registered webhook endpoint",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":       map[string]any{"type": "string", "description": "Name of the registered endpoint"},
				"event_type": map[string]any{"type": "string", "description": "Event type to use in the test payload (default: gate.test)"},
			},
			"required": []string{"name"},
		},
	}, func(raw json.RawMessage) (any, error) {
		var args struct {
			Name      string `json:"name"`
			EventType string `json:"event_type"`
		}
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("parse args: %w", err)
		}
		if args.Name == "" {
			return nil, fmt.Errorf("name is required")
		}
		if args.EventType == "" {
			args.EventType = "gate.test"
		}
		testData, _ := json.Marshal(map[string]string{"message": "test event from c4.gate"})
		event := gate.Event{
			ID:   fmt.Sprintf("test-%s", args.Name),
			Type: args.EventType,
			Data: testData,
		}
		if err := wm.DispatchTo(args.Name, event); err != nil {
			return nil, fmt.Errorf("dispatch test event: %w", err)
		}
		return map[string]any{
			"status":     "sent",
			"name":       args.Name,
			"event_type": args.EventType,
		}, nil
	})

	// c4_gate_schedule_add — add a cron schedule
	reg.Register(mcp.ToolSchema{
		Name:        "cq_gate_schedule_add",
		Description: "Add a recurring cron schedule (supports @every <duration>, e.g. @every 1m)",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":            map[string]any{"type": "string", "description": "Unique schedule ID (informational, stored in description)"},
				"cron":          map[string]any{"type": "string", "description": "Cron expression: @every <duration> (e.g. @every 5m)"},
				"action_type":   map[string]any{"type": "string", "description": "Type of action to run (e.g. webhook, log)"},
				"action_config": map[string]any{"type": "object", "description": "Action configuration (arbitrary JSON)"},
			},
			"required": []string{"id", "cron", "action_type"},
		},
	}, func(raw json.RawMessage) (any, error) {
		var args struct {
			ID           string         `json:"id"`
			Cron         string         `json:"cron"`
			ActionType   string         `json:"action_type"`
			ActionConfig map[string]any `json:"action_config"`
		}
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("parse args: %w", err)
		}
		if args.ID == "" {
			return nil, fmt.Errorf("id is required")
		}
		if args.Cron == "" {
			return nil, fmt.Errorf("cron is required")
		}
		if args.ActionType == "" {
			return nil, fmt.Errorf("action_type is required")
		}

		// action fires a log entry; extend via action_type for real actions
		actionDesc := fmt.Sprintf("action_type=%s id=%s", args.ActionType, args.ID)
		job, err := sched.Schedule(args.Cron, func() {
			_ = actionDesc // action placeholder
		})
		if err != nil {
			return nil, fmt.Errorf("schedule: %w", err)
		}
		return map[string]any{
			"status":      "scheduled",
			"job_id":      job.ID,
			"user_id":     args.ID,
			"cron":        args.Cron,
			"action_type": args.ActionType,
		}, nil
	})

	// c4_gate_schedule_list — list active schedules
	reg.Register(mcp.ToolSchema{
		Name:        "cq_gate_schedule_list",
		Description: "List all active cron schedules",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}, func(raw json.RawMessage) (any, error) {
		jobs, err := sched.ListJobs()
		if err != nil {
			return nil, fmt.Errorf("list jobs: %w", err)
		}
		items := make([]map[string]any, 0, len(jobs))
		for _, j := range jobs {
			items = append(items, map[string]any{
				"job_id": j.ID,
				"cron":   j.CronExpr,
			})
		}
		return map[string]any{
			"schedules": items,
			"count":     len(items),
		}, nil
	})

	// c4_gate_connector_status — check Slack/GitHub connector status
	reg.Register(mcp.ToolSchema{
		Name:        "cq_gate_connector_status",
		Description: "Check the status of Slack and GitHub connectors",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}, func(raw json.RawMessage) (any, error) {
		slackStatus := "disabled"
		if slack != nil {
			slackStatus = "enabled"
		}
		githubStatus := "disabled"
		if github != nil {
			githubStatus = "enabled"
		}
		return map[string]any{
			"connectors": map[string]any{
				"slack": map[string]any{
					"status": slackStatus,
				},
				"github": map[string]any{
					"status": githubStatus,
				},
			},
		}, nil
	})
}
