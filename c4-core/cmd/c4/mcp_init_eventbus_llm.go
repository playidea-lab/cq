//go:build c3_eventbus && llm_gateway

package main

import (
	"context"
	"fmt"
	"os"

	"github.com/changmin/c4-core/internal/eventbus"
	"github.com/changmin/c4-core/internal/llm"
)

func init() {
	registerEBWireHook(wireLLMCallerEventBus)
}

// gatewayLLMCaller adapts *llm.Gateway to the eventbus.LLMCaller interface.
// It lives in cmd/c4 to avoid a cross-import between eventbus and llm packages.
type gatewayLLMCaller struct {
	gw *llm.Gateway
}

func (g *gatewayLLMCaller) Call(ctx context.Context, systemPrompt, userMessage, model string) (string, error) {
	req := &llm.ChatRequest{
		Model:  model,
		System: systemPrompt,
		Messages: []llm.Message{
			{Role: "user", Content: userMessage},
		},
	}
	resp, err := g.gw.Chat(ctx, "dooray_respond_llm", req)
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}

// wireLLMCallerEventBus wires the LLM Gateway (if available) to the EventBus Dispatcher.
// This enables the "dooray_respond_llm" action type.
func wireLLMCallerEventBus(ctx *initContext, _ *eventbus.Client) {
	if ctx.embeddedEB == nil {
		return
	}
	var gw *llm.Gateway
	if ctx.llmGateway != nil {
		gw = ctx.llmGateway
	} else if ctx.cfgMgr != nil && ctx.cfgMgr.GetConfig().LLMGateway.Enabled {
		gw = llm.NewGatewayFromConfig(toLLMGatewayConfig(ctx.cfgMgr, ctx.secretStore))
	}
	if gw == nil {
		return
	}
	ctx.embeddedEB.Dispatcher().SetLLMCaller(&gatewayLLMCaller{gw: gw})
	fmt.Fprintln(os.Stderr, "cq: eventbus dooray_respond_llm LLM caller wired")
}
