package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/changmin/c4-core/internal/cloud"
	"github.com/changmin/c4-core/internal/config"
	"github.com/changmin/c4-core/internal/eventbus"
	"github.com/changmin/c4-core/internal/knowledge"
	"github.com/changmin/c4-core/internal/llm"
	"github.com/changmin/c4-core/internal/mcp/handlers"
	"github.com/changmin/c4-core/internal/serve"
	"github.com/changmin/c4-core/internal/serve/hubpoller"
	"github.com/changmin/c4-core/internal/serve/hypothesissuggester"
	"github.com/changmin/c4-core/internal/serve/knowledgesync"
	"github.com/changmin/c4-core/internal/serve/mcphttp"
	"github.com/changmin/c4-core/internal/serve/sessionsummarizer"
	"github.com/changmin/c4-core/internal/serve/suggestpoller"
)

// registerCoreServeComponents registers always-available components:
// EventBus, GPU, and Agent. Returns the started EventBus component
// so optional components (EventSink, HubPoller) can wire up a publisher,
// and the GPU component so the caller can mount its HTTP handler.
func registerCoreServeComponents(mgr *serve.Manager, cfg config.C4Config, home string, secComp *secretsSyncComponent) (*serve.EventBusComponent, *serve.GPUComponent) {
	var ebComp *serve.EventBusComponent
	var gpuComp *serve.GPUComponent

	// EventBus
	if cfg.Serve.EventBus.Enabled {
		ebComp = serve.NewEventBusComponent(serve.EventBusConfigFromConfig(cfg.EventBus, projectDir))
		mgr.Register(ebComp)
		fmt.Fprintf(os.Stderr, "cq serve: registered eventbus\n")
	}

	// GPU scheduler
	if cfg.Serve.GPU.Enabled {
		dataDir := filepath.Join(home, ".c4", "daemon")
		gpuComp = serve.NewGPUComponent(serve.GPUComponentConfig{
			DataDir: dataDir,
			MaxJobs: 0,
			Version: "dev",
		})
		mgr.Register(gpuComp)
		fmt.Fprintf(os.Stderr, "cq serve: registered gpu\n")
	}

	// Agent (Supabase Realtime @cq mention listener)
	if cfg.Serve.Agent.Enabled && cfg.Cloud.URL != "" && cfg.Cloud.AnonKey != "" {
		mgr.Register(serve.NewAgent(serve.AgentConfig{
			SupabaseURL: cfg.Cloud.URL,
			APIKey:      cfg.Cloud.AnonKey,
			ProjectID:   cfg.Cloud.ProjectID,
			ProjectDir:  projectDir,
		}))
		fmt.Fprintf(os.Stderr, "cq serve: registered agent\n")
	}

	return ebComp, gpuComp
}

// registerHarnessWatcherServeComponent registers HarnessWatcherComponent, which
// captures LLM usage from ~/.claude/projects/**/*.jsonl via TraceCollector and
// (optionally) pushes journal entries to Supabase when cloud.url is configured.
// db may be nil — persistence is skipped but in-process trace recording still works.
func registerHarnessWatcherServeComponent(mgr *serve.Manager, cfg config.C4Config, db *sql.DB, cloudTP *cloud.TokenProvider, cloudProjectID string) {
	// Open knowledge store for session LLM usage report (optional — non-fatal on failure).
	var ks *knowledge.Store
	if db != nil {
		knowledgeDir := filepath.Join(projectDir, ".c4", "knowledge")
		if err := os.MkdirAll(knowledgeDir, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "cq serve: harness_watcher: mkdir knowledge %s: %v\n", knowledgeDir, err)
		} else if s, err := knowledge.NewStore(knowledgeDir); err != nil {
			fmt.Fprintf(os.Stderr, "cq serve: harness_watcher: open knowledge store failed: %v\n", err)
		} else {
			ks = s
		}
	}

	var tokenFunc func() string
	if cloudTP != nil {
		tokenFunc = cloudTP.Token
	}
	// Use resolved UUID (not slug) for Supabase project_id columns.
	tenantID := cloudProjectID
	if tenantID == "" {
		tenantID = cfg.Cloud.ProjectID
	}
	comp := serve.NewHarnessWatcherComponent(serve.HarnessWatcherConfig{
		SupabaseURL:    cfg.Cloud.URL,
		AnonKey:        cfg.Cloud.AnonKey,
		TokenFunc:      tokenFunc,
		TenantID:       tenantID,
		DB:             db,
		KnowledgeStore: ks,
	})
	mgr.Register(comp)
	fmt.Fprintf(os.Stderr, "cq serve: registered harness_watcher\n")
}

