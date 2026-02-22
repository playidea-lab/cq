package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/changmin/c4-core/internal/config"
	"github.com/changmin/c4-core/internal/serve"
)

func init() {
	registerShutdownHook(shutdownAgent)
}

// startAgentIfNeeded starts the Agent component inside the MCP server process
// if all guard conditions are met:
//  1. cfg.Serve.Agent.Enabled == true
//  2. cfg.Cloud.URL != "" (Supabase configured — same guard as serve_components.go)
//  3. IsServeRunningCtx(ctx) == false (cq serve is not already managing Agent)
//
// ctx is derived from context.Background() and is cancelled by the shutdownAgent
// hook via ctx4.agentCancel when the MCP server shuts down.
// A 30s recheck goroutine monitors for cq serve starting and yields to it.
//
// Called from newMCPServer() after all component init hooks have run.
func startAgentIfNeeded(ctx context.Context, ctx4 *initContext) {
	if ctx4.cfgMgr == nil {
		return
	}
	cfg := ctx4.cfgMgr.GetConfig()
	startAgentIfNeededWith(ctx, ctx4, cfg, serve.IsServeRunningCtx)
}

// startAgentIfNeededWith is the testable inner function.
// isServeRunning is injectable for tests.
func startAgentIfNeededWith(
	ctx context.Context,
	ctx4 *initContext,
	cfg config.C4Config,
	isServeRunning func(context.Context) bool,
) {
	// Guard 1: agent must be enabled
	if !cfg.Serve.Agent.Enabled {
		return
	}

	// Guard 2: Supabase must be configured (same condition as serve_components.go)
	if cfg.Cloud.URL == "" || cfg.Cloud.AnonKey == "" {
		return
	}

	// Guard 3: cq serve must not already be running (it manages Agent)
	if isServeRunning(ctx) {
		fmt.Fprintln(os.Stderr, serve.StatusMessage("agent"))
		return
	}

	agent := serve.NewAgent(serve.AgentConfig{
		SupabaseURL: cfg.Cloud.URL,
		APIKey:      cfg.Cloud.AnonKey,
		ProjectID:   cfg.Cloud.ProjectID,
		ProjectDir:  projectDir,
	})
	ctx4.agentComp = agent

	// Start agent in a background goroutine.
	// agent.Start runs until ctx is done (cancelled by shutdownAgent hook).
	go func() {
		if err := agent.Start(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "cq: [mcp-agent] start failed: %v\n", err)
		}
	}()

	// 30s recheck goroutine: yield to cq serve if it starts after us.
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				// Use context-derived timeout so HTTP check cancels immediately
				// if ctx is already done.
				if isServeRunning(ctx) {
					fmt.Fprintln(os.Stderr, "cq: [mcp-agent] cq serve detected, stopping embedded agent")
					if ctx4.agentCancel != nil {
						ctx4.agentCancel()
					}
					return
				}
			}
		}
	}()

	fmt.Fprintln(os.Stderr, "cq: [mcp-agent] agent started (MCP-embedded)")
}

// shutdownAgent stops the MCP-embedded Agent if it was started.
// Called by the componentShutdownHooks in mcpServer.shutdown().
func shutdownAgent(ctx *initContext) {
	if ctx.agentCancel != nil {
		ctx.agentCancel()
	}
	if ctx.agentComp != nil {
		_ = ctx.agentComp.Stop(context.Background())
	}
}
