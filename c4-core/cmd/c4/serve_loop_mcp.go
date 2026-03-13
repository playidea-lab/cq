//go:build research

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/changmin/c4-core/internal/knowledge"
	"github.com/changmin/c4-core/internal/mcp"
)

func init() {
	registerInitHook(registerLoopMCPHandlers)
}

func registerLoopMCPHandlers(ctx *initContext) error {
	if ctx.knowledgeStore == nil {
		return nil
	}
	lo, ok := ctx.loopOrchestrator.(*LoopOrchestrator)
	if !ok || lo == nil {
		return nil
	}

	ctx.reg.Register(mcp.ToolSchema{
		Name:        "c4_research_loop_start",
		Description: "자율 연구 루프를 시작합니다.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"hypothesis":     map[string]any{"type": "string"},
				"hypothesis_id":  map[string]any{"type": "string"},
				"max_iterations": map[string]any{"type": "integer"},
			},
		},
	}, loopStartHandler(lo, ctx.knowledgeStore))

	ctx.reg.Register(mcp.ToolSchema{
		Name:        "c4_research_loop_stop",
		Description: "실행 중인 자율 연구 루프를 중지합니다.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"hypothesis_id": map[string]any{"type": "string"},
			},
			"required": []string{"hypothesis_id"},
		},
	}, loopStopHandler(lo))

	ctx.reg.Register(mcp.ToolSchema{
		Name:        "c4_research_loop_status",
		Description: "자율 연구 루프의 현재 상태를 조회합니다.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"hypothesis_id": map[string]any{"type": "string"},
			},
			"required": []string{"hypothesis_id"},
		},
	}, loopStatusHandler(lo))

	return nil
}

func loopStartHandler(lo *LoopOrchestrator, ks *knowledge.Store) mcp.HandlerFunc {
	return func(rawArgs json.RawMessage) (any, error) {
		var params struct {
			Hypothesis    string `json:"hypothesis"`
			HypothesisID  string `json:"hypothesis_id"`
			MaxIterations int    `json:"max_iterations"`
		}
		if len(rawArgs) > 0 {
			if err := json.Unmarshal(rawArgs, &params); err != nil {
				return nil, fmt.Errorf("parsing arguments: %w", err)
			}
		}
		hypID := params.HypothesisID
		if hypID == "" && params.Hypothesis != "" {
			var err error
			hypID, err = ks.Create(knowledge.TypeHypothesis, map[string]any{
				"title":  params.Hypothesis,
				"status": "pending",
				"source": "research_loop_start",
			}, "## Hypothesis\n\n"+params.Hypothesis)
			if err != nil {
				return nil, fmt.Errorf("create hypothesis: %w", err)
			}
		}
		if hypID == "" {
			return nil, errors.New("hypothesis or hypothesis_id is required")
		}
		session := &LoopSession{
			HypothesisID:  hypID,
			MaxIterations: params.MaxIterations,
		}
		if err := lo.StartLoop(context.Background(), session); err != nil {
			return nil, err
		}
		return map[string]any{
			"hypothesis_id":  hypID,
			"status":         "running",
			"max_iterations": params.MaxIterations,
		}, nil
	}
}

func loopStopHandler(lo *LoopOrchestrator) mcp.HandlerFunc {
	return func(rawArgs json.RawMessage) (any, error) {
		var params struct {
			HypothesisID string `json:"hypothesis_id"`
		}
		if err := json.Unmarshal(rawArgs, &params); err != nil {
			return nil, fmt.Errorf("parsing arguments: %w", err)
		}
		if params.HypothesisID == "" {
			return nil, errors.New("hypothesis_id is required")
		}
		if err := lo.StopLoop(context.Background(), params.HypothesisID); err != nil {
			return nil, err
		}
		return map[string]any{"hypothesis_id": params.HypothesisID, "status": "stopped"}, nil
	}
}

func loopStatusHandler(lo *LoopOrchestrator) mcp.HandlerFunc {
	return func(rawArgs json.RawMessage) (any, error) {
		var params struct {
			HypothesisID string `json:"hypothesis_id"`
		}
		if err := json.Unmarshal(rawArgs, &params); err != nil {
			return nil, fmt.Errorf("parsing arguments: %w", err)
		}
		if params.HypothesisID == "" {
			return nil, errors.New("hypothesis_id is required")
		}
		s := lo.GetLoop(params.HypothesisID)
		if s == nil {
			return nil, errors.New("loop not found: " + params.HypothesisID)
		}
		return map[string]any{
			"hypothesis_id":     s.HypothesisID,
			"status":            s.Status,
			"round":             s.Round,
			"null_result_count": s.NullResultCount,
			"explore_flag":      s.ExploreFlag,
			"max_iterations":    s.MaxIterations,
		}, nil
	}
}