// registerStaleCheckerServeComponent registers the StaleChecker component when enabled.
// It opens the project database and creates a minimal SQLiteStore for stale-task queries.
// eb is the EventBus component used to wire a publisher (nil if EventBus is disabled).
func registerStaleCheckerServeComponent(mgr *serve.Manager, cfg config.C4Config, eb *serve.EventBusComponent) {
	if !cfg.Serve.StaleChecker.Enabled {
		return
	}

	db, err := openDB()
	if err != nil {
		fmt.Fprintf(os.Stderr, "cq serve: stale_checker: open db failed: %v\n", err)
		return
	}

	sqliteStore, err := handlers.NewSQLiteStore(db)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cq serve: stale_checker: create store failed: %v\n", err)
		db.Close()
		return
	}

	var pub eventbus.Publisher
	if eb != nil {
		pub = eb.Publisher()
	}

	mgr.Register(serve.NewStaleChecker(sqliteStore, pub, cfg.Serve.StaleChecker).WithCloser(db))
	fmt.Fprintf(os.Stderr, "cq serve: registered stale_checker\n")
}

// registerKnowledgeHubPollerServeComponent registers the knowledge HubPoller when
// hub is enabled and a hub URL is configured. It opens the project knowledge store
// and polls C5 Hub for completed jobs, recording stdout KEY=VALUE metrics as
// knowledge.TypeExperiment documents.
// Note: the poller shares the hub.enabled/hub.url gate (no separate serve.hub_poller
// toggle) because it is a lightweight side-effect of hub connectivity.
// Returns the created poller (nil if not registered).
func registerKnowledgeHubPollerServeComponent(mgr *serve.Manager, cfg config.C4Config, cloudTP *cloud.TokenProvider) *hubpoller.KnowledgeHubPoller {
	if !cfg.Hub.Enabled || cfg.Hub.URL == "" {
		return nil
	}

	knowledgeDir := filepath.Join(projectDir, ".c4", "knowledge")
	if err := os.MkdirAll(knowledgeDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "cq serve: hub_knowledge_poller: mkdir %s: %v\n", knowledgeDir, err)
		return nil
	}
	ks, err := knowledge.NewStore(knowledgeDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cq serve: hub_knowledge_poller: open knowledge store failed: %v\n", err)
		return nil
	}

	pollerCfg := hubpoller.Config{
		HubURL:       cfg.Hub.URL,
		APIKey:       cfg.Hub.APIKey,
		APIKeyEnv:    cfg.Hub.APIKeyEnv,
		APIPrefix:    cfg.Hub.APIPrefix,
		SupabaseURL:  cfg.Cloud.URL,
		SupabaseKey:  cfg.Cloud.AnonKey,
		Store:        ks,
		SeenPath:     filepath.Join(projectDir, ".c4", "hub_poller_seen.json"),
		PollInterval: 30 * time.Second,
	}
	// Wire cloud token auto-refresh so the poller survives JWT rotation.
	if cloudTP != nil && cloudTP.Token() != "" {
		pollerCfg.TokenFunc = cloudTP.Token
	}
	poller := hubpoller.New(pollerCfg)
	mgr.Register(poller)
	fmt.Fprintf(os.Stderr, "cq serve: registered hub_knowledge_poller\n")
	return poller
}

// serveGatewayLLMCaller adapts *llm.Gateway to the suggestpoller.LLMCaller interface.
type serveGatewayLLMCaller struct {
	gw *llm.Gateway
}

func (s *serveGatewayLLMCaller) Call(ctx context.Context, prompt string) (string, error) {
	req := &llm.ChatRequest{
		Messages: []llm.Message{
			{Role: "user", Content: prompt},
		},
	}
	resp, err := s.gw.Chat(ctx, "knowledge_suggest", req)
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}

