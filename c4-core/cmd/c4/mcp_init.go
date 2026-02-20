package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/joho/godotenv"

	"github.com/changmin/c4-core/internal/bridge"
	"github.com/changmin/c4-core/internal/cdp"
	"github.com/changmin/c4-core/internal/cloud"
	"github.com/changmin/c4-core/internal/config"
	"github.com/changmin/c4-core/internal/daemon"
	"github.com/changmin/c4-core/internal/drive"
	"github.com/changmin/c4-core/internal/eventbus"
	"github.com/changmin/c4-core/internal/hub"
	"github.com/changmin/c4-core/internal/knowledge"
	"github.com/changmin/c4-core/internal/llm"
	"github.com/changmin/c4-core/internal/mcp"
	"github.com/changmin/c4-core/internal/mcp/handlers"
	"github.com/changmin/c4-core/internal/research"
	"github.com/changmin/c4-core/internal/worker"
	_ "modernc.org/sqlite"
)

// newMCPServer creates and initializes the MCP server with all tools registered.
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

	// Create research store (optional — native research tools use this)
	researchDir := filepath.Join(projectDir, ".c4", "research")
	os.MkdirAll(researchDir, 0755)
	researchStore, researchErr := research.NewStore(researchDir)
	if researchErr != nil {
		fmt.Fprintf(os.Stderr, "cq: research store init failed (proxy fallback): %v\n", researchErr)
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

		// Create embedder from LLM Gateway if OpenAI is available
		var embedder knowledge.Embedder
		embDim := 1536
		if cfgMgr != nil && cfgMgr.GetConfig().LLMGateway.Enabled {
			embGateway := llm.NewGatewayFromConfig(cfgMgr.GetConfig())
			// Add embedding route if not configured
			embGateway.Resolve("embedding", "")
			embedder = llm.NewEmbeddingProvider(embGateway, embDim)
			fmt.Fprintln(os.Stderr, "cq: knowledge real embeddings enabled (1536d)")
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

	// Create LLM Gateway early so it can be shared with knowledge distill
	var llmGateway *llm.Gateway
	if cfgMgr != nil && cfgMgr.GetConfig().LLMGateway.Enabled {
		llmGateway = llm.NewGatewayFromConfig(cfgMgr.GetConfig())
	}

	// Create daemon store and scheduler (graceful fallback on failure)
	var daemonStore *daemon.Store
	var scheduler *daemon.Scheduler
	var schedulerCancel context.CancelFunc
	daemonDBPath := filepath.Join(projectDir, ".c4", "daemon.db")
	if ds, dsErr := daemon.NewStore(daemonDBPath); dsErr != nil {
		fmt.Fprintf(os.Stderr, "cq: daemon store init failed (job scheduler unavailable): %v\n", dsErr)
	} else {
		daemonStore = ds
		daemonDataDir := filepath.Join(projectDir, ".c4", "daemon")
		gpuMon := daemon.NewGpuMonitor()
		gpuCount := 0
		if gpus, gpuErr := gpuMon.GetAllGPUs(); gpuErr == nil {
			gpuCount = len(gpus)
		}
		scheduler = daemon.NewScheduler(daemonStore, daemon.SchedulerConfig{
			DataDir:  daemonDataDir,
			GPUCount: gpuCount,
		})
		schedulerCtx, schedCancel := context.WithCancel(context.Background())
		schedulerCancel = schedCancel
		scheduler.Start(schedulerCtx)
		fmt.Fprintf(os.Stderr, "cq: daemon scheduler started (gpus=%d)\n", gpuCount)
	}

	// Create registry and register all tools (proxy is created inside with lazy sidecar)
	reg := mcp.NewRegistry()
	nativeOpts := &handlers.NativeOpts{
		ResearchStore:     researchStore,
		KnowledgeStore:    knowledgeStore,
		KnowledgeSearcher: knowledgeSearcher,
		KnowledgeCloud:    knowledgeCloud,
		KnowledgeUsage:    knowledgeUsage,
		LLMGateway:        llmGateway,
		GPUStore:          daemonStore,
		GPUScheduler:      scheduler,
	}
	proxy := handlers.RegisterAllHandlersLazyWithOpts(reg, nil, projectDir, lazySidecar, knowledgeCloud, nativeOpts)

	// Wire proxy restart and sidecar health check onRestart callback
	proxy.SetRestarter(lazySidecar)
	lazySidecar.SetOnRestart(func(newAddr string) {
		proxy.UpdateAddr(newAddr)
	})

	// Create store with all options
	storeOpts := []handlers.StoreOption{
		handlers.WithProjectRoot(projectDir),
	}
	if cfgMgr != nil {
		storeOpts = append(storeOpts, handlers.WithConfig(cfgMgr))
	}
	if proxy != nil {
		storeOpts = append(storeOpts, handlers.WithProxy(proxy))
	}
	if knowledgeStore != nil && knowledgeSearcher != nil {
		storeOpts = append(storeOpts, handlers.WithKnowledge(knowledgeStore, knowledgeSearcher))
	}
	storeOpts = append(storeOpts, handlers.WithRegistry(reg))
	sqliteStore, err := handlers.NewSQLiteStore(db, storeOpts...)
	if err != nil {
		return nil, fmt.Errorf("creating store: %w", err)
	}

	// Wrap with HybridStore if cloud is enabled (reuse auth token from knowledge client setup)
	var store handlers.Store = sqliteStore
	if cfgMgr != nil && cfgMgr.GetConfig().Cloud.Enabled {
		cloudCfg := cfgMgr.GetConfig().Cloud
		if cloudCfg.URL != "" && cloudCfg.AnonKey != "" {
			cloudURL := cloudCfg.URL + "/rest/v1"
			cloudStore := cloud.NewCloudStore(cloudURL, cloudCfg.AnonKey, cloudTP, cloudProjectID)
			store = cloud.NewHybridStore(sqliteStore, cloudStore)
			fmt.Fprintln(os.Stderr, "cq: cloud sync enabled (hybrid mode)")
		} else {
			fmt.Fprintln(os.Stderr, "cq: cloud enabled but URL/key not configured, using local only")
		}
	}

	// Re-register handlers with the actual store (core + discovery + persona + soul tools)
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
	handlers.RegisterLighthouseHandlers(reg, sqliteStore)
	if n := handlers.LoadLighthousesOnStartup(reg, sqliteStore); n > 0 {
		fmt.Fprintf(os.Stderr, "cq: %d lighthouse stubs loaded\n", n)
	}

	// Option B: manual recovery tools for stuck workers.
	handlers.RegisterTaskAdminHandlers(reg, sqliteStore)

	// Option C: implicit heartbeat — refresh active worker's task updated_at on
	// every tool call, so the 30-min stale timeout isn't triggered by genuine work.
	reg.OnCall = sqliteStore.TouchCurrentWorkerHeartbeat

	// Register LLM Gateway handlers if enabled (reuse gateway created earlier)
	if llmGateway != nil {
		handlers.RegisterLLMHandlers(reg, llmGateway)
		fmt.Fprintf(os.Stderr, "cq: LLM gateway enabled (%d providers)\n", llmGateway.ProviderCount())
	}

	// Register Hub handlers if enabled
	var hubClient *hub.Client
	var hubPollerCancel context.CancelFunc
	if cfgMgr != nil && cfgMgr.GetConfig().Hub.Enabled {
		hubCfg := cfgMgr.GetConfig().Hub
		hubClient = hub.NewClient(hub.HubConfig{
			Enabled:   hubCfg.Enabled,
			URL:       hubCfg.URL,
			APIPrefix: hubCfg.APIPrefix,
			APIKey:    hubCfg.APIKey,
			APIKeyEnv: hubCfg.APIKeyEnv,
			TeamID:    hubCfg.TeamID,
		})
		if hubClient.IsAvailable() {
			handlers.RegisterHubHandlers(reg, hubClient)
			fmt.Fprintf(os.Stderr, "cq: hub connected (%s)\n", hubCfg.URL)
		} else {
			fmt.Fprintln(os.Stderr, "cq: hub enabled but URL not configured")
			hubClient = nil
		}
	}

	// Register Drive handlers if cloud is enabled
	if cfgMgr != nil && cfgMgr.GetConfig().Cloud.Enabled {
		cloudCfg := cfgMgr.GetConfig().Cloud
		if cloudCfg.URL != "" && cloudCfg.AnonKey != "" {
			driveClient := drive.NewClient(cloudCfg.URL, cloudCfg.AnonKey, cloudTP, cloudProjectID, cloudCfg.BucketName)
			handlers.RegisterDriveHandlers(reg, driveClient)
			fmt.Fprintln(os.Stderr, "cq: drive enabled (6 tools)")
		}
	}

	// Register C1 handlers if cloud is enabled
	var keeper *handlers.ContextKeeper
	if cfgMgr != nil && cfgMgr.GetConfig().Cloud.Enabled {
		cloudCfg := cfgMgr.GetConfig().Cloud
		if cloudCfg.URL != "" && cloudCfg.AnonKey != "" && cloudTP.Token() != "" && cloudProjectID != "" {
			c1Handler := handlers.NewC1Handler(cloudCfg.URL+"/rest/v1", cloudCfg.AnonKey, cloudTP, cloudProjectID)
			handlers.RegisterC1Handlers(reg, c1Handler)

			// Create ContextKeeper (wired to Dispatcher below)
			var keeperGateway *llm.Gateway
			if cfgMgr.GetConfig().LLMGateway.Enabled {
				keeperGateway = llm.NewGatewayFromConfig(cfgMgr.GetConfig())
			}
			keeper = handlers.NewContextKeeper(c1Handler, keeperGateway)
			if err := keeper.EnsureSystemChannels(); err != nil {
				fmt.Fprintf(os.Stderr, "cq: system channels setup failed: %v\n", err)
			}
			fmt.Fprintln(os.Stderr, "cq: c1 enabled (3 tools + keeper)")
		}
	}

	// Register Worker standby tools if Hub is available
	if hubClient != nil {
		shutdownStore, shutdownErr := worker.NewShutdownStore(db)
		if shutdownErr != nil {
			fmt.Fprintf(os.Stderr, "cq: worker shutdown store failed: %v\n", shutdownErr)
		} else {
			handlers.RegisterWorkerHandlers(reg, &handlers.WorkerDeps{
				HubClient:     hubClient,
				ShutdownStore: shutdownStore,
				Keeper:        keeper,
			})
			fmt.Fprintln(os.Stderr, "cq: worker standby tools registered (3 tools)")
		}
	}

	// wireEventBusClient connects an EventBus client to all components that need it.
	// Centralised to prevent wiring omissions when adding new components.
	wireEventBusClient := func(ebClient *eventbus.Client) {
		handlers.RegisterEventBusHandlers(reg, ebClient)
		sqliteStore.SetEventBus(ebClient)
		handlers.SetDriveEventBus(ebClient)
		handlers.SetValidationEventBus(ebClient)
		handlers.SetKnowledgeEventBus(ebClient, sqliteStore.GetProjectID())
		handlers.SetResearchEventBus(ebClient, sqliteStore.GetProjectID())
		handlers.SetSoulEventBus(ebClient, sqliteStore.GetProjectID())
		handlers.SetPersonaEventBus(ebClient, sqliteStore.GetProjectID())
		handlers.SetHubEventBus(ebClient, sqliteStore.GetProjectID())
		proxy.SetEventBus(ebClient)
	}

	// Register EventBus handlers
	var embeddedEB *eventbus.EmbeddedServer
	if cfgMgr != nil && cfgMgr.GetConfig().EventBus.Enabled {
		ebCfg := cfgMgr.GetConfig().EventBus

		// Auto-start: launch in-process embedded server
		if ebCfg.AutoStart {
			dataDir := ebCfg.DataDir
			if dataDir == "" {
				dataDir = filepath.Join(projectDir, ".c4", "eventbus")
			}
			// Resolve default rules path: prefer project-local, fall back to data dir
			defaultRulesPath := filepath.Join(projectDir, "c4-core", "internal", "eventbus", "default_rules.yaml")
			if _, err := os.Stat(defaultRulesPath); err != nil {
				defaultRulesPath = filepath.Join(dataDir, "default_rules.yaml")
				if _, err := os.Stat(defaultRulesPath); err != nil {
					defaultRulesPath = ""
				}
			}

			eb, ebErr := eventbus.StartEmbedded(eventbus.EmbeddedConfig{
				DataDir:          dataDir,
				RetentionDays:    ebCfg.RetentionDays,
				MaxEvents:        ebCfg.MaxEvents,
				DefaultRulesPath: defaultRulesPath,
				WSPort:           ebCfg.WSPort,
				WSHost:           ebCfg.WSHost,
			})
			if ebErr != nil {
				fmt.Fprintf(os.Stderr, "cq: eventbus auto-start failed: %v\n", ebErr)
			} else {
				embeddedEB = eb

				if keeper != nil {
					eb.Dispatcher().SetC1Poster(keeper)
				}

				ebClient, ebErr := eventbus.NewClient(eb.SocketPath())
				if ebErr == nil {
					wireEventBusClient(ebClient)
					sqliteStore.SetDispatcher(eb.Dispatcher())
					if hubClient != nil {
						eb.Dispatcher().SetHubSubmitter(hubClient)
					}
					fmt.Fprintf(os.Stderr, "cq: eventbus auto-started (embedded, %s)\n", eb.SocketPath())
				}
			}
		} else {
			// Connect to remote daemon
			sockPath := ebCfg.SocketPath
			if sockPath == "" {
				home, _ := os.UserHomeDir()
				sockPath = filepath.Join(home, ".c4", "eventbus", "c3.sock")
			}
			ebClient, ebErr := eventbus.NewClient(sockPath)
			if ebErr != nil {
				fmt.Fprintf(os.Stderr, "cq: eventbus not reachable (unix:%s): %v\n", sockPath, ebErr)
			} else {
				wireEventBusClient(ebClient)
				fmt.Fprintf(os.Stderr, "cq: eventbus connected (unix:%s, 6 tools)\n", sockPath)
			}
		}
	}

	// Wire local Dispatcher for C1 posting if no embedded server
	if keeper != nil && embeddedEB == nil {
		localDBPath := filepath.Join(projectDir, ".c4", "eventbus", "local.db")
		localStore, localErr := eventbus.NewStore(localDBPath)
		if localErr != nil {
			fmt.Fprintf(os.Stderr, "cq: local eventbus store failed: %v\n", localErr)
		} else {
			localDispatcher := eventbus.NewDispatcher(localStore)
			localDispatcher.SetC1Poster(keeper)

			rules, _ := localStore.MatchRules("task.completed")
			if len(rules) == 0 {
				localStore.AddRule(
					"c1-task-updates",
					"task.*",
					"",
					"c1_post",
					`{"channel":"#updates","template":"[{{event_type}}] {{task_id}}: {{title}}"}`,
					true,
					0,
				)
			}

			sqliteStore.SetDispatcher(localDispatcher)
			fmt.Fprintln(os.Stderr, "cq: local dispatcher wired (c1_post rules)")
		}
	}

	// Register CDP handlers (always available — connects on demand)
	cdpRunner := cdp.NewRunner()
	handlers.RegisterCDPHandlers(reg, cdpRunner)

	// Set project role for Soul stage integration
	projectName := filepath.Base(projectDir)
	if projectName != "" && projectName != "." {
		handlers.SetProjectRoleForStage("project-" + projectName)
	}

	// Wire lazy sidecar for auto-restart (LazyStarter implements Restarter)
	proxy.SetRestarter(lazySidecar)

	// Register health check handler
	handlers.RegisterHealthHandler(reg, &handlers.HealthDeps{
		DB:             db,
		Sidecar:        lazySidecar,
		KnowledgeStore: knowledgeStore,
		StartTime:      time.Now(),
	})

	// Register config handlers
	handlers.RegisterConfigHandler(reg, cfgMgr)
	handlers.RegisterConfigSetHandler(reg, cfgMgr, projectDir)

	// Start HubPoller and EventSink using hubEventPub (set by SetHubEventBus above).
	// These must be started after EventBus wiring so hubEventPub is populated.
	// hubEventPubOrNoop returns the hub event publisher, falling back to NoopPublisher.
	hubEventPubOrNoop := handlers.GetHubEventPub()

	if hubClient != nil {
		pollerCtx, pollerCancel := context.WithCancel(context.Background())
		hubPollerCancel = pollerCancel
		poller := handlers.NewHubPoller(hubClient, hubEventPubOrNoop, 30*time.Second)
		poller.SetProjectID(sqliteStore.GetProjectID())
		poller.Start(pollerCtx)
		fmt.Fprintln(os.Stderr, "cq: hub poller started (30s interval)")
	}

	// Start EventSink HTTP server (config from .c4/config.yaml, env overrides applied in config.New)
	var eventsinkSrv *http.Server
	if cfgMgr != nil && cfgMgr.GetConfig().EventSink.Enabled {
		esCfg := cfgMgr.GetConfig().EventSink
		esSrv, esErr := handlers.StartEventSinkServer(esCfg.Port, esCfg.Token, hubEventPubOrNoop)
		if esErr != nil {
			fmt.Fprintf(os.Stderr, "cq: eventsink start failed: %v\n", esErr)
		} else if esSrv != nil {
			eventsinkSrv = esSrv
			fmt.Fprintf(os.Stderr, "cq: eventsink listening on :%d\n", esCfg.Port)
		}
	}

	return &mcpServer{
		registry:        reg,
		sidecar:         lazySidecar,
		db:              db,
		embeddedEB:      embeddedEB,
		researchStore:   researchStore,
		knowledgeStore:  knowledgeStore,
		knowledgeUsage:  knowledgeUsage,
		eventsinkSrv:    eventsinkSrv,
		hubPollerCancel: hubPollerCancel,
		daemonStore:     daemonStore,
		scheduler:       scheduler,
		schedulerCancel: schedulerCancel,
	}, nil
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
