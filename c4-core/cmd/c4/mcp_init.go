package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/joho/godotenv"

	"github.com/changmin/c4-core/internal/bridge"
	"github.com/changmin/c4-core/internal/chat"
	"github.com/changmin/c4-core/internal/cloud"
	"github.com/changmin/c4-core/internal/config"
	"github.com/changmin/c4-core/internal/eventbus"
	"github.com/changmin/c4-core/internal/hub"
	"github.com/changmin/c4-core/internal/knowledge"
	"github.com/changmin/c4-core/internal/llm"
	"github.com/changmin/c4-core/internal/mcp"
	"github.com/changmin/c4-core/internal/mcp/handlers"
	"github.com/changmin/c4-core/internal/secrets"
	storepackage "github.com/changmin/c4-core/internal/store"
	_ "modernc.org/sqlite"
)

// newMCPServer creates and initializes the MCP server with all tools registered.
// Initialization proceeds in four phases:
//  1. Core setup: DB, config, sidecar, cloud/token, knowledge store
//  2. componentPreStoreHooks: LLM, GPU, Research — populate NativeOpts fields
//  3. Registry + proxy + sqliteStore creation + core handler registration
//  4. componentInitHooks: C1, Drive, Hub, CDP, EventBus — require ctx.sqliteStore
func newMCPServer() (*mcpServer, error) {
	// Load .env files (best-effort, non-fatal)
	// Search: projectDir/.env, projectDir/../.env (monorepo root)
	for _, candidate := range []string{
		filepath.Join(projectDir, ".env"),
		filepath.Join(projectDir, "..", ".env"),
	} {
		if err := godotenv.Load(candidate); err == nil {
			fmt.Fprintf(os.Stderr, "cq: loaded %s\n", candidate)
			break
		}
	}

	// Open SQLite database
	db, err := openDB()
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	// Load config (non-fatal on failure)
	cfgMgr, err := config.New(projectDir, config.CloudDefaults{
		URL:     builtinSupabaseURL,
		AnonKey: builtinSupabaseKey,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "cq: config load failed (using defaults): %v\n", err)
		cfgMgr = nil
	}

	// Export permission_reviewer config to .c4/hook-config.json for bash hook.
	var hookCfg *config.C4Config
	if cfgMgr != nil {
		c := cfgMgr.GetConfig()
		hookCfg = &c
	}
	writeHookConfigJSON(projectDir, hookCfg)

	// Wire validation config so c4_run_validation prefers config.yaml commands.
	// Note: validCfg is a snapshot taken at startup. Changes to validation.*
	// via c4_config_set take effect only after MCP server restart (per config.yaml SSOT).
	if cfgMgr != nil {
		validCfg := cfgMgr.GetConfig().Validation
		handlers.SetValidationConfig(&validCfg)
	}

	// Create lazy Python sidecar (will start on first proxy tool call)
	bridgeCfg := bridge.DefaultSidecarConfig()
	bridgeCfg.PidFile = filepath.Join(projectDir, ".c4", "sidecar.pid")
	lazySidecar := bridge.NewLazyStarter(bridgeCfg)
	fmt.Fprintln(os.Stderr, "cq: Python sidecar will start on first proxy tool call")

	// Create TokenProvider and KnowledgeCloudClient if cloud is enabled
	var knowledgeCloud *cloud.KnowledgeCloudClient
	var cloudTP *cloud.TokenProvider
	var cloudProjectID string
	if cfgMgr != nil && cfgMgr.GetConfig().Cloud.Enabled {
		cloudCfg := cfgMgr.GetConfig().Cloud
		if cloudCfg.URL != "" && cloudCfg.AnonKey != "" {
			authClient := cloud.NewAuthClient(cloudCfg.URL, cloudCfg.AnonKey)
			if session, sessErr := authClient.GetSession(); sessErr == nil && session != nil {
				// Auto-refresh if token expired
				if session.ExpiresAt > 0 && time.Now().Unix() >= session.ExpiresAt {
					if refreshed, refErr := authClient.RefreshToken(); refErr == nil {
						fmt.Fprintln(os.Stderr, "cq: cloud session refreshed")
						session = refreshed
					} else {
						fmt.Fprintf(os.Stderr, "cq: cloud session expired, refresh failed: %v\n", refErr)
					}
				}
				cloudTP = cloud.NewTokenProvider(session.AccessToken, session.ExpiresAt, authClient)
			}
			cloudProjectID = cloudCfg.ProjectID
			if cloudProjectID == "" {
				cloudProjectID = cfgMgr.GetConfig().ProjectID
			}
			// Resolve project name to UUID if not already a UUID
			if cloudTP != nil && cloudProjectID != "" && !isUUID(cloudProjectID) {
				if uuid, err := resolveProjectUUID(cloudCfg.URL, cloudCfg.AnonKey, cloudTP.Token(), cloudProjectID); err == nil {
					fmt.Fprintf(os.Stderr, "cq: cloud project %q → %s\n", cloudProjectID, uuid)
					cloudProjectID = uuid
				} else {
					fmt.Fprintf(os.Stderr, "cq: could not resolve project UUID: %v\n", err)
				}
			}
			if cloudTP == nil {
				cloudTP = cloud.NewStaticTokenProvider("")
			}
			knowledgeCloud = cloud.NewKnowledgeCloudClient(
				cloudCfg.URL+"/rest/v1", cloudCfg.AnonKey, cloudTP, cloudProjectID)
			// Write ~/.c4/supabase.json for Rust c1 app (list_channels, get_project_id_cmd)
			writeSupabaseJSON(cloudCfg.URL, cloudCfg.AnonKey)
		}
	}
	if cloudTP == nil {
		cloudTP = cloud.NewStaticTokenProvider("")
	}

	// Open global secret store early (used by toLLMGatewayConfig for API key fallback).
	// Opened before knowledge/LLM init so the same instance can be reused throughout.
	var secretStore *secrets.Store
	if ss, secErr := secrets.New(); secErr != nil {
		fmt.Fprintf(os.Stderr, "cq: secret store init failed: %v\n", secErr)
		fmt.Fprintln(os.Stderr, "cq: c4_secret_* tools not registered (store unavailable)")
	} else {
		secretStore = ss
		fmt.Fprintln(os.Stderr, "cq: secret store ready (~/.c4/secrets.db)")
	}

	// Create knowledge store (optional — native knowledge tools use this, Tier 2)
	knowledgeDir := filepath.Join(projectDir, ".c4", "knowledge")
	os.MkdirAll(knowledgeDir, 0755)
	var knowledgeStore *knowledge.Store
	var knowledgeSearcher *knowledge.Searcher
	var knowledgeUsage *knowledge.UsageTracker
	if ks, ksErr := knowledge.NewStore(knowledgeDir); ksErr != nil {
		fmt.Fprintf(os.Stderr, "cq: knowledge store init failed (proxy fallback): %v\n", ksErr)
	} else {
		knowledgeStore = ks

		// Create embedder from LLM Gateway
		var embedder knowledge.Embedder
		embDim := 1536
		if cfgMgr != nil && cfgMgr.GetConfig().LLMGateway.Enabled {
			gwCfg := cfgMgr.GetConfig().LLMGateway
			embGateway := llm.NewGatewayFromConfig(toLLMGatewayConfig(cfgMgr, secretStore))

			// Configure embedding route from config (embedding_provider / embedding_model)
			embProvider := gwCfg.EmbeddingProvider
			embModel := gwCfg.EmbeddingModel
			if embProvider != "" {
				embGateway.SetRoute("embedding", llm.ModelRef{Provider: embProvider, Model: embModel})
			}

			// Determine dimension based on provider/model
			ref := embGateway.Resolve("embedding", "")
			if ref.Provider == "ollama" {
				embDim = 768 // nomic-embed-text default
				fmt.Fprintf(os.Stderr, "cq: knowledge embeddings via ollama/%s (%dd)\n", ref.Model, embDim)
			} else {
				fmt.Fprintf(os.Stderr, "cq: knowledge embeddings via %s/%s (%dd)\n", ref.Provider, ref.Model, embDim)
			}

			embedder = llm.NewEmbeddingProvider(embGateway, embDim)
		} else {
			embDim = 384 // fallback to mock dimension
			fmt.Fprintln(os.Stderr, "cq: knowledge using mock embeddings (384d)")
		}

		// Create vector store + searcher for hybrid search
		if vs, vsErr := knowledge.NewVectorStore(ks.DB(), embDim, embedder); vsErr != nil {
			fmt.Fprintf(os.Stderr, "cq: knowledge vector store init failed (FTS-only): %v\n", vsErr)
			knowledgeSearcher = knowledge.NewSearcher(ks, nil)
		} else {
			knowledgeSearcher = knowledge.NewSearcher(ks, vs)
		}

		// Create usage tracker for popularity-boosted search (3-way RRF)
		if ut, utErr := knowledge.NewUsageTracker(ks.DB()); utErr != nil {
			fmt.Fprintf(os.Stderr, "cq: knowledge usage tracker init failed: %v\n", utErr)
		} else {
			knowledgeUsage = ut
			if knowledgeSearcher != nil {
				knowledgeSearcher.SetUsageTracker(ut)
			}
		}
	}

	// --- Phase 1: Create registry and initContext with core fields ---
	reg := mcp.NewRegistry()
	ctx := &initContext{
		projectDir:        projectDir,
		db:                db,
		cfgMgr:            cfgMgr,
		reg:               reg,
		lazySidecar:       lazySidecar,
		cloudTP:           cloudTP,
		cloudProjectID:    cloudProjectID,
		knowledgeCloud:    knowledgeCloud,
		knowledgeStore:    knowledgeStore,
		knowledgeSearcher: knowledgeSearcher,
		knowledgeUsage:    knowledgeUsage,
		secretStore:       secretStore,
	}

	// --- Phase 2: Run pre-store hooks (LLM, GPU, Research) ---
	// These populate ctx.llmGateway, ctx.daemonStore, ctx.researchStore
	// which are consumed by NativeOpts before handler registration.
	for _, fn := range componentPreStoreHooks {
		if err := fn(ctx); err != nil {
			return nil, fmt.Errorf("pre-store component init: %w", err)
		}
	}

	// --- Phase 3: Create proxy, sqliteStore, hybridStore, register core handlers ---
	// Initialize global knowledge manager (non-fatal: nil if home dir unavailable)
	globalKnowledgeManager := knowledge.NewGlobalKnowledgeManager()

	nativeOpts := &handlers.NativeOpts{
		ResearchStore:          ctx.researchStore,
		KnowledgeStore:         knowledgeStore,
		KnowledgeSearcher:      knowledgeSearcher,
		KnowledgeUsage:         knowledgeUsage,
		KnowledgeGlobalManager: globalKnowledgeManager,
		LLMGateway:             ctx.llmGateway,
		GPUStore:               ctx.daemonStore,
		GPUScheduler:           ctx.scheduler,
	}
	// Avoid typed-nil interface bug: only assign when concrete pointer is non-nil.
	// A nil *cloud.KnowledgeCloudClient assigned to an interface field creates a
	// non-nil interface (typed nil), causing opts.Cloud != nil to be true and
	// subsequent method calls to panic with nil pointer dereference.
	var kcForProxy handlers.KnowledgeSyncer
	if knowledgeCloud != nil {
		nativeOpts.KnowledgeCloud = knowledgeCloud
		kcForProxy = knowledgeCloud
	}
	proxy := handlers.RegisterAllHandlersLazyWithOpts(reg, nil, projectDir, lazySidecar, kcForProxy, nativeOpts)
	ctx.proxy = proxy

	// Wire proxy restart and sidecar health check onRestart callback
	proxy.SetRestarter(lazySidecar)
	lazySidecar.SetOnRestart(func(newAddr string) {
		proxy.UpdateAddr(newAddr)
	})

	// Create store with all options
	sessionID := os.Getenv("CQ_SESSION_NAME")
	if sessionID == "" {
		sessionID = fmt.Sprintf("pid-%d", os.Getpid())
	}
	storeOpts := []handlers.StoreOption{
		handlers.WithProjectRoot(projectDir),
		handlers.WithSessionID(sessionID),
	}
	if cfgMgr != nil {
		storeOpts = append(storeOpts, handlers.WithConfig(cfgMgr))
	}
	if proxy != nil {
		storeOpts = append(storeOpts, handlers.WithProxy(proxy))
	}
	if knowledgeStore != nil || knowledgeSearcher != nil {
		w, r, s := handlers.AdaptKnowledge(knowledgeStore, knowledgeSearcher)
		storeOpts = append(storeOpts, handlers.WithKnowledge(w, r, s))
	}
	if knowledgeStore != nil || knowledgeSearcher != nil {
		knowledgeHitTracker := handlers.NewKnowledgeHitTracker()
		storeOpts = append(storeOpts, handlers.WithKnowledgeHitTracker(knowledgeHitTracker))
	}
	storeOpts = append(storeOpts, handlers.WithRegistry(reg))
	sqliteStore, err := handlers.NewSQLiteStore(db, storeOpts...)
	if err != nil {
		return nil, fmt.Errorf("creating store: %w", err)
	}
	ctx.sqliteStore = sqliteStore

	// Wrap with cloud store if cloud is enabled.
	// cloud.mode selects the strategy:
	//   "local-first"   (default) → HybridStore: writes local first, async push to cloud
	//   "cloud-primary"           → CloudPrimaryStore: writes cloud first, async sync to local
	var store handlers.Store = sqliteStore
	if cfgMgr != nil && cfgMgr.GetConfig().Cloud.Enabled {
		cloudCfg := cfgMgr.GetConfig().Cloud
		if cloudCfg.URL != "" && cloudCfg.AnonKey != "" {
			cloudURL := cloudCfg.URL + "/rest/v1"
			cloudStore := cloud.NewCloudStore(cloudURL, cloudCfg.AnonKey, cloudTP, cloudProjectID)
			store = selectCloudStore(cloudCfg.Mode, sqliteStore, cloudStore)
			fmt.Fprintf(os.Stderr, "cq: cloud sync enabled (%s mode)\n", cloudCfg.Mode)
		} else {
			fmt.Fprintln(os.Stderr, "cq: cloud enabled but URL/key not configured, using local only")
		}
	}
	ctx.store = store

	// Register core handlers (task, state, discovery, persona, team, soul, twin, lighthouse).
	// Note: RegisterAll and RegisterDiscoveryHandlers accept handlers.Store interface,
	// so they work with both SQLiteStore and HybridStore.
	// Persona, Twin, and Lighthouse handlers require *SQLiteStore directly
	// (they use SQLite-specific queries like DetectPatterns, growth snapshots).
	handlers.RegisterAll(reg, store)
	handlers.RegisterDiscoveryHandlers(reg, store, projectDir)
	handlers.RegisterPersonaHandlers(reg, sqliteStore)
	handlers.RegisterTeamHandlers(reg, projectDir)
	handlers.RegisterSoulHandlers(reg, projectDir)
	handlers.RegisterTwinHandlers(reg, sqliteStore)
	handlers.RegisterPopReflectHandlers(reg, knowledgeStore)
	handlers.RegisterLighthouseHandlers(reg, sqliteStore)
	if n := handlers.LoadLighthousesOnStartup(reg, sqliteStore); n > 0 {
		fmt.Fprintf(os.Stderr, "cq: %d lighthouse stubs loaded\n", n)
	}

	// Auto-register CLI commands in lighthouse so agents can discover them.
	if cliCmds := collectCLICommands(rootCmd, "cq"); len(cliCmds) > 0 {
		if n := handlers.RegisterCLICommands(sqliteStore, cliCmds); n > 0 {
			fmt.Fprintf(os.Stderr, "cq: %d CLI commands registered in lighthouse\n", n)
		}
	}

	// Manual recovery tools for stuck workers.
	handlers.RegisterTaskAdminHandlers(reg, sqliteStore)

	// Implicit heartbeat — refresh active worker's task updated_at on every tool call,
	// so the 30-min stale timeout isn't triggered by genuine work.
	reg.OnCall = sqliteStore.TouchCurrentWorkerHeartbeat

	// Set project role for Soul stage integration
	projectName := filepath.Base(projectDir)
	if projectName != "" && projectName != "." {
		handlers.SetProjectRoleForStage("project-" + projectName)
	}

	// Register health check handler
	handlers.RegisterHealthHandler(reg, &handlers.HealthDeps{
		DB:             db,
		Sidecar:        lazySidecar,
		KnowledgeStore: knowledgeStore,
		StartTime:      time.Now(),
	})

	// Register config handlers
	handlers.RegisterConfigHandlers(reg, cfgMgr, projectDir)

	// Register secret handlers using the already-open store from ctx.
	if ctx.secretStore != nil {
		handlers.RegisterSecretHandlers(reg, ctx.secretStore)
	}

	// Build chat router (best-effort): mirrors c4_notify / c4_mail_send to c1_messages.
	// Requires CQ_CHAT_CHANNEL_ID env var + Supabase cloud credentials.
	var chatRouter *chat.Router
	if channelID := os.Getenv("CQ_CHAT_CHANNEL_ID"); channelID != "" && cfgMgr != nil {
		cloudCfgChat := cfgMgr.GetConfig().Cloud
		if cloudCfgChat.URL != "" && cloudCfgChat.AnonKey != "" {
			var accessToken string
			if cloudTP != nil {
				accessToken = cloudTP.Token()
			}
			chatClient := chat.New(cloudCfgChat.URL, cloudCfgChat.AnonKey, accessToken)
			if r, routerErr := chat.NewRouter(chatClient, channelID); routerErr == nil {
				chatRouter = r
				fmt.Fprintf(os.Stderr, "cq: chat router active (channel %s)\n", channelID)
			} else {
				fmt.Fprintf(os.Stderr, "cq: chat router init failed: %v\n", routerErr)
			}
		}
	}
	ctx.chatRouter = chatRouter

	// Register notification handlers (c4_notification_set, c4_notification_get, c4_notify).
	handlers.RegisterNotifyHandlers(reg, projectDir, chatRouter)

	// Register experiment registry handlers (c4_experiment_register,
	// c4_run_checkpoint, c4_run_complete, c4_run_should_continue).
	// When hub.url is configured the handlers proxy requests to the Hub API;
	// otherwise the local SQLite store is used as fallback.
	if expStore, expErr := storepackage.NewSQLiteExperimentStore(db); expErr != nil {
		fmt.Fprintf(os.Stderr, "cq: experiment store init failed: %v\n", expErr)
	} else {
		expHandlers := handlers.ExperimentHandlers{Store: expStore}
		if knowledgeStore != nil {
			expHandlers.KnowledgeRecord = func(_ context.Context, title, content, domain string) error {
				meta := map[string]any{"title": title, "domain": domain}
				_, err := knowledgeStore.Create(knowledge.TypeInsight, meta, content)
				return err
			}
		}
		if cfgMgr != nil {
			if hubURL := cfgMgr.GetConfig().Hub.URL; hubURL != "" {
				expHandlers.HubBaseURL = hubURL
				if secretStore != nil {
					if v, err := secretStore.Get("hub.api_key"); err == nil && v != "" {
						expHandlers.HubAPIKey = v
					}
				}
				if expHandlers.HubAPIKey == "" {
					expHandlers.HubAPIKey = cfgMgr.GetConfig().Hub.APIKey
				}
			}
		}
		handlers.RegisterExperimentHandlers(reg, expHandlers)
	}

	// --- Phase 4: Run post-store hooks (C1, Drive, Hub, CDP, EventBus) ---
	// ctx.sqliteStore and ctx.proxy are now set; EventBus wiring can proceed.
	for _, fn := range componentInitHooks {
		if err := fn(ctx); err != nil {
			return nil, fmt.Errorf("component init: %w", err)
		}
	}

	// Start HubPoller after EventBus wiring so hubEventPub is populated.
	// startHubPoller is defined in mcp_init_hub.go (hub) / mcp_init_hub_stub.go (!hub).
	startHubPoller(ctx)

	// Start EventSink HTTP server (config from .c4/config.yaml).
	// startEventSink is defined in mcp_init_eventbus.go (c3_eventbus) / mcp_init_eventbus_stub.go.
	startEventSink(ctx)

	// Start Agent inside MCP server (lazy, no cq serve required).
	// Defined in mcp_init_agent.go — no build tag.
	// agentCtx is cancelled by shutdownAgent hook (via ctx.agentCancel).
	agentCtx, agentCancel := context.WithCancel(context.Background())
	ctx.agentCancel = agentCancel
	startAgentIfNeeded(agentCtx, ctx)

	// Background knowledge pull: sync cloud knowledge to local FTS on session start.
	// Best-effort — errors logged, not fatal. Goroutine is abandoned after 10s timeout.
	if knowledgeStore != nil && knowledgeCloud != nil {
		go func() {
			type pullResult struct {
				r   *knowledge.PullResult
				err error
			}
			ch := make(chan pullResult, 1)
			go func() {
				r, err := knowledge.Pull(knowledgeStore, knowledgeCloud, "", 50, false)
				ch <- pullResult{r, err}
			}()
			select {
			case res := <-ch:
				if res.err != nil {
					fmt.Fprintf(os.Stderr, "cq: knowledge sync: skipped (%v)\n", res.err)
				} else {
					fmt.Fprintf(os.Stderr, "cq: knowledge sync: pulled %d, updated %d\n", res.r.Pulled, res.r.Updated)
				}
			case <-time.After(10 * time.Second):
				fmt.Fprintln(os.Stderr, "cq: knowledge sync: timed out (10s)")
			}
		}()
	} else {
		fmt.Fprintln(os.Stderr, "cq: knowledge sync: skipped (no cloud)")
	}

	return &mcpServer{
		registry:       reg,
		sidecar:        lazySidecar,
		db:             db,
		initCtx:        ctx,
		knowledgeStore: knowledgeStore,
		knowledgeUsage: knowledgeUsage,
		secretStore:    secretStore,
	}, nil
}

