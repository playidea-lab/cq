//go:build research

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/changmin/c4-core/internal/eventbus"
	"github.com/changmin/c4-core/internal/serve/orchestrator"
)

// loopOrchestratorCancel holds the cancel func for the standalone orchestrator goroutine.
var loopOrchestratorCancel context.CancelFunc

func init() {
	registerInitHook(initLoopOrchestrator)
	registerShutdownHook(shutdownLoopOrchestrator)
}

// initLoopOrchestrator creates a standalone LoopOrchestrator for the cq mcp context.
// In cq serve, the orchestrator is registered as a serve component (serve_loop_orchestrator.go).
// In cq mcp, we initialize it here so that MCP tools (c4_research_loop_start/stop/status) work.
func initLoopOrchestrator(ctx *initContext) error {
	// Skip if already set by serve path.
	if ctx.loopOrchestrator != nil {
		return nil
	}
	if ctx.knowledgeStore == nil || ctx.llmGateway == nil {
		return nil
	}

	var hc orchestrator.HubClient
	if ctx.hubClient != nil {
		if c, ok := ctx.hubClient.(orchestrator.HubClient); ok {
			hc = c
		}
	}

	cfg := orchestrator.Config{
		Store: ctx.knowledgeStore,
		Hub:   hc,
	}
	o := orchestrator.New(cfg)

	// Wire debate components.
	caller := &debateLLMCaller{gw: ctx.llmGateway}
	store := &knowledgeStoreAdapter{s: ctx.knowledgeStore}
	var hubCli orchestrator.LoopHubClient
	if hc != nil {
		hubCli = orchestrator.NewHubClientAdapter(hc)
	}
	o.WireDebateComponents(caller, store, ctx.knowledgeStore, hubCli)

	// Wire SpecPipeline.
	o.SetSpecPipeline(caller, store)

	// Gate duration from config (default 24h).
	gateDur := 24 * time.Hour
	if ctx.cfgMgr != nil {
		if s := ctx.cfgMgr.GetConfig().Serve.ResearchLoop.GateDuration; s != "" {
			if d, err := time.ParseDuration(s); err == nil && d > 0 {
				gateDur = d
			}
		}
	}
	gate := orchestrator.NewGateController(gateDur)
	state := orchestrator.NewStateYAMLWriter(filepath.Join(ctx.projectDir, ".c9"))
	notify := orchestrator.NewNotifyBridge(nil, 5*time.Minute)
	o.SetupComponents(gate, state, notify)

	// Wire EventBus subscriber if available.
	if ebc, ok := ctx.ebClient.(*eventbus.Client); ok && ebc != nil {
		o.EBSub = ebc
	}

	// Start the orchestrator polling loop in background.
	bgCtx, cancel := context.WithCancel(context.Background())
	loopOrchestratorCancel = cancel
	go func() {
		if err := o.Start(bgCtx); err != nil {
			fmt.Fprintf(os.Stderr, "cq mcp: loop_orchestrator start error: %v\n", err)
		}
	}()

	ctx.loopOrchestrator = o
	fmt.Fprintf(os.Stderr, "cq mcp: registered loop_orchestrator (standalone, type=%T)\n", o)
	return nil
}

func shutdownLoopOrchestrator(_ *initContext) {
	if loopOrchestratorCancel != nil {
		loopOrchestratorCancel()
	}
}
