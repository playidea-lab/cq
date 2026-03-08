package knowledgehandler

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

const hypothesisPrompt = `You are a research hypothesis generator. Analyze these experiment records and generate a concise, actionable hypothesis.

Experiments:
%s

Output a JSON object with these fields:
- insight: a 1-2 sentence insight (max 200 chars)
- yaml_draft: a minimal cq.yaml snippet demonstrating the suggested experiment approach

Respond ONLY with valid JSON.`

// researchSuggestNativeHandler implements c4_research_suggest: reads TypeExperiment docs,
// calls LLM to synthesize a hypothesis, stores it as TypeHypothesis, returns hyp-ID.
func researchSuggestNativeHandler(opts *KnowledgeNativeOpts) mcp.HandlerFunc {
	return func(rawArgs json.RawMessage) (any, error) {
		params := parseParams(rawArgs)

		tag, _ := params["tag"].(string)
		limit := 10
		if l, ok := params["limit"].(float64); ok && l > 0 {
			limit = int(l)
		}

		if opts.LLM == nil {
			return map[string]any{"error": "llm_error", "message": "LLM gateway not configured"}, nil
		}

		// Fetch experiment documents
		docs, err := opts.Store.List("experiment", "", limit*3) // fetch extra to allow tag filtering
		if err != nil {
			return map[string]any{"error": "llm_error", "message": fmt.Sprintf("store list failed: %v", err)}, nil
		}

		// Apply tag filter
		if tag != "" {
			var filtered []map[string]any
			for _, d := range docs {
				tags := toStringSliceAny(d["tags"])
				for _, t := range tags {
					if t == tag {
						filtered = append(filtered, d)
						break
					}
				}
			}
			docs = filtered
		}

		// Truncate to limit
		if len(docs) > limit {
			docs = docs[:limit]
		}

		if len(docs) == 0 {
			return map[string]any{"error": "no_experiments"}, nil
		}

		// Build experiment summary text
		var lines []string
		for _, d := range docs {
			title, _ := d["title"].(string)
			lines = append(lines, fmt.Sprintf("- %s", title))
		}
		experimentText := strings.Join(lines, "\n")

		// Call LLM
		prompt := fmt.Sprintf(hypothesisPrompt, experimentText)
		ref := opts.LLM.Resolve("scout", "")
		llmCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		resp, llmErr := opts.LLM.Chat(llmCtx, "scout", &llm.ChatRequest{
			Model:       ref.Model,
			Messages:    []llm.Message{{Role: "user", Content: prompt}},
			MaxTokens:   400,
			Temperature: 0.4,
		})
		cancel()

		if llmErr != nil {
			return map[string]any{"error": "llm_error", "message": llmErr.Error()}, nil
		}

		// Parse LLM JSON response
		content := strings.TrimSpace(resp.Content)
		// Strip markdown code fences if present (handles ```json, ```JSON, ``` etc.)
		if strings.HasPrefix(content, "```") {
			// Strip opening fence line entirely
			if i := strings.Index(content, "\n"); i >= 0 {
				content = content[i+1:]
			} else {
				content = ""
			}
			// Strip closing fence
			if idx := strings.LastIndex(content, "```"); idx >= 0 {
				content = content[:idx]
			}
			content = strings.TrimSpace(content)
		}

		var parsed map[string]any
		if jsonErr := json.Unmarshal([]byte(content), &parsed); jsonErr != nil {
			// Fallback: use raw content as insight
			parsed = map[string]any{"insight": truncate(content, 200), "yaml_draft": ""}
		}

		insight, _ := parsed["insight"].(string)
		yamlDraft, _ := parsed["yaml_draft"].(string)

		// Truncate insight to 200 chars
		insight = truncate(insight, 200)

		// Compute tags from input tag
		var hypTags []string
		if tag != "" {
			hypTags = []string{tag}
		}

		// Store as TypeHypothesis
		expiresAt := time.Now().Add(30 * 24 * time.Hour).UTC().Format(time.RFC3339)
		metadata := map[string]any{
			"title":      truncate(fmt.Sprintf("Hypothesis: %s", insight), 80),
			"tags":       hypTags,
			"visibility": "team",
			"status":     "pending",
			"expires_at": expiresAt,
		}
		body := fmt.Sprintf("## Insight\n%s\n\n## YAML Draft\n```yaml\n%s\n```\n\n## Source Experiments\n%s",
			insight, yamlDraft, experimentText)

		hypID, createErr := opts.Store.Create(knowledge.TypeHypothesis, metadata, body)
		if createErr != nil {
			return map[string]any{"error": "llm_error", "message": fmt.Sprintf("store create failed: %v", createErr)}, nil
		}

		return map[string]any{
			"hypothesis_id":      hypID,
			"insight":            insight,
			"yaml_draft_preview": truncate(yamlDraft, 300),
			"expires_at":         expiresAt,
			"experiment_count":   len(docs),
			"tags":               hypTags,
		}, nil
	}
}
