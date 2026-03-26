//go:build research

package researchhandler

import "github.com/changmin/c4-core/internal/mcp"

// RegisterResearchProxyHandlers registers MCP tools for the research loop tracker.
// All tools proxy to the Python sidecar's ResearchStore via JSON-RPC.
func RegisterResearchProxyHandlers(reg *mcp.Registry, proxy Caller) {
	reg.Register(mcp.ToolSchema{
		Name:        "cq_research_start",
		Description: "Start a research project (paper + experiments iteration loop)",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":         map[string]any{"type": "string", "description": "Project name (e.g., 'PPAD Paper 1')"},
				"paper_path":   map[string]any{"type": "string", "description": "Path to the paper file"},
				"repo_path":    map[string]any{"type": "string", "description": "Path to the experiment code repository"},
				"target_score": map[string]any{"type": "number", "description": "Target review score to achieve (default: 7.0)"},
			},
			"required": []string{"name"},
		},
	}, proxyHandler(proxy, "ResearchStart"))

	reg.Register(mcp.ToolSchema{
		Name:        "cq_research_status",
		Description: "Get research project status with iteration history",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"project_id": map[string]any{"type": "string", "description": "Research project ID"},
			},
			"required": []string{"project_id"},
		},
	}, proxyHandler(proxy, "ResearchStatus"))

	reg.Register(mcp.ToolSchema{
		Name:        "cq_research_record",
		Description: "Record review scores, gaps, or experiment results for the current iteration",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"project_id":   map[string]any{"type": "string", "description": "Research project ID"},
				"review_score": map[string]any{"type": "number", "description": "Overall review score (6-axis weighted average)"},
				"axis_scores":  map[string]any{"type": "object", "description": "Per-axis scores (e.g., {quality: 7, novelty: 5})"},
				"gaps":         map[string]any{"type": "array", "description": "Identified gaps [{type, desc, priority}]", "items": map[string]any{"type": "object"}},
				"experiments":  map[string]any{"type": "array", "description": "Experiment entries [{name, status, job_id}]", "items": map[string]any{"type": "object"}},
				"status":       map[string]any{"type": "string", "description": "Iteration status: reviewing, planning, experimenting, done"},
			},
			"required": []string{"project_id"},
		},
	}, proxyHandler(proxy, "ResearchRecord"))

	reg.Register(mcp.ToolSchema{
		Name:        "cq_research_approve",
		Description: "Approve iteration outcome: continue to next iteration, pause, or complete the project",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"project_id": map[string]any{"type": "string", "description": "Research project ID"},
				"action":     map[string]any{"type": "string", "description": "Action: continue (new iteration), pause, or complete", "enum": []string{"continue", "pause", "complete"}},
			},
			"required": []string{"project_id", "action"},
		},
	}, proxyHandler(proxy, "ResearchApprove"))

	reg.Register(mcp.ToolSchema{
		Name:        "cq_research_next",
		Description: "Suggest next action for a research project based on current state",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"project_id": map[string]any{"type": "string", "description": "Research project ID"},
			},
			"required": []string{"project_id"},
		},
	}, proxyHandler(proxy, "ResearchNext"))
}
