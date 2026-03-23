package handlers

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/changmin/c4-core/internal/mcp"
	"github.com/changmin/c4-core/internal/mcp/apps"
)

const nodesMapResourceURI = "ui://cq/nodes-map"

// NodesMapDeps holds optional dependencies for the nodes map handler.
type NodesMapDeps struct {
	ResourceStore *apps.ResourceStore
	NodesMapHTML  string
}

// RegisterNodesMapHandler registers the nodes map widget resource and the
// c4_nodes_map MCP tool that returns connected node status cards.
func RegisterNodesMapHandler(reg *mcp.Registry, store *SQLiteStore, deps *NodesMapDeps) {
	if deps == nil {
		deps = &NodesMapDeps{}
	}

	// Register the HTML widget in the resource store.
	if deps.ResourceStore != nil && deps.NodesMapHTML != "" {
		deps.ResourceStore.Register(nodesMapResourceURI, deps.NodesMapHTML)
	}

	reg.Register(mcp.ToolSchema{
		Name:        "c4_nodes_map",
		Description: "Get connected node status — agents, workers, and edge devices with online/offline state and last heartbeat",
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
		return handleNodesMap(store, args.Format)
	})
}

// handleNodesMap collects node data and returns either a widget or text response.
func handleNodesMap(store *SQLiteStore, format string) (any, error) {
	nodes := collectNodesMapData(store)

	if format == "widget" {
		return map[string]any{
			"data": nodes,
			"_meta": map[string]any{
				"ui": map[string]any{
					"resourceUri": nodesMapResourceURI,
				},
			},
		}, nil
	}

	// text (default): return node list
	return map[string]any{"nodes": nodes}, nil
}

// nodeInfo is a lightweight node representation for the nodes map widget.
type nodeInfo struct {
	WorkerID    string `json:"worker_id"`
	NodeType    string `json:"node_type"`
	Status      string `json:"status"`
	CurrentTask string `json:"current_task,omitempty"`
	LastSeen    string `json:"last_seen,omitempty"`
}

// collectNodesMapData queries worker/agent/edge node info from the store.
func collectNodesMapData(store *SQLiteStore) []nodeInfo {
	if store == nil {
		return nil
	}

	status, err := store.GetStatus()
	if err != nil {
		fmt.Fprintf(os.Stderr, "c4_nodes_map: get status: %v\n", err)
		return nil
	}

	var nodes []nodeInfo

	// Build from Workers list in ProjectStatus
	for _, w := range status.Workers {
		n := nodeInfo{
			WorkerID:    w.ID,
			NodeType:    classifyNodeType(w.ID),
			Status:      w.Status,
			CurrentTask: w.CurrentTask,
		}
		if !w.LastSeen.IsZero() {
			n.LastSeen = w.LastSeen.Format("2006-01-02T15:04:05Z")
		}
		nodes = append(nodes, n)
	}

	// Fall back to in-progress task worker_id if Workers list is empty
	if len(nodes) == 0 {
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
				nodes = append(nodes, nodeInfo{
					WorkerID: wid,
					NodeType: classifyNodeType(wid),
					Status:   "busy",
				})
			}
		}
	}

	return nodes
}

// classifyNodeType returns the node type based on the worker_id prefix.
func classifyNodeType(id string) string {
	if len(id) >= 6 && id[:6] == "worker" {
		return "worker"
	}
	if len(id) >= 5 && id[:5] == "agent" {
		return "agent"
	}
	if len(id) >= 4 && id[:4] == "edge" {
		return "edge"
	}
	return "other"
}
