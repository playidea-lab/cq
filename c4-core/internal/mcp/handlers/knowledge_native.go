package handlers

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/changmin/c4-core/internal/knowledge"
	"github.com/changmin/c4-core/internal/mcp"
)

// KnowledgeNativeOpts holds dependencies for native knowledge handlers.
type KnowledgeNativeOpts struct {
	Store    *knowledge.Store
	Searcher *knowledge.Searcher
	Cloud    knowledge.CloudSyncer // nil if cloud disabled
}

// RegisterKnowledgeNativeHandlers registers 7 knowledge tools as Go native handlers.
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
				"tags":     map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Optional tags"},
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
}

// =========================================================================
// Handler implementations
// =========================================================================

func knowledgeRecordNativeHandler(opts *KnowledgeNativeOpts) mcp.HandlerFunc {
	return func(rawArgs json.RawMessage) (any, error) {
		params := parseParams(rawArgs)

		docType, _ := params["doc_type"].(string)
		if docType == "" {
			return map[string]any{"error": "doc_type is required"}, nil
		}
		title, _ := params["title"].(string)
		if title == "" {
			return map[string]any{"error": "title is required"}, nil
		}
		body, _ := params["content"].(string)

		metadata := map[string]any{
			"title":  title,
			"domain": params["domain"],
			"tags":   params["tags"],
		}
		// Copy extra fields
		for _, key := range []string{"id", "task_id", "hypothesis", "hypothesis_status",
			"confidence", "evidence_count", "insight_type", "status"} {
			if v, ok := params[key]; ok {
				metadata[key] = v
			}
		}

		docID, err := opts.Store.Create(knowledge.DocumentType(docType), metadata, body)
		if err != nil {
			return map[string]any{"error": fmt.Sprintf("KnowledgeRecord failed: %v", err)}, nil
		}

		// Index for vector search
		if opts.Searcher != nil {
			doc, _ := opts.Store.Get(docID)
			if doc != nil {
				opts.Searcher.IndexDocument(docID, doc)
			}
		}

		// Async cloud push
		if opts.Cloud != nil {
			go func() {
				if syncErr := knowledge.SyncAfterRecord(opts.Cloud, params, docID); syncErr != nil {
					fmt.Fprintf(os.Stderr, "c4: knowledge cloud sync: %v\n", syncErr)
				}
			}()
		}

		return map[string]any{
			"success": true,
			"doc_id":  docID,
		}, nil
	}
}

func knowledgeGetNativeHandler(opts *KnowledgeNativeOpts) mcp.HandlerFunc {
	return func(rawArgs json.RawMessage) (any, error) {
		params := parseParams(rawArgs)
		docID, _ := params["doc_id"].(string)
		if docID == "" {
			return map[string]any{"error": "doc_id is required"}, nil
		}

		doc, err := opts.Store.Get(docID)
		if err != nil {
			return map[string]any{"error": fmt.Sprintf("KnowledgeGet failed: %v", err)}, nil
		}
		if doc == nil {
			return map[string]any{"error": fmt.Sprintf("Document not found: %s", docID)}, nil
		}

		return documentToMap(doc), nil
	}
}

func knowledgeSearchNativeHandler(opts *KnowledgeNativeOpts) mcp.HandlerFunc {
	return func(rawArgs json.RawMessage) (any, error) {
		params := parseParams(rawArgs)
		query, _ := params["query"].(string)
		if query == "" {
			return map[string]any{"error": "query is required"}, nil
		}

		docType, _ := params["doc_type"].(string)
		limit := 10
		if l, ok := params["limit"].(float64); ok && l > 0 {
			limit = int(l)
		}

		var filters map[string]string
		if docType != "" {
			filters = map[string]string{"type": docType}
		}

		var results []knowledge.SearchResult
		var err error
		if opts.Searcher != nil {
			results, err = opts.Searcher.Search(query, limit, filters)
		} else {
			// FTS-only fallback
			ftsResults, ftsErr := opts.Store.SearchFTS(query, limit)
			if ftsErr != nil {
				return map[string]any{"error": fmt.Sprintf("search failed: %v", ftsErr)}, nil
			}
			for _, r := range ftsResults {
				if docType != "" {
					if t, _ := r["type"].(string); t != docType {
						continue
					}
				}
				results = append(results, knowledge.SearchResult{
					ID:     r["id"].(string),
					Title:  stringFromAny(r["title"]),
					Type:   stringFromAny(r["type"]),
					Domain: stringFromAny(r["domain"]),
				})
			}
		}
		if err != nil {
			return map[string]any{"error": fmt.Sprintf("search failed: %v", err)}, nil
		}

		resultList := make([]map[string]any, len(results))
		for i, r := range results {
			resultList[i] = map[string]any{
				"id":        r.ID,
				"title":     r.Title,
				"type":      r.Type,
				"domain":    r.Domain,
				"rrf_score": r.RRFScore,
			}
		}

		response := map[string]any{
			"results": resultList,
			"count":   len(resultList),
		}

		// Merge cloud results if available
		if opts.Cloud != nil {
			cloudDocs, cloudErr := opts.Cloud.SearchDocuments(query, docType, limit)
			if cloudErr == nil && len(cloudDocs) > 0 {
				response["cloud_results"] = cloudDocs
				response["cloud_count"] = len(cloudDocs)
			}
		}

		return response, nil
	}
}

