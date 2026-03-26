package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/changmin/c4-core/internal/fileindex"
	"github.com/changmin/c4-core/internal/mcp"
)

// RegisterFileIndexHandlers registers the cq_file_find MCP tool.
// The tool searches for files across indexed directories by name, path, or query fragment.
func RegisterFileIndexHandlers(reg *mcp.Registry, db *sql.DB) {
	reg.Register(mcp.ToolSchema{
		Name:        "cq_file_find",
		Description: "Search for files across indexed directories by name, path, or natural language query",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{"type": "string", "description": "Search query (file name, path fragment, or description)"},
				"limit": map[string]any{"type": "integer", "description": "Max results (default: 10)"},
			},
			"required": []string{"query"},
		},
	}, func(raw json.RawMessage) (any, error) {
		var args struct {
			Query string `json:"query"`
			Limit int    `json:"limit"`
		}
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("parse args: %w", err)
		}
		if args.Query == "" {
			return nil, fmt.Errorf("query is required")
		}
		if args.Limit <= 0 {
			args.Limit = 10
		}
		results, err := fileindex.Search(db, args.Query, args.Limit)
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"results": results,
			"count":   len(results),
			"query":   args.Query,
		}, nil
	})
}
