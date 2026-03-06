// Package skillevalhandler implements the c4_skill_eval_run MCP tool.
package skillevalhandler

import (
	"context"
	"encoding/json"
	"fmt"
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

// Register registers the c4_skill_eval_run tool.
func Register(reg *mcp.Registry, opts *Opts) {
	if opts == nil || opts.LLM == nil {
		return
	}

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

		k := 5
		if kv, ok := params["k"].(float64); ok && kv > 0 {
			k = int(kv)
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
