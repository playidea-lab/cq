//go:build c7_observe && llm_gateway

package main

import "github.com/changmin/c4-core/internal/llm"

func init() {
	// Wire TraceCollector → Gateway after both observe and LLM are initialized.
	// initObserve (preStoreHook) sets ctx.traceCollector.
	// initLLM (preStoreHook) sets ctx.llmGateway.
	// This hook runs after all preStoreHooks complete.
	registerPreStoreHook(wireObserveLLM)
}

// wireObserveLLM connects the TraceCollector to the LLM Gateway via the
// package-level TraceHook setter. No-op if either component is disabled.
func wireObserveLLM(ctx *initContext) error {
	if ctx.traceCollector == nil || ctx.llmGateway == nil {
		return nil
	}
	llm.SetTraceHook(ctx.traceCollector)
	return nil
}
