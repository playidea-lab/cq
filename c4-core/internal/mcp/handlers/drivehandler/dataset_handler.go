//go:build c0_drive

package drivehandler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/changmin/c4-core/internal/drive"
	"github.com/changmin/c4-core/internal/mcp"
)

// RegisterDatasetHandlers registers c4_drive_dataset_* MCP tools.
func RegisterDatasetHandlers(reg *mcp.Registry, client *drive.DatasetClient) {
	// c4_drive_dataset_upload — Upload a local directory as a versioned dataset
	reg.Register(mcp.ToolSchema{
		Name:        "c4_drive_dataset_upload",
		Description: "Upload a local directory to C4 Drive as a versioned dataset (content-addressed storage)",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":        map[string]any{"type": "string", "description": "Local directory path to upload"},
				"name":        map[string]any{"type": "string", "description": "Dataset name"},
				"ignore_file": map[string]any{"type": "string", "description": "Optional path to a .cqdriveignore file"},
			},
			"required": []string{"path", "name"},
		},
	}, func(raw json.RawMessage) (any, error) {
		var args struct {
			Path       string `json:"path"`
			Name       string `json:"name"`
			IgnoreFile string `json:"ignore_file"`
		}
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("parse args: %w", err)
		}
		if args.Path == "" || args.Name == "" {
			return nil, errors.New("path and name are required")
		}
		result, err := client.Upload(context.Background(), args.Path, args.Name, args.IgnoreFile)
		if err != nil {
			return nil, err
		}
		return result, nil
	})

	// c4_drive_dataset_list — List dataset versions
	reg.Register(mcp.ToolSchema{
		Name:        "c4_drive_dataset_list",
		Description: "List versions of a dataset (or all datasets) in C4 Drive",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{"type": "string", "description": "Optional dataset name to filter by"},
			},
		},
	}, func(raw json.RawMessage) (any, error) {
		var args struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("parse args: %w", err)
		}
		versions, err := client.List(context.Background(), args.Name)
		if err != nil {
			return nil, err
		}
		return versions, nil
	})

	// c4_drive_dataset_pull — Download a dataset version to local directory
	reg.Register(mcp.ToolSchema{
		Name:        "c4_drive_dataset_pull",
		Description: "Pull a dataset version from C4 Drive to a local directory (incremental download)",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":    map[string]any{"type": "string", "description": "Dataset name"},
				"dest":    map[string]any{"type": "string", "description": "Local destination directory (defaults to ./<name>)"},
				"version": map[string]any{"type": "string", "description": "Version hash prefix to pull (defaults to latest)"},
			},
			"required": []string{"name"},
		},
	}, func(raw json.RawMessage) (any, error) {
		var args struct {
			Name    string `json:"name"`
			Dest    string `json:"dest"`
			Version string `json:"version"`
		}
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("parse args: %w", err)
		}
		if args.Name == "" {
			return nil, errors.New("name is required")
		}
		dest := args.Dest
		if dest == "" {
			dest = "./" + args.Name
		}
		result, err := client.Pull(context.Background(), args.Name, dest, args.Version)
		if err != nil {
			return nil, err
		}
		return result, nil
	})
}
