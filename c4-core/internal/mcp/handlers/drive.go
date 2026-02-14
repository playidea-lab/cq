package handlers

import (
	"encoding/json"
	"fmt"

	"github.com/changmin/c4-core/internal/drive"
	"github.com/changmin/c4-core/internal/mcp"
)

// RegisterDriveHandlers registers c4_drive_* MCP tools.
func RegisterDriveHandlers(reg *mcp.Registry, driveClient *drive.Client) {
	// c4_drive_upload — Upload a local file to Drive
	reg.Register(mcp.ToolSchema{
		Name:        "c4_drive_upload",
		Description: "Upload a local file to C4 Drive (Supabase Storage)",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"local_path": map[string]any{"type": "string", "description": "Local file path to upload"},
				"drive_path": map[string]any{"type": "string", "description": "Destination path in Drive (e.g. /docs/report.pdf)"},
			},
			"required": []string{"local_path", "drive_path"},
		},
	}, func(raw json.RawMessage) (any, error) {
		var args struct {
			LocalPath string `json:"local_path"`
			DrivePath string `json:"drive_path"`
		}
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("parse args: %w", err)
		}
		if args.LocalPath == "" || args.DrivePath == "" {
			return nil, fmt.Errorf("local_path and drive_path are required")
		}
		info, err := driveClient.Upload(args.LocalPath, args.DrivePath, nil)
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"status":       "uploaded",
			"name":         info.Name,
			"path":         info.Path,
			"storage_path": info.StoragePath,
			"size_bytes":   info.SizeBytes,
			"content_hash": info.ContentHash,
		}, nil
	})

	// c4_drive_download — Download a file from Drive to local
	reg.Register(mcp.ToolSchema{
		Name:        "c4_drive_download",
		Description: "Download a file from C4 Drive to local filesystem",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"drive_path": map[string]any{"type": "string", "description": "Path in Drive (e.g. /docs/report.pdf)"},
				"local_path": map[string]any{"type": "string", "description": "Local destination path"},
			},
			"required": []string{"drive_path", "local_path"},
		},
	}, func(raw json.RawMessage) (any, error) {
		var args struct {
			DrivePath string `json:"drive_path"`
			LocalPath string `json:"local_path"`
		}
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("parse args: %w", err)
		}
		if args.DrivePath == "" || args.LocalPath == "" {
			return nil, fmt.Errorf("drive_path and local_path are required")
		}
		if err := driveClient.Download(args.DrivePath, args.LocalPath); err != nil {
			return nil, err
		}
		return map[string]any{
			"status":     "downloaded",
			"drive_path": args.DrivePath,
			"local_path": args.LocalPath,
		}, nil
	})

	// c4_drive_list — List files in a Drive folder
	reg.Register(mcp.ToolSchema{
		Name:        "c4_drive_list",
		Description: "List files and folders in a C4 Drive directory",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{"type": "string", "description": "Folder path to list (default: /)"},
			},
		},
	}, func(raw json.RawMessage) (any, error) {
		var args struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("parse args: %w", err)
		}
		if args.Path == "" {
			args.Path = "/"
		}
		files, err := driveClient.List(args.Path)
		if err != nil {
			return nil, err
		}
		items := make([]map[string]any, 0, len(files))
		for _, f := range files {
			items = append(items, map[string]any{
				"name":       f.Name,
				"path":       f.Path,
				"size_bytes": f.SizeBytes,
				"is_folder":  f.IsFolder,
				"updated_at": f.UpdatedAt,
			})
		}
		return map[string]any{
			"folder": args.Path,
			"count":  len(items),
			"files":  items,
		}, nil
	})

	// c4_drive_delete — Delete a file or folder from Drive
	reg.Register(mcp.ToolSchema{
		Name:        "c4_drive_delete",
		Description: "Delete a file or folder from C4 Drive",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{"type": "string", "description": "Path to delete (e.g. /docs/old-report.pdf)"},
			},
			"required": []string{"path"},
		},
	}, func(raw json.RawMessage) (any, error) {
		var args struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("parse args: %w", err)
		}
		if args.Path == "" {
			return nil, fmt.Errorf("path is required")
		}
		if err := driveClient.Delete(args.Path); err != nil {
			return nil, err
		}
		return map[string]any{
			"status": "deleted",
			"path":   args.Path,
		}, nil
	})

	// c4_drive_info — Get file metadata
	reg.Register(mcp.ToolSchema{
		Name:        "c4_drive_info",
		Description: "Get metadata for a file or folder in C4 Drive",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{"type": "string", "description": "File or folder path"},
			},
			"required": []string{"path"},
		},
	}, func(raw json.RawMessage) (any, error) {
		var args struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("parse args: %w", err)
		}
		if args.Path == "" {
			return nil, fmt.Errorf("path is required")
		}
		info, err := driveClient.Info(args.Path)
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"id":           info.ID,
			"name":         info.Name,
			"path":         info.Path,
			"storage_path": info.StoragePath,
			"size_bytes":   info.SizeBytes,
			"content_hash": info.ContentHash,
			"content_type": info.ContentType,
			"is_folder":    info.IsFolder,
			"created_at":   info.CreatedAt,
			"updated_at":   info.UpdatedAt,
		}, nil
	})
}
