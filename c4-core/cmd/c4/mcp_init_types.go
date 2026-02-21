package main

import (
	"context"
	"database/sql"
	"net/http"

	"github.com/changmin/c4-core/internal/bridge"
	"github.com/changmin/c4-core/internal/cloud"
	"github.com/changmin/c4-core/internal/config"
	"github.com/changmin/c4-core/internal/daemon"
	"github.com/changmin/c4-core/internal/eventbus"
	"github.com/changmin/c4-core/internal/knowledge"
	"github.com/changmin/c4-core/internal/llm"
	"github.com/changmin/c4-core/internal/mcp"
	"github.com/changmin/c4-core/internal/mcp/handlers"
	"github.com/changmin/c4-core/internal/research"
	"github.com/changmin/c4-core/internal/secrets"
	"github.com/changmin/c4-core/internal/serve"
)

// initContext carries shared dependencies between component init functions.
// Created by core init in newMCPServer, populated by component init hooks.
type initContext struct {
	// Core dependencies (always available)
	projectDir  string
	db          *sql.DB
	cfgMgr      *config.Manager
	reg         *mcp.Registry
	sqliteStore *handlers.SQLiteStore
	store       handlers.Store
	proxy       *handlers.BridgeProxy
	lazySidecar *bridge.LazyStarter
	secretStore *secrets.Store // global secret store (~/.c4/secrets.db)

	// Cloud (set during core init if enabled)
	cloudTP        *cloud.TokenProvider
	cloudProjectID string
	knowledgeCloud *cloud.KnowledgeCloudClient

	// Knowledge (created during core init)
	knowledgeStore    *knowledge.Store
	knowledgeSearcher *knowledge.Searcher
	knowledgeUsage    *knowledge.UsageTracker

	// Research (set by initResearch pre-store hook)
	researchStore *research.Store

	// LLM Gateway (set by initLLM pre-store hook, consumed by knowledge embedder + c1 keeper)
	llmGateway *llm.Gateway

	// GPU/Daemon (set by initGPU pre-store hook)
	daemonStore     *daemon.Store
	scheduler       *daemon.Scheduler
	schedulerCancel context.CancelFunc

	// Hub (set by initHub post-store hook)
	hubClient       hubClientInterface
	hubPollerCancel context.CancelFunc

	// C1 (set by initC1 post-store hook)
	keeper *handlers.ContextKeeper

	// EventBus (set by initEventBus post-store hook)
	embeddedEB   *eventbus.EmbeddedServer
	eventsinkSrv *http.Server

	// Gate (set by initGate post-store hook, c8_gate build tag)
	gateWebhookManager gateWebhookManagerInterface
	gateScheduler      gateSchedulerInterface

	// Guard (set by initGuard post-store hook, c6_guard build tag)
	// Typed as any to avoid importing guard in build-tag-agnostic files.
	guardEngine any

	// Agent (set by startAgentIfNeeded in mcp_init_agent.go)
	agentComp   *serve.Agent
	agentCancel context.CancelFunc
}

// hubClientInterface abstracts hub.Client so the stub doesn't need to import hub.
type hubClientInterface interface {
	IsAvailable() bool
}

// gateWebhookManagerInterface abstracts gate.WebhookManager for the initContext.
type gateWebhookManagerInterface interface{}

// gateSchedulerInterface abstracts gate.Scheduler for the initContext.
type gateSchedulerInterface interface {
	Stop()
}


// componentPreStoreHooks run before registry/proxy/sqliteStore creation.
// Use for components that only need projectDir, cfgMgr, db, reg (LLM, GPU, Research).
// Their results (llmGateway, daemonStore, researchStore) populate NativeOpts.
var componentPreStoreHooks []func(*initContext) error

// componentInitHooks run after sqliteStore and proxy are set in initContext.
// Use for components that need ctx.sqliteStore, ctx.proxy (C1, Drive, Hub, CDP, EventBus).
var componentInitHooks []func(*initContext) error

// componentEBWireHooks are called by initEventBus after an eventbus client is created.
var componentEBWireHooks []func(*initContext, *eventbus.Client)

// componentShutdownHooks are called during mcpServer.shutdown() in reverse order.
var componentShutdownHooks []func(*initContext)

// registerPreStoreHook appends a pre-store initialization function (LLM, GPU, Research).
// These run before the registry, proxy, and sqliteStore are created.
func registerPreStoreHook(fn func(*initContext) error) {
	componentPreStoreHooks = append(componentPreStoreHooks, fn)
}

// registerInitHook appends a post-store initialization function (C1, Drive, Hub, CDP, EventBus).
// These run after ctx.sqliteStore and ctx.proxy are set.
func registerInitHook(fn func(*initContext) error) {
	componentInitHooks = append(componentInitHooks, fn)
}

// registerEBWireHook appends an eventbus wiring function called by initEventBus.
func registerEBWireHook(fn func(*initContext, *eventbus.Client)) {
	componentEBWireHooks = append(componentEBWireHooks, fn)
}

// registerShutdownHook appends a shutdown cleanup function.
func registerShutdownHook(fn func(*initContext)) {
	componentShutdownHooks = append(componentShutdownHooks, fn)
}