// registerKnowledgeSuggestPollerServeComponent registers the suggestpoller.KnowledgeSuggestPoller when
// hub is enabled and a hub URL is configured. It shares the hub.enabled/hub.url gate with
// the hub knowledge poller.
func registerKnowledgeSuggestPollerServeComponent(mgr *serve.Manager, cfg config.C4Config, gw *llm.Gateway) {
	if !cfg.Hub.Enabled || cfg.Hub.URL == "" {
		return
	}

	knowledgeDir := filepath.Join(projectDir, ".c4", "knowledge")
	if err := os.MkdirAll(knowledgeDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "cq serve: hub_suggest_poller: mkdir %s: %v\n", knowledgeDir, err)
		return
	}
	ks, err := knowledge.NewStore(knowledgeDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cq serve: hub_suggest_poller: open knowledge store failed: %v\n", err)
		return
	}

	var caller suggestpoller.LLMCaller
	if gw != nil {
		caller = &serveGatewayLLMCaller{gw: gw}
	}

	poller := suggestpoller.New(suggestpoller.Config{
		Store:         ks,
		LLMCaller:     caller,
		WatermarkPath: filepath.Join(projectDir, ".c4", "suggest_poller_watermark.json"),
	})
	mgr.Register(poller)
	fmt.Fprintf(os.Stderr, "cq serve: registered hub_suggest_poller\n")
}

// registerHypothesisSuggesterComponent delegates to the hypothesissuggester package.
func registerHypothesisSuggesterComponent(mgr *serve.Manager, cfg config.C4Config, gw *llm.Gateway, kStore *knowledge.Store) {
	hypothesissuggester.RegisterComponent(mgr, cfg, gw, kStore)
}

// registerKnowledgeCloudSyncComponent registers the knowledge Cloud→Local sync component
// when cloud credentials are available and the knowledge store is open.
// Errors are logged only — never crashes serve.
func registerKnowledgeCloudSyncComponent(mgr *serve.Manager, kStore *knowledge.Store, cloudClient knowledgesync.CloudSyncer) {
	if kStore == nil || cloudClient == nil {
		return
	}
	comp := knowledgesync.New(kStore, cloudClient)
	mgr.Register(comp)
	fmt.Fprintf(os.Stderr, "cq serve: registered knowledge-cloud-sync\n")
}

// mcpServerRequestHandler adapts *mcpServer to the mcphttp.RequestHandler interface.
type mcpServerRequestHandler struct {
	srv *mcpServer
}

func (a *mcpServerRequestHandler) HandleRawRequest(ctx context.Context, body []byte) []byte {
	var req mcpRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return []byte(`{"jsonrpc":"2.0","error":{"code":-32700,"message":"parse error"}}`)
	}
	result := a.srv.handleRequestWithCtx(&req, ctx)
	resp, err := json.Marshal(result)
	if err != nil {
		return []byte(`{"jsonrpc":"2.0","error":{"code":-32603,"message":"internal marshal error"}}`)
	}
	return resp
}

// registerMCPHTTPComponent registers the MCP HTTP component with the serve manager.
func registerMCPHTTPComponent(mgr *serve.Manager, cfg config.ServeMCPHTTPConfig, srv *mcpServer) {
	handler := &mcpServerRequestHandler{srv: srv}
	var secretStore mcphttp.SecretGetter
	if srv.secretStore != nil {
		secretStore = srv.secretStore
	}
	mcphttp.RegisterComponent(mgr, cfg, handler, secretStore)
}

// registerSessionSummarizerServeComponent registers the SessionSummarizer component
// when a database and knowledge store are available.
// Errors are logged only — never crashes serve.
func registerSessionSummarizerServeComponent(mgr *serve.Manager, db *sql.DB, ks *knowledge.Store, gw *llm.Gateway) {
	if db == nil {
		return
	}
	comp := sessionsummarizer.New(sessionsummarizer.Config{
		DB:             db,
		KnowledgeStore: ks,
		LLMGateway:     gw,
	})
	mgr.Register(comp)
	fmt.Fprintf(os.Stderr, "cq serve: registered session-summarizer\n")
}
