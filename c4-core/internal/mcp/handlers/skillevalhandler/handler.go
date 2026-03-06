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

// Register registers c4_skill_eval_generate, c4_skill_eval_run, and c4_skill_eval_status tools.
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

		// Save to C9 knowledge store as experiment
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
				"id":     result.ExpID,
				"title":  fmt.Sprintf("Skill Eval: %s", skillName),
				"domain": "skilleval",
				"tags":   []string{"skill-eval", "trigger-accuracy"},
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
		total, ok, warn, unknown := 0, 0, 0, 0

		for _, d := range docs {
			body, _ := d["body"].(string)
			var acc float64
			var skillName string
			for _, line := range strings.Split(body, "\n") {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "skill:") {
					skillName = strings.TrimSpace(strings.TrimPrefix(line, "skill:"))
				} else if strings.HasPrefix(line, "trigger_accuracy:") {
					val := strings.TrimSpace(strings.TrimPrefix(line, "trigger_accuracy:"))
					fmt.Sscanf(val, "%f", &acc) //nolint:errcheck
				}
			}
			if skillName == "" {
				continue
			}
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
			"summary": map[string]int{"total": total, "ok": ok, "warn": warn, "unknown": unknown},
		}, nil
	}
}
