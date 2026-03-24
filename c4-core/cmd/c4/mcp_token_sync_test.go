package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestUpdateMCPTokensInFile_UpdatesWorkerEntries(t *testing.T) {
	dir := t.TempDir()
	mcpPath := filepath.Join(dir, ".mcp.json")

	config := map[string]any{
		"mcpServers": map[string]any{
			"cq": map[string]any{
				"type":    "stdio",
				"command": "/usr/local/bin/cq",
			},
			"worker-gpu-server": map[string]any{
				"type": "http",
				"url":  "https://relay.example.com/w/gpu-server/mcp",
				"headers": map[string]any{
					"Authorization": "Bearer old-token-123",
				},
			},
			"worker-cpu-box": map[string]any{
				"type": "http",
				"url":  "https://relay.example.com/w/cpu-box/mcp",
				"headers": map[string]any{
					"Authorization": "Bearer old-token-456",
				},
			},
		},
	}
	data, _ := json.MarshalIndent(config, "", "  ")
	os.WriteFile(mcpPath, data, 0644)

	n, err := updateMCPTokensInFile(mcpPath, "fresh-token-789")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 2 {
		t.Errorf("expected 2 updates, got %d", n)
	}

	// Verify file was updated
	updated, _ := os.ReadFile(mcpPath)
	var result map[string]any
	json.Unmarshal(updated, &result)

	servers := result["mcpServers"].(map[string]any)

	// Worker entries should have new token
	for _, key := range []string{"worker-gpu-server", "worker-cpu-box"} {
		entry := servers[key].(map[string]any)
		headers := entry["headers"].(map[string]any)
		auth := headers["Authorization"].(string)
		if auth != "Bearer fresh-token-789" {
			t.Errorf("%s: expected 'Bearer fresh-token-789', got %q", key, auth)
		}
	}

	// Non-worker entry should be untouched
	cq := servers["cq"].(map[string]any)
	if cq["type"] != "stdio" {
		t.Errorf("cq entry was modified: %v", cq)
	}
}

func TestUpdateMCPTokensInFile_SkipsAlreadyCurrent(t *testing.T) {
	dir := t.TempDir()
	mcpPath := filepath.Join(dir, ".mcp.json")

	config := map[string]any{
		"mcpServers": map[string]any{
			"worker-box": map[string]any{
				"type": "http",
				"url":  "https://relay.example.com/w/box/mcp",
				"headers": map[string]any{
					"Authorization": "Bearer same-token",
				},
			},
		},
	}
	data, _ := json.MarshalIndent(config, "", "  ")
	os.WriteFile(mcpPath, data, 0644)

	n, err := updateMCPTokensInFile(mcpPath, "same-token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0 updates (token unchanged), got %d", n)
	}
}

func TestUpdateMCPTokensInFile_MissingFile(t *testing.T) {
	n, err := updateMCPTokensInFile("/nonexistent/.mcp.json", "token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0 updates for missing file, got %d", n)
	}
}

func TestFindMCPJSONPaths_IncludesProjectDir(t *testing.T) {
	dir := t.TempDir()
	mcpPath := filepath.Join(dir, ".mcp.json")
	os.WriteFile(mcpPath, []byte(`{}`), 0644)

	paths := findMCPJSONPaths(dir)
	found := false
	for _, p := range paths {
		if p == mcpPath {
			found = true
		}
	}
	if !found {
		t.Errorf("expected %s in paths, got %v", mcpPath, paths)
	}
}

func TestFindMCPJSONPaths_IncludesParent(t *testing.T) {
	parent := t.TempDir()
	child := filepath.Join(parent, "sub")
	os.MkdirAll(child, 0755)

	parentMCP := filepath.Join(parent, ".mcp.json")
	os.WriteFile(parentMCP, []byte(`{}`), 0644)

	paths := findMCPJSONPaths(child)
	found := false
	for _, p := range paths {
		if p == parentMCP {
			found = true
		}
	}
	if !found {
		t.Errorf("expected parent %s in paths, got %v", parentMCP, paths)
	}
}

func TestFindMCPJSONPaths_IncludesSibling(t *testing.T) {
	// Layout: /tmp/git/cq/ (projectDir) + /tmp/git/other-project/.mcp.json
	root := t.TempDir()
	projectDir := filepath.Join(root, "cq")
	sibling := filepath.Join(root, "other-project")
	os.MkdirAll(projectDir, 0755)
	os.MkdirAll(sibling, 0755)

	siblingMCP := filepath.Join(sibling, ".mcp.json")
	os.WriteFile(siblingMCP, []byte(`{}`), 0644)

	paths := findMCPJSONPaths(projectDir)
	found := false
	for _, p := range paths {
		if p == siblingMCP {
			found = true
		}
	}
	if !found {
		t.Errorf("expected sibling %s in paths, got %v", siblingMCP, paths)
	}
}
