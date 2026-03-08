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

func init() {
	registerInitHook(registerResearchCheckpointHandler)
}

// registerResearchCheckpointHandler registers the c4_research_checkpoint MCP tool.
func registerResearchCheckpointHandler(ctx *initContext) error {
	if ctx.knowledgeStore == nil || ctx.llmGateway == nil {
		return nil
	}
	caller := &checkpointGatewayLLMCaller{gw: ctx.llmGateway}
	ctx.reg.RegisterBlocking(mcp.ToolSchema{
		Name:        "c4_research_checkpoint",
		Description: "Review an ExperimentSpec DoD with LLM-Optimizer and LLM-Skeptic roles to assess validity before Inner Loop starts",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"spec_id":       map[string]any{"type": "string", "description": "TypeExperiment spec doc ID"},
				"hypothesis_id": map[string]any{"type": "string", "description": "Optional hypothesis ID"},
			},
			"required": []string{"spec_id"},
		},
	}, researchCheckpointHandler(ctx.knowledgeStore, caller))
	return nil
}

// checkpointCaller abstracts LLM calls for testability.
type checkpointCaller interface {
	call(ctx context.Context, system, user string) (string, error)
}

// checkpointGatewayLLMCaller adapts *llm.Gateway for use in the checkpoint handler.
type checkpointGatewayLLMCaller struct {
	gw *llm.Gateway
}

func (c *checkpointGatewayLLMCaller) call(ctx context.Context, system, user string) (string, error) {
	resp, err := c.gw.Chat(ctx, "research_checkpoint", &llm.ChatRequest{
		System:   system,
		Messages: []llm.Message{{Role: "user", Content: user}},
	})
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}

const checkpointOptimizerSystem = `You are an experiment design optimization expert. Analyze the ExperimentSpec DoD for:
1. Whether the success_condition clearly confirms the hypothesis
2. Whether the null_condition is measurable and distinct
3. Whether controlled_variables are sufficient
Respond with: ASSESSMENT: [positive/negative], FEEDBACK: [your analysis]`

const checkpointSkepticSystem = `You are an experiment design critic. Challenge the ExperimentSpec DoD:
1. Attribution ambiguity (multiple variables changed at once?)
2. Unmeasurable conditions (how will success/null be determined?)
3. Escalation trigger vagueness
Respond with: ASSESSMENT: [positive/negative], FEEDBACK: [your analysis], ISSUES: [list any specific problems]`

func researchCheckpointHandler(store *knowledge.Store, caller checkpointCaller) mcp.BlockingHandlerFunc {
	return func(ctx context.Context, rawArgs json.RawMessage) (any, error) {
		var params struct {
			SpecID       string `json:"spec_id"`
			HypothesisID string `json:"hypothesis_id"`
		}
		if len(rawArgs) > 0 {
			if err := json.Unmarshal(rawArgs, &params); err != nil {
				return nil, fmt.Errorf("parsing arguments: %w", err)
			}
		}
		if params.SpecID == "" {
			return nil, fmt.Errorf("spec_id required")
		}

		doc, err := store.Get(params.SpecID)
		if err != nil {
			return nil, fmt.Errorf("spec not found: %s: %w", params.SpecID, err)
		}
		if doc == nil {
			return nil, fmt.Errorf("spec not found: %s", params.SpecID)
		}

		userMsg := fmt.Sprintf("ExperimentSpec:\n%s", doc.Body)

		optimizerOut, err := caller.call(ctx, checkpointOptimizerSystem, userMsg)
		if err != nil {
			return nil, fmt.Errorf("optimizer LLM: %w", err)
		}

		skepticOut, err := caller.call(ctx, checkpointSkepticSystem, userMsg)
		if err != nil {
			return nil, fmt.Errorf("skeptic LLM: %w", err)
		}

		verdict := "approved"
		var suggestions []string
		upperSkeptic := strings.ToUpper(skepticOut)
		if strings.Contains(upperSkeptic, "ASSESSMENT: NEGATIVE") ||
			strings.Contains(upperSkeptic, "ISSUES:") {
			verdict = "revision_requested"
			// Search in original string to avoid multi-byte offset mismatch.
			if idx := strings.Index(skepticOut, "ISSUES:"); idx >= 0 {
				issueText := skepticOut[idx+7:]
				for _, line := range strings.Split(issueText, "\n") {
					line = strings.TrimSpace(line)
					if line != "" {
						suggestions = append(suggestions, line)
					}
				}
			}
		}

		result := map[string]any{
			"verdict":            verdict,
			"optimizer_feedback": optimizerOut,
			"skeptic_feedback":   skepticOut,
			"suggestions":        suggestions,
		}
		b, _ := json.Marshal(result)
		return string(b), nil
	}
}
