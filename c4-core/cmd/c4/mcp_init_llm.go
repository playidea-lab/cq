//go:build llm_gateway

package main

import (
	"fmt"
	"os"

	"github.com/changmin/c4-core/internal/llm"
	"github.com/changmin/c4-core/internal/mcp/handlers/llmhandler"
)

func init() {
	registerPreStoreHook(initLLM)
	registerShutdownHook(shutdownLLM)
}

// initLLM creates the LLM Gateway and registers LLM handlers.
// Runs as a pre-store hook so ctx.llmGateway is available for NativeOpts
// (knowledge distill) and for c1 keeper creation.
func initLLM(ctx *initContext) error {
	if ctx.cfgMgr == nil || !ctx.cfgMgr.GetConfig().LLMGateway.Enabled {
		return nil
	}
	gw := llm.NewGatewayFromConfig(toLLMGatewayConfig(ctx.cfgMgr, ctx.secretStore))
	ctx.llmGateway = gw
	// Wire async SQLite persistence: DB is opened in core init before pre-store hooks.
	if ctx.db != nil {
		gw.Tracker().SetDB(ctx.db)
	}
	llmhandler.RegisterLLMHandlers(ctx.reg, gw)
	fmt.Fprintf(os.Stderr, "cq: LLM gateway enabled (%d providers)\n", gw.ProviderCount())
	return nil
}

// shutdownLLM drains the cost tracker's async write buffer before the DB is closed.
func shutdownLLM(ctx *initContext) {
	if ctx.llmGateway != nil {
		ctx.llmGateway.Tracker().Close()
	}
}
