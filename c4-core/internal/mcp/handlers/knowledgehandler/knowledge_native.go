package knowledgehandler

import (
	"encoding/json"

	"github.com/changmin/c4-core/internal/eventbus"
	"github.com/changmin/c4-core/internal/knowledge"
	"github.com/changmin/c4-core/internal/llm"
	"github.com/changmin/c4-core/internal/mcp"
)

// CloudSemanticSearcher abstracts the semantic search capability of the cloud knowledge client.
// Implemented by *cloud.KnowledgeCloudClient.
type CloudSemanticSearcher interface {
	SemanticSearch(embedding []float32, limit int, similarityThreshold float32) ([]map[string]any, error)
}

// KnowledgeNativeOpts holds dependencies for native knowledge handlers.
type KnowledgeNativeOpts struct {
	Store         *knowledge.Store
	Searcher      *knowledge.Searcher
	Cloud         knowledge.CloudSyncer          // nil if cloud disabled
	CloudSearch   CloudSemanticSearcher          // nil if semantic cloud search unavailable
	CloudMode     string                         // "cloud-primary" or "local-first" (default)
	Usage         *knowledge.UsageTracker        // nil if usage tracking disabled
	LLM           *llm.Gateway                   // nil if LLM gateway disabled (distill unavailable)
	GlobalManager *knowledge.GlobalKnowledgeManager // nil if global store unavailable
}

var knowledgeEventPub eventbus.Publisher
var knowledgeProjectID string

// SetKnowledgeEventBus sets the EventBus publisher and project ID for knowledge event publishing.
func SetKnowledgeEventBus(pub eventbus.Publisher, projectID string) {
	knowledgeEventPub = pub
	knowledgeProjectID = projectID
}