func experimentRecordNativeHandler(opts *KnowledgeNativeOpts) mcp.HandlerFunc {
	return func(rawArgs json.RawMessage) (any, error) {
		params := parseParams(rawArgs)

		title, _ := params["title"].(string)
		if title == "" {
			return map[string]any{"error": "title is required"}, nil
		}
		body, _ := params["content"].(string)

		metadata := map[string]any{
			"title": title,
			"tags":  params["tags"],
		}

		docID, err := opts.Store.Create(knowledge.TypeExperiment, metadata, body)
		if err != nil {
			return map[string]any{"error": fmt.Sprintf("ExperimentRecord failed: %v", err)}, nil
		}

		// Index + cloud sync
		if opts.Searcher != nil {
			doc, _ := opts.Store.Get(docID)
			if doc != nil {
				opts.Searcher.IndexDocument(docID, doc)
			}
		}
		if opts.Cloud != nil {
			params["doc_type"] = "experiment"
			go func() {
				knowledge.SyncAfterRecord(opts.Cloud, params, docID)
			}()
		}

		return map[string]any{
			"success": true,
			"doc_id":  docID,
		}, nil
	}
}

func experimentSearchNativeHandler(opts *KnowledgeNativeOpts) mcp.HandlerFunc {
	return func(rawArgs json.RawMessage) (any, error) {
		params := parseParams(rawArgs)
		query, _ := params["query"].(string)
		if query == "" {
			return map[string]any{"error": "query is required"}, nil
		}

		limit := 10
		if l, ok := params["limit"].(float64); ok && l > 0 {
			limit = int(l)
		}

		filters := map[string]string{"type": "experiment"}

		var results []knowledge.SearchResult
		var err error
		if opts.Searcher != nil {
			results, err = opts.Searcher.Search(query, limit, filters)
		} else {
			ftsResults, ftsErr := opts.Store.SearchFTS(query, limit)
			if ftsErr != nil {
				return map[string]any{"error": fmt.Sprintf("search failed: %v", ftsErr)}, nil
			}
			for _, r := range ftsResults {
				if t, _ := r["type"].(string); t != "experiment" {
					continue
				}
				results = append(results, knowledge.SearchResult{
					ID:    r["id"].(string),
					Title: stringFromAny(r["title"]),
					Type:  stringFromAny(r["type"]),
				})
			}
		}
		if err != nil {
			return map[string]any{"error": fmt.Sprintf("search failed: %v", err)}, nil
		}

		resultList := make([]map[string]any, len(results))
		for i, r := range results {
			resultList[i] = map[string]any{
				"id":        r.ID,
				"title":     r.Title,
				"type":      r.Type,
				"domain":    r.Domain,
				"rrf_score": r.RRFScore,
			}
		}

		response := map[string]any{
			"results": resultList,
			"count":   len(resultList),
		}

		if opts.Cloud != nil {
			cloudDocs, cloudErr := opts.Cloud.SearchDocuments(query, "experiment", limit)
			if cloudErr == nil && len(cloudDocs) > 0 {
				response["cloud_results"] = cloudDocs
				response["cloud_count"] = len(cloudDocs)
			}
		}

		return response, nil
	}
}

func patternSuggestNativeHandler(opts *KnowledgeNativeOpts) mcp.HandlerFunc {
	return func(rawArgs json.RawMessage) (any, error) {
		params := parseParams(rawArgs)
		context, _ := params["context"].(string)
		if context == "" {
			return map[string]any{"error": "context is required"}, nil
		}

		filters := map[string]string{"type": "pattern"}
		limit := 10

		var results []knowledge.SearchResult
		var err error
		if opts.Searcher != nil {
			results, err = opts.Searcher.Search(context, limit, filters)
		} else {
			ftsResults, ftsErr := opts.Store.SearchFTS(context, limit)
			if ftsErr != nil {
				return map[string]any{"error": fmt.Sprintf("search failed: %v", ftsErr)}, nil
			}
			for _, r := range ftsResults {
				if t, _ := r["type"].(string); t != "pattern" {
					continue
				}
				results = append(results, knowledge.SearchResult{
					ID:    r["id"].(string),
					Title: stringFromAny(r["title"]),
					Type:  stringFromAny(r["type"]),
				})
			}
		}
		if err != nil {
			return map[string]any{"error": fmt.Sprintf("search failed: %v", err)}, nil
		}

		resultList := make([]map[string]any, len(results))
		for i, r := range results {
			resultList[i] = map[string]any{
				"id":        r.ID,
				"title":     r.Title,
				"type":      r.Type,
				"domain":    r.Domain,
				"rrf_score": r.RRFScore,
			}
		}

		response := map[string]any{
			"results": resultList,
			"count":   len(resultList),
		}

		if opts.Cloud != nil {
			cloudDocs, cloudErr := opts.Cloud.SearchDocuments(context, "pattern", limit)
			if cloudErr == nil && len(cloudDocs) > 0 {
				response["cloud_results"] = cloudDocs
				response["cloud_count"] = len(cloudDocs)
			}
		}

		return response, nil
	}
}

func knowledgePullNativeHandler(opts *KnowledgeNativeOpts) mcp.HandlerFunc {
	return func(rawArgs json.RawMessage) (any, error) {
		if opts.Cloud == nil {
			return nil, fmt.Errorf("cloud not configured")
		}
		params := parseParams(rawArgs)

		docType, _ := params["doc_type"].(string)
		limit := 50
		if l, ok := params["limit"].(float64); ok && l > 0 {
			limit = int(l)
		}
		force, _ := params["force"].(bool)

		result, err := knowledge.Pull(opts.Store, opts.Cloud, docType, limit, force)
		if err != nil {
			return nil, err
		}

		return map[string]any{
			"pulled":  result.Pulled,
			"updated": result.Updated,
			"skipped": result.Skipped,
			"errors":  result.Errors,
			"details": map[string]any{
				"pulled_ids":  result.PulledIDs,
				"updated_ids": result.UpdatedIDs,
				"skipped_ids": result.SkippedIDs,
			},
		}, nil
	}
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
