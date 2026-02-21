//go:build c1_messenger

package main

import (
	"fmt"
	"os"

	"github.com/changmin/c4-core/internal/eventbus"
	"github.com/changmin/c4-core/internal/llm"
	"github.com/changmin/c4-core/internal/mcp/handlers"
)

func init() {
	registerInitHook(initC1)
	registerEBWireHook(wireC1EventBus)
}

// initC1 registers C1 Messenger handlers and creates the ContextKeeper.
func initC1(ctx *initContext) error {
	if ctx.cfgMgr == nil || !ctx.cfgMgr.GetConfig().Cloud.Enabled {
		return nil
	}
	cloudCfg := ctx.cfgMgr.GetConfig().Cloud
	if cloudCfg.URL == "" || cloudCfg.AnonKey == "" || ctx.cloudTP.Token() == "" || ctx.cloudProjectID == "" {
		return nil
	}
	c1Handler := handlers.NewC1Handler(cloudCfg.URL+"/rest/v1", cloudCfg.AnonKey, ctx.cloudTP, ctx.cloudProjectID)
	handlers.RegisterC1Handlers(ctx.reg, c1Handler)

	// Create ContextKeeper — use existing LLM gateway or create a dedicated one
	var keeperGateway *llm.Gateway
	if ctx.llmGateway != nil {
		keeperGateway = ctx.llmGateway
	} else if ctx.cfgMgr.GetConfig().LLMGateway.Enabled {
		keeperGateway = llm.NewGatewayFromConfig(toLLMGatewayConfig(ctx.cfgMgr, ctx.secretStore))
	}
	ctx.keeper = handlers.NewContextKeeper(c1Handler, keeperGateway)
	if err := ctx.keeper.EnsureSystemChannels(); err != nil {
		fmt.Fprintf(os.Stderr, "cq: system channels setup failed: %v\n", err)
	}
	fmt.Fprintln(os.Stderr, "cq: c1 enabled (3 tools + keeper)")
	return nil
}

// wireC1EventBus wires the eventbus to C1-related components (persona + soul).
func wireC1EventBus(ctx *initContext, ebClient *eventbus.Client) {
	handlers.SetPersonaEventBus(ebClient, ctx.sqliteStore.GetProjectID())
	handlers.SetSoulEventBus(ebClient, ctx.sqliteStore.GetProjectID())
}
