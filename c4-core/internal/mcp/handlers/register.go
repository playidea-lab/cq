package handlers

import (
	"github.com/changmin/c4-core/internal/daemon"
	"github.com/changmin/c4-core/internal/knowledge"
	"github.com/changmin/c4-core/internal/mcp"
	"github.com/changmin/c4-core/internal/research"
)

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
	if store != nil {
		RegisterDiscoveryHandlers(reg, store, rootDir)
	}
	RegisterArtifactHandlers(reg, rootDir)
}

// NativeOpts holds optional dependencies for native handler registration.
// Fields may be nil when their backing service is unavailable.
type NativeOpts struct {
	ResearchStore     *research.Store     // nil if research DB unavailable
	GPUStore          *daemon.Store       // nil if GPU scheduler unavailable
	KnowledgeStore    *knowledge.Store    // nil if knowledge DB unavailable
	KnowledgeSearcher *knowledge.Searcher // nil = FTS-only (no vector search)
	KnowledgeCloud    knowledge.CloudSyncer // nil if cloud disabled
}

// RegisterAllHandlers registers all MCP tool handlers including Python proxy tools.
// If store is nil, only native and proxy handlers are registered (store handlers added later).
// knowledgeCloud may be nil when cloud is disabled.
// Returns the BridgeProxy so callers can attach a Restarter for auto-recovery.
func RegisterAllHandlers(reg *mcp.Registry, store Store, rootDir string, bridgeAddr string, knowledgeCloud KnowledgeSyncer) *BridgeProxy {
	return RegisterAllHandlersWithOpts(reg, store, rootDir, bridgeAddr, nil, knowledgeCloud, nil)
}

// RegisterAllHandlersLazy is like RegisterAllHandlers but uses lazy sidecar initialization.
// The sidecar will only start when the first proxy tool is called.
func RegisterAllHandlersLazy(reg *mcp.Registry, store Store, rootDir string, lazyAddr LazyAddrGetter, knowledgeCloud KnowledgeSyncer) *BridgeProxy {
	return RegisterAllHandlersLazyWithOpts(reg, store, rootDir, lazyAddr, knowledgeCloud, nil)
}

// RegisterAllHandlersWithOpts is the full-featured registration with native opts.
func RegisterAllHandlersWithOpts(reg *mcp.Registry, store Store, rootDir string, bridgeAddr string, lazyAddr LazyAddrGetter, knowledgeCloud KnowledgeSyncer, opts *NativeOpts) *BridgeProxy {
	if store != nil {
		RegisterAll(reg, store)
	}
	RegisterNativeHandlers(reg, rootDir, store)

	var proxy *BridgeProxy
	if lazyAddr != nil {
		proxy = NewBridgeProxyLazy(lazyAddr)
	} else {
		proxy = NewBridgeProxy(bridgeAddr)
	}

	// Register proxy tools (LSP + Onboard — still Python-dependent)
	RegisterProxyHandlers(reg, proxy)

	// Register native tools that replaced proxy calls (Research, GPU, C2, Knowledge)
	registerNativeReplacements(reg, proxy, opts, knowledgeCloud)

	return proxy
}

// RegisterAllHandlersLazyWithOpts is like RegisterAllHandlersWithOpts with lazy sidecar.
func RegisterAllHandlersLazyWithOpts(reg *mcp.Registry, store Store, rootDir string, lazyAddr LazyAddrGetter, knowledgeCloud KnowledgeSyncer, opts *NativeOpts) *BridgeProxy {
	return RegisterAllHandlersWithOpts(reg, store, rootDir, "", lazyAddr, knowledgeCloud, opts)
}

// registerNativeReplacements registers the 20 tools that moved from proxy to Go native.
// Tier 1 (13): Research 5 + C2 6 + GPU 2
// Tier 2 (7): Knowledge 7
func registerNativeReplacements(reg *mcp.Registry, proxy *BridgeProxy, opts *NativeOpts, knowledgeCloud KnowledgeSyncer) {
	// Research (5 tools) — Go native
	if opts != nil && opts.ResearchStore != nil {
		RegisterResearchNativeHandlers(reg, opts.ResearchStore)
	} else {
		// Fallback: still use proxy if store unavailable
		RegisterResearchProxyHandlers(reg, proxy)
	}

	// GPU (2 tools) — Go native
	if opts != nil {
		RegisterGPUNativeHandlers(reg, opts.GPUStore)
	} else {
		RegisterGPUNativeHandlers(reg, nil)
	}

	// C2 Workspace/Profile/Persona (6 tools) — Go native
	RegisterC2NativeHandlers(reg)

	// C2 Document parsing (2 tools) — still Python proxy
	RegisterC2DocProxyHandlers(reg, proxy)

	// Knowledge (7 tools) — Go native (Tier 2) or Python proxy fallback
	if opts != nil && opts.KnowledgeStore != nil {
		RegisterKnowledgeNativeHandlers(reg, &KnowledgeNativeOpts{
			Store:    opts.KnowledgeStore,
			Searcher: opts.KnowledgeSearcher,
			Cloud:    opts.KnowledgeCloud,
		})
	} else {
		registerKnowledgeProxy(reg, proxy, knowledgeCloud)
	}
}
