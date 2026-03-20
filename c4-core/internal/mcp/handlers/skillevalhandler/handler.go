// Package skillevalhandler implements the c4_skill_eval_run MCP tool.
package skillevalhandler

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/changmin/c4-core/internal/knowledge"
	"github.com/changmin/c4-core/internal/llm"
	"github.com/changmin/c4-core/internal/mcp"
	"github.com/changmin/c4-core/internal/skilleval"
)

// Opts holds dependencies for the skilleval MCP handler.
type Opts struct {
	ProjectDir     string
	LLM            *llm.Gateway
	KnowledgeStore *knowledge.Store
}

// Register registers c4_skill_eval_generate, c4_skill_eval_run, c4_skill_eval_status, and c4_skill_optimize tools.
func Register(reg *mcp.Registry, opts *Opts) {
	if opts == nil || opts.LLM == nil {
		return
	}

	reg.RegisterBlocking(mcp.ToolSchema{
		Name:        "c4_skill_eval_generate",
		Description: "SKILL.md를 분석하여 EVAL.md를 자동 생성한다. skill='all'이면 전체 스킬 일괄 생성.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"skill": map[string]any{
					"type":        "string",
					"description": "스킬 이름 (e.g. 'c4-finish'), 또는 'all'로 전체 일괄 생성",
				},
			},
			"required": []string{"skill"},
		},
	}, generateHandler(opts))

	reg.Register(mcp.ToolSchema{
		Name:        "c4_skill_eval_status",
		Description: "전체 스킬 헬스 요약 조회. C9 experiment_record에서 최신 trigger_accuracy를 읽어 반환한다.",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}, statusHandler(opts))

	reg.RegisterBlocking(mcp.ToolSchema{
		Name:        "c4_skill_optimize",
		Description: "스킬의 SKILL.md를 자동 최적화한다. 베이스라인 측정 후 변이→평가→유지/폐기 루프를 반복해 pass rate를 높인다.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"skill": map[string]any{
					"type":        "string",
					"description": "스킬 이름 (e.g. 'c4-finish')",
				},
				"evals": map[string]any{
					"type":        "array",
					"description": "평가 기준 목록 [{name, question, pass, fail}]",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"name":     map[string]any{"type": "string"},
							"question": map[string]any{"type": "string"},
							"pass":     map[string]any{"type": "string"},
							"fail":     map[string]any{"type": "string"},
						},
						"required": []string{"name", "question", "pass", "fail"},
					},
				},
				"runs_per_experiment": map[string]any{
					"type":        "integer",
					"description": "실험당 시뮬레이션 실행 횟수 (기본 3)",
				},
				"budget_cap": map[string]any{
					"type":        "integer",
					"description": "최대 변이 실험 횟수 (기본 10)",
				},
				"test_inputs": map[string]any{
					"type":        "array",
					"description": "테스트 입력 목록 (참고용)",
					"items":       map[string]any{"type": "string"},
				},
			},
			"required": []string{"skill", "evals"},
		},
	}, optimizeHandler(opts))

	reg.RegisterBlocking(mcp.ToolSchema{
		Name:        "c4_skill_eval_run",
		Description: "스킬의 트리거 정확도를 haiku 분류 호출로 측정하고 결과를 C9에 저장한다.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"skill": map[string]any{
					"type":        "string",
					"description": "스킬 이름 (e.g. 'c4-finish')",
				},
				"k": map[string]any{
					"type":        "integer",
					"description": "pass@k 반복 횟수 (기본 5)",
				},
			},
			"required": []string{"skill"},
		},
	}, runHandler(opts))
}

