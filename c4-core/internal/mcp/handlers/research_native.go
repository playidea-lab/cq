package handlers

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/changmin/c4-core/internal/mcp"
	"github.com/changmin/c4-core/internal/research"
)

// RegisterResearchNativeHandlers registers 5 research tools as Go native handlers.
// Replaces RegisterResearchProxyHandlers — no Python sidecar needed.
func RegisterResearchNativeHandlers(reg *mcp.Registry, store *research.Store) {
	reg.Register(mcp.ToolSchema{
		Name:        "c4_research_start",
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
	}, researchStartHandler(store))

	reg.Register(mcp.ToolSchema{
		Name:        "c4_research_status",
		Description: "Get research project status with iteration history",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"project_id": map[string]any{"type": "string", "description": "Research project ID"},
			},
			"required": []string{"project_id"},
		},
	}, researchStatusHandler(store))

	reg.Register(mcp.ToolSchema{
		Name:        "c4_research_record",
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
	}, researchRecordHandler(store))

	reg.Register(mcp.ToolSchema{
		Name:        "c4_research_approve",
		Description: "Approve iteration outcome: continue to next iteration, pause, or complete the project",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"project_id": map[string]any{"type": "string", "description": "Research project ID"},
				"action":     map[string]any{"type": "string", "description": "Action: continue (new iteration), pause, or complete", "enum": []string{"continue", "pause", "complete"}},
			},
			"required": []string{"project_id", "action"},
		},
	}, researchApproveHandler(store))

	reg.Register(mcp.ToolSchema{
		Name:        "c4_research_next",
		Description: "Suggest next action for a research project based on current state",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"project_id": map[string]any{"type": "string", "description": "Research project ID"},
			},
			"required": []string{"project_id"},
		},
	}, researchNextHandler(store))
}

func researchStartHandler(store *research.Store) mcp.HandlerFunc {
	return func(rawArgs json.RawMessage) (any, error) {
		var params struct {
			Name        string   `json:"name"`
			PaperPath   *string  `json:"paper_path"`
			RepoPath    *string  `json:"repo_path"`
			TargetScore *float64 `json:"target_score"`
		}
		if len(rawArgs) > 0 {
			if err := json.Unmarshal(rawArgs, &params); err != nil {
				return nil, fmt.Errorf("parsing arguments: %w", err)
			}
		}
		if params.Name == "" {
			return map[string]any{"error": "name is required"}, nil
		}

		targetScore := 7.0
		if params.TargetScore != nil {
			targetScore = *params.TargetScore
		}

		projectID, err := store.CreateProject(params.Name, params.PaperPath, params.RepoPath, targetScore)
		if err != nil {
			return map[string]any{"error": fmt.Sprintf("ResearchStart failed: %v", err)}, nil
		}

		iterationID, err := store.CreateIteration(projectID)
		if err != nil {
			return map[string]any{"error": fmt.Sprintf("ResearchStart failed: %v", err)}, nil
		}

		return map[string]any{
			"success":      true,
			"project_id":   projectID,
			"iteration_id": iterationID,
		}, nil
	}
}

func researchStatusHandler(store *research.Store) mcp.HandlerFunc {
	return func(rawArgs json.RawMessage) (any, error) {
		var params struct {
			ProjectID string `json:"project_id"`
		}
		if len(rawArgs) > 0 {
			json.Unmarshal(rawArgs, &params)
		}
		if params.ProjectID == "" {
			return map[string]any{"error": "project_id is required"}, nil
		}

		project, err := store.GetProject(params.ProjectID)
		if err != nil {
			return map[string]any{"error": fmt.Sprintf("ResearchStatus failed: %v", err)}, nil
		}
		if project == nil {
			return map[string]any{"error": fmt.Sprintf("Project not found: %s", params.ProjectID)}, nil
		}

		iterations, err := store.ListIterations(params.ProjectID)
		if err != nil {
			return map[string]any{"error": fmt.Sprintf("ResearchStatus failed: %v", err)}, nil
		}

		current, err := store.GetCurrentIteration(params.ProjectID)
		if err != nil {
			return map[string]any{"error": fmt.Sprintf("ResearchStatus failed: %v", err)}, nil
		}

		// Convert to JSON-friendly format matching Python's model_dump(mode="json")
		iterList := make([]any, len(iterations))
		for i, iter := range iterations {
			iterList[i] = iterationToMap(iter)
		}

		var currentMap any
		if current != nil {
			currentMap = iterationToMap(current)
		}

		return map[string]any{
			"project":           projectToMap(project),
			"iterations":        iterList,
			"current_iteration": currentMap,
		}, nil
	}
}

