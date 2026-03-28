//go:build research

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/changmin/c4-core/internal/knowledge"
	"github.com/changmin/c4-core/internal/mcp"
	"github.com/changmin/c4-core/internal/serve/orchestrator"
)

func init() {
	registerInitHook(registerLoopMCPHandlers)
}

// loopConvergenceDefaults holds config-level defaults for convergence parameters.
type loopConvergenceDefaults struct {
	MaxPatience          int
	ConvergenceThreshold float64
	MetricLowerIsBetter  bool
}

func registerLoopMCPHandlers(ctx *initContext) error {
	if ctx.knowledgeStore == nil {
		return nil
	}
	lo, ok := ctx.loopOrchestrator.(*orchestrator.LoopOrchestrator)
	if !ok || lo == nil {
		if ctx.loopOrchestrator != nil {
			fmt.Fprintf(os.Stderr, "warn: loopOrchestrator is not *orchestrator.LoopOrchestrator (%T); loop MCP handlers not registered\n", ctx.loopOrchestrator)
		}
		return nil
	}

	// Read convergence defaults from config.
	defaults := loopConvergenceDefaults{MetricLowerIsBetter: true}
	if ctx.cfgMgr != nil {
		rc := ctx.cfgMgr.GetConfig().Serve.ResearchLoop
		defaults.MaxPatience = rc.Patience
		defaults.ConvergenceThreshold = rc.ConvergenceThreshold
		// *bool: nil = not configured (keep default true), non-nil = explicit.
		if rc.MetricLowerIsBetter != nil {
			defaults.MetricLowerIsBetter = *rc.MetricLowerIsBetter
		}
	}

	ctx.reg.Register(mcp.ToolSchema{
		Name:        "cq_research_loop_start",
		Description: "자율 연구 루프를 시작합니다. command+workdir를 지정하면 SpecPipeline을 건너뛰고 직접 Hub에 제출합니다.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"hypothesis":            map[string]any{"type": "string"},
				"hypothesis_id":         map[string]any{"type": "string"},
				"max_iterations":        map[string]any{"type": "integer"},
				"max_patience":          map[string]any{"type": "integer", "description": "수렴 판정 최대 patience 라운드 수. 0=비활성. config 기본값 override."},
				"convergence_threshold": map[string]any{"type": "number", "description": "patience 리셋 최소 개선량. config 기본값 override."},
				"command":               map[string]any{"type": "string", "description": "Hub에 제출할 실행 명령. 지정 시 SpecPipeline 우회."},
				"workdir":               map[string]any{"type": "string", "description": "워커의 작업 디렉토리."},
			},
		},
	}, loopStartHandler(lo, ctx.knowledgeStore, defaults))

	ctx.reg.Register(mcp.ToolSchema{
		Name:        "cq_research_loop_stop",
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
		Name:        "cq_research_loop_status",
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

func loopStartHandler(lo *orchestrator.LoopOrchestrator, ks *knowledge.Store, defaults loopConvergenceDefaults) mcp.HandlerFunc {
	return func(rawArgs json.RawMessage) (any, error) {
		var params struct {
			Hypothesis           string  `json:"hypothesis"`
			HypothesisID         string  `json:"hypothesis_id"`
			MaxIterations        int     `json:"max_iterations"`
			MaxPatience          *int    `json:"max_patience"`
			ConvergenceThreshold *float64 `json:"convergence_threshold"`
			Command              string  `json:"command"`
			Workdir              string  `json:"workdir"`
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

		// Apply convergence params: caller-supplied values override config defaults.
		maxPatience := defaults.MaxPatience
		if params.MaxPatience != nil {
			maxPatience = *params.MaxPatience
		}
		convergenceThreshold := defaults.ConvergenceThreshold
		if params.ConvergenceThreshold != nil {
			convergenceThreshold = *params.ConvergenceThreshold
		}

		session := &orchestrator.LoopSession{
			HypothesisID:         hypID,
			MaxIterations:        params.MaxIterations,
			MaxPatience:          maxPatience,
			ConvergenceThreshold: convergenceThreshold,
			MetricLowerIsBetter:  defaults.MetricLowerIsBetter,
			Command:              params.Command,
			Workdir:              params.Workdir,
		}
		// mcp.HandlerFunc does not carry a context; StartLoop is a sync.Map operation
		// and completes instantly, so context.Background() is intentional here.
		if err := lo.StartLoop(context.Background(), session); err != nil {
			return nil, err
		}
		return map[string]any{
			"hypothesis_id":         hypID,
			"status":                "running",
			"max_iterations":        params.MaxIterations,
			"max_patience":          maxPatience,
			"convergence_threshold": convergenceThreshold,
		}, nil
	}
}

func loopStopHandler(lo *orchestrator.LoopOrchestrator) mcp.HandlerFunc {
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

func loopStatusHandler(lo *orchestrator.LoopOrchestrator) mcp.HandlerFunc {
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
		converged := s.MaxPatience > 0 && s.PatienceCount >= s.MaxPatience
		return map[string]any{
			"hypothesis_id":     s.HypothesisID,
			"status":            s.Status,
			"round":             s.Round,
			"null_result_count": s.NullResultCount,
			"explore_flag":      s.ExploreFlag,
			"max_iterations":    s.MaxIterations,
			"convergence": map[string]any{
				"patience_count": s.PatienceCount,
				"best_metric":    s.BestMetric,
				"converged":      converged,
				"threshold":      s.ConvergenceThreshold,
				"max_patience":   s.MaxPatience,
			},
		}, nil
	}
}
