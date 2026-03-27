package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
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
	"github.com/changmin/c4-core/internal/mcp/apps"
	"github.com/changmin/c4-core/internal/mcp/handlers"
	"github.com/changmin/c4-core/internal/mcp/handlers/gpuhandler"
	"github.com/changmin/c4-core/internal/mcp/handlers/knowledgehandler"
	"github.com/changmin/c4-core/internal/mcp/handlers/relayhandler"
	"github.com/changmin/c4-core/internal/mcp/handlers/llmhandler"
	"github.com/changmin/c4-core/internal/ontology"
	"github.com/changmin/c4-core/internal/relay"
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
			// Auto-join: if logged in + project_id set + auto_join enabled, join project if not already member
			if cloudCfg.AutoJoin && cloudProjectID != "" && cloudTP != nil {
				go func() {
					tc := cloud.NewTeamClient(cloudCfg.URL, cloudCfg.AnonKey, cloudTP.Token())
					members, err := tc.ListMembers(cloudProjectID)
					if err != nil {
						return // silent fail
					}
					sess, _ := cloud.NewAuthClient(cloudCfg.URL, cloudCfg.AnonKey).GetSession()
					if sess == nil || sess.User.ID == "" {
						return
					}
					for _, m := range members {
						if m.UserID == sess.User.ID {
							return // already a member
						}
					}
					result, err := tc.InviteOrAdd(cloudProjectID, sess.User.Email)
					if err != nil {
						fmt.Fprintf(os.Stderr, "cq: auto-join failed: %v\n", err)
						return
					}
					if result.Status == "added" || result.Status == "already_member" {
						fmt.Fprintf(os.Stderr, "cq: auto-joined project %s\n", cloudProjectID)
					}
				}()
			}
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
			embGateway := llm.NewGatewayFromConfig(toLLMGatewayConfig(cfgMgr, secretStore, cloudTP))

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

	// Create resource store for MCP Apps ui:// resources early so it can be
	// wired into nativeOpts (for c4_run_validation format=widget support).
	appStore := apps.NewResourceStore()

	// Build KnowledgeNativeOpts as a shared pointer — Phase 4 hooks (Drive)
	// can mutate it after handler registration (handlers capture the pointer).
	knowledgeNativeOpts := &knowledgehandler.KnowledgeNativeOpts{
		Store:         knowledgeStore,
		Searcher:      knowledgeSearcher,
		Usage:         knowledgeUsage,
		LLM:           ctx.llmGateway,
		GlobalManager: globalKnowledgeManager,
	}
	ctx.knowledgeOpts = knowledgeNativeOpts

	nativeOpts := &handlers.NativeOpts{
		ResearchStore:          ctx.researchStore,
		KnowledgeStore:         knowledgeStore,
		KnowledgeSearcher:      knowledgeSearcher,
		KnowledgeUsage:         knowledgeUsage,
		KnowledgeGlobalManager: globalKnowledgeManager,
		LLMGateway:             ctx.llmGateway,
		GPUStore:               ctx.daemonStore,
		GPUScheduler:           ctx.scheduler,
		AppResourceStore:       appStore,
		KnowledgeOpts:          knowledgeNativeOpts,
	}
	// Avoid typed-nil interface bug: only assign when concrete pointer is non-nil.
	// A nil *cloud.KnowledgeCloudClient assigned to an interface field creates a
	// non-nil interface (typed nil), causing opts.Cloud != nil to be true and
	// subsequent method calls to panic with nil pointer dereference.
	var kcForProxy handlers.KnowledgeSyncer
	if knowledgeCloud != nil {
		nativeOpts.KnowledgeCloud = knowledgeCloud
		nativeOpts.KnowledgeCloudSearch = knowledgeCloud
		knowledgeNativeOpts.Cloud = knowledgeCloud
		knowledgeNativeOpts.CloudSearch = knowledgeCloud
		kcForProxy = knowledgeCloud
	}
	if cfgMgr != nil {
		nativeOpts.KnowledgeCloudMode = cfgMgr.GetConfig().Cloud.Mode
		knowledgeNativeOpts.CloudMode = cfgMgr.GetConfig().Cloud.Mode
	}
	// Wire ontology CloudStore when cloud is enabled (same credentials as knowledge cloud).
	// *ontology.CloudStore implements L1/L2 upsert via Supabase PostgREST.
	if cfgMgr != nil && cfgMgr.GetConfig().Cloud.Enabled && cloudTP != nil {
		cloudCfg := cfgMgr.GetConfig().Cloud
		if cloudCfg.URL != "" && cloudCfg.AnonKey != "" {
			nativeOpts.OntologyCloud = ontology.NewCloudStore(
				cloudCfg.URL+"/rest/v1", cloudCfg.AnonKey, cloudTP)
			nativeOpts.OntologyCloudMode = cloudCfg.Mode
		}
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
	var knowledgeHitTracker *handlers.KnowledgeHitTracker
	if knowledgeStore != nil || knowledgeSearcher != nil {
		knowledgeHitTracker = handlers.NewKnowledgeHitTracker()
		storeOpts = append(storeOpts, handlers.WithKnowledgeHitTracker(knowledgeHitTracker))
	}
	// Cloud team search for task enrichment (enrichUnified)
	if knowledgeNativeOpts.CloudSearch != nil && knowledgeNativeOpts.CloudMode != "" {
		opts := knowledgeNativeOpts // capture pointer
		storeOpts = append(storeOpts, handlers.WithCloudSearch(func(query, docType string, limit int) ([]handlers.KnowledgeSearchResult, bool) {
			results, used, _ := knowledgehandler.CloudPrimarySearch(opts, query, docType, limit)
			if !used {
				return nil, false
			}
			out := make([]handlers.KnowledgeSearchResult, len(results))
			for i, r := range results {
				out[i] = handlers.KnowledgeSearchResult{ID: r.ID, Title: r.Title, Type: r.Type, Domain: r.Domain}
			}
			return out, true
		}))
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

	// Register MCP Apps ui:// resources (appStore created above in Phase 3).
	handlers.RegisterDashboardHandler(reg, sqliteStore, &handlers.DashboardDeps{
		KnowledgeStore: knowledgeStore,
		ResourceStore:  appStore,
		DashboardHTML:  apps.DashboardHTML,
	})
	fmt.Fprintln(os.Stderr, "cq: c4_dashboard registered (ui://cq/dashboard)")

	// Register task graph widget for dependency visualization.
	handlers.RegisterTaskGraphHandler(reg, sqliteStore, &handlers.TaskGraphDeps{
		ResourceStore: appStore,
		TaskGraphHTML: apps.TaskGraphHTML,
	})
	fmt.Fprintln(os.Stderr, "cq: c4_task_graph registered (ui://cq/task-graph)")

	// Register job widgets for GPU handler (job progress + job result).
	gpuhandler.RegisterJobProgressWidget(appStore, apps.JobProgressHTML)
	gpuhandler.RegisterJobResultWidget(appStore, apps.JobResultHTML)

	// Register knowledge feed widget for search result cards.
	if apps.KnowledgeFeedHTML != "" {
		appStore.Register("ui://cq/knowledge-feed", apps.KnowledgeFeedHTML)
		fmt.Fprintln(os.Stderr, "cq: knowledge-feed registered (ui://cq/knowledge-feed)")
	}

	// Register cost tracker widget for LLM handler.
	llmhandler.RegisterCostTrackerWidget(appStore, apps.CostTrackerHTML)
	fmt.Fprintln(os.Stderr, "cq: c4_llm_costs widget registered (ui://cq/cost-tracker)")

	// Register git diff summary widget.
	handlers.RegisterGitDiffHandler(reg, &handlers.GitDiffDeps{
		ResourceStore: appStore,
		GitDiffHTML:   apps.GitDiffHTML,
		ProjectRoot:   projectDir,
	})
	fmt.Fprintln(os.Stderr, "cq: c4_diff_summary registered (ui://cq/git-diff)")

	// Register error trace widget for stack trace visualization.
	handlers.RegisterErrorTraceHandler(reg, sqliteStore, &handlers.ErrorTraceDeps{
		ResourceStore:  appStore,
		ErrorTraceHTML: apps.ErrorTraceHTML,
	})
	fmt.Fprintln(os.Stderr, "cq: c4_error_trace registered (ui://cq/error-trace)")

	// Register nodes map widget for agent/worker/edge status.
	handlers.RegisterNodesMapHandler(reg, sqliteStore, &handlers.NodesMapDeps{
		ResourceStore: appStore,
		NodesMapHTML:  apps.NodesMapHTML,
	})
	fmt.Fprintln(os.Stderr, "cq: c4_nodes_map registered (ui://cq/nodes-map)")

	// Register experiment compare widget for side-by-side results.
	knowledgehandler.RegisterExperimentCompareWidget(appStore, apps.ExperimentCompareHTML)
	fmt.Fprintln(os.Stderr, "cq: experiment-compare widget registered (ui://cq/experiment-compare)")

	// Register test results widget (ui://cq/test-results) — c4_run_validation format=widget.
	if apps.TestResultsHTML != "" {
		appStore.Register("ui://cq/test-results", apps.TestResultsHTML)
		fmt.Fprintln(os.Stderr, "cq: test-results widget registered (ui://cq/test-results)")
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

	// Register intelligence stats handler (knowledge + ontology + circulation monitoring)
	handlers.RegisterIntelligenceStatsHandler(reg, &handlers.IntelligenceStatsDeps{
		KnowledgeStore: knowledgeStore,
		HitTracker:     knowledgeHitTracker,
		ProjectRoot:    projectDir,
		OntologyUser:   os.Getenv("USER"),
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

	// Register backward-compatibility aliases: c4_xxx → cq_xxx
	handlers.RegisterLegacyAliases(reg)

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

	// Wire relay client if relay.enabled and relay.url are configured.
	// Only connect in serve mode — cq mcp instances must not compete for the same worker_id.
	// Multiple processes connecting with the same hostname causes connection cycling
	// because the relay server closes the previous connection on each new connect.
	if serveMode && ctx.cfgMgr != nil {
		relayCfg := ctx.cfgMgr.GetConfig().Relay
		if relayCfg.Enabled && relayCfg.URL != "" {
			workerID, _ := os.Hostname()
			var tokenFunc func() string
			if ctx.cloudTP != nil {
				tokenFunc = ctx.cloudTP.Token
			}
			// relayMCPHandler handles JSON-RPC requests over relay:
			// - initialize: return server capabilities
			// - tools/list: return all registered tool schemas
			// - tools/call: dispatch to local MCP registry
			// - notifications/initialized: acknowledge (no response needed)
			relayHandler := relay.MCPHandler(func(rctx context.Context, request json.RawMessage) (json.RawMessage, error) {
				var req struct {
					JSONRPC string          `json:"jsonrpc"`
					ID      interface{}     `json:"id"`
					Method  string          `json:"method"`
					Params  json.RawMessage `json:"params"`
				}
				if err := json.Unmarshal(request, &req); err != nil {
					return nil, fmt.Errorf("relay: invalid JSON-RPC request: %w", err)
				}

				switch req.Method {
				case "initialize":
					return json.Marshal(map[string]interface{}{
						"jsonrpc": "2.0",
						"id":      req.ID,
						"result": map[string]interface{}{
							"protocolVersion": "2025-03-26",
							"capabilities": map[string]interface{}{
								"tools": map[string]interface{}{},
							},
							"serverInfo": map[string]interface{}{
								"name":    "cq-relay-worker",
								"version": version,
							},
						},
					})

				case "notifications/initialized":
					// Client acknowledgement — no response needed for notifications
					return json.Marshal(map[string]interface{}{
						"jsonrpc": "2.0",
						"id":      req.ID,
					})

				case "tools/list":
					tools := reg.ListTools()
					toolList := make([]map[string]interface{}, 0, len(tools))
					for _, t := range tools {
						tool := map[string]interface{}{
							"name":        t.Name,
							"description": t.Description,
						}
						if t.InputSchema != nil {
							tool["inputSchema"] = t.InputSchema
						}
						toolList = append(toolList, tool)
					}
					return json.Marshal(map[string]interface{}{
						"jsonrpc": "2.0",
						"id":      req.ID,
						"result": map[string]interface{}{
							"tools": toolList,
						},
					})

				case "tools/call":
					var params struct {
						Name      string          `json:"name"`
						Arguments json.RawMessage `json:"arguments"`
					}
					if err := json.Unmarshal(req.Params, &params); err != nil {
						return nil, fmt.Errorf("relay: invalid tools/call params: %w", err)
					}
					result, err := reg.CallWithContext(rctx, params.Name, params.Arguments)
					if err != nil {
						return nil, err
					}
					resultJSON, err := json.Marshal(result)
					if err != nil {
						return nil, fmt.Errorf("relay: marshal result: %w", err)
					}
					return json.Marshal(map[string]interface{}{
						"jsonrpc": "2.0",
						"id":      req.ID,
						"result": map[string]interface{}{
							"content": []map[string]interface{}{
								{"type": "text", "text": string(resultJSON)},
							},
						},
					})

				default:
					return json.Marshal(map[string]interface{}{
						"jsonrpc": "2.0",
						"id":      req.ID,
						"error": map[string]interface{}{
							"code":    -32601,
							"message": fmt.Sprintf("method not found: %s", req.Method),
						},
					})
				}
			})
			rc := relay.NewRelayClient(relayCfg.URL, workerID, tokenFunc, relayHandler)
			relayCtx, relayCancel := context.WithCancel(context.Background())
			go func() {
				if err := rc.Connect(relayCtx); err != nil {
					fmt.Fprintf(os.Stderr, "cq: relay connect failed: %v\n", err)
					relayCancel()
					return
				}
				httpURL := strings.Replace(relayCfg.URL, "wss://", "https://", 1)
				httpURL = strings.Replace(httpURL, "ws://", "http://", 1)
				fmt.Fprintf(os.Stderr, "cq: relay connected — %s/w/%s/mcp\n", httpURL, workerID)
			}()
			registerShutdownHook(func(_ *initContext) {
				rc.Close()
				relayCancel()
			})

		}
	}

	// Register cq_workers and cq_relay_call MCP tools for remote worker access.
	// Always registered regardless of relay config — if relay URL is not set,
	// the handlers return a helpful "relay not configured" message instead of
	// being invisible to the agent (which causes hallucinated CLI commands).
	{
		var relayURL, anonKey string
		var tokenFunc func() string
		if ctx.cfgMgr != nil {
			relayURL = ctx.cfgMgr.GetConfig().Relay.URL
			anonKey = ctx.cfgMgr.GetConfig().Cloud.AnonKey
		}
		if ctx.cloudTP != nil {
			tokenFunc = ctx.cloudTP.Token
		}
		deps := &relayhandler.Deps{
			RelayURL:  relayURL,
			AnonKey:   anonKey,
			TokenFunc: tokenFunc,
		}
		// Wire hub lister for unified cq_workers() view.
		if ctx.hubClient != nil {
			deps.HubLister = newHubListerAdapter(ctx.hubClient)
		}
		relayhandler.Register(reg, deps)
	}

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
				r, err := knowledge.Pull(knowledgeStore, knowledgeCloud, "", 50, false, knowledgeSearcher)
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

	// Tool tiering: restrict visible tools to reduce cognitive load.
	// Default: "basic" (core tools only). Set CQ_TOOL_TIER=full for all tools.
	if os.Getenv("CQ_TOOL_TIER") != "full" {
		reg.SetVisibleTools(coreMCPTools)
		fmt.Fprintf(os.Stderr, "cq: tool tier: basic (%d visible, %d total). Set CQ_TOOL_TIER=full for all.\n",
			len(coreMCPTools), len(reg.ListAllTools()))
	}

	return &mcpServer{
		registry:       reg,
		sidecar:        lazySidecar,
		db:             db,
		initCtx:        ctx,
		knowledgeStore: knowledgeStore,
		knowledgeUsage: knowledgeUsage,
		secretStore:    secretStore,
		resourceStore:  appStore,
	}, nil
}

// coreMCPTools is the default set of tools visible to AI agents.
// All other tools remain callable but don't appear in tools/list.
// Set CQ_TOOL_TIER=full to expose everything.
var coreMCPTools = []string{
	// Project management
	"cq_status",
	"cq_add_todo",
	"cq_task_list",
	"cq_start",
	"cq_claim",
	"cq_report",
	"cq_mark_blocked",
	"cq_stale_tasks",

	// Worker protocol
	"cq_get_task",
	"cq_submit",
	"cq_request_changes",
	"cq_run_validation",
	"cq_worker_heartbeat",

	// File operations
	"cq_find_file",
	"cq_search_for_pattern",
	"cq_read_file",
	"cq_replace_content",
	"cq_create_text_file",
	"cq_list_dir",

	// Knowledge
	"cq_knowledge_search",
	"cq_knowledge_record",

	// Execution & system
	"cq_execute",
	"cq_health",
	"cq_config_get",
	"cq_notify",
	"cq_diff_summary",
	"cq_gpu_status",

	// Remote workspace
	"cq_workers",
	"cq_relay_call",

	// Hub
	"cq_hub_submit",
	"cq_hub_workers",
	"cq_hub_status",
	"cq_hub_list",
	"cq_job_submit",
	"cq_job_status",

	// LLM
	"cq_llm_call",

	// Specs & design (planning)
	"cq_save_spec",
	"cq_save_design",
	"cq_get_spec",
	"cq_get_design",
	"cq_list_specs",
	"cq_list_designs",
	"cq_discovery_complete",
	"cq_design_complete",

	// Lighthouse
	"cq_lighthouse",
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
func toLLMGatewayConfig(cfgMgr *config.Manager, ss *secrets.Store, cloudTP *cloud.TokenProvider) llm.GatewayConfig {
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
	if cloudTP != nil {
		// Auto-register cq-proxy provider with JWT token.
		// base_url priority: config > builtinSupabaseURL + /functions/v1/llm-proxy
		baseURL := ""
		if p, ok := cfg.LLMGateway.Providers["cq-proxy"]; ok {
			baseURL = p.BaseURL
		}
		if baseURL == "" && builtinSupabaseURL != "" {
			baseURL = strings.TrimRight(builtinSupabaseURL, "/") + "/functions/v1/llm-proxy"
		}
		if baseURL != "" && cloudTP.Token() != "" {
			providers["cq-proxy"] = llm.GatewayProviderConfig{
				Enabled:      true,
				TokenFunc:    cloudTP.Token,
				BaseURL:      baseURL,
				DefaultModel: "claude-haiku-4-5-20251001",
			}
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