func optimizeHandler(opts *Opts) mcp.BlockingHandlerFunc {
	return func(ctx context.Context, rawArgs json.RawMessage) (any, error) {
		var params map[string]any
		if err := json.Unmarshal(rawArgs, &params); err != nil {
			return map[string]any{"error": "invalid args"}, nil
		}

		skillName, _ := params["skill"].(string)
		if skillName == "" {
			return map[string]any{"error": "skill is required"}, nil
		}
		if strings.Contains(skillName, "..") || strings.Contains(skillName, "/") || strings.Contains(skillName, "\\") {
			return map[string]any{"error": "invalid skill name"}, nil
		}

		// Parse evals array
		evalsRaw, _ := params["evals"].([]any)
		if len(evalsRaw) == 0 {
			return map[string]any{"error": "evals is required and must not be empty"}, nil
		}
		evals := make([]skilleval.BinaryEval, 0, len(evalsRaw))
		for _, e := range evalsRaw {
			em, ok := e.(map[string]any)
			if !ok {
				continue
			}
			name, _ := em["name"].(string)
			question, _ := em["question"].(string)
			pass, _ := em["pass"].(string)
			fail, _ := em["fail"].(string)
			if name == "" || question == "" {
				continue
			}
			evals = append(evals, skilleval.BinaryEval{
				Name:     name,
				Question: question,
				Pass:     pass,
				Fail:     fail,
			})
		}
		if len(evals) == 0 {
			return map[string]any{"error": "no valid evals provided"}, nil
		}

		runsPerExperiment := 3
		if v, ok := params["runs_per_experiment"].(float64); ok && v > 0 {
			runsPerExperiment = int(v)
		}

		budgetCap := 10
		if v, ok := params["budget_cap"].(float64); ok && v > 0 {
			budgetCap = int(v)
		}
		// Cap to prevent unbounded LLM cost.
		if budgetCap > 50 {
			budgetCap = 50
		}

		// Parse test_inputs (optional).
		var testInputs []string
		if arr, ok := params["test_inputs"].([]any); ok {
			for _, v := range arr {
				if s, ok := v.(string); ok && s != "" {
					testInputs = append(testInputs, s)
				}
			}
		}

		optimizer := &skilleval.SkillOptimizer{
			ProjectDir: opts.ProjectDir,
			LLM:        opts.LLM,
			KStore:     opts.KnowledgeStore,
		}

		result, err := optimizer.Run(ctx, skillName, evals, skilleval.OptimizeOpts{
			RunsPerExperiment: runsPerExperiment,
			BudgetCap:         budgetCap,
			TestInputs:        testInputs,
		})
		if err != nil {
			return map[string]any{"error": fmt.Sprintf("optimize failed: %v", err)}, nil
		}

		improvementPercent := 0.0
		if result.Baseline > 0 {
			improvementPercent = (result.FinalScore - result.Baseline) / result.Baseline * 100
		}

		return map[string]any{
			"baseline":            result.Baseline,
			"final_score":         result.FinalScore,
			"experiments":         result.Experiments,
			"kept_count":          result.KeptCount,
			"improvement_percent": improvementPercent,
		}, nil
	}
}

func runHandler(opts *Opts) mcp.BlockingHandlerFunc {
	return func(ctx context.Context, rawArgs json.RawMessage) (any, error) {
		var params map[string]any
		if err := json.Unmarshal(rawArgs, &params); err != nil {
			return map[string]any{"error": "invalid args"}, nil
		}

		skillName, _ := params["skill"].(string)
		if skillName == "" {
			return map[string]any{"error": "skill is required"}, nil
		}
		// Path traversal guard — matches pattern in persona.go
		if strings.Contains(skillName, "..") || strings.Contains(skillName, "/") || strings.Contains(skillName, "\\") {
			return map[string]any{"error": "invalid skill name"}, nil
		}

		k := 5
		if kv, ok := params["k"].(float64); ok && kv > 0 {
			k = int(kv)
		}
		// Cap k to prevent unbounded LLM cost amplification.
		if k > 20 {
			k = 20
		}

		result, err := skilleval.RunEval(ctx, opts.LLM, opts.ProjectDir, skillName, k)
		if err != nil {
			return map[string]any{"error": fmt.Sprintf("eval failed: %v", err)}, nil
		}

		// Save to C9 knowledge store as experiment.
		// trigger_accuracy is stored in the confidence field so statusHandler
		// can read it from List() results (which do not include body).
		if opts.KnowledgeStore != nil {
			body := fmt.Sprintf(
				"skill: %s\ntrigger_accuracy: %.4f\npass_at_k: %.4f\npass_k: %.4f\nk: %d\ntest_count: %d\ndate: %s",
				result.Skill,
				result.TriggerAccuracy,
				result.PassAtK,
				result.PassK,
				result.K,
				result.TestCount,
				time.Now().Format("2006-01-02"),
			)
			metadata := map[string]any{
				"id":         result.ExpID,
				"title":      fmt.Sprintf("Skill Eval: %s", skillName),
				"domain":     "skilleval",
				"confidence": result.TriggerAccuracy, // indexed — read by statusHandler via List()
				"tags":       []string{"skill-eval", "trigger-accuracy"},
			}
			// best-effort; ignore error to not block result return
			opts.KnowledgeStore.Create(knowledge.TypeExperiment, metadata, body) //nolint:errcheck
		}

		return map[string]any{
			"skill":            result.Skill,
			"trigger_accuracy": result.TriggerAccuracy,
			"pass_at_k":        result.PassAtK,
			"pass_k":           result.PassK,
			"k":                result.K,
			"test_count":       result.TestCount,
			"exp_id":           result.ExpID,
			"cases":            result.Cases,
		}, nil
	}
}

