//go:build research

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/changmin/c4-core/internal/knowledge"
	"github.com/changmin/c4-core/internal/llm"
	"github.com/changmin/c4-core/internal/mcp"
)

func init() {
	registerInitHook(registerResearchSuggestHandler)
}

// registerResearchSuggestHandler registers the c4_research_suggest MCP tool.
func registerResearchSuggestHandler(ctx *initContext) error {
	if ctx.knowledgeStore == nil || ctx.llmGateway == nil {
		// Skip registration if dependencies are unavailable.
		return nil
	}
	caller := &suggestGatewayLLMCaller{gw: ctx.llmGateway}
	ctx.reg.Register(mcp.ToolSchema{
		Name:        "c4_research_suggest",
		Description: "Generate a research hypothesis using LLM, grounded in recent experiments from the knowledge store",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"tag":   map[string]any{"type": "string", "description": "Optional tag to filter experiments"},
				"limit": map[string]any{"type": "integer", "description": "Max experiments to include (default: 10)"},
			},
		},
	}, researchSuggestHandler(ctx.knowledgeStore, caller))
	return nil
}

// suggestGatewayLLMCaller adapts *llm.Gateway for use in the suggest handler.
// Named distinctly from gatewayLLMCaller in mcp_init_eventbus_llm.go.
type suggestGatewayLLMCaller struct {
	gw *llm.Gateway
}

func (s *suggestGatewayLLMCaller) call(ctx context.Context, systemPrompt, userMessage string) (string, error) {
	req := &llm.ChatRequest{
		Model:  "",
		System: systemPrompt,
		Messages: []llm.Message{
			{Role: "user", Content: userMessage},
		},
	}
	resp, err := s.gw.Chat(ctx, "research_suggest", req)
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}

// truncateRunes truncates s to at most n runes (safe for Korean/CJK text).
func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}

func researchSuggestHandler(ks *knowledge.Store, caller *suggestGatewayLLMCaller) mcp.HandlerFunc {
	return func(rawArgs json.RawMessage) (any, error) {
		var params struct {
			Tag   string `json:"tag"`
			Limit int    `json:"limit"`
		}
		if len(rawArgs) > 0 {
			if err := json.Unmarshal(rawArgs, &params); err != nil {
				return nil, fmt.Errorf("parsing arguments: %w", err)
			}
		}
		if params.Limit <= 0 {
			params.Limit = 10
		}

		// Fetch recent experiments for context.
		experiments, err := ks.List(string(knowledge.TypeExperiment), params.Tag, params.Limit)
		if err != nil {
			return nil, fmt.Errorf("listing experiments: %w", err)
		}
		experimentCount := len(experiments)

		// Build user message from experiment summaries.
		var sb strings.Builder
		sb.WriteString("Recent experiments:\n")
		for _, e := range experiments {
			title, _ := e["title"].(string)
			domain, _ := e["domain"].(string)
			sb.WriteString(fmt.Sprintf("- %s (domain: %s)\n", title, domain))
		}
		if params.Tag != "" {
			sb.WriteString(fmt.Sprintf("\nFocus tag: %s\n", params.Tag))
		}
		sb.WriteString("\nGenerate a research hypothesis and YAML experiment draft.")

		systemPrompt := "You are a research hypothesis generator. Based on the provided experiments, generate:\n" +
			"1. A concise research hypothesis (insight)\n" +
			"2. A YAML experiment draft to test it\n\n" +
			"Format your response as:\nINSIGHT: <hypothesis text>\nYAML_DRAFT:\n<yaml content>"

		llmOut, err := caller.call(context.Background(), systemPrompt, sb.String())
		if err != nil {
			return nil, fmt.Errorf("LLM call failed: %w", err)
		}

		// Parse LLM output.
		insight, yamlDraft := parseResearchSuggestOutput(llmOut)

		// Store full text as a hypothesis document.
		expiresAt := time.Now().UTC().Add(7 * 24 * time.Hour)
		tags := []string{"research-suggest"}
		if params.Tag != "" {
			tags = append(tags, params.Tag)
		}
		hypothesisID, err := ks.Create(knowledge.TypeHypothesis, map[string]any{
			"title":             truncateRunes(insight, 80),
			"domain":            "research",
			"tags":              tags,
			"hypothesis":        insight,
			"hypothesis_status": "pending",
		}, llmOut)
		if err != nil {
			return nil, fmt.Errorf("storing hypothesis: %w", err)
		}

		return map[string]any{
			"hypothesis_id":    hypothesisID,
			"insight":          truncateRunes(insight, 200),
			"yaml_draft_preview": truncateRunes(yamlDraft, 200),
			"expires_at":       expiresAt.Format(time.RFC3339),
			"experiment_count": experimentCount,
			"tags":             tags,
		}, nil
	}
}

// parseResearchSuggestOutput extracts insight and yaml_draft from LLM output.
func parseResearchSuggestOutput(out string) (insight, yamlDraft string) {
	lines := strings.Split(out, "\n")
	var yamlLines []string
	inYAML := false
	for _, line := range lines {
		if strings.HasPrefix(line, "INSIGHT:") {
			insight = strings.TrimSpace(strings.TrimPrefix(line, "INSIGHT:"))
		} else if strings.HasPrefix(line, "YAML_DRAFT:") {
			inYAML = true
		} else if inYAML {
			yamlLines = append(yamlLines, line)
		}
	}
	yamlDraft = strings.TrimSpace(strings.Join(yamlLines, "\n"))
	if insight == "" {
		insight = strings.TrimSpace(out)
	}
	return insight, yamlDraft
}
