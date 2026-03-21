package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/changmin/c4-core/internal/mcp"
	"github.com/changmin/c4-core/internal/persona"
	"github.com/changmin/c4-core/internal/workspace"
)

// RegisterC2NativeHandlers registers 6 C2 tools as Go native handlers.
// Replaces proxy calls for workspace, profile, and persona tools.
// Document parsing tools (parse_document, extract_text) remain on Python proxy.
func RegisterC2NativeHandlers(reg *mcp.Registry) {
	// Workspace tools (3)
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

	// Persona learning tool (1)
	reg.Register(mcp.ToolSchema{
		Name:        "c4_persona_learn",
		Description: "Compare AI draft vs user final edit to extract writing patterns and update profile",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"draft_path":   map[string]any{"type": "string", "description": "Path to AI-generated draft"},
				"final_path":   map[string]any{"type": "string", "description": "Path to user-edited final version"},
				"profile_path": map[string]any{"type": "string", "description": "Path to .c2/profile.yaml (default: .c2/profile.yaml)"},
				"auto_apply":   map[string]any{"type": "boolean", "description": "Auto-apply discovered patterns to profile (default: false)"},
			},
			"required": []string{"draft_path", "final_path"},
		},
	}, personaLearnHandler())

	reg.Register(mcp.ToolSchema{
		Name:        "c4_persona_learn_from_diff",
		Description: "Extract coding patterns from git diff and append to raw_patterns.json. Use after polish/finish to learn from user edits.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"commit_range": map[string]any{"type": "string", "description": "Git commit range (e.g. 'HEAD~3..HEAD', 'abc123..def456'). Analyzes changed files."},
			},
			"required": []string{"commit_range"},
		},
	}, personaLearnFromDiffHandler())

	// Profile tools (2)
	reg.Register(mcp.ToolSchema{
		Name:        "c4_profile_load",
		Description: "Load c2 user profile from YAML",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"profile_path": map[string]any{"type": "string", "description": "Path to profile.yaml (default: .c2/profile.yaml)"},
			},
		},
	}, profileLoadHandler())

	reg.Register(mcp.ToolSchema{
		Name:        "c4_profile_save",
		Description: "Save c2 user profile to YAML",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"profile_path": map[string]any{"type": "string", "description": "Path to profile.yaml (default: .c2/profile.yaml)"},
				"data":         map[string]any{"type": "object", "description": "Profile data to save"},
			},
			"required": []string{"data"},
		},
	}, profileSaveHandler())
}

// RegisterC2DocProxyHandlers registers only the document parsing tools that still need Python.
func RegisterC2DocProxyHandlers(reg *mcp.Registry, proxy *BridgeProxy) {
	reg.Register(mcp.ToolSchema{
		Name:        "c4_parse_document",
		Description: "Parse multi-format document (HWP, DOCX, PDF, XLSX, PPTX) into IR blocks",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"file_path": map[string]any{"type": "string", "description": "Path to the document file"},
			},
			"required": []string{"file_path"},
		},
	}, proxyHandlerWithTimeout(proxy, "C2ParseDocument", 30*time.Second))

	reg.Register(mcp.ToolSchema{
		Name:        "c4_extract_text",
		Description: "Extract plain text from any supported document format",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"file_path": map[string]any{"type": "string", "description": "Path to the document file"},
			},
			"required": []string{"file_path"},
		},
	}, proxyHandlerWithTimeout(proxy, "C2ExtractText", 30*time.Second))
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

func personaLearnHandler() mcp.HandlerFunc {
	return func(rawArgs json.RawMessage) (any, error) {
		var params struct {
			DraftPath   string `json:"draft_path"`
			FinalPath   string `json:"final_path"`
			ProfilePath string `json:"profile_path"`
			AutoApply   bool   `json:"auto_apply"`
		}
		if len(rawArgs) > 0 {
			if err := json.Unmarshal(rawArgs, &params); err != nil {
				return map[string]any{"error": fmt.Sprintf("invalid arguments: %v", err)}, nil
			}
		}
		if params.DraftPath == "" || params.FinalPath == "" {
			return map[string]any{"error": "draft_path and final_path are required"}, nil
		}

		diff, err := persona.RunPersonaLearn(params.DraftPath, params.FinalPath, params.ProfilePath, params.AutoApply)
		if err != nil {
			return map[string]any{"error": fmt.Sprintf("C2PersonaLearn failed: %v", err)}, nil
		}

		patternMaps := make([]map[string]any, len(diff.NewPatterns))
		for i, p := range diff.NewPatterns {
			patternMaps[i] = map[string]any{
				"category":    p.Category,
				"description": p.Description,
				"frequency":   p.Frequency,
				"examples":    p.Examples,
			}
		}

		return map[string]any{
			"summary":           diff.Summary,
			"new_patterns":      patternMaps,
			"tone_updates":      diff.ToneUpdates,
			"structure_updates": diff.StructureUpdates,
		}, nil
	}
}

