package handlers

import (
	"github.com/changmin/c4-core/internal/daemon"
	"github.com/changmin/c4-core/internal/knowledge"
	"github.com/changmin/c4-core/internal/llm"
	"github.com/changmin/c4-core/internal/mcp"
	"github.com/changmin/c4-core/internal/mcp/handlers/fileops"
	"github.com/changmin/c4-core/internal/mcp/handlers/gitops"
	handlerswc "github.com/changmin/c4-core/internal/mcp/handlers/webcontent"
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
	fileops.Register(reg, rootDir)
	gitops.Register(reg, rootDir)
	RegisterValidationHandlers(reg, rootDir)
	if store != nil {
		RegisterDiscoveryHandlers(reg, store, rootDir)
	}
	RegisterArtifactHandlers(reg, rootDir)
	RegisterPhaseLockHandlers(reg, rootDir)
}

// NativeOpts holds optional dependencies for native handler registration.
// Fields may be nil when their backing service is unavailable.
type NativeOpts struct {
	ResearchStore     *research.Store         // nil if research DB unavailable
	GPUStore          *daemon.Store           // nil if GPU scheduler unavailable
	GPUScheduler      *daemon.Scheduler       // nil if scheduler not running (cancel does store-only)
	KnowledgeStore    *knowledge.Store        // nil if knowledge DB unavailable
	KnowledgeSearcher *knowledge.Searcher     // nil = FTS-only (no vector search)
	KnowledgeCloud    knowledge.CloudSyncer   // nil if cloud disabled
	KnowledgeUsage    *knowledge.UsageTracker // nil if usage tracking disabled
	LLMGateway        *llm.Gateway            // nil if LLM gateway disabled
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
	// rootDir enables Go-native symbol parsing for .go files via go/ast
	RegisterProxyHandlers(reg, proxy, rootDir)

	// Register web content tools (c4_web_fetch — no dependencies)
	handlerswc.Register(reg)

	// Register native tools that replaced proxy calls.
	// Each component has its own build-tagged wrapper or is always-compiled inline.
	// Research (5 tools) — build-tagged: registerResearchNative in handlers_research_wrapper.go / handlers_research_stub.go
	registerResearchNative(reg, proxy, opts)
	// GPU (6 tools) — build-tagged: registerGPUNative in handlers_gpu_wrapper.go / handlers_gpu_stub.go
	registerGPUNative(reg, opts)
	// C2 (8 tools) — always compiled (no heavy deps)
	registerC2Native(reg, proxy)
	// Knowledge (13+ tools) — always compiled (uses interfaces from interfaces.go)
	registerKnowledgeNative(reg, proxy, opts, knowledgeCloud)

	return proxy
}

// RegisterAllHandlersLazyWithOpts is like RegisterAllHandlersWithOpts with lazy sidecar.
func RegisterAllHandlersLazyWithOpts(reg *mcp.Registry, store Store, rootDir string, lazyAddr LazyAddrGetter, knowledgeCloud KnowledgeSyncer, opts *NativeOpts) *BridgeProxy {
	return RegisterAllHandlersWithOpts(reg, store, rootDir, "", lazyAddr, knowledgeCloud, opts)
}

// registerC2Native registers C2 Workspace/Profile/Persona (6 tools) + Doc parsing (2 tools).
// Always compiled — no build tag (C2 native has no heavy external dependencies).
func registerC2Native(reg *mcp.Registry, proxy *BridgeProxy) {
	RegisterC2NativeHandlers(reg)
	RegisterC2DocProxyHandlers(reg, proxy)
}

// registerKnowledgeNative registers Knowledge tools (13+).
// Always compiled — no build tag (Knowledge native uses interfaces from interfaces.go).
func registerKnowledgeNative(reg *mcp.Registry, proxy *BridgeProxy, opts *NativeOpts, knowledgeCloud KnowledgeSyncer) {
	if opts != nil && opts.KnowledgeStore != nil {
		RegisterKnowledgeNativeHandlers(reg, &KnowledgeNativeOpts{
			Store:    opts.KnowledgeStore,
			Searcher: opts.KnowledgeSearcher,
			Cloud:    opts.KnowledgeCloud,
			Usage:    opts.KnowledgeUsage,
			LLM:      opts.LLMGateway,
		})
	} else {
		registerKnowledgeProxy(reg, proxy, knowledgeCloud)
	}
}
