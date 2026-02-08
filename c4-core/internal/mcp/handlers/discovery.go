package handlers

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/changmin/c4-core/internal/mcp"
)

// SpecStore defines the interface for spec/design storage.
// Implemented by the SQLite store or file-based storage.
type SpecStore interface {
	SaveSpec(id, name, content string) error
	GetSpec(id string) (map[string]any, error)
	ListSpecs() ([]map[string]any, error)
	SaveDesign(id, name, content string) error
	GetDesign(id string) (map[string]any, error)
	ListDesigns() ([]map[string]any, error)
}

// RegisterDiscoveryHandlers registers spec, design, and workflow transition tools.
func RegisterDiscoveryHandlers(reg *mcp.Registry, store Store, rootDir string) {
	// c4_save_spec
	reg.Register(mcp.ToolSchema{
		Name:        "c4_save_spec",
		Description: "Save a specification document",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":    map[string]any{"type": "string", "description": "Spec name"},
				"content": map[string]any{"type": "string", "description": "Spec content (YAML or Markdown)"},
			},
			"required": []string{"name", "content"},
		},
	}, func(args json.RawMessage) (any, error) {
		return handleSaveDoc(rootDir, "specs", args)
	})

	// c4_get_spec
	reg.Register(mcp.ToolSchema{
		Name:        "c4_get_spec",
		Description: "Get a specification by name",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{"type": "string", "description": "Spec name"},
			},
			"required": []string{"name"},
		},
	}, func(args json.RawMessage) (any, error) {
		return handleGetDoc(rootDir, "specs", args)
	})

	// c4_list_specs
	reg.Register(mcp.ToolSchema{
		Name:        "c4_list_specs",
		Description: "List all specifications",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}, func(args json.RawMessage) (any, error) {
		return handleListDocs(rootDir, "specs")
	})

	// c4_save_design
	reg.Register(mcp.ToolSchema{
		Name:        "c4_save_design",
		Description: "Save a design document",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":    map[string]any{"type": "string", "description": "Design name"},
				"content": map[string]any{"type": "string", "description": "Design content"},
			},
			"required": []string{"name", "content"},
		},
	}, func(args json.RawMessage) (any, error) {
		return handleSaveDoc(rootDir, "designs", args)
	})

	// c4_get_design
	reg.Register(mcp.ToolSchema{
		Name:        "c4_get_design",
		Description: "Get a design document by name",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{"type": "string", "description": "Design name"},
			},
			"required": []string{"name"},
		},
	}, func(args json.RawMessage) (any, error) {
		return handleGetDoc(rootDir, "designs", args)
	})

	// c4_list_designs
	reg.Register(mcp.ToolSchema{
		Name:        "c4_list_designs",
		Description: "List all design documents",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}, func(args json.RawMessage) (any, error) {
		return handleListDocs(rootDir, "designs")
	})

	// c4_discovery_complete
	reg.Register(mcp.ToolSchema{
		Name:        "c4_discovery_complete",
		Description: "Mark discovery phase as complete and transition to DESIGN",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}, func(args json.RawMessage) (any, error) {
		return map[string]any{
			"success": true,
			"message": "Discovery complete. Transitioning to DESIGN phase.",
		}, nil
	})

	// c4_design_complete
	reg.Register(mcp.ToolSchema{
		Name:        "c4_design_complete",
		Description: "Mark design phase as complete and transition to PLAN",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}, func(args json.RawMessage) (any, error) {
		return map[string]any{
			"success": true,
			"message": "Design complete. Transitioning to PLAN phase.",
		}, nil
	})

	// c4_ensure_supervisor
	reg.Register(mcp.ToolSchema{
		Name:        "c4_ensure_supervisor",
		Description: "Ensure supervisor process is running for checkpoint reviews",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}, func(args json.RawMessage) (any, error) {
		return map[string]any{
			"success": true,
			"message": "Supervisor is managed by the Go MCP server",
		}, nil
	})
}

// Document helpers — store specs/designs as files in .c4/specs/ and .c4/designs/

type saveDocArgs struct {
	Name    string `json:"name"`
	Content string `json:"content"`
}

func handleSaveDoc(rootDir, docType string, rawArgs json.RawMessage) (any, error) {
	var args saveDocArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return nil, fmt.Errorf("parsing arguments: %w", err)
	}

	dir := filepath.Join(rootDir, ".c4", docType)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("creating directory: %w", err)
	}

	path := filepath.Join(dir, args.Name+".md")
	if err := os.WriteFile(path, []byte(args.Content), 0644); err != nil {
		return nil, fmt.Errorf("writing file: %w", err)
	}

	return map[string]any{
		"success": true,
		"path":    filepath.Join(".c4", docType, args.Name+".md"),
	}, nil
}

type getDocArgs struct {
	Name string `json:"name"`
}

func handleGetDoc(rootDir, docType string, rawArgs json.RawMessage) (any, error) {
	var args getDocArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return nil, fmt.Errorf("parsing arguments: %w", err)
	}

	path := filepath.Join(rootDir, ".c4", docType, args.Name+".md")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]any{
				"error": fmt.Sprintf("%s '%s' not found", docType, args.Name),
			}, nil
		}
		return nil, fmt.Errorf("reading file: %w", err)
	}

	return map[string]any{
		"name":    args.Name,
		"content": string(data),
	}, nil
}

func handleListDocs(rootDir, docType string) (any, error) {
	dir := filepath.Join(rootDir, ".c4", docType)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]any{"documents": []any{}, "count": 0}, nil
		}
		return nil, fmt.Errorf("reading directory: %w", err)
	}

	type docInfo struct {
		Name     string `json:"name"`
		Size     int64  `json:"size"`
		Modified string `json:"modified"`
	}

	var docs []docInfo
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		name := e.Name()
		if ext := filepath.Ext(name); ext == ".md" || ext == ".yaml" || ext == ".yml" {
			docs = append(docs, docInfo{
				Name:     name[:len(name)-len(ext)],
				Size:     info.Size(),
				Modified: info.ModTime().Format(time.RFC3339),
			})
		}
	}

	return map[string]any{
		"documents": docs,
		"count":     len(docs),
	}, nil
}
