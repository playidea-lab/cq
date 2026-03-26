//go:build hub

package hubhandler

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/changmin/c4-core/internal/hub"
	"github.com/changmin/c4-core/internal/mcp"
)

func registerCronHandlers(reg *mcp.Registry, hubClient *hub.Client) {
	// c4_cron_create — Create a new cron schedule
	reg.Register(mcp.ToolSchema{
		Name:        "cq_cron_create",
		Description: "Create a new cron schedule. Provide either command (runs as a job) or dag_id (triggers a DAG).",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":      map[string]any{"type": "string", "description": "Schedule name"},
				"cron_expr": map[string]any{"type": "string", "description": "5-field cron expression (e.g. '0 * * * *')"},
				"command":   map[string]any{"type": "string", "description": "Shell command to run as a job (mutually exclusive with dag_id)"},
				"dag_id":    map[string]any{"type": "string", "description": "DAG ID to trigger (mutually exclusive with command)"},
				"priority":  map[string]any{"type": "integer", "description": "Job priority (default: 0, only used with command)"},
			},
			"required": []string{"name", "cron_expr"},
		},
	}, func(raw json.RawMessage) (any, error) {
		return handleCronCreate(hubClient, raw)
	})

	// c4_cron_list — List all cron schedules
	reg.Register(mcp.ToolSchema{
		Name:        "cq_cron_list",
		Description: "List all cron schedules",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}, func(_ json.RawMessage) (any, error) {
		return handleCronList(hubClient)
	})

	// c4_cron_delete — Delete a cron schedule
	reg.Register(mcp.ToolSchema{
		Name:        "cq_cron_delete",
		Description: "Delete a cron schedule by ID",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"cron_id": map[string]any{"type": "string", "description": "Cron schedule ID to delete"},
			},
			"required": []string{"cron_id"},
		},
	}, func(raw json.RawMessage) (any, error) {
		return handleCronDelete(hubClient, raw)
	})
}

// =========================================================================
// Cron handler implementations
// =========================================================================

func handleCronCreate(client *hub.Client, raw json.RawMessage) (any, error) {
	var params struct {
		Name     string `json:"name"`
		CronExpr string `json:"cron_expr"`
		Command  string `json:"command"`
		DagID    string `json:"dag_id"`
		Priority int    `json:"priority"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, fmt.Errorf("parsing params: %w", err)
	}
	if params.Name == "" || params.CronExpr == "" {
		return nil, fmt.Errorf("name and cron_expr are required")
	}
	if params.Command == "" && params.DagID == "" {
		return nil, fmt.Errorf("must provide either command or dag_id")
	}
	if params.Command != "" && params.DagID != "" {
		return nil, fmt.Errorf("command and dag_id are mutually exclusive")
	}

	// S-03: Validate cron expression before saving
	if _, err := hub.ParseCronExpr(params.CronExpr, time.Now()); err != nil {
		return nil, fmt.Errorf("invalid cron_expr %q: %w", params.CronExpr, err)
	}

	sched := &hub.CronSchedule{
		Name:     params.Name,
		CronExpr: params.CronExpr,
		Enabled:  true,
	}

	if params.Command != "" {
		tmpl, err := json.Marshal(map[string]any{
			"command":  params.Command,
			"priority": params.Priority,
		})
		if err != nil {
			return nil, fmt.Errorf("encoding job_template: %w", err)
		}
		sched.JobTemplate = string(tmpl)
	} else {
		sched.DagID = params.DagID
	}

	created, err := client.CreateCron(sched)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"created":   true,
		"cron_id":   created.ID,
		"name":      created.Name,
		"cron_expr": created.CronExpr,
		"dag_id":    created.DagID,
	}, nil
}

func handleCronList(client *hub.Client) (any, error) {
	schedules, err := client.ListCrons("")
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"schedules": schedules,
		"count":     len(schedules),
	}, nil
}

func handleCronDelete(client *hub.Client, raw json.RawMessage) (any, error) {
	var params struct {
		CronID string `json:"cron_id"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, fmt.Errorf("parsing params: %w", err)
	}
	if params.CronID == "" {
		return nil, fmt.Errorf("cron_id is required")
	}

	if err := client.DeleteCron(params.CronID); err != nil {
		return nil, err
	}

	return map[string]any{
		"deleted": true,
		"cron_id": params.CronID,
	}, nil
}
