//go:build research

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/changmin/c4-core/internal/knowledge"
	"github.com/changmin/c4-core/internal/llm"
	"github.com/changmin/c4-core/internal/mcp"
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
				"context":        map[string]any{"type": "string", "description": "Optional additional context"},
			},
			"required": []string{"hypothesis_id"},
		},
	}, func(ctx context.Context, rawArgs json.RawMessage) (any, error) {
		var params struct {
			HypothesisID  string `json:"hypothesis_id"`
			TriggerReason string `json:"trigger_reason"`
			Context       string `json:"context"`
		}
		if len(rawArgs) > 0 {
			if err := json.Unmarshal(rawArgs, &params); err != nil {
				return nil, fmt.Errorf("parsing arguments: %w", err)
			}
		}
		if params.TriggerReason == "" {
			params.TriggerReason = "manual"
		}
		return runDebate(ctx, caller, store, params.HypothesisID, params.TriggerReason, params.Context)
	})
	return nil
}

// debateCaller is the interface for testability.
type debateCaller interface {
	call(ctx context.Context, system, user string) (string, error)
}

// debateLLMCaller adapts *llm.Gateway for use in the debate handler.
type debateLLMCaller struct{ gw *llm.Gateway }

func (d *debateLLMCaller) call(ctx context.Context, system, user string) (string, error) {
	resp, err := d.gw.Chat(ctx, "research_debate", &llm.ChatRequest{
		System:   system,
		Messages: []llm.Message{{Role: "user", Content: user}},
	})
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}

// debateStore abstracts knowledge.Store for testability.
type debateStore interface {
	get(id string) (*knowledge.Document, error)
	create(docType knowledge.DocumentType, metadata map[string]any, body string) (string, error)
}

// knowledgeStoreAdapter wraps *knowledge.Store to implement debateStore.
type knowledgeStoreAdapter struct{ s *knowledge.Store }

func (a *knowledgeStoreAdapter) get(id string) (*knowledge.Document, error) { return a.s.Get(id) }
func (a *knowledgeStoreAdapter) create(dt knowledge.DocumentType, meta map[string]any, body string) (string, error) {
	return a.s.Create(dt, meta, body)
}

// runDebate executes the Optimizer→Skeptic→Synthesis debate flow.
func runDebate(ctx context.Context, caller debateCaller, store debateStore, hypID, triggerReason, extraContext string) (any, error) {
	if hypID == "" {
		return nil, fmt.Errorf("hypothesis_id required")
	}

	hypDoc, err := store.get(hypID)
	if err != nil || hypDoc == nil {
		return nil, fmt.Errorf("hypothesis not found: %s", hypID)
	}

	userMsg := fmt.Sprintf("Hypothesis: %s\nTrigger: %s\nContext: %s\n\nHypothesis body:\n%s",
		hypID, triggerReason, extraContext, hypDoc.Body)

	optimizerSystem := `You are a research direction optimizer. Analyze the hypothesis and experimental results. Propose the most promising next research direction. Format: DIRECTION: [direction], RATIONALE: [rationale], NEXT_HYPOTHESIS: [draft hypothesis text]`
	skepticSystem := `You are a research hypothesis critic. Challenge the current hypothesis and proposed directions. Identify blind spots, alternative explanations, and exploration directions being ignored. Format: CHALLENGE: [main challenge], ALTERNATIVE: [alternative direction], VERDICT: [approved|null_result|escalate]`

	optimizerOut, err := caller.call(ctx, optimizerSystem, userMsg)
	if err != nil {
		return nil, fmt.Errorf("optimizer: %w", err)
	}

	skepticOut, err := caller.call(ctx, skepticSystem, userMsg)
	if err != nil {
		return nil, fmt.Errorf("skeptic: %w", err)
	}

	synthSystem := `You are a research synthesis expert. Given optimizer and skeptic perspectives, determine the final verdict and next hypothesis. Output JSON: {"verdict":"approved|null_result|escalate","next_hypothesis_draft":"...","experiment_spec_draft":"..."}`
	synthUser := fmt.Sprintf("Optimizer:\n%s\n\nSkeptic:\n%s", optimizerOut, skepticOut)
	synthOut, err := caller.call(ctx, synthSystem, synthUser)
	if err != nil {
		return nil, fmt.Errorf("synthesis: %w", err)
	}

	// Determine verdict: try synth JSON first, fall back to skeptic text.
	verdict := "approved"
	var synthJSON struct {
		Verdict string `json:"verdict"`
	}
	if start := strings.Index(synthOut, "{"); start >= 0 {
		if err := json.Unmarshal([]byte(synthOut[start:]), &synthJSON); err == nil && synthJSON.Verdict != "" {
			verdict = synthJSON.Verdict
		}
	}
	if verdict == "approved" {
		lower := strings.ToLower(skepticOut)
		if strings.Contains(lower, "verdict: null_result") {
			verdict = "null_result"
		} else if strings.Contains(lower, "verdict: escalate") {
			verdict = "escalate"
		}
	}

	// Extract next_hypothesis_draft from optimizer
	nextHypDraft := ""
	if idx := strings.Index(strings.ToUpper(optimizerOut), "NEXT_HYPOTHESIS:"); idx >= 0 {
		nextHypDraft = strings.TrimSpace(optimizerOut[idx+16:])
		if nl := strings.Index(nextHypDraft, "\n"); nl >= 0 {
			nextHypDraft = nextHypDraft[:nl]
		}
	}

	debateBody := fmt.Sprintf("## Debate Record\n\nhypothesis_id: %s\ntrigger_reason: %s\n\n### Optimizer\n%s\n\n### Skeptic\n%s\n\n### Synthesis\n%s",
		hypID, triggerReason, optimizerOut, skepticOut, synthOut)

	debateDocID, err := store.create(knowledge.TypeDebate, map[string]any{
		"title":          "Debate: " + hypID,
		"hypothesis_id":  hypID,
		"trigger_reason": triggerReason,
		"verdict":        verdict,
	}, debateBody)
	if err != nil {
		return nil, fmt.Errorf("create debate doc: %w", err)
	}

	return map[string]any{
		"debate_doc_id":         debateDocID,
		"verdict":               verdict,
		"next_hypothesis_draft": nextHypDraft,
		"experiment_spec_draft": synthOut,
	}, nil
}