// providerDefaultEnvVar returns the default environment variable name for a provider's API key.
func providerDefaultEnvVar(provider string) string {
	switch provider {
	case "anthropic":
		return "ANTHROPIC_API_KEY"
	case "openai":
		return "OPENAI_API_KEY"
	case "gemini":
		return "GEMINI_API_KEY"
	default:
		return ""
	}
}

// toLLMGatewayConfig converts config.C4Config to llm.GatewayConfig,
// breaking the llm→config import dependency.
// Key resolution priority:
//  1. secrets store (name.api_key) — ~/.c4/secrets.db (AES-256-GCM)
//  2. default environment variable (e.g. ANTHROPIC_API_KEY)
//
// Ollama is exempt from the no-key check (it does not require an API key).
// If the config file contains deprecated api_key or api_key_env fields under
// llm_gateway.providers.*, a deprecation warning is logged via slog.
// cfgMgr may be nil (config failed to load); in that case no deprecation check is done.
// ss may be nil (secret store unavailable); env fallback is still attempted.
func toLLMGatewayConfig(cfgMgr *config.Manager, ss *secrets.Store) llm.GatewayConfig {
	var cfg config.C4Config
	if cfgMgr != nil {
		cfg = cfgMgr.GetConfig()
		// Detect deprecated api_key / api_key_env fields in config.yaml.
		// Since LLMProviderConfig no longer has these fields, viper silently drops them
		// during Unmarshal. Use IsSet() to detect if they were present in the config file.
		for name := range cfg.LLMGateway.Providers {
			if cfgMgr.IsSet("llm_gateway.providers."+name+".api_key") ||
				cfgMgr.IsSet("llm_gateway.providers."+name+".api_key_env") {
				slog.Warn("llm_gateway api_key in config deprecated; use: cq secret set <provider>.api_key <value>",
					"provider", name)
			}
		}
	}

	providers := make(map[string]llm.GatewayProviderConfig, len(cfg.LLMGateway.Providers))
	for name, p := range cfg.LLMGateway.Providers {
		var apiKey string
		// 1st: secrets store (preferred — AES-256-GCM encrypted)
		if ss != nil {
			if v, err := ss.Get(name + ".api_key"); err == nil {
				apiKey = v
			}
		}
		// 2nd: default environment variable (e.g. ANTHROPIC_API_KEY)
		if apiKey == "" {
			if envVar := providerDefaultEnvVar(name); envVar != "" {
				apiKey = os.Getenv(envVar)
			}
		}
		// Ollama does not require an API key; all other providers need one.
		if apiKey == "" && name != "ollama" {
			slog.Warn("no API key for provider; provider disabled", "provider", name)
		}
		providers[name] = llm.GatewayProviderConfig{
			Enabled:      p.Enabled && (apiKey != "" || name == "ollama"),
			APIKey:       apiKey,
			BaseURL:      p.BaseURL,
			DefaultModel: p.DefaultModel,
		}
	}
	return llm.GatewayConfig{
		Default:        cfg.LLMGateway.Default,
		CacheByDefault: cfg.LLMGateway.CacheByDefault,
		Providers:      providers,
	}
}

// hubJobSubmitterAdapter adapts hub.Client to eventbus.JobSubmitter interface,
// breaking the eventbus→hub import dependency.
type hubJobSubmitterAdapter struct {
	client *hub.Client
}

func (a *hubJobSubmitterAdapter) Submit(spec *eventbus.JobSubmitSpec) (string, error) {
	resp, err := a.client.SubmitJob(&hub.JobSubmitRequest{
		Name:        spec.Name,
		Workdir:     spec.Workdir,
		Command:     spec.Command,
		Env:         spec.Env,
		Tags:        spec.Tags,
		RequiresGPU: spec.RequiresGPU,
		Priority:    spec.Priority,
		ExpID:       spec.ExpID,
		Memo:        spec.Memo,
		TimeoutSec:  spec.TimeoutSec,
	})
	if err != nil {
		return "", err
	}
	return resp.JobID, nil
}
