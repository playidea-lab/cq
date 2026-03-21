package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/changmin/c4-core/internal/config"
	"github.com/changmin/c4-core/internal/eventbus"
	"github.com/changmin/c4-core/internal/knowledge"
	"github.com/changmin/c4-core/internal/llm"
	"github.com/changmin/c4-core/internal/mcp/handlers"
	"github.com/changmin/c4-core/internal/serve"
	"github.com/changmin/c4-core/internal/serve/hubpoller"
	"github.com/changmin/c4-core/internal/serve/hypothesissuggester"
	"github.com/changmin/c4-core/internal/serve/mcphttp"
	"github.com/changmin/c4-core/internal/serve/suggestpoller"
)

// registerCoreServeComponents registers always-available components:
// EventBus, GPU, Hub, and Agent. Returns the started EventBus component
// so optional components (EventSink, HubPoller) can wire up a publisher,
// and the GPU component so the caller can mount its HTTP handler.
// secComp provides access to the secrets store for env injection into C5.
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

	// C5 Hub subprocess
	if cfg.Serve.Hub.Enabled {
		hubEnv := loadC4CloudEnv(cfg)
		hubEnv = append(hubEnv, secComp.GetForEnv(cfg.Serve.Secrets.EnvInject)...)
		hubCfg := serve.HubComponentConfig{
			Binary: cfg.Serve.Hub.Binary,
			Port:   cfg.Serve.Hub.Port,
			Args:   cfg.Serve.Hub.Args,
			Env:    hubEnv,
		}
		// Wire embedded binary extractor when available (c5_embed build tag).
		if EmbeddedC5FS != nil {
			hubCfg.ExtractBinary = ExtractEmbeddedC5
		}
		mgr.Register(serve.NewHubComponent(hubCfg))
		fmt.Fprintf(os.Stderr, "cq serve: registered hub\n")
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

// loadC4CloudEnv returns env vars to inject into the C5 subprocess from cfg.Cloud.
func loadC4CloudEnv(cfg config.C4Config) []string {
	var envs []string
	if cfg.Cloud.URL != "" {
		envs = append(envs, "C5_SUPABASE_URL="+cfg.Cloud.URL)
	}
	if cfg.Cloud.AnonKey != "" {
		envs = append(envs, "C5_SUPABASE_KEY="+cfg.Cloud.AnonKey)
	}
	return envs
}

// registerHarnessWatcherServeComponent registers HarnessWatcherComponent, which
// captures LLM usage from ~/.claude/projects/**/*.jsonl via TraceCollector and
// (optionally) pushes journal entries to Supabase when cloud.url is configured.
// db may be nil — persistence is skipped but in-process trace recording still works.
func registerHarnessWatcherServeComponent(mgr *serve.Manager, cfg config.C4Config, db *sql.DB) {
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

	comp := serve.NewHarnessWatcherComponent(serve.HarnessWatcherConfig{
		SupabaseURL:    cfg.Cloud.URL,
		AnonKey:        cfg.Cloud.AnonKey,
		TenantID:       cfg.Cloud.ProjectID,
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
func registerKnowledgeHubPollerServeComponent(mgr *serve.Manager, cfg config.C4Config) *hubpoller.KnowledgeHubPoller {
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

	poller := hubpoller.New(hubpoller.Config{
		HubURL:       cfg.Hub.URL,
		APIKey:       cfg.Hub.APIKey,
		APIKeyEnv:    cfg.Hub.APIKeyEnv,
		APIPrefix:    cfg.Hub.APIPrefix,
		Store:        ks,
		SeenPath:     filepath.Join(projectDir, ".c4", "hub_poller_seen.json"),
		PollInterval: 30 * time.Second,
	})
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
