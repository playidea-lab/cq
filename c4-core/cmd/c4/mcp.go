package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	"github.com/changmin/c4-core/internal/bridge"
	"github.com/changmin/c4-core/internal/cdp"
	"github.com/changmin/c4-core/internal/cloud"
	"github.com/changmin/c4-core/internal/config"
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

// NOTE: The "mcp" subcommand is registered by fallback.go (mcpFallbackCmd)
// which wraps the Go MCP server with a Python fallback mechanism.
// The runMCP function below is the core Go MCP server implementation.

// mcpRequest represents a JSON-RPC 2.0 request.
type mcpRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// mcpResponse represents a JSON-RPC 2.0 response.
type mcpResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id,omitempty"`
	Result  interface{} `json:"result,omitempty"`
	Error   *mcpError   `json:"error,omitempty"`
}

// mcpError represents a JSON-RPC 2.0 error.
type mcpError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// serverInfo is returned on initialize.
type serverInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// initializeResult is the response to the initialize method.
type initializeResult struct {
	ProtocolVersion string     `json:"protocolVersion"`
	ServerInfo      serverInfo `json:"serverInfo"`
	Capabilities    struct {
		Tools struct{} `json:"tools"`
	} `json:"capabilities"`
}

// mcpServer holds the state of a running MCP server instance.
type mcpServer struct {
	registry         *mcp.Registry
	sidecar          *bridge.LazyStarter      // lazy-initialized Python sidecar
	db               *sql.DB
	embeddedEB       *eventbus.EmbeddedServer // v3: in-process EventBus
	researchStore    *research.Store          // Go native research store
	knowledgeStore   *knowledge.Store         // Go native knowledge store (Tier 2)
	knowledgeUsage   *knowledge.UsageTracker  // usage tracking for 3-way RRF
	eventsinkSrv     *http.Server             // EventSink HTTP server (nil if disabled)
	hubPollerCancel  context.CancelFunc       // cancel func for HubPoller goroutine
}

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

	// Create registry and register all tools (proxy is created inside with lazy sidecar)
	reg := mcp.NewRegistry()
	nativeOpts := &handlers.NativeOpts{
		ResearchStore:     researchStore,
		KnowledgeStore:    knowledgeStore,
		KnowledgeSearcher: knowledgeSearcher,
		KnowledgeCloud:    knowledgeCloud,
		KnowledgeUsage:    knowledgeUsage,
		LLMGateway:        llmGateway,
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
	}, nil
}

// serve runs the stdio MCP server loop with concurrent request handling.
// Requests are read sequentially from stdin but handled concurrently in goroutines.
// Responses are written back through a mutex-protected encoder to preserve stdio integrity.
// This prevents slow sidecar proxy calls (e.g. c4_find_symbol) from blocking
// fast native operations (e.g. c4_status) — eliminating head-of-line blocking.
func (s *mcpServer) serve() error {
	decoder := json.NewDecoder(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)
	var writerMu sync.Mutex

	// Wire up tools/list_changed notification so clients re-fetch after lighthouse register.
	s.registry.OnChange = func() {
		writerMu.Lock()
		_ = encoder.Encode(map[string]any{
			"jsonrpc": "2.0",
			"method":  "notifications/tools/list_changed",
		})
		writerMu.Unlock()
	}

	for {
		var req mcpRequest
		if err := decoder.Decode(&req); err != nil {
			if err == io.EOF {
				return nil
			}
			return fmt.Errorf("decoding request: %w", err)
		}

		// Notifications (no ID) don't need responses — handle inline.
		if req.ID == nil {
			_ = s.handleRequest(&req)
			continue
		}

		// Handle each request concurrently to avoid head-of-line blocking.
		go func(r mcpRequest) {
			resp := s.handleRequest(&r)
			if resp != nil {
				writerMu.Lock()
				_ = encoder.Encode(resp)
				writerMu.Unlock()
			}
		}(req)
	}
}

// shutdown cleans up resources.
func (s *mcpServer) shutdown() {
	if s.hubPollerCancel != nil {
		s.hubPollerCancel()
	}
	if s.eventsinkSrv != nil {
		s.eventsinkSrv.Close()
	}
	if s.embeddedEB != nil {
		s.embeddedEB.Stop()
	}
	if s.sidecar != nil {
		_ = s.sidecar.Stop()
	}
	if s.researchStore != nil {
		s.researchStore.Close()
	}
	if s.knowledgeUsage != nil {
		s.knowledgeUsage.Close()
	}
	if s.knowledgeStore != nil {
		s.knowledgeStore.Close()
	}
	if s.db != nil {
		_ = s.db.Close()
	}
}

