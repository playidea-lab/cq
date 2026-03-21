package handlers

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/changmin/c4-core/internal/mcp"
	"github.com/changmin/c4-core/internal/workspace"
)

// RegisterWorkspaceNativeHandlers registers 3 c4_workspace_* tools as Go native handlers.
func RegisterWorkspaceNativeHandlers(reg *mcp.Registry) {
	reg.Register(mcp.ToolSchema{
		Name:        "c4_workspace_create",
		Description: "Create a new c2 workspace with default sections for a project type",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":         map[string]any{"type": "string", "description": "Project name"},
				"project_type": map[string]any{"type": "string", "description": "Type: academic_paper, proposal, report (default: academic_paper)"},
				"goal":         map[string]any{"type": "string", "description": "One-line goal description"},
				"sections":     map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Optional custom section names"},
			},
			"required": []string{"name"},
		},
	}, workspaceCreateHandler())

	reg.Register(mcp.ToolSchema{
		Name:        "c4_workspace_load",
		Description: "Load and parse a c2_workspace.md file from a project directory",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"project_dir": map[string]any{"type": "string", "description": "Project directory containing c2_workspace.md"},
			},
			"required": []string{"project_dir"},
		},
	}, workspaceLoadHandler())

	reg.Register(mcp.ToolSchema{
		Name:        "c4_workspace_save",
		Description: "Save workspace state as c2_workspace.md in the project directory",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"project_dir": map[string]any{"type": "string", "description": "Project directory path"},
				"state":       map[string]any{"type": "object", "description": "WorkspaceState JSON object"},
			},
			"required": []string{"project_dir", "state"},
		},
	}, workspaceSaveHandler())
}

func workspaceCreateHandler() mcp.HandlerFunc {
	return func(rawArgs json.RawMessage) (any, error) {
		var params struct {
			Name        string   `json:"name"`
			ProjectType string   `json:"project_type"`
			Goal        string   `json:"goal"`
			Sections    []string `json:"sections"`
		}
		if len(rawArgs) > 0 {
			if err := json.Unmarshal(rawArgs, &params); err != nil {
				return map[string]any{"error": fmt.Sprintf("invalid arguments: %v", err)}, nil
			}
		}
		if params.Name == "" {
			return map[string]any{"error": "name is required"}, nil
		}

		pt := workspace.ProjectType(params.ProjectType)
		if pt == "" {
			pt = workspace.AcademicPaper
		}
		// Validate project type
		switch pt {
		case workspace.AcademicPaper, workspace.Proposal, workspace.Report:
			// valid
		default:
			pt = workspace.AcademicPaper
		}

		state := workspace.CreateWorkspace(params.Name, pt, params.Goal, params.Sections)
		return map[string]any{"state": state}, nil
	}
}

func workspaceLoadHandler() mcp.HandlerFunc {
	return func(rawArgs json.RawMessage) (any, error) {
		var params struct {
			ProjectDir string `json:"project_dir"`
		}
		if len(rawArgs) > 0 {
			if err := json.Unmarshal(rawArgs, &params); err != nil {
				return map[string]any{"error": fmt.Sprintf("invalid arguments: %v", err)}, nil
			}
		}
		if params.ProjectDir == "" {
			return map[string]any{"error": "project_dir is required"}, nil
		}

		wsPath := filepath.Join(params.ProjectDir, "c2_workspace.md")
		data, err := os.ReadFile(wsPath)
		if err != nil {
			if os.IsNotExist(err) {
				return map[string]any{"error": fmt.Sprintf("Workspace not found: %s", wsPath)}, nil
			}
			return map[string]any{"error": fmt.Sprintf("C2WorkspaceLoad failed: %v", err)}, nil
		}

		state := workspace.ParseWorkspace(string(data))
		return map[string]any{"state": state}, nil
	}
}

func workspaceSaveHandler() mcp.HandlerFunc {
	return func(rawArgs json.RawMessage) (any, error) {
		var raw map[string]json.RawMessage
		if len(rawArgs) > 0 {
			if err := json.Unmarshal(rawArgs, &raw); err != nil {
				return map[string]any{"error": fmt.Sprintf("invalid arguments: %v", err)}, nil
			}
		}

		var projectDir string
		if pd, ok := raw["project_dir"]; ok {
			json.Unmarshal(pd, &projectDir)
		}
		if projectDir == "" {
			return map[string]any{"error": "project_dir is required"}, nil
		}

		stateRaw, ok := raw["state"]
		if !ok || len(stateRaw) == 0 {
			return map[string]any{"error": "state is required"}, nil
		}

		var state workspace.WorkspaceState
		if err := json.Unmarshal(stateRaw, &state); err != nil {
			return map[string]any{"error": fmt.Sprintf("C2WorkspaceSave failed: %v", err)}, nil
		}

		savedPath, err := workspace.SaveWorkspace(&state, projectDir)
		if err != nil {
			return map[string]any{"error": fmt.Sprintf("C2WorkspaceSave failed: %v", err)}, nil
		}

		return map[string]any{"success": true, "path": savedPath}, nil
	}
}