// RegisterKnowledgeNativeHandlers registers 12 knowledge tools as Go native handlers.
// Replaces the proxy-based knowledge tools in RegisterProxyHandlers.
func RegisterKnowledgeNativeHandlers(reg *mcp.Registry, opts *KnowledgeNativeOpts) {
	if opts == nil || opts.Store == nil {
		return
	}

	// 1. c4_knowledge_record
	reg.Register(mcp.ToolSchema{
		Name:        "c4_knowledge_record",
		Description: "Record a new knowledge document",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"doc_type": map[string]any{"type": "string", "description": "Document type: experiment, pattern, insight, hypothesis"},
				"title":    map[string]any{"type": "string", "description": "Document title"},
				"content":  map[string]any{"type": "string", "description": "Document content (markdown)"},
				"tags":       map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Optional tags"},
				"visibility": map[string]any{"type": "string", "description": "Visibility: private, team, public (default: team)"},
				"scope":      map[string]any{"type": "string", "enum": []string{"project", "global", "auto"}, "description": "Storage scope: project (default), global (~/.c4/knowledge/), auto (global if domain is go/python/ts/debugging/testing)"},
			},
			"required": []string{"doc_type", "title", "content"},
		},
	}, knowledgeRecordNativeHandler(opts))

	// 2. c4_knowledge_get
	reg.Register(mcp.ToolSchema{
		Name:        "c4_knowledge_get",
		Description: "Get a knowledge document by ID",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"doc_id": map[string]any{"type": "string", "description": "Document ID"},
				"cite":   map[string]any{"type": "boolean", "description": "Mark as cited (boosts future ranking)"},
			},
			"required": []string{"doc_id"},
		},
	}, knowledgeGetNativeHandler(opts))

	// 3. c4_knowledge_search
	reg.Register(mcp.ToolSchema{
		Name:        "c4_knowledge_search",
		Description: "Search knowledge base documents with hybrid vector + FTS search",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query":    map[string]any{"type": "string", "description": "Search query"},
				"doc_type": map[string]any{"type": "string", "description": "Filter by type (experiment, pattern, insight, hypothesis)"},
				"limit":    map[string]any{"type": "integer", "description": "Max results (default: 10)"},
				"format":   map[string]any{"type": "string", "enum": []string{"widget", "text"}, "description": "Response format: 'widget' returns MCP Apps widget response with _meta; 'text' returns plain JSON (default)"},
			},
			"required": []string{"query"},
		},
	}, knowledgeSearchNativeHandler(opts))

	// 4. c4_experiment_record (alias: creates type=experiment)
	reg.Register(mcp.ToolSchema{
		Name:        "c4_experiment_record",
		Description: "Record an experiment result",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"title":   map[string]any{"type": "string", "description": "Experiment title"},
				"content": map[string]any{"type": "string", "description": "Experiment details and results"},
				"tags":    map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			},
			"required": []string{"title", "content"},
		},
	}, experimentRecordNativeHandler(opts))

	// 5. c4_experiment_search (alias: search with type=experiment filter)
	reg.Register(mcp.ToolSchema{
		Name:        "c4_experiment_search",
		Description: "Search experiment records",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{"type": "string", "description": "Search query"},
				"limit": map[string]any{"type": "integer", "description": "Max results"},
				"format": map[string]any{
					"type":        "string",
					"description": "Response format: 'widget' returns MCP Apps comparison table with _meta; 'text' returns plain JSON (default)",
					"enum":        []string{"widget", "text"},
				},
			},
			"required": []string{"query"},
		},
	}, experimentSearchNativeHandler(opts))

	// 6. c4_pattern_suggest (alias: search with type=pattern, context→query)
	reg.Register(mcp.ToolSchema{
		Name:        "c4_pattern_suggest",
		Description: "Get pattern suggestions based on current context",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"context": map[string]any{"type": "string", "description": "Current context or problem description"},
			},
			"required": []string{"context"},
		},
	}, patternSuggestNativeHandler(opts))

	// 7. c4_knowledge_pull
	reg.Register(mcp.ToolSchema{
		Name:        "c4_knowledge_pull",
		Description: "Pull knowledge documents from cloud to local store",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"doc_type": map[string]any{"type": "string", "description": "Filter by type (experiment, pattern, insight, hypothesis)"},
				"limit":    map[string]any{"type": "integer", "description": "Max documents to pull (default: 50)"},
				"force":    map[string]any{"type": "boolean", "description": "Overwrite existing local docs (default: false)"},
			},
		},
	}, knowledgePullNativeHandler(opts))

	// 8. c4_knowledge_delete
	reg.Register(mcp.ToolSchema{
		Name:        "c4_knowledge_delete",
		Description: "Delete a knowledge document (FTS5 + vector + markdown)",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"doc_id": map[string]any{"type": "string", "description": "Document ID to delete"},
			},
			"required": []string{"doc_id"},
		},
	}, knowledgeDeleteNativeHandler(opts))

	// 9. c4_knowledge_discover — cross-project public knowledge search
	reg.Register(mcp.ToolSchema{
		Name:        "c4_knowledge_discover",
		Description: "Search public knowledge documents across all projects for cross-project discovery",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query":    map[string]any{"type": "string", "description": "Search query"},
				"doc_type": map[string]any{"type": "string", "description": "Filter by type (experiment, pattern, insight, hypothesis)"},
				"limit":    map[string]any{"type": "integer", "description": "Max results (default: 10)"},
			},
			"required": []string{"query"},
		},
	}, knowledgeDiscoverNativeHandler(opts))

	// 10. c4_knowledge_ingest — document ingestion (file/URL → chunk → embed → search)
	reg.Register(mcp.ToolSchema{
		Name:        "c4_knowledge_ingest",
		Description: "Ingest a document file or URL into knowledge base with chunking and embedding for RAG search",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"file_path":  map[string]any{"type": "string", "description": "Path to the document file to ingest"},
				"url":        map[string]any{"type": "string", "description": "URL to fetch and ingest (alternative to file_path)"},
				"title":      map[string]any{"type": "string", "description": "Optional title override (defaults to filename or page title)"},
				"tags":       map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Optional tags"},
				"visibility": map[string]any{"type": "string", "description": "Visibility: private, team, public (default: team)"},
				"max_tokens": map[string]any{"type": "integer", "description": "Chunk size in tokens (default: 512)"},
			},
		},
	}, knowledgeIngestNativeHandler(opts))

	// 11. c4_knowledge_stats
	reg.Register(mcp.ToolSchema{
		Name:        "c4_knowledge_stats",
		Description: "Get knowledge base statistics: document counts by type and visibility",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}, knowledgeStatsNativeHandler(opts))

	// 12. c4_knowledge_reindex
	reg.Register(mcp.ToolSchema{
		Name:        "c4_knowledge_reindex",
		Description: "Rebuild the knowledge search index from Markdown source files",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}, knowledgeReindexNativeHandler(opts))

	// 13. c4_knowledge_publish — opt-in community publishing
	reg.Register(mcp.ToolSchema{
		Name:        "c4_knowledge_publish",
		Description: "Publish a knowledge document to the community pool (opt-in, metadata stripped)",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"doc_id": map[string]any{"type": "string", "description": "ID of the document to publish"},
			},
			"required": []string{"doc_id"},
		},
	}, knowledgePublishNativeHandler(opts))

	// 14. c4_knowledge_ingest_paper — LLM-based lesson extraction from paper/URL/text
	if opts.LLM != nil {
		reg.Register(mcp.ToolSchema{
			Name:        "c4_knowledge_ingest_paper",
			Description: "Extract software development lessons from a paper, URL, or text using LLM and save them as knowledge documents",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"source":  map[string]any{"type": "string", "description": "URL, file path, or raw text to extract lessons from"},
					"context": map[string]any{"type": "string", "description": "Project context for lesson extraction (default: 일반 소프트웨어 프로젝트)"},
				},
				"required": []string{"source"},
			},
		}, IngestPaperHandler(opts.LLM, &storeKnowledgeWriter{store: opts.Store}))
	}

	// 15. c4_research_suggest — on-demand hypothesis generation from experiment documents
	if opts.LLM != nil {
		reg.Register(mcp.ToolSchema{
			Name:        "c4_research_suggest",
			Description: "Analyze experiment records and generate a hypothesis with a cq.yaml draft",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"tag":   map[string]any{"type": "string", "description": "Filter experiments by tag"},
					"limit": map[string]any{"type": "integer", "description": "Max experiments to analyze (default: 10)"},
				},
			},
		}, researchSuggestNativeHandler(opts))
	}

	// 15. c4_knowledge_distill — LLM-based pattern extraction from similar document clusters
	if opts.LLM != nil && opts.Searcher != nil {
		reg.Register(mcp.ToolSchema{
			Name:        "c4_knowledge_distill",
			Description: "Auto-distill similar document clusters into pattern documents using LLM",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"threshold":   map[string]any{"type": "number", "description": "Similarity threshold for clustering (default: 0.7)"},
					"min_cluster": map[string]any{"type": "integer", "description": "Minimum cluster size (default: 3)"},
					"dry_run":     map[string]any{"type": "boolean", "description": "Preview clusters without creating patterns (default: true)"},
				},
			},
		}, knowledgeDistillNativeHandler(opts))
	}

}