// handleRequest dispatches a JSON-RPC request.
func (s *mcpServer) handleRequest(req *mcpRequest) *mcpResponse {
	switch req.Method {
	case "initialize":
		return &mcpResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: initializeResult{
				ProtocolVersion: "2024-11-05",
				ServerInfo: serverInfo{
					Name:    "c4",
					Version: version,
				},
			},
		}

	case "tools/list":
		schemas := s.registry.ListTools()
		tools := make([]map[string]any, 0, len(schemas))
		for _, t := range schemas {
			tools = append(tools, map[string]any{
				"name":        t.Name,
				"description": t.Description,
				"inputSchema": t.InputSchema,
			})
		}
		return &mcpResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  map[string]any{"tools": tools},
		}

	case "tools/call":
		var params struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return &mcpResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error:   &mcpError{Code: -32602, Message: "invalid params"},
			}
		}

		result, err := s.registry.Call(params.Name, params.Arguments)
		if err != nil {
			return &mcpResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error:   &mcpError{Code: -32000, Message: err.Error()},
			}
		}

		// Marshal result to indented JSON for readability in tool responses
		resultJSON, jsonErr := json.MarshalIndent(result, "", "  ")
		if jsonErr != nil {
			return &mcpResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error:   &mcpError{Code: -32000, Message: "failed to serialize result"},
			}
		}

		return &mcpResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]any{
				"content": []map[string]any{
					{"type": "text", "text": string(resultJSON)},
				},
			},
		}

	case "notifications/initialized":
		return nil

	default:
		return &mcpResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &mcpError{Code: -32601, Message: fmt.Sprintf("method not found: %s", req.Method)},
		}
	}
}

// runMCP creates and runs the Go MCP server.
// This is called from fallback.go's startGoMCPServer.
func runMCP() error {
	if verbose {
		fmt.Fprintln(os.Stderr, "cq: Go MCP server starting on stdio...")
	}

	srv, err := newMCPServer()
	if err != nil {
		return fmt.Errorf("initializing MCP server: %w", err)
	}
	defer srv.shutdown()

	// Signal handler: ensure sidecar cleanup on SIGTERM/SIGINT.
	// Without this, signals terminate the process before defer runs,
	// leaving orphan sidecar processes.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		fmt.Fprintf(os.Stderr, "cq: received %s, shutting down\n", sig)
		srv.shutdown()
		os.Exit(0)
	}()

	tools := srv.registry.ListTools()
	fmt.Fprintf(os.Stderr, "cq: %d tools registered\n", len(tools))

	return srv.serve()
}

// isUUID returns true if s looks like a UUID (8-4-4-4-12 hex pattern).
func isUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return false
			}
		} else {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
				return false
			}
		}
	}
	return true
}

// resolveProjectUUID queries Supabase PostgREST to look up a project UUID by name.
func resolveProjectUUID(supabaseURL, anonKey, authToken, projectName string) (string, error) {
	url := supabaseURL + "/rest/v1/c4_projects?select=id&name=eq." + projectName + "&limit=1"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("apikey", anonKey)
	req.Header.Set("Authorization", "Bearer "+authToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	var rows []struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rows); err != nil {
		return "", err
	}
	if len(rows) == 0 {
		return "", fmt.Errorf("project %q not found", projectName)
	}
	return rows[0].ID, nil
}

// writeSupabaseJSON writes ~/.c4/supabase.json so Rust c1 app can read Supabase credentials.
// This is a no-op if the file already exists with the same content.
func writeSupabaseJSON(supabaseURL, anonKey string) {
	if supabaseURL == "" || anonKey == "" {
		return
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	dir := filepath.Join(home, ".c4")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return
	}
	path := filepath.Join(dir, "supabase.json")
	content := fmt.Sprintf(`{"url":%q,"anon_key":%q}`, supabaseURL, anonKey)
	// Skip write if file already has the same content
	if existing, err := os.ReadFile(path); err == nil && string(existing) == content {
		return
	}
	_ = os.WriteFile(path, []byte(content), 0600)
}

