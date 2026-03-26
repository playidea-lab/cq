package handlers

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/changmin/c4-core/internal/mcp"
	"github.com/changmin/c4-core/internal/mcp/apps"
)

const taskGraphResourceURI = "ui://cq/task-graph"

// TaskGraphDeps holds optional dependencies for the task graph handler.
type TaskGraphDeps struct {
	ResourceStore *apps.ResourceStore
	TaskGraphHTML string
}

// RegisterTaskGraphHandler registers the task graph widget resource and adds
// format=widget support to the task graph endpoint.
func RegisterTaskGraphHandler(reg *mcp.Registry, store *SQLiteStore, deps *TaskGraphDeps) {
	if deps == nil {
		deps = &TaskGraphDeps{}
	}

	// Register the HTML widget in the resource store.
	if deps.ResourceStore != nil && deps.TaskGraphHTML != "" {
		deps.ResourceStore.Register(taskGraphResourceURI, deps.TaskGraphHTML)
	}

	reg.Register(mcp.ToolSchema{
		Name:        "cq_task_graph",
		Description: "Get task dependency graph — nodes (tasks) with edges (dependencies) and status colors",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"format": map[string]any{
					"type":        "string",
					"description": "Response format: 'widget' returns MCP Apps widget with _meta; 'text' returns plain JSON (default)",
					"enum":        []string{"widget", "text"},
				},
			},
		},
	}, func(rawArgs json.RawMessage) (any, error) {
		var args struct {
			Format string `json:"format"`
		}
		if len(rawArgs) > 0 {
			if err := json.Unmarshal(rawArgs, &args); err != nil {
				return nil, fmt.Errorf("invalid arguments: %w", err)
			}
		}
		return handleTaskGraph(store, args.Format)
	})
}

// handleTaskGraph collects task data with dependencies and returns a graph payload.
func handleTaskGraph(store *SQLiteStore, format string) (any, error) {
	tasks := collectTaskGraphData(store)

	if format == "widget" {
		return map[string]any{
			"data": tasks,
			"_meta": map[string]any{
				"ui": map[string]any{
					"resourceUri": taskGraphResourceURI,
				},
			},
		}, nil
	}

	// text (default): return task list with dependencies
	return map[string]any{"tasks": tasks}, nil
}

// taskGraphNode is a lightweight task representation for the graph widget.
type taskGraphNode struct {
	TaskID       string   `json:"task_id"`
	Title        string   `json:"title"`
	Status       string   `json:"status"`
	Dependencies []string `json:"dependencies"`
}

// collectTaskGraphData queries all tasks with their dependencies.
func collectTaskGraphData(store *SQLiteStore) []taskGraphNode {
	if store == nil {
		return nil
	}

	sfClause := store.sessionClause("WHERE", "")
	sfArgs := store.sessionArgs()

	rows, err := store.db.Query(
		"SELECT task_id, title, status, dependencies FROM c4_tasks"+sfClause+" ORDER BY priority DESC, created_at ASC",
		sfArgs...,
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "c4_task_graph: query tasks: %v\n", err)
		return nil
	}
	defer rows.Close()

	var nodes []taskGraphNode
	for rows.Next() {
		var n taskGraphNode
		var depsRaw string
		if err := rows.Scan(&n.TaskID, &n.Title, &n.Status, &depsRaw); err != nil {
			continue
		}
		if depsRaw != "" && depsRaw != "[]" {
			_ = json.Unmarshal([]byte(depsRaw), &n.Dependencies)
		}
		if n.Dependencies == nil {
			n.Dependencies = []string{}
		}
		nodes = append(nodes, n)
	}

	return nodes
}
