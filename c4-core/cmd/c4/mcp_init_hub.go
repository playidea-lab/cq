//go:build hub

package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/changmin/c4-core/internal/eventbus"
	"github.com/changmin/c4-core/internal/hub"
	"github.com/changmin/c4-core/internal/mcp/handlers"
	"github.com/changmin/c4-core/internal/serve"
	"github.com/changmin/c4-core/internal/worker"
)

func init() {
	registerInitHook(initHub)
	registerEBWireHook(wireHubEventBus)
	registerShutdownHook(shutdownHubCron)
}

// initHub creates the Hub client and registers Hub + Worker handlers.
func initHub(ctx *initContext) error {
	if ctx.cfgMgr == nil {
		return nil
	}
	hubCfg := ctx.cfgMgr.GetConfig().Hub

	// Apply env/builtin/cloud fallback: C5_HUB_URL env → builtinHubURL → cloud.url (Supabase).
	cloudCfg := ctx.cfgMgr.GetConfig().Cloud
	if hubCfg.URL == "" {
		if v := os.Getenv("C5_HUB_URL"); v != "" {
			hubCfg.URL = v
			hubCfg.Enabled = true
		} else if builtinHubURL != "" {
			hubCfg.URL = builtinHubURL
			hubCfg.Enabled = true
		} else if cloudCfg.Enabled && cloudCfg.URL != "" {
			// Cloud-primary mode: use Supabase as Hub backend
			hubCfg.URL = cloudCfg.URL
			hubCfg.Enabled = true
		}
	}

	if !hubCfg.Enabled {
		return nil
	}

	// API key resolution priority:
	//  1. secrets store (~/.c4/secrets.db) — "hub.api_key" (AES-256-GCM encrypted)
	//  2. env var (hubCfg.APIKeyEnv, default C5_API_KEY; legacy C4_HUB_API_KEY fallback) — handled by hub.NewClient
	//  3. config.yaml (hubCfg.APIKey) — plaintext, deprecated
	apiKey := hubCfg.APIKey
	if ctx.secretStore != nil {
		if v, err := ctx.secretStore.Get("hub.api_key"); err == nil && v != "" {
			apiKey = v
			if hubCfg.APIKey != "" {
				slog.Warn("hub.api_key in config.yaml is overridden by secrets store; remove it from config")
			}
		}
	}

	// Inherit Supabase credentials from cloud config when hub uses cloud URL.
	var supabaseURL, supabaseKey string
	if cloudCfg.Enabled && cloudCfg.URL != "" {
		supabaseURL = cloudCfg.URL
	}
	if cloudCfg.AnonKey != "" {
		supabaseKey = cloudCfg.AnonKey
	}

	hc := hub.NewClient(hub.HubConfig{
		Enabled:     hubCfg.Enabled,
		URL:         hubCfg.URL,
		APIPrefix:   hubCfg.APIPrefix,
		APIKey:      apiKey,
		APIKeyEnv:   hubCfg.APIKeyEnv,
		TeamID:      hubCfg.TeamID,
		SupabaseURL: supabaseURL,
		SupabaseKey: supabaseKey,
	})
	if !hc.IsAvailable() {
		fmt.Fprintln(os.Stderr, "cq: hub enabled but URL not configured")
		return nil
	}

	// If no static API key is resolved, fall back to cloud session token with auto-refresh.
	// Check both config api_key AND the actual env var value — api_key_env may be set
	// but the environment variable itself may be empty (common in OS service mode).
	envKeyValue := ""
	if hubCfg.APIKeyEnv != "" {
		envKeyValue = os.Getenv(hubCfg.APIKeyEnv)
	}
	if apiKey == "" && envKeyValue == "" && ctx.cloudTP != nil && ctx.cloudTP.Token() != "" {
		hc.SetTokenFunc(ctx.cloudTP.Token)
		fmt.Fprintln(os.Stderr, "cq: hub using cloud session token (auto-refresh enabled)")
	}

	ctx.hubClient = hc
	handlers.RegisterHubHandlers(ctx.reg, hc)
	handlers.RegisterDispatchHandler(ctx.reg)
	fmt.Fprintf(os.Stderr, "cq: hub connected (%s)\n", hubCfg.URL)

	// Register Worker standby tools
	shutdownStore, shutdownErr := worker.NewShutdownStore(ctx.db)
	if shutdownErr != nil {
		fmt.Fprintf(os.Stderr, "cq: worker shutdown store failed: %v\n", shutdownErr)
	} else {
		// Compute local MCP HTTP URL for Hub push-dispatch.
		// Priority: CQ_WORKER_MCP_URL env > Serve.MCPHTTP config.
		mcpURL := os.Getenv("CQ_WORKER_MCP_URL")
		if mcpURL == "" {
			mcpHTTPCfg := ctx.cfgMgr.GetConfig().Serve.MCPHTTP
			if mcpHTTPCfg.Enabled && mcpHTTPCfg.Port > 0 {
				bind := mcpHTTPCfg.Bind
				if bind == "" {
					bind = "127.0.0.1"
				}
				mcpURL = fmt.Sprintf("http://%s:%d", bind, mcpHTTPCfg.Port)
			}
		}
		// Resolve Postgres direct URL for LISTEN/NOTIFY.
		// Priority: C4_CLOUD_DIRECT_URL env > cloud.direct_url config.
		directURL := os.Getenv("C4_CLOUD_DIRECT_URL")
		if directURL == "" && ctx.cfgMgr != nil {
			directURL = ctx.cfgMgr.GetConfig().Cloud.DirectURL
		}
		if directURL == "" {
			fmt.Fprintln(os.Stderr, "cq: job listener: direct_url not configured, using polling")
		}
		handlers.RegisterWorkerHandlers(ctx.reg, &handlers.WorkerDeps{
			HubClient:     hc,
			ShutdownStore: shutdownStore,
			Keeper:        ctx.keeper,
			MCPURL:        mcpURL,
			DirectURL:     directURL,
		})
		fmt.Fprintln(os.Stderr, "cq: worker standby tools registered (3 tools)")
	}
	return nil
}

