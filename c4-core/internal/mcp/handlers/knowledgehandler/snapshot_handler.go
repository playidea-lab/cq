package knowledgehandler

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/changmin/c4-core/internal/knowledge"
	"github.com/changmin/c4-core/internal/mcp"
)

// cqSnapshotHandler saves a conversation snapshot to the knowledge base.
// It wraps knowledge_record with doc_type="snapshot" and structured markdown content.
func cqSnapshotHandler(opts *KnowledgeNativeOpts) mcp.HandlerFunc {
	return func(rawArgs json.RawMessage) (any, error) {
		params := parseParams(rawArgs)

		summary, _ := params["summary"].(string)
		if summary == "" {
			return map[string]any{"error": "summary is required"}, nil
		}

		sourceLLM, _ := params["source_llm"].(string)
		decisions := toStringSliceAny(params["decisions"])
		openQuestions := toStringSliceAny(params["open_questions"])
		tags := toStringSliceAny(params["tags"])

		// Build structured markdown content
		var b strings.Builder
		fmt.Fprintf(&b, "## Summary\n\n%s\n", summary)

		if sourceLLM != "" {
			fmt.Fprintf(&b, "\n**Source LLM**: %s\n", sourceLLM)
		}

		if len(decisions) > 0 {
			b.WriteString("\n## Decisions\n\n")
			for _, d := range decisions {
				fmt.Fprintf(&b, "- %s\n", d)
			}
		}

		if len(openQuestions) > 0 {
			b.WriteString("\n## Open Questions\n\n")
			for _, q := range openQuestions {
				fmt.Fprintf(&b, "- %s\n", q)
			}
		}

		content := b.String()

		// Derive title from summary (truncate to 80 chars)
		title := summary
		if len(title) > 80 {
			title = title[:77] + "..."
		}

		// Add snapshot tag always
		tags = append([]string{"snapshot"}, tags...)

		metadata := map[string]any{
			"title":  title,
			"tags":   tags,
			"domain": "snapshot",
		}
		if sourceLLM != "" {
			metadata["source_llm"] = sourceLLM
		}

		docID, err := opts.Store.Create(knowledge.TypeInsight, metadata, content)
		if err != nil {
			return map[string]any{"error": fmt.Sprintf("cq_snapshot failed: %v", err)}, nil
		}

		// Index for vector search (best-effort)
		if opts.Searcher != nil {
			doc, _ := opts.Store.Get(docID)
			if doc != nil {
				_ = opts.Searcher.IndexDocument(docID, doc)
			}
		}

		return map[string]any{
			"success":    true,
			"doc_id":     docID,
			"title":      title,
			"saved_at":   time.Now().UTC().Format(time.RFC3339),
		}, nil
	}
}

// cqRecallHandler searches for conversation snapshots in the knowledge base.
// It wraps knowledge_search with doc_type="snapshot" filter (type=insight, tag=snapshot).
func cqRecallHandler(opts *KnowledgeNativeOpts) mcp.HandlerFunc {
	return func(rawArgs json.RawMessage) (any, error) {
		params := parseParams(rawArgs)

		query, _ := params["query"].(string)
		if query == "" {
			return map[string]any{"error": "query is required"}, nil
		}

		limit := 5
		if l, ok := params["limit"].(float64); ok && l > 0 {
			limit = int(l)
		}

		// Search with doc_type filter for snapshots (stored as insight with domain=snapshot)
		filters := map[string]string{"type": string(knowledge.TypeInsight)}

		var results []knowledge.SearchResult
		var err error

		if opts.Searcher != nil {
			results, err = opts.Searcher.Search(query, limit*2, filters)
		} else {
			ftsResults, ftsErr := opts.Store.SearchFTS(query, limit*2)
			if ftsErr != nil {
				return map[string]any{"error": fmt.Sprintf("recall search failed: %v", ftsErr)}, nil
			}
			for _, r := range ftsResults {
				if t, _ := r["type"].(string); t != string(knowledge.TypeInsight) {
					continue
				}
				results = append(results, knowledge.SearchResult{
					ID:     stringFromAny(r["id"]),
					Title:  stringFromAny(r["title"]),
					Type:   stringFromAny(r["type"]),
					Domain: stringFromAny(r["domain"]),
				})
			}
		}
		if err != nil {
			return map[string]any{"error": fmt.Sprintf("recall search failed: %v", err)}, nil
		}

		// Filter to snapshot domain only
		var snapshotResults []map[string]any
		for _, r := range results {
			if len(snapshotResults) >= limit {
				break
			}
			if r.Domain != "snapshot" {
				// Check document tags for "snapshot" tag
				doc, docErr := opts.Store.Get(r.ID)
				if docErr != nil || doc == nil {
					continue
				}
				hasTag := false
				for _, tag := range doc.Tags {
					if tag == "snapshot" {
						hasTag = true
						break
					}
				}
				if !hasTag {
					continue
				}
			}
			snapshotResults = append(snapshotResults, map[string]any{
				"id":        r.ID,
				"title":     r.Title,
				"type":      r.Type,
				"domain":    r.Domain,
				"rrf_score": r.RRFScore,
			})
		}

		return map[string]any{
			"results": snapshotResults,
			"count":   len(snapshotResults),
			"query":   query,
		}, nil
	}
}

// RegisterSnapshotHandlers registers cq_snapshot and cq_recall tools.
func RegisterSnapshotHandlers(reg *mcp.Registry, opts *KnowledgeNativeOpts) {
	reg.Register(mcp.ToolSchema{
		Name:        "cq_snapshot",
		Description: "Save a conversation snapshot to the knowledge base for later recall",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"summary":        map[string]any{"type": "string", "description": "Brief summary of the conversation or session"},
				"decisions":      map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Key decisions made"},
				"open_questions": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Unresolved questions or next steps"},
				"source_llm":     map[string]any{"type": "string", "description": "Name of the LLM that generated this snapshot (e.g. claude-sonnet-4-6)"},
				"tags":           map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Additional tags for categorization"},
			},
			"required": []string{"summary"},
		},
	}, cqSnapshotHandler(opts))

	reg.Register(mcp.ToolSchema{
		Name:        "cq_recall",
		Description: "Search for conversation snapshots in the knowledge base",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{"type": "string", "description": "Search query to find relevant snapshots"},
				"limit": map[string]any{"type": "integer", "description": "Max results (default: 5)"},
			},
			"required": []string{"query"},
		},
	}, cqRecallHandler(opts))
}
