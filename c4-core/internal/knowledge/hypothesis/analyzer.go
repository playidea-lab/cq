// Package hypothesis provides LLM-based analysis of experiment documents
// to generate insights and cq.yaml drafts. It is a shared package imported
// by both the poller (T-HYP-001) and MCP handler (T-HYP-002).
package hypothesis

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/changmin/c4-core/internal/knowledge"
	"github.com/changmin/c4-core/internal/llm"
)

const analyzePrompt = `You are a research assistant analyzing experiment results.

Given the following experiment summaries, produce:
1. A concise insight (2-3 sentences) about patterns, findings, or hypotheses.
2. A minimal cq.yaml draft that captures the next experiment to run.

Experiments:
%s

Respond in this exact format:
INSIGHT: <your insight here>
CQ_YAML:
<yaml content here>`

// HypothesisResult holds the output of an Analyze call.
type HypothesisResult struct {
	Insight     string
	CQYAMLDraft string
}

// Analyze generates a hypothesis insight and cq.yaml draft from experiment documents.
// Returns error("no_experiments") immediately if docs is empty without calling the LLM.
// LLM errors are returned as-is; no retries, no panics.
func Analyze(ctx context.Context, gw *llm.Gateway, docs []knowledge.Document) (*HypothesisResult, error) {
	if len(docs) == 0 {
		return nil, errors.New("no_experiments")
	}

	summaries := buildSummaries(docs)
	prompt := fmt.Sprintf(analyzePrompt, summaries)

	resp, err := gw.Chat(ctx, "hypothesis", &llm.ChatRequest{
		Messages: []llm.Message{
			{Role: "user", Content: prompt},
		},
		MaxTokens: 1024,
	})
	if err != nil {
		return nil, fmt.Errorf("llm chat: %w", err)
	}

	return parseResponse(resp.Content), nil
}

// buildSummaries formats documents into a 200-char-per-item summary list.
func buildSummaries(docs []knowledge.Document) string {
	var sb strings.Builder
	for i, d := range docs {
		summary := d.Title + ": " + d.Body
		if len(summary) > 200 {
			summary = summary[:200]
		}
		fmt.Fprintf(&sb, "%d. %s\n", i+1, summary)
	}
	return sb.String()
}

// parseResponse extracts INSIGHT and CQ_YAML sections from LLM output.
func parseResponse(content string) *HypothesisResult {
	result := &HypothesisResult{}
	lines := strings.SplitN(content, "\n", -1)

	var yamlLines []string
	inYAML := false

	for _, line := range lines {
		if strings.HasPrefix(line, "INSIGHT: ") {
			result.Insight = strings.TrimPrefix(line, "INSIGHT: ")
			continue
		}
		if line == "CQ_YAML:" {
			inYAML = true
			continue
		}
		if inYAML {
			yamlLines = append(yamlLines, line)
		}
	}

	result.CQYAMLDraft = strings.Join(yamlLines, "\n")
	return result
}