func personaLearnFromDiffHandler() mcp.HandlerFunc {
	return func(rawArgs json.RawMessage) (any, error) {
		var params struct {
			CommitRange string `json:"commit_range"`
		}
		if err := json.Unmarshal(rawArgs, &params); err != nil {
			return nil, fmt.Errorf("parse params: %w", err)
		}
		if params.CommitRange == "" {
			return nil, fmt.Errorf("commit_range is required")
		}

		// Get list of changed files from git diff
		cmd := exec.CommandContext(context.Background(), "git", "diff", "--name-only", params.CommitRange)
		out, err := cmd.Output()
		if err != nil {
			return map[string]any{"error": fmt.Sprintf("git diff --name-only: %v", err)}, nil
		}

		files := strings.Split(strings.TrimSpace(string(out)), "\n")
		if len(files) == 0 || (len(files) == 1 && files[0] == "") {
			return map[string]any{"patterns_found": 0, "message": "no changed files"}, nil
		}

		// For each file, get before/after content
		var allPatterns []persona.EditPattern
		parts := strings.SplitN(params.CommitRange, "..", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("commit_range must be in 'before..after' format")
		}
		beforeRef, afterRef := parts[0], parts[1]

		for _, file := range files {
			if file == "" {
				continue
			}
			// Skip non-code files
			if !isCodeFile(file) {
				continue
			}

			before, _ := exec.CommandContext(context.Background(), "git", "show", beforeRef+":"+file).Output()
			after, _ := exec.CommandContext(context.Background(), "git", "show", afterRef+":"+file).Output()

			if len(before) == 0 && len(after) == 0 {
				continue
			}

			patterns := persona.AnalyzeEdits(string(before), string(after))
			for i := range patterns {
				patterns[i].Description = fmt.Sprintf("[%s] %s", file, patterns[i].Description)
			}
			allPatterns = append(allPatterns, patterns...)
		}

		if len(allPatterns) == 0 {
			return map[string]any{"patterns_found": 0, "message": "no patterns detected"}, nil
		}

		// Append to raw_patterns.json
		username := os.Getenv("USER")
		if username == "" {
			username = "default"
		}
		patternsPath := filepath.Join(".c4", "souls", username, "raw_patterns.json")

		var existing []persona.EditPattern
		if data, err := os.ReadFile(patternsPath); err == nil && len(data) > 0 {
			_ = json.Unmarshal(data, &existing)
		}
		existing = append(existing, allPatterns...)

		// Ensure directory exists
		_ = os.MkdirAll(filepath.Dir(patternsPath), 0755)

		data, _ := json.MarshalIndent(existing, "", "  ")
		if err := os.WriteFile(patternsPath, data, 0644); err != nil {
			return map[string]any{"error": fmt.Sprintf("write raw_patterns: %v", err)}, nil
		}

		return map[string]any{
			"patterns_found":   len(allPatterns),
			"total_patterns":   len(existing),
			"patterns_path":    patternsPath,
			"files_analyzed":   len(files),
		}, nil
	}
}

// isCodeFile returns true for files likely to contain code/config patterns.
func isCodeFile(path string) bool {
	for _, ext := range []string{".go", ".py", ".ts", ".tsx", ".js", ".jsx", ".rs", ".yaml", ".yml", ".toml", ".json", ".md"} {
		if strings.HasSuffix(path, ext) {
			return true
		}
	}
	return false
}

func profileLoadHandler() mcp.HandlerFunc {
	return func(rawArgs json.RawMessage) (any, error) {
		var params struct {
			ProfilePath string `json:"profile_path"`
		}
		if len(rawArgs) > 0 {
			if err := json.Unmarshal(rawArgs, &params); err != nil {
				return map[string]any{"error": fmt.Sprintf("invalid arguments: %v", err)}, nil
			}
		}

		profile, err := persona.LoadProfile(params.ProfilePath)
		if err != nil {
			return map[string]any{"error": fmt.Sprintf("C2ProfileLoad failed: %v", err)}, nil
		}

		return map[string]any{"profile": profile}, nil
	}
}

func profileSaveHandler() mcp.HandlerFunc {
	return func(rawArgs json.RawMessage) (any, error) {
		var raw map[string]json.RawMessage
		if len(rawArgs) > 0 {
			if err := json.Unmarshal(rawArgs, &raw); err != nil {
				return map[string]any{"error": fmt.Sprintf("invalid arguments: %v", err)}, nil
			}
		}

		dataRaw, ok := raw["data"]
		if !ok || len(dataRaw) == 0 {
			return map[string]any{"error": "data (dict) is required"}, nil
		}

		var data map[string]any
		if err := json.Unmarshal(dataRaw, &data); err != nil {
			return map[string]any{"error": fmt.Sprintf("C2ProfileSave failed: invalid data: %v", err)}, nil
		}

		var profilePath string
		if pp, ok := raw["profile_path"]; ok {
			json.Unmarshal(pp, &profilePath)
		}

		if err := persona.SaveProfile(data, profilePath); err != nil {
			return map[string]any{"error": fmt.Sprintf("C2ProfileSave failed: %v", err)}, nil
		}

		return map[string]any{"success": true}, nil
	}
}