// wireHubEventBus wires the eventbus to Hub-related components.
func wireHubEventBus(ctx *initContext, ebClient *eventbus.Client) {
	handlers.SetHubEventBus(ebClient, ctx.sqliteStore.GetProjectID())
}

// hubJobSubmitter returns a JobSubmitter wrapping the hub client, or nil.
func hubJobSubmitter(ctx *initContext) eventbus.JobSubmitter {
	if ctx.hubClient == nil {
		return nil
	}
	hc, ok := ctx.hubClient.(*hub.Client)
	if !ok || hc == nil {
		return nil
	}
	return &hubJobSubmitterAdapter{client: hc}
}

// startHubPoller starts the HubPoller goroutine (called after eventbus wiring).
func startHubPoller(ctx *initContext) {
	if ctx.hubClient == nil {
		return
	}
	if serve.IsServeRunning() {
		fmt.Fprintln(os.Stderr, serve.StatusMessage("hub poller"))
		return
	}
	hc, ok := ctx.hubClient.(*hub.Client)
	if !ok || hc == nil {
		return
	}
	hubEventPub := handlers.GetHubEventPub()
	pollerCtx, pollerCancel := context.WithCancel(context.Background())
	ctx.hubPollerCancel = pollerCancel
	poller := handlers.NewHubPoller(hc, hubEventPub, 30*time.Second)
	poller.SetProjectID(ctx.sqliteStore.GetProjectID())
	poller.Start(pollerCtx)
	fmt.Fprintln(os.Stderr, "cq: hub poller started (30s interval)")

	// Start CronScheduler now that hub client is confirmed available.
	startHubCronScheduler(ctx, hc)
}

// startHubCronScheduler starts the CronScheduler in a background goroutine.
// It is called from startHubPoller after the hub client is confirmed available.
// The scheduler is stopped when ctx.cronCancel is called (via shutdownHubCron).
func startHubCronScheduler(ctx *initContext, hc *hub.Client) {
	cronCtx, cronCancel := context.WithCancel(context.Background())
	ctx.cronCancel = cronCancel
	scheduler := hub.NewCronScheduler(hc, slog.Default())
	go scheduler.Start(cronCtx)
	fmt.Fprintln(os.Stderr, "cq: cron scheduler started")
}

// shutdownHubCron cancels the CronScheduler goroutine on server shutdown.
func shutdownHubCron(ctx *initContext) {
	if ctx.cronCancel != nil {
		ctx.cronCancel()
	}
}
