package knowledgehandler

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/changmin/c4-core/internal/knowledge"
	"github.com/changmin/c4-core/internal/mcp"
)

// RegisterResearchLoopHandlers registers the c4_research_loop_start tool.
func RegisterResearchLoopHandlers(reg *mcp.Registry, opts *KnowledgeNativeOpts) {
	reg.Register(mcp.ToolSchema{
		Name:        "c4_research_loop_start",
		Description: "자율 연구 루프 시작 — hypothesis_id를 기점으로 실험→Debate→가설 등록 루프를 자동 실행",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"hypothesis_id":        map[string]any{"type": "string", "description": "루프 시작 기점 가설 ID"},
				"max_iterations":       map[string]any{"type": "integer", "description": "최대 루프 횟수 (기본: 무제한)"},
				"max_cost_usd":         map[string]any{"type": "number", "description": "비용 상한 USD (기본: 무제한)"},
				"null_result_threshold": map[string]any{"type": "integer", "description": "연속 null_result 시 explore 강제 전환 횟수 (기본: 2)"},
			},
			"required": []string{"hypothesis_id"},
		},
	}, researchLoopStartHandler(opts))
}

func researchLoopStartHandler(opts *KnowledgeNativeOpts) mcp.HandlerFunc {
	return func(rawArgs json.RawMessage) (any, error) {
		var params struct {
			HypothesisID        string   `json:"hypothesis_id"`
			MaxIterations       *int     `json:"max_iterations"`
			MaxCostUSD          *float64 `json:"max_cost_usd"`
			NullResultThreshold *int     `json:"null_result_threshold"`
		}
		if len(rawArgs) > 0 {
			if err := json.Unmarshal(rawArgs, &params); err != nil {
				return map[string]any{"error": fmt.Sprintf("invalid arguments: %v", err)}, nil
			}
		}
		if params.HypothesisID == "" {
			return map[string]any{"error": "hypothesis_id is required"}, nil
		}

		// 1. Verify hypothesis exists
		hyp, err := opts.Store.Get(params.HypothesisID)
		if err != nil {
			return map[string]any{"error": fmt.Sprintf("get hypothesis: %v", err)}, nil
		}
		if hyp == nil {
			return map[string]any{"error": fmt.Sprintf("hypothesis not found: %s", params.HypothesisID)}, nil
		}
		if hyp.Type != knowledge.TypeHypothesis {
			return map[string]any{"error": fmt.Sprintf("document %s is not a hypothesis (type=%s)", params.HypothesisID, hyp.Type)}, nil
		}

		// 2. Check for already-running loop on this hypothesis
		if existing, err := findRunningLoop(opts.Store, params.HypothesisID); err != nil {
			return map[string]any{"error": fmt.Sprintf("check existing loop: %v", err)}, nil
		} else if existing != "" {
			return map[string]any{"error": fmt.Sprintf("loop already running for hypothesis %s (loop_id=%s)", params.HypothesisID, existing)}, nil
		}

		// 3. Build budget map for body and response
		budget := map[string]any{}
		if params.MaxIterations != nil {
			budget["max_iterations"] = *params.MaxIterations
		}
		if params.MaxCostUSD != nil {
			budget["max_cost_usd"] = *params.MaxCostUSD
		}
		nullThreshold := 2
		if params.NullResultThreshold != nil {
			nullThreshold = *params.NullResultThreshold
		}
		budget["null_result_threshold"] = nullThreshold

		// 4. Create loop session document (TypeInsight, insight_type="research_loop")
		budgetJSON, _ := json.Marshal(budget)
		body := fmt.Sprintf("## Research Loop\n\nhypothesis_id: %s\nstatus: running\nstarted_at: %s\nbudget: %s\n",
			params.HypothesisID, time.Now().UTC().Format(time.RFC3339), string(budgetJSON))

		loopID, err := opts.Store.Create(knowledge.TypeInsight, map[string]any{
			"title":       fmt.Sprintf("Research Loop: %s", params.HypothesisID),
			"insight_type": "research_loop",
			"status":      "running",
			"tags":        []string{"research-loop", params.HypothesisID},
			"visibility":  "team",
		}, body)
		if err != nil {
			return map[string]any{"error": fmt.Sprintf("create loop session: %v", err)}, nil
		}

		return map[string]any{
			"loop_id":       loopID,
			"hypothesis_id": params.HypothesisID,
			"status":        "running",
			"budget":        budget,
		}, nil
	}
}

// findRunningLoop returns the loop_id of an active (status=running) loop for the given hypothesis,
// or "" if none exists.
func findRunningLoop(store *knowledge.Store, hypothesisID string) (string, error) {
	docs, err := store.List(string(knowledge.TypeInsight), "", 100)
	if err != nil {
		return "", err
	}
	for _, d := range docs {
		tags := toStringSliceAny(d["tags"])
		hasHypTag := false
		for _, tag := range tags {
			if tag == hypothesisID {
				hasHypTag = true
				break
			}
		}
		if !hasHypTag {
			continue
		}
		// Read full document to check insight_type and status
		docID, _ := d["id"].(string)
		doc, err := store.Get(docID)
		if err != nil || doc == nil {
			continue
		}
		if doc.InsightType == "research_loop" && isLoopRunning(doc) {
			return docID, nil
		}
	}
	return "", nil
}

// isLoopRunning returns true if the loop document body contains "status: running".
func isLoopRunning(doc *knowledge.Document) bool {
	if doc.Status == "running" {
		return true
	}
	// Fallback: check body for status line
	return strings.Contains(doc.Body, "status: running")
}
