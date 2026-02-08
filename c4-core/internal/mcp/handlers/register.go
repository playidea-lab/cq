package handlers

import "github.com/changmin/c4-core/internal/mcp"

// RegisterAll registers all MCP tool handlers on the registry.
//
// Core tools (10): status, start, clear, get_task, submit, add_todo, mark_blocked, claim, report, checkpoint
// File tools (6): find_file, search_for_pattern, read_file, replace_content, create_text_file, list_dir
// Git tools (4): worktree_status, worktree_cleanup, analyze_history, search_commits
// Validation (1): run_validation
func RegisterAll(reg *mcp.Registry, store Store) {
	RegisterStateHandlers(reg, store)
	RegisterTaskHandlers(reg, store)
	RegisterTrackingHandlers(reg, store)
}

// RegisterNativeHandlers registers Go-native file, git, validation, discovery, and artifact handlers.
// These do not require Python — they operate directly on the filesystem and git.
func RegisterNativeHandlers(reg *mcp.Registry, rootDir string, store Store) {
	RegisterFileHandlers(reg, rootDir)
	RegisterGitHandlers(reg, rootDir)
	RegisterValidationHandlers(reg, rootDir)
	RegisterDiscoveryHandlers(reg, store, rootDir)
	RegisterArtifactHandlers(reg, rootDir)
}

// RegisterAllHandlers registers all MCP tool handlers including Python proxy tools.
// Total: 50 tools (10 core + 11 native + 16 proxy + 13 discovery/artifact)
func RegisterAllHandlers(reg *mcp.Registry, store Store, rootDir string, bridgeAddr string) {
	RegisterAll(reg, store)
	RegisterNativeHandlers(reg, rootDir, store)
	proxy := NewBridgeProxy(bridgeAddr)
	RegisterProxyHandlers(reg, proxy)
}