func researchRecordHandler(store *research.Store) mcp.HandlerFunc {
	return func(rawArgs json.RawMessage) (any, error) {
		var raw map[string]any
		if len(rawArgs) > 0 {
			json.Unmarshal(rawArgs, &raw)
		}
		projectID, _ := raw["project_id"].(string)
		if projectID == "" {
			return map[string]any{"error": "project_id is required"}, nil
		}

		current, err := store.GetCurrentIteration(projectID)
		if err != nil || current == nil {
			return map[string]any{"error": "No active iteration"}, nil
		}

		updates := map[string]any{}
		for _, key := range []string{"review_score", "axis_scores", "gaps", "experiments", "status"} {
			if v, ok := raw[key]; ok {
				updates[key] = v
			}
		}

		if len(updates) > 0 {
			if err := store.UpdateIteration(current.ID, updates); err != nil {
				return map[string]any{"error": fmt.Sprintf("ResearchRecord failed: %v", err)}, nil
			}
		}

		return map[string]any{
			"success":      true,
			"iteration_id": current.ID,
		}, nil
	}
}

func researchApproveHandler(store *research.Store) mcp.HandlerFunc {
	return func(rawArgs json.RawMessage) (any, error) {
		var params struct {
			ProjectID string `json:"project_id"`
			Action    string `json:"action"`
		}
		if len(rawArgs) > 0 {
			json.Unmarshal(rawArgs, &params)
		}
		if params.ProjectID == "" {
			return map[string]any{"error": "project_id is required"}, nil
		}
		if params.Action != "continue" && params.Action != "pause" && params.Action != "complete" {
			return map[string]any{"error": "action must be 'continue', 'pause', or 'complete'"}, nil
		}

		switch params.Action {
		case "continue":
			store.UpdateProject(params.ProjectID, map[string]any{"status": "active"})
			current, _ := store.GetCurrentIteration(params.ProjectID)
			if current != nil && current.Status != research.IterDone {
				store.UpdateIteration(current.ID, map[string]any{"status": "done"})
			}
			iterID, err := store.CreateIteration(params.ProjectID)
			if err != nil {
				return map[string]any{"error": fmt.Sprintf("ResearchApprove failed: %v", err)}, nil
			}
			return map[string]any{"success": true, "iteration_id": iterID}, nil

		case "pause":
			store.UpdateProject(params.ProjectID, map[string]any{"status": "paused"})
			return map[string]any{"success": true}, nil

		case "complete":
			store.UpdateProject(params.ProjectID, map[string]any{"status": "completed"})
			current, _ := store.GetCurrentIteration(params.ProjectID)
			if current != nil && current.Status != research.IterDone {
				store.UpdateIteration(current.ID, map[string]any{"status": "done"})
			}
			return map[string]any{"success": true}, nil
		}

		return map[string]any{"error": "unknown action"}, nil
	}
}

func researchNextHandler(store *research.Store) mcp.HandlerFunc {
	return func(rawArgs json.RawMessage) (any, error) {
		var params struct {
			ProjectID string `json:"project_id"`
		}
		if len(rawArgs) > 0 {
			json.Unmarshal(rawArgs, &params)
		}
		if params.ProjectID == "" {
			return map[string]any{"error": "project_id is required"}, nil
		}

		return store.SuggestNext(params.ProjectID), nil
	}
}

// =========================================================================
// Model → map[string]any conversion (matches Python model_dump(mode="json"))
// =========================================================================

func projectToMap(p *research.Project) map[string]any {
	m := map[string]any{
		"id":                p.ID,
		"name":              p.Name,
		"paper_path":        p.PaperPath,
		"repo_path":         p.RepoPath,
		"target_score":      p.TargetScore,
		"current_iteration": p.CurrentIteration,
		"status":            string(p.Status),
	}
	if p.CreatedAt != nil {
		m["created_at"] = p.CreatedAt.Format(time.RFC3339)
	}
	if p.UpdatedAt != nil {
		m["updated_at"] = p.UpdatedAt.Format(time.RFC3339)
	}
	return m
}

func iterationToMap(i *research.Iteration) map[string]any {
	m := map[string]any{
		"id":            i.ID,
		"project_id":    i.ProjectID,
		"iteration_num": i.IterationNum,
		"review_score":  i.ReviewScore,
		"status":        string(i.Status),
	}
	if i.AxisScores != nil {
		var v any
		json.Unmarshal(i.AxisScores, &v)
		m["axis_scores"] = v
	}
	if i.Gaps != nil {
		var v any
		json.Unmarshal(i.Gaps, &v)
		m["gaps"] = v
	}
	if i.Experiments != nil {
		var v any
		json.Unmarshal(i.Experiments, &v)
		m["experiments"] = v
	}
	if i.StartedAt != nil {
		m["started_at"] = i.StartedAt.Format(time.RFC3339)
	}
	if i.CompletedAt != nil {
		m["completed_at"] = i.CompletedAt.Format(time.RFC3339)
	}
	return m
}
