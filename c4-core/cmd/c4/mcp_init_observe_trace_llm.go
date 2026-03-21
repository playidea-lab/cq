//go:build c7_observe && llm_gateway

package main

import "github.com/changmin/c4-core/internal/mcp/handlers"

func init() {
	// Wire llm.Gateway into trace state after both observe and LLM are initialized.
	// initObserve (preStoreHook) sets trace state; initLLM (preStoreHook) sets ctx.llmGateway.
	registerPreStoreHook(wireObserveTraceLLM)
}

// wireObserveTraceLLM connects the llm.Gateway to the trace observe handlers so
// c4_observe_policy can compare the current routing table with suggested routes.
// No-op if either component is disabled.
func wireObserveTraceLLM(ctx *initContext) error {
	if ctx.llmGateway == nil {
		return nil
	}
	handlers.SetObserveTraceGateway(ctx.llmGateway)
	return nil
}
