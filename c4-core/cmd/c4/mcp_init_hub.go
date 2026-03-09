//go:build c5_hub

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
}

// initHub creates the Hub client and registers Hub + Worker handlers.
func initHub(ctx *initContext) error {
	if ctx.cfgMgr == nil {
		return nil
	}
	hubCfg := ctx.cfgMgr.GetConfig().Hub

	// Apply env/builtin fallback: C5_HUB_URL env → builtinHubURL (ldflags).
	if hubCfg.URL == "" {
		if v := os.Getenv("C5_HUB_URL"); v != "" {
			hubCfg.URL = v
			hubCfg.Enabled = true
		} else if builtinHubURL != "" {
			hubCfg.URL = builtinHubURL
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

	hc := hub.NewClient(hub.HubConfig{
		Enabled:   hubCfg.Enabled,
		URL:       hubCfg.URL,
		APIPrefix: hubCfg.APIPrefix,
		APIKey:    apiKey,
		APIKeyEnv: hubCfg.APIKeyEnv,
		TeamID:    hubCfg.TeamID,
	})
	if !hc.IsAvailable() {
		fmt.Fprintln(os.Stderr, "cq: hub enabled but URL not configured")
		return nil
	}

	// If hub.api_key is not configured by any source but a cloud session token is
	// available, use the cloud JWT as the Hub Bearer token with auto-refresh support.
	if apiKey == "" && hubCfg.APIKeyEnv == "" && ctx.cloudTP != nil && ctx.cloudTP.Token() != "" {
		hc.SetTokenFunc(ctx.cloudTP.Token)
		fmt.Fprintln(os.Stderr, "cq: hub using cloud session token (auto-refresh enabled)")
	}

	ctx.hubClient = hc
	handlers.RegisterHubHandlers(ctx.reg, hc)
	fmt.Fprintf(os.Stderr, "cq: hub connected (%s)\n", hubCfg.URL)

	// Register Worker standby tools
	shutdownStore, shutdownErr := worker.NewShutdownStore(ctx.db)
	if shutdownErr != nil {
		fmt.Fprintf(os.Stderr, "cq: worker shutdown store failed: %v\n", shutdownErr)
	} else {
		handlers.RegisterWorkerHandlers(ctx.reg, &handlers.WorkerDeps{
			HubClient:     hc,
			ShutdownStore: shutdownStore,
			Keeper:        ctx.keeper,
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
}
