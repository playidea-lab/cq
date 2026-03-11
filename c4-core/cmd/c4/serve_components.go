package main

import (
	"context"
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
func registerKnowledgeHubPollerServeComponent(mgr *serve.Manager, cfg config.C4Config) *knowledgeHubPoller {
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

	poller := newKnowledgeHubPoller(knowledgeHubPollerConfig{
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

// serveGatewayLLMCaller adapts *llm.Gateway to the LLMCaller interface used by knowledgeSuggestPoller.
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

// registerKnowledgeSuggestPollerServeComponent registers the knowledgeSuggestPoller when
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

	var caller LLMCaller
	if gw != nil {
		caller = &serveGatewayLLMCaller{gw: gw}
	}

	poller := newKnowledgeSuggestPoller(knowledgeSuggestPollerConfig{
		Store:         ks,
		LLMCaller:     caller,
		WatermarkPath: filepath.Join(projectDir, ".c4", "suggest_poller_watermark.json"),
	})
	mgr.Register(poller)
	fmt.Fprintf(os.Stderr, "cq serve: registered hub_suggest_poller\n")
}
