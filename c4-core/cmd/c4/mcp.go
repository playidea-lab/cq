package main

import (
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

	"github.com/joho/godotenv"

	"github.com/changmin/c4-core/internal/bridge"
	"github.com/changmin/c4-core/internal/cdp"
	"github.com/changmin/c4-core/internal/cloud"
	"github.com/changmin/c4-core/internal/config"
	"github.com/changmin/c4-core/internal/drive"
	"github.com/changmin/c4-core/internal/eventbus"
	"github.com/changmin/c4-core/internal/hub"
	"github.com/changmin/c4-core/internal/llm"
	"github.com/changmin/c4-core/internal/mcp"
	"github.com/changmin/c4-core/internal/mcp/handlers"
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
	registry   *mcp.Registry
	sidecar    *bridge.LazyStarter // lazy-initialized Python sidecar
	db         *sql.DB
	embeddedEB *eventbus.EmbeddedServer // v3: in-process EventBus
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
			fmt.Fprintf(os.Stderr, "c4: loaded %s\n", candidate)
			break
		}
	}

	// Open SQLite database
	db, err := openDB()
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	// Load config (non-fatal on failure)
	cfgMgr, err := config.New(projectDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "c4: config load failed (using defaults): %v\n", err)
		cfgMgr = nil
	}

	// Create lazy Python sidecar (will start on first proxy tool call)
	bridgeCfg := bridge.DefaultSidecarConfig()
	bridgeCfg.PidFile = filepath.Join(projectDir, ".c4", "sidecar.pid")
	lazySidecar := bridge.NewLazyStarter(bridgeCfg)
	fmt.Fprintln(os.Stderr, "c4: Python sidecar will start on first proxy tool call")

	// Create KnowledgeCloudClient if cloud is enabled
	var knowledgeCloud *cloud.KnowledgeCloudClient
	var cloudAuthToken string
	var cloudProjectID string
	if cfgMgr != nil && cfgMgr.GetConfig().Cloud.Enabled {
		cloudCfg := cfgMgr.GetConfig().Cloud
		if cloudCfg.URL != "" && cloudCfg.AnonKey != "" {
			authClient := cloud.NewAuthClient(cloudCfg.URL, cloudCfg.AnonKey)
			if session, sessErr := authClient.GetSession(); sessErr == nil && session != nil {
				cloudAuthToken = session.AccessToken
			}
			cloudProjectID = cloudCfg.ProjectID
			if cloudProjectID == "" {
				cloudProjectID = cfgMgr.GetConfig().ProjectID
			}
			// Resolve project name to UUID if not already a UUID
			if cloudAuthToken != "" && cloudProjectID != "" && !isUUID(cloudProjectID) {
				if uuid, err := resolveProjectUUID(cloudCfg.URL, cloudCfg.AnonKey, cloudAuthToken, cloudProjectID); err == nil {
					fmt.Fprintf(os.Stderr, "c4: cloud project %q → %s\n", cloudProjectID, uuid)
					cloudProjectID = uuid
				} else {
					fmt.Fprintf(os.Stderr, "c4: could not resolve project UUID: %v\n", err)
				}
			}
			knowledgeCloud = cloud.NewKnowledgeCloudClient(
				cloudCfg.URL+"/rest/v1", cloudCfg.AnonKey, cloudAuthToken, cloudProjectID)
		}
	}

	// Create registry and register all tools (proxy is created inside with lazy sidecar)
	reg := mcp.NewRegistry()
	proxy := handlers.RegisterAllHandlersLazy(reg, nil, projectDir, lazySidecar, knowledgeCloud)

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
			cloudStore := cloud.NewCloudStore(cloudURL, cloudCfg.AnonKey, cloudAuthToken, cloudProjectID)
			store = cloud.NewHybridStore(sqliteStore, cloudStore)
			fmt.Fprintln(os.Stderr, "c4: cloud sync enabled (hybrid mode)")
		} else {
			fmt.Fprintln(os.Stderr, "c4: cloud enabled but URL/key not configured, using local only")
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
		fmt.Fprintf(os.Stderr, "c4: %d lighthouse stubs loaded\n", n)
	}

	// Register LLM Gateway handlers if enabled
	if cfgMgr != nil && cfgMgr.GetConfig().LLMGateway.Enabled {
		gateway := llm.NewGatewayFromConfig(cfgMgr.GetConfig())
		handlers.RegisterLLMHandlers(reg, gateway)
		fmt.Fprintf(os.Stderr, "c4: LLM gateway enabled (%d providers)\n", gateway.ProviderCount())
	}

	// Register Hub handlers if enabled
	if cfgMgr != nil && cfgMgr.GetConfig().Hub.Enabled {
		hubCfg := cfgMgr.GetConfig().Hub
		hubClient := hub.NewClient(hub.HubConfig{
			Enabled:   hubCfg.Enabled,
			URL:       hubCfg.URL,
			APIPrefix: hubCfg.APIPrefix,
			APIKey:    hubCfg.APIKey,
			APIKeyEnv: hubCfg.APIKeyEnv,
			TeamID:    hubCfg.TeamID,
		})
		if hubClient.IsAvailable() {
			handlers.RegisterHubHandlers(reg, hubClient)
			fmt.Fprintf(os.Stderr, "c4: hub connected (%s)\n", hubCfg.URL)
		} else {
			fmt.Fprintln(os.Stderr, "c4: hub enabled but URL not configured")
		}
	}

	// Register Drive handlers if cloud is enabled
	if cfgMgr != nil && cfgMgr.GetConfig().Cloud.Enabled {
		cloudCfg := cfgMgr.GetConfig().Cloud
		if cloudCfg.URL != "" && cloudCfg.AnonKey != "" {
			driveClient := drive.NewClient(cloudCfg.URL, cloudCfg.AnonKey, cloudAuthToken, cloudProjectID)
			handlers.RegisterDriveHandlers(reg, driveClient)
			fmt.Fprintln(os.Stderr, "c4: drive enabled (6 tools)")
		}
	}

	// Register C1 handlers if cloud is enabled
	var keeper *handlers.ContextKeeper
	if cfgMgr != nil && cfgMgr.GetConfig().Cloud.Enabled {
		cloudCfg := cfgMgr.GetConfig().Cloud
		if cloudCfg.URL != "" && cloudCfg.AnonKey != "" && cloudAuthToken != "" && cloudProjectID != "" {
			c1Handler := handlers.NewC1Handler(cloudCfg.URL+"/rest/v1", cloudCfg.AnonKey, cloudAuthToken, cloudProjectID)
			handlers.RegisterC1Handlers(reg, c1Handler)

			// Create ContextKeeper (wired to Dispatcher below)
			var keeperGateway *llm.Gateway
			if cfgMgr.GetConfig().LLMGateway.Enabled {
				keeperGateway = llm.NewGatewayFromConfig(cfgMgr.GetConfig())
			}
			keeper = handlers.NewContextKeeper(c1Handler, keeperGateway)
			fmt.Fprintln(os.Stderr, "c4: c1 enabled (3 tools + keeper)")
		}
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
			})
			if ebErr != nil {
				fmt.Fprintf(os.Stderr, "c4: eventbus auto-start failed: %v\n", ebErr)
			} else {
				embeddedEB = eb

				// Wire C1 poster if keeper available
				if keeper != nil {
					eb.Dispatcher().SetC1Poster(keeper)
				}

				// Connect client to embedded server
				ebClient, ebErr := eventbus.NewClient(eb.SocketPath())
				if ebErr == nil {
					handlers.RegisterEventBusHandlers(reg, ebClient)
					sqliteStore.SetEventBus(ebClient)
					handlers.SetDriveEventBus(ebClient)
					handlers.SetValidationEventBus(ebClient)
					proxy.SetEventBus(ebClient)
					sqliteStore.SetDispatcher(eb.Dispatcher())
					fmt.Fprintf(os.Stderr, "c4: eventbus auto-started (embedded, %s)\n", eb.SocketPath())
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
				fmt.Fprintf(os.Stderr, "c4: eventbus not reachable (unix:%s): %v\n", sockPath, ebErr)
			} else {
				handlers.RegisterEventBusHandlers(reg, ebClient)
				sqliteStore.SetEventBus(ebClient)
				handlers.SetDriveEventBus(ebClient)
				handlers.SetValidationEventBus(ebClient)
				proxy.SetEventBus(ebClient)
				fmt.Fprintf(os.Stderr, "c4: eventbus connected (unix:%s, 6 tools)\n", sockPath)
			}
		}
	}

	// Wire local Dispatcher for C1 posting if no embedded server
	// (embedded server already has its own dispatcher wired above)
	if keeper != nil && embeddedEB == nil {
		localDBPath := filepath.Join(projectDir, ".c4", "eventbus", "local.db")
		localStore, localErr := eventbus.NewStore(localDBPath)
		if localErr != nil {
			fmt.Fprintf(os.Stderr, "c4: local eventbus store failed: %v\n", localErr)
		} else {
			localDispatcher := eventbus.NewDispatcher(localStore)
			localDispatcher.SetC1Poster(keeper)

			// Ensure default c1_post rule exists for task events
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
			fmt.Fprintln(os.Stderr, "c4: local dispatcher wired (c1_post rules)")
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

	return &mcpServer{
		registry:   reg,
		sidecar:    lazySidecar,
		db:         db,
		embeddedEB: embeddedEB,
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
	if s.embeddedEB != nil {
		s.embeddedEB.Stop()
	}
	if s.sidecar != nil {
		_ = s.sidecar.Stop()
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

		// Marshal result to JSON text for MCP content response
		resultJSON, jsonErr := json.Marshal(result)
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
		fmt.Fprintln(os.Stderr, "c4: Go MCP server starting on stdio...")
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
		fmt.Fprintf(os.Stderr, "c4: received %s, shutting down\n", sig)
		srv.shutdown()
		os.Exit(0)
	}()

	tools := srv.registry.ListTools()
	fmt.Fprintf(os.Stderr, "c4: %d tools registered\n", len(tools))

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

