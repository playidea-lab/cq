package handlers

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/changmin/c4-core/internal/knowledge"
	"github.com/changmin/c4-core/internal/mcp"
	"github.com/changmin/c4-core/internal/mcp/apps"
)

const dashboardResourceURI = "ui://cq/dashboard"

// DashboardDeps holds optional dependencies for the dashboard handler.
// All fields are optional; missing sources yield zero/empty values.
type DashboardDeps struct {
	KnowledgeStore *knowledge.Store    // nil if knowledge DB unavailable
	ResourceStore  *apps.ResourceStore // nil if MCP Apps infra not wired
	DashboardHTML  string              // HTML content for the dashboard widget
}

// RegisterDashboardHandler registers the c4_dashboard MCP tool and, if deps.ResourceStore
// is non-nil, registers the dashboard HTML at ui://cq/dashboard.
// It also promotes the c4_dashboard lighthouse entry to "implemented" (best-effort).
func RegisterDashboardHandler(reg *mcp.Registry, store *SQLiteStore, deps *DashboardDeps) {
	if deps == nil {
		deps = &DashboardDeps{}
	}

	// Register the HTML widget in the resource store so clients can fetch it via resources/read.
	if deps.ResourceStore != nil && deps.DashboardHTML != "" {
		deps.ResourceStore.Register(dashboardResourceURI, deps.DashboardHTML)
	}

	// Promote c4_dashboard in the lighthouse registry (best-effort, non-fatal).
	if store != nil {
		if _, err := lighthouseRegisterExisting(store, "c4_dashboard",
			"Get CQ system status dashboard — task queue, workers, knowledge, and recent activity",
			`{"type":"object","properties":{"format":{"type":"string","enum":["widget","text"]}}}`,
			"Returns status data + MCP Apps widget (_meta.ui.resourceUri=ui://cq/dashboard).",
			"auto-register",
		); err != nil {
			fmt.Fprintf(os.Stderr, "c4_dashboard: lighthouse register: %v\n", err)
		}
	}

	reg.Register(mcp.ToolSchema{
		Name:        "c4_dashboard",
		Description: "Get CQ system status dashboard — task queue, workers, knowledge, and recent activity",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"format": map[string]any{
					"type":        "string",
					"description": "Response format: 'widget' returns MCP Apps widget response with _meta; 'text' returns plain JSON (default)",
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
		return handleDashboard(store, deps, args.Format)
	})
}

// handleDashboard collects status data and returns either a widget or text response.
func handleDashboard(store *SQLiteStore, deps *DashboardDeps, format string) (any, error) {
	data := collectDashboardData(store, deps)

	if format == "widget" {
		return map[string]any{
			"data": data,
			"_meta": map[string]any{
				"ui": map[string]any{
					"resourceUri": dashboardResourceURI,
				},
			},
		}, nil
	}

	// text (default): flat JSON compatible with existing c4_status consumers
	return data, nil
}

// collectDashboardData gathers all dashboard metrics from available sources.
func collectDashboardData(store *SQLiteStore, deps *DashboardDeps) map[string]any {
	data := map[string]any{
		"status":              "online",
		"version":             "unknown",
		"memory_count":        0,
		"nodes":               map[string]int{},
		"jobs_total":          0,
		"jobs_running":        0,
		"cluster_nodes":       0,
		"cluster_sync_at":     nil,
		"recent_learnings":    []string{},
	}

	// Task/worker counts from SQLiteStore
	if store != nil {
		status, err := store.GetStatus()
		if err != nil {
			fmt.Fprintf(os.Stderr, "c4_dashboard: get status: %v\n", err)
		} else {
			data["status"] = status.State
			data["jobs_total"] = status.TotalTasks
			data["jobs_running"] = status.InProgress

			// nodes: count workers by type from in-progress tasks
			nodes := buildNodeCounts(store, status)
			data["nodes"] = nodes
			data["cluster_nodes"] = len(status.Workers)
		}
	}

	// memory_count from knowledge store
	if deps.KnowledgeStore != nil {
		docs, err := deps.KnowledgeStore.List("", "", 1000000)
		if err == nil {
			data["memory_count"] = len(docs)
			data["recent_learnings"] = extractRecentLearnings(docs, 4)
		} else {
			fmt.Fprintf(os.Stderr, "c4_dashboard: knowledge list: %v\n", err)
		}
	}

	return data
}

// buildNodeCounts returns a map of worker/agent/edge node type counts.
// Uses in-progress worker_id prefixes to classify: "worker-*", "agent-*", "edge-*".
func buildNodeCounts(store *SQLiteStore, status *ProjectStatus) map[string]int {
	counts := map[string]int{}
	if status == nil {
		return counts
	}

	// Count workers by prefix from Workers list (best-effort)
	for _, w := range status.Workers {
		switch {
		case len(w.ID) >= 6 && w.ID[:6] == "worker":
			counts["worker"]++
		case len(w.ID) >= 5 && w.ID[:5] == "agent":
			counts["agent"]++
		case len(w.ID) >= 4 && w.ID[:4] == "edge":
			counts["edge"]++
		default:
			counts["other"]++
		}
	}

	// Fall back to in-progress task worker_id column if Workers is empty
	if len(counts) == 0 && store != nil {
		rows, err := store.db.Query(
			"SELECT DISTINCT worker_id FROM c4_tasks WHERE status='in_progress' AND worker_id != ''",
		)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var wid string
				if scanErr := rows.Scan(&wid); scanErr != nil {
					continue
				}
				switch {
				case len(wid) >= 6 && wid[:6] == "worker":
					counts["worker"]++
				case len(wid) >= 5 && wid[:5] == "agent":
					counts["agent"]++
				case len(wid) >= 4 && wid[:4] == "edge":
					counts["edge"]++
				default:
					counts["other"]++
				}
			}
		}
	}

	return counts
}

// extractRecentLearnings returns up to n knowledge titles from the most recently created docs.
func extractRecentLearnings(docs []map[string]any, n int) []string {
	var titles []string
	for i := len(docs) - 1; i >= 0 && len(titles) < n; i-- {
		doc := docs[i]
		title, _ := doc["title"].(string)
		if title == "" {
			title, _ = doc["id"].(string)
		}
		if title != "" {
			titles = append(titles, title)
		}
	}
	return titles
}