// =========================================================================
// Internal adapters
// =========================================================================

// storeKnowledgeWriter adapts *knowledge.Store to satisfy PaperKnowledgeWriter.
type storeKnowledgeWriter struct {
	store *knowledge.Store
}

func (a *storeKnowledgeWriter) CreateExperiment(metadata map[string]any, body string) (string, error) {
	return a.store.Create(knowledge.TypeExperiment, metadata, body)
}

// =========================================================================
// Helpers
// =========================================================================

func documentToMap(doc *knowledge.Document) map[string]any {
	m := map[string]any{
		"id":         doc.ID,
		"type":       string(doc.Type),
		"title":      doc.Title,
		"domain":     doc.Domain,
		"tags":       doc.Tags,
		"body":       doc.Body,
		"visibility": doc.Visibility,
		"created_at": doc.CreatedAt,
		"updated_at": doc.UpdatedAt,
		"version":    doc.Version,
	}
	if doc.TaskID != "" {
		m["task_id"] = doc.TaskID
	}
	if doc.Hypothesis != "" {
		m["hypothesis"] = doc.Hypothesis
	}
	if doc.HypothesisStatus != "" {
		m["hypothesis_status"] = doc.HypothesisStatus
	}
	if doc.Confidence != 0 {
		m["confidence"] = doc.Confidence
	}
	if doc.InsightType != "" {
		m["insight_type"] = doc.InsightType
	}
	if doc.Status != "" {
		m["status"] = doc.Status
	}
	return m
}

func stringFromAny(v any) string {
	s, _ := v.(string)
	return s
}

func toStringSliceAny(v any) []string {
	if v == nil {
		return nil
	}
	switch t := v.(type) {
	case []string:
		return t
	case []any:
		var result []string
		for _, item := range t {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return result
	}
	return nil
}


func parseParams(rawArgs json.RawMessage) map[string]any {
	var params map[string]any
	if len(rawArgs) > 0 {
		if err := json.Unmarshal(rawArgs, &params); err != nil {
			params = nil
		}
	}
	if params == nil {
		params = make(map[string]any)
	}
	return params
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
