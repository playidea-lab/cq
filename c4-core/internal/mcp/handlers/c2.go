package handlers

import (
	"time"

	"github.com/changmin/c4-core/internal/mcp"
)

// RegisterC2ProxyHandlers registers MCP tools for the C2 document lifecycle system.
// All tools proxy to the Python sidecar's C2 module via JSON-RPC.
func RegisterC2ProxyHandlers(reg *mcp.Registry, proxy *BridgeProxy) {
	// Document parsing tools (2)

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
	}, proxyHandler(proxy, "C2WorkspaceCreate"))

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
	}, proxyHandler(proxy, "C2WorkspaceLoad"))

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
	}, proxyHandler(proxy, "C2WorkspaceSave"))

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
	}, proxyHandler(proxy, "C2PersonaLearn"))

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
	}, proxyHandler(proxy, "C2ProfileLoad"))

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
	}, proxyHandler(proxy, "C2ProfileSave"))
}
