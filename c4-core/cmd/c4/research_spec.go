//go:build research

package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/changmin/c4-core/internal/knowledge"
	"github.com/changmin/c4-core/internal/mcp"
)

func init() {
	registerInitHook(registerResearchSpecHandler)
}

// registerResearchSpecHandler registers the c4_research_spec MCP tool.
func registerResearchSpecHandler(ctx *initContext) error {
	if ctx.knowledgeStore == nil {
		return nil
	}
	ctx.reg.Register(mcp.ToolSchema{
		Name:        "c4_research_spec",
		Description: "Create an experiment spec (TypeExperiment) from an existing hypothesis (TypeHypothesis)",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"hypothesis_id":           map[string]any{"type": "string", "description": "ID of the TypeHypothesis document"},
				"success_condition":       map[string]any{"type": "string"},
				"null_condition":          map[string]any{"type": "string"},
				"escalation_trigger":      map[string]any{"type": "string"},
				"controlled_variables":    map[string]any{"type": "string"},
				"expected_metrics_range":  map[string]any{"type": "string", "description": "JSON array of {name,min,max}"},
			},
			"required": []string{"hypothesis_id"},
		},
	}, researchSpecHandler(ctx.knowledgeStore))
	return nil
}

func researchSpecHandler(ks *knowledge.Store) mcp.HandlerFunc {
	return func(rawArgs json.RawMessage) (any, error) {
		var params struct {
			HypothesisID          string `json:"hypothesis_id"`
			SuccessCondition      string `json:"success_condition"`
			NullCondition         string `json:"null_condition"`
			EscalationTrigger     string `json:"escalation_trigger"`
			ControlledVariables   string `json:"controlled_variables"`
			ExpectedMetricsRange  string `json:"expected_metrics_range"`
		}
		if len(rawArgs) > 0 {
			if err := json.Unmarshal(rawArgs, &params); err != nil {
				return nil, fmt.Errorf("parsing arguments: %w", err)
			}
		}
		if params.HypothesisID == "" {
			return nil, fmt.Errorf("hypothesis_id is required")
		}

		doc, err := ks.Get(params.HypothesisID)
		if err != nil {
			return nil, fmt.Errorf("fetching hypothesis: %w", err)
		}
		if doc == nil {
			return nil, fmt.Errorf("hypothesis_id %s not found", params.HypothesisID)
		}
		if doc.Type != knowledge.TypeHypothesis {
			return nil, fmt.Errorf("hypothesis_id %s has wrong type %s, expected hypothesis", params.HypothesisID, doc.Type)
		}

		body := fmt.Sprintf("Experiment spec for hypothesis: %s\n\nsuccess_condition: %s\nnull_condition: %s\nescalation_trigger: %s\ncontrolled_variables: %s\nexpected_metrics_range: %s",
			params.HypothesisID,
			params.SuccessCondition,
			params.NullCondition,
			params.EscalationTrigger,
			params.ControlledVariables,
			params.ExpectedMetricsRange,
		)

		sanitize := func(s string) string { return strings.ReplaceAll(s, "\n", " ") }
		cqYAMLDraft := fmt.Sprintf("hypothesis_id: %s\nsuccess_condition: %s\nnull_condition: %s\n",
			sanitize(params.HypothesisID),
			sanitize(params.SuccessCondition),
			sanitize(params.NullCondition),
		)

		specID, err := ks.Create(knowledge.TypeExperiment, map[string]any{
			"title":         fmt.Sprintf("Spec for %s", params.HypothesisID),
			"domain":        "research",
			"hypothesis_id": params.HypothesisID,
		}, body)
		if err != nil {
			return nil, fmt.Errorf("creating experiment spec: %w", err)
		}

		return map[string]any{
			"spec_id":       specID,
			"hypothesis_id": params.HypothesisID,
			"cq_yaml_draft": cqYAMLDraft,
		}, nil
	}
}