func generateHandler(opts *Opts) mcp.BlockingHandlerFunc {
	return func(ctx context.Context, rawArgs json.RawMessage) (any, error) {
		var params map[string]any
		if err := json.Unmarshal(rawArgs, &params); err != nil {
			return map[string]any{"error": "invalid args"}, nil
		}
		skillName, _ := params["skill"].(string)
		if skillName == "" {
			return map[string]any{"error": "skill is required"}, nil
		}
		if strings.Contains(skillName, "..") || strings.Contains(skillName, "/") || strings.Contains(skillName, "\\") {
			return map[string]any{"error": "invalid skill name"}, nil
		}

		if skillName == "all" {
			generated, failed := skilleval.GenerateAllEvalMD(ctx, opts.LLM, opts.ProjectDir)
			return map[string]any{"generated": generated, "failed": failed}, nil
		}

		path, err := skilleval.GenerateEvalMD(ctx, opts.LLM, opts.ProjectDir, skillName)
		if err != nil {
			return map[string]any{"error": fmt.Sprintf("generate failed: %v", err)}, nil
		}
		return map[string]any{"skill": skillName, "path": path}, nil
	}
}

func statusHandler(opts *Opts) mcp.HandlerFunc {
	return func(rawArgs json.RawMessage) (any, error) {
		if opts.KnowledgeStore == nil {
			return map[string]any{"error": "knowledge store unavailable"}, nil
		}
		docs, err := opts.KnowledgeStore.List("", "skilleval", 200)
		if err != nil {
			return map[string]any{"error": fmt.Sprintf("list failed: %v", err)}, nil
		}

		type skillEntry struct {
			Skill           string  `json:"skill"`
			TriggerAccuracy float64 `json:"trigger_accuracy"`
			Status          string  `json:"status"`
		}
		skills := []skillEntry{}
		total, ok, warn := 0, 0, 0

		// Deduplicate by skill name — keep the most recent entry (List returns DESC updated_at).
		seen := map[string]bool{}
		for _, d := range docs {
			// Skill name is encoded in title as "Skill Eval: <name>".
			title, _ := d["title"].(string)
			skillName := strings.TrimPrefix(title, "Skill Eval: ")
			if skillName == title || skillName == "" {
				continue
			}
			if seen[skillName] {
				continue // already have the latest entry for this skill
			}
			seen[skillName] = true

			// trigger_accuracy is stored in the confidence field (float64, indexed).
			acc, _ := d["confidence"].(float64)
			total++
			status := "ok"
			if acc < 0.90 {
				status = "warn"
				warn++
			} else {
				ok++
			}
			skills = append(skills, skillEntry{Skill: skillName, TriggerAccuracy: acc, Status: status})
		}

		return map[string]any{
			"skills":  skills,
			"summary": map[string]int{"total": total, "ok": ok, "warn": warn},
		}, nil
	}
}
