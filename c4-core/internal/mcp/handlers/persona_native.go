package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/changmin/c4-core/internal/mcp"
	"github.com/changmin/c4-core/internal/persona"
)

// RegisterPersonaNativeHandlers registers c4_persona_* and c4_profile_* tools as Go native handlers.
func RegisterPersonaNativeHandlers(reg *mcp.Registry) {
	// Persona learning tools (2)
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
			"patterns_found": len(allPatterns),
			"total_patterns": len(existing),
			"patterns_path":  patternsPath,
			"files_analyzed": len(files),
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
