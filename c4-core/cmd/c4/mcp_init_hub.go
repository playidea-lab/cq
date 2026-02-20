//go:build c5_hub

package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/changmin/c4-core/internal/eventbus"
	"github.com/changmin/c4-core/internal/hub"
	"github.com/changmin/c4-core/internal/mcp/handlers"
	"github.com/changmin/c4-core/internal/worker"
)

func init() {
	registerInitHook(initHub)
	registerEBWireHook(wireHubEventBus)
}

// initHub creates the Hub client and registers Hub + Worker handlers.
func initHub(ctx *initContext) error {
	if ctx.cfgMgr == nil || !ctx.cfgMgr.GetConfig().Hub.Enabled {
		return nil
	}
	hubCfg := ctx.cfgMgr.GetConfig().Hub
	hc := hub.NewClient(hub.HubConfig{
		Enabled:   hubCfg.Enabled,
		URL:       hubCfg.URL,
		APIPrefix: hubCfg.APIPrefix,
		APIKey:    hubCfg.APIKey,
		APIKeyEnv: hubCfg.APIKeyEnv,
		TeamID:    hubCfg.TeamID,
	})
	if !hc.IsAvailable() {
		fmt.Fprintln(os.Stderr, "cq: hub enabled but URL not configured")
		return nil
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
