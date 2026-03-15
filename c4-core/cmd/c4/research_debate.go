//go:build research

package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/changmin/c4-core/internal/knowledge"
	"github.com/changmin/c4-core/internal/llm"
	"github.com/changmin/c4-core/internal/mcp"
	"github.com/changmin/c4-core/internal/serve/orchestrator"
)

func init() { registerInitHook(registerResearchDebateHandler) }

func registerResearchDebateHandler(ictx *initContext) error {
	if ictx.knowledgeStore == nil || ictx.llmGateway == nil {
		return nil
	}
	caller := &debateLLMCaller{gw: ictx.llmGateway}
	store := &knowledgeStoreAdapter{s: ictx.knowledgeStore}
	ictx.reg.RegisterBlocking(mcp.ToolSchema{
		Name:        "c4_research_debate",
		Description: "Trigger multi-agent debate (Optimizer + Skeptic) on a hypothesis, record TypeDebate knowledge doc, and generate next hypothesis draft",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"hypothesis_id":  map[string]any{"type": "string"},
				"trigger_reason": map[string]any{"type": "string", "enum": []string{"dod_success", "dod_null", "escalation", "manual"}},
				"context":         map[string]any{"type": "string", "description": "Optional additional context"},
				"lineage_context": map[string]any{"type": "string", "description": "Hypothesis lineage context from LineageBuilder"},
			},
			"required": []string{"hypothesis_id"},
		},
	}, func(ctx context.Context, rawArgs json.RawMessage) (any, error) {
		var params struct {
			HypothesisID   string `json:"hypothesis_id"`
			TriggerReason  string `json:"trigger_reason"`
			Context        string `json:"context"`
			LineageContext string `json:"lineage_context"`
		}
		if len(rawArgs) > 0 {
			if err := json.Unmarshal(rawArgs, &params); err != nil {
				return nil, fmt.Errorf("parsing arguments: %w", err)
			}
		}
		if params.TriggerReason == "" {
			params.TriggerReason = "manual"
		}
		return orchestrator.RunDebate(ctx, caller, store, params.HypothesisID, params.TriggerReason, params.Context, params.LineageContext)
	})
	return nil
}

// debateLLMCaller adapts *llm.Gateway to orchestrator.DebateCaller.
type debateLLMCaller struct{ gw *llm.Gateway }

func (d *debateLLMCaller) Call(ctx context.Context, system, user string) (string, error) {
	resp, err := d.gw.Chat(ctx, "research_debate", &llm.ChatRequest{
		System:   system,
		Messages: []llm.Message{{Role: "user", Content: user}},
	})
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}

// knowledgeStoreAdapter wraps *knowledge.Store to implement orchestrator.DebateStore.
type knowledgeStoreAdapter struct{ s *knowledge.Store }

func (a *knowledgeStoreAdapter) Get(id string) (*knowledge.Document, error) { return a.s.Get(id) }
func (a *knowledgeStoreAdapter) Create(dt knowledge.DocumentType, meta map[string]any, body string) (string, error) {
	return a.s.Create(dt, meta, body)
}
