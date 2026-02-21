//go:build llm_gateway

package main

import (
	"fmt"
	"os"

	"github.com/changmin/c4-core/internal/llm"
	"github.com/changmin/c4-core/internal/mcp/handlers"
)

func init() {
	registerPreStoreHook(initLLM)
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
	handlers.RegisterLLMHandlers(ctx.reg, gw)
	fmt.Fprintf(os.Stderr, "cq: LLM gateway enabled (%d providers)\n", gw.ProviderCount())
	return nil
}
