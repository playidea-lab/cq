//go:build hub

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/changmin/c4-core/internal/config"
	"github.com/changmin/c4-core/internal/hub"
	"github.com/changmin/c4-core/internal/relay"
	"github.com/changmin/c4-core/internal/serve"
)

// runWatchdog starts the Watchdog supervisor in place of the normal serve loop.
// It re-invokes the current binary with the same arguments minus --watchdog,
// supervising it and restarting on crash with exponential backoff.
//
// If relay is configured, the watchdog also maintains its own WebSocket connection
// to the relay server (independent of the child process lifecycle) so that
// worker/restart commands can be received even while the child is restarting.
func runWatchdog(ctx context.Context, hubClientAny any, extraArgs []string) error {
	var hubClient *hub.Client
	if c, ok := hubClientAny.(*hub.Client); ok {
		hubClient = c
	}

	hostname, _ := os.Hostname()
	wd := serve.NewWatchdog(extraArgs, hubClient, hostname)

	// Wire relay restart handler if relay is configured.
	// The watchdog maintains its own relay WebSocket connection independent of
	// the child process so that worker/restart is always reachable.
	if relayClient := buildWatchdogRelayClient(ctx, wd, hostname); relayClient != nil {
		defer relayClient.Close()
	}

	return wd.Run(ctx)
}

// buildWatchdogRelayClient creates and connects a RelayClient for the watchdog
// that handles worker/restart JSON-RPC calls. Returns nil if relay is not
// configured or connection fails (relay is best-effort for the watchdog).
func buildWatchdogRelayClient(ctx context.Context, wd *serve.Watchdog, workerID string) *relay.RelayClient {
	cfgMgr, err := config.New(projectDir)
	if err != nil {
		return nil
	}
	cfg := cfgMgr.GetConfig()

	if !cfg.Relay.Enabled || cfg.Relay.URL == "" {
		return nil
	}

	handler := func(_ context.Context, req json.RawMessage) (json.RawMessage, error) {
		var rpc struct {
			Method string `json:"method"`
		}
		if err := json.Unmarshal(req, &rpc); err != nil {
			return nil, fmt.Errorf("parse request: %w", err)
		}
		if rpc.Method != "worker/restart" {
			return json.RawMessage(`{"jsonrpc":"2.0","error":{"code":-32601,"message":"method not found"}}`), nil
		}
		fmt.Fprintln(os.Stderr, "watchdog: received worker/restart via relay")
		wd.RestartChild()
		return json.RawMessage(`{"jsonrpc":"2.0","id":1,"result":"ok"}`), nil
	}

	// Use watchdog-specific worker ID so it doesn't collide with the child's relay connection.
	watchdogWorkerID := workerID + "-watchdog"
	rc := relay.NewRelayClient(cfg.Relay.URL, watchdogWorkerID, func() string { return "" }, handler)
	if err := rc.Connect(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "watchdog: relay connect (best-effort): %v\n", err)
		return nil
	}
	fmt.Fprintf(os.Stderr, "watchdog: relay connected (worker_id=%s)\n", watchdogWorkerID)
	return rc
}
