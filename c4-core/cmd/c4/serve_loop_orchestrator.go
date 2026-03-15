//go:build research

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/changmin/c4-core/internal/eventbus"
	"github.com/changmin/c4-core/internal/serve"
	"github.com/changmin/c4-core/internal/serve/orchestrator"
)

// registerLoopOrchestratorComponent registers the LoopOrchestrator in the serve ecosystem.
func registerLoopOrchestratorComponent(mgr *serve.Manager, ictx *initContext) {
	if ictx.knowledgeStore == nil || ictx.llmGateway == nil {
		return
	}
	// Hub is optional; LoopOrchestrator can run without it (StartLoop will require it).
	var hc orchestrator.HubClient
	if ictx.hubClient != nil {
		if c, ok := ictx.hubClient.(orchestrator.HubClient); ok {
			hc = c
		}
	}
	cfg := orchestrator.Config{
		Store: ictx.knowledgeStore,
		Hub:   hc,
	}
	o := orchestrator.New(cfg)

	// Wire debate components — reuse adapters from research_debate.go.
	caller := &debateLLMCaller{gw: ictx.llmGateway}
	store := &knowledgeStoreAdapter{s: ictx.knowledgeStore}
	var hubCli orchestrator.LoopHubClient
	if hc != nil {
		hubCli = orchestrator.NewHubClientAdapter(hc)
	}
	o.WireDebateComponents(caller, store, ictx.knowledgeStore, hubCli)

	// Wire SpecPipeline: reuse the debate caller and knowledge store adapter.
	o.SetSpecPipeline(caller, store)

	// Gate duration from config (default 24h).
	gateDur := 24 * time.Hour
	if ictx.cfgMgr != nil {
		if s := ictx.cfgMgr.GetConfig().Serve.ResearchLoop.GateDuration; s != "" {
			if d, err := time.ParseDuration(s); err == nil && d > 0 {
				gateDur = d
			}
		}
	}
	gate := orchestrator.NewGateController(gateDur)
	state := orchestrator.NewStateYAMLWriter(filepath.Join(ictx.projectDir, ".c9"))
	notify := orchestrator.NewNotifyBridge(nil, 5*time.Minute)
	o.SetupComponents(gate, state, notify)

	// Wire EventBus subscriber if available (c3_eventbus + research tags active).
	if ebc, ok := ictx.ebClient.(*eventbus.Client); ok && ebc != nil {
		o.EBSub = ebc
	}

	mgr.Register(o)
	ictx.loopOrchestrator = o
	fmt.Fprintf(os.Stderr, "cq serve: registered loop_orchestrator\n")
}

// debateLLMCaller and knowledgeStoreAdapter are defined in research_debate.go.
