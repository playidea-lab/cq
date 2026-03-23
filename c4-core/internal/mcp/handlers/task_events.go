package handlers

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/changmin/c4-core/internal/mcp"
)

// RegisterTaskEventsHandler registers the c4_task_events MCP tool.
// It reads .c4/events/task-*.json files, returns them, and deletes them.
func RegisterTaskEventsHandler(reg *mcp.Registry, projectRoot string) {
	reg.Register(mcp.ToolSchema{
		Name:        "c4_task_events",
		Description: "Poll for task status change events. Returns events since last call and clears them.",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}, func(args json.RawMessage) (any, error) {
		return handleTaskEvents(projectRoot)
	})
}

func handleTaskEvents(projectRoot string) (any, error) {
	eventsDir := filepath.Join(projectRoot, ".c4", "events")

	entries, err := os.ReadDir(eventsDir)
	if os.IsNotExist(err) {
		return map[string]any{"events": []any{}, "count": 0}, nil
	}
	if err != nil {
		return nil, err
	}

	var events []any
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		// Only process task-*.json files
		matched, err := filepath.Match("task-*.json", name)
		if err != nil || !matched {
			continue
		}

		filePath := filepath.Join(eventsDir, name)
		data, err := os.ReadFile(filePath)
		if err != nil {
			// Skip unreadable files
			continue
		}

		var event any
		if err := json.Unmarshal(data, &event); err != nil {
			// Skip malformed files
			continue
		}

		events = append(events, event)
		// Delete after successful read
		_ = os.Remove(filePath)
	}

	if events == nil {
		events = []any{}
	}

	return map[string]any{
		"events": events,
		"count":  len(events),
	}, nil
}
