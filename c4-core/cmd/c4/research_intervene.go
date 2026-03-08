//go:build research

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/changmin/c4-core/internal/knowledge"
	"github.com/changmin/c4-core/internal/mcp"
	"github.com/changmin/c4-core/internal/research"
	"github.com/google/uuid"
)

func init() {
	registerInitHook(registerResearchInterveneHandler)
}

// registerResearchInterveneHandler registers the c4_research_intervene MCP tool.
func registerResearchInterveneHandler(ctx *initContext) error {
	if ctx.researchStore == nil || ctx.knowledgeStore == nil {
		return nil
	}
	ctx.reg.Register(mcp.ToolSchema{
		Name:        "c4_research_intervene",
		Description: "자율 루프에 사람이 개입합니다. 사람의 판단이 항상 루프 자동 판단보다 우선합니다.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"loop_id":          map[string]any{"type": "string", "description": "개입할 루프 ID"},
				"type":             map[string]any{"type": "string", "enum": []string{"steering", "injection", "abort"}, "description": "개입 유형"},
				"context":          map[string]any{"type": "string", "description": "Steering: 다음 Debate에 주입할 추가 컨텍스트"},
				"hypothesis_draft": map[string]any{"type": "string", "description": "Injection: 삽입할 새 가설 텍스트"},
				"abort_reason":     map[string]any{"type": "string", "description": "Abort: 취소 사유"},
			},
			"required": []string{"loop_id", "type"},
		},
	}, researchInterveneHandler(ctx.researchStore, ctx.knowledgeStore))
	return nil
}

func researchInterveneHandler(rs *research.Store, ks *knowledge.Store) mcp.HandlerFunc {
	return func(rawArgs json.RawMessage) (any, error) {
		var params struct {
			LoopID          string `json:"loop_id"`
			Type            string `json:"type"`
			Context         string `json:"context"`
			HypothesisDraft string `json:"hypothesis_draft"`
			AbortReason     string `json:"abort_reason"`
		}
		if len(rawArgs) > 0 {
			if err := json.Unmarshal(rawArgs, &params); err != nil {
				return nil, fmt.Errorf("parsing arguments: %w", err)
			}
		}
		if params.LoopID == "" {
			return nil, errors.New("loop_id is required")
		}
		if params.Type == "" {
			return nil, errors.New("type is required")
		}

		project, err := rs.GetProject(params.LoopID)
		if err != nil {
			return nil, fmt.Errorf("get project: %w", err)
		}
		if project == nil {
			return nil, errors.New("loop not found: " + params.LoopID)
		}

		interventionID := "iv-" + uuid.New().String()[:8]
		now := time.Now().UTC()

		switch params.Type {
		case "steering":
			return applySteering(rs, ks, project, params.LoopID, interventionID, params.Context, now)
		case "injection":
			return applyInjection(rs, ks, project, params.LoopID, interventionID, params.HypothesisDraft, now)
		case "abort":
			return applyAbort(rs, ks, project, params.LoopID, interventionID, params.AbortReason, now)
		default:
			return nil, errors.New("type must be 'steering', 'injection', or 'abort'")
		}
	}
}

// applySteering injects additional context into the current iteration's next Debate.
// The loop continues running; context is recorded in the knowledge store.
func applySteering(rs *research.Store, ks *knowledge.Store, project *research.Project, loopID, interventionID, extraContext string, now time.Time) (any, error) {
	if extraContext == "" {
		return nil, errors.New("context is required for steering intervention")
	}

	current, err := rs.GetCurrentIteration(loopID)
	if err != nil {
		return nil, fmt.Errorf("get current iteration: %w", err)
	}

	// Record steering context as a knowledge insight for the next Debate to pick up.
	body := fmt.Sprintf("## Steering Intervention\n\nloop_id: %s\nintervention_id: %s\napplied_at: next_debate\n\n### Context\n%s",
		loopID, interventionID, extraContext)
	meta := map[string]any{
		"title":             fmt.Sprintf("Steering: %s", interventionID),
		"domain":            "research",
		"tags":              []string{"research-steering", loopID},
		"intervention_id":   interventionID,
		"intervention_type": "steering",
		"loop_id":           loopID,
	}
	if current != nil {
		meta["iteration_id"] = current.ID
	}
	if _, err := ks.Create(knowledge.TypeInsight, meta, body); err != nil {
		return nil, fmt.Errorf("record steering context: %w", err)
	}

	return map[string]any{
		"intervention_id": interventionID,
		"type":            "steering",
		"applied_at":      "next_debate",
		"loop_status":     "running",
	}, nil
}

// applyInjection inserts a new TypeHypothesis into the loop for parallel exploration.
func applyInjection(rs *research.Store, ks *knowledge.Store, project *research.Project, loopID, interventionID, hypothesisDraft string, now time.Time) (any, error) {
	if hypothesisDraft == "" {
		return nil, errors.New("hypothesis_draft is required for injection intervention")
	}

	title := strings.TrimSpace(hypothesisDraft)
	if len([]rune(title)) > 80 {
		title = string([]rune(title)[:80])
	}

	body := fmt.Sprintf("## Injected Hypothesis\n\nloop_id: %s\nintervention_id: %s\n\n### Hypothesis\n%s",
		loopID, interventionID, hypothesisDraft)
	hypID, err := ks.Create(knowledge.TypeHypothesis, map[string]any{
		"title":             title,
		"domain":            "research",
		"tags":              []string{"research-injection", loopID},
		"hypothesis":        hypothesisDraft,
		"hypothesis_status": "pending",
		"intervention_id":   interventionID,
		"loop_id":           loopID,
	}, body)
	if err != nil {
		return nil, fmt.Errorf("create hypothesis: %w", err)
	}

	return map[string]any{
		"intervention_id": interventionID,
		"type":            "injection",
		"applied_at":      "immediately",
		"loop_status":     "running",
		"hypothesis_id":   hypID,
	}, nil
}

// applyAbort pauses the loop and records a TypeDebate(human_abort) knowledge doc.
func applyAbort(rs *research.Store, ks *knowledge.Store, project *research.Project, loopID, interventionID, abortReason string, now time.Time) (any, error) {
	if abortReason == "" {
		return nil, errors.New("abort_reason is required for abort intervention")
	}

	// Pause the project to stop the loop.
	if err := rs.UpdateProject(loopID, map[string]any{"status": "paused"}); err != nil {
		return nil, fmt.Errorf("pause project: %w", err)
	}

	// Record human_abort as a TypeDebate knowledge doc.
	body := fmt.Sprintf("## Human Abort\n\nloop_id: %s\nintervention_id: %s\ntrigger_reason: human_abort\n\n### Abort Reason\n%s",
		loopID, interventionID, abortReason)
	debateID, err := ks.Create(knowledge.TypeDebate, map[string]any{
		"title":           fmt.Sprintf("Abort: %s", interventionID),
		"domain":          "research",
		"tags":            []string{"research-abort", loopID},
		"trigger_reason":  "human_abort",
		"verdict":         "aborted",
		"intervention_id": interventionID,
		"loop_id":         loopID,
	}, body)
	if err != nil {
		return nil, fmt.Errorf("record abort debate: %w", err)
	}

	return map[string]any{
		"intervention_id": interventionID,
		"type":            "abort",
		"applied_at":      "immediately",
		"loop_status":     "aborted",
		"debate_doc_id":   debateID,
	}, nil
}
