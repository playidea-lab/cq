package handlers

import (
	"encoding/json"
	"fmt"
	"math"
	"os"

	"github.com/changmin/c4-core/internal/knowledge"
	"github.com/changmin/c4-core/internal/mcp"
)

// KnowledgeNativeOpts holds dependencies for native knowledge handlers.
type KnowledgeNativeOpts struct {
	Store    *knowledge.Store
	Searcher *knowledge.Searcher
	Cloud    knowledge.CloudSyncer     // nil if cloud disabled
	Usage    *knowledge.UsageTracker   // nil if usage tracking disabled
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

	// 10. c4_knowledge_ingest — document ingestion (file → chunk → embed → search)
	reg.Register(mcp.ToolSchema{
		Name:        "c4_knowledge_ingest",
		Description: "Ingest a document file into knowledge base with chunking and embedding for RAG search",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"file_path":  map[string]any{"type": "string", "description": "Path to the document file to ingest"},
				"title":      map[string]any{"type": "string", "description": "Optional title override (defaults to filename)"},
				"tags":       map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Optional tags"},
				"visibility": map[string]any{"type": "string", "description": "Visibility: private, team, public (default: team)"},
				"max_tokens": map[string]any{"type": "integer", "description": "Chunk size in tokens (default: 512)"},
			},
			"required": []string{"file_path"},
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

		visibility, _ := params["visibility"].(string)

		metadata := map[string]any{
			"title":      title,
			"domain":     params["domain"],
			"tags":       params["tags"],
			"visibility": visibility,
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

		// Index for vector search (skip silently on embedding failure — will be caught up on reindex)
		var embedWarning string
		if opts.Searcher != nil {
			doc, _ := opts.Store.Get(docID)
			if doc != nil {
				if idxErr := opts.Searcher.IndexDocument(docID, doc); idxErr != nil {
					embedWarning = fmt.Sprintf("embedding skipped (will embed on reindex): %v", idxErr)
					fmt.Fprintf(os.Stderr, "c4: knowledge: %s\n", embedWarning)
				}
			}
		}

		// Find related documents (best-effort)
		var relatedList []map[string]any
		if opts.Searcher != nil && embedWarning == "" {
			searchText := title
			if body != "" {
				searchText += " " + body
			}
			related := opts.Searcher.FindRelated(searchText, docID, 3)
			if len(related) > 0 {
				relatedList = make([]map[string]any, len(related))
				for i, r := range related {
					relatedList[i] = map[string]any{
						"id":         r.ID,
						"title":      r.Title,
						"type":       r.Type,
						"similarity": r.RRFScore,
					}
				}
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

		result := map[string]any{
			"success": true,
			"doc_id":  docID,
		}
		if embedWarning != "" {
			result["warning"] = embedWarning
		}
		if len(relatedList) > 0 {
			result["related"] = relatedList
		}
		return result, nil
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

		// Track usage
		if opts.Usage != nil {
			opts.Usage.Record(docID, knowledge.ActionView)
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
				"id":               r.ID,
				"title":            r.Title,
				"type":             r.Type,
				"domain":           r.Domain,
				"rrf_score":        r.RRFScore,
				"embedding_source": r.EmbeddingSource,
				"source":           "local",
			}
		}

		// Track search_hit usage
		if opts.Usage != nil {
			for _, r := range results {
				opts.Usage.Record(r.ID, knowledge.ActionSearchHit)
			}
		}

		localCount := len(resultList)
		communityCount := 0

		// Blend community results if available
		if opts.Cloud != nil {
			cloudDocs, cloudErr := opts.Cloud.DiscoverPublic(query, docType, limit)
			if cloudErr == nil && len(cloudDocs) > 0 {
				localIDs := make(map[string]bool, localCount)
				for _, r := range results {
					localIDs[r.ID] = true
				}
				for _, cd := range cloudDocs {
					cdID, _ := cd["id"].(string)
					if localIDs[cdID] {
						continue
					}
					resultList = append(resultList, map[string]any{
						"id":     cdID,
						"title":  cd["title"],
						"type":   cd["type"],
						"domain": cd["domain"],
						"source": "community",
					})
					communityCount++
				}
			}
		}

		response := map[string]any{
			"results": resultList,
			"count":   len(resultList),
		}
		if communityCount > 0 {
			response["local_count"] = localCount
			response["community_count"] = communityCount
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
		var embedWarning2 string
		if opts.Searcher != nil {
			doc, _ := opts.Store.Get(docID)
			if doc != nil {
				if idxErr := opts.Searcher.IndexDocument(docID, doc); idxErr != nil {
					embedWarning2 = fmt.Sprintf("embedding skipped (will embed on reindex): %v", idxErr)
					fmt.Fprintf(os.Stderr, "c4: knowledge: %s\n", embedWarning2)
				}
			}
		}
		// Find related documents (best-effort)
		var relatedList []map[string]any
		if opts.Searcher != nil && embedWarning2 == "" {
			searchText := title
			if body != "" {
				searchText += " " + body
			}
			related := opts.Searcher.FindRelated(searchText, docID, 3)
			if len(related) > 0 {
				relatedList = make([]map[string]any, len(related))
				for i, r := range related {
					relatedList[i] = map[string]any{
						"id":         r.ID,
						"title":      r.Title,
						"type":       r.Type,
						"similarity": r.RRFScore,
					}
				}
			}
		}

		if opts.Cloud != nil {
			params["doc_type"] = "experiment"
			go func() {
				knowledge.SyncAfterRecord(opts.Cloud, params, docID)
			}()
		}

		result := map[string]any{
			"success": true,
			"doc_id":  docID,
		}
		if embedWarning2 != "" {
			result["warning"] = embedWarning2
		}
		if len(relatedList) > 0 {
			result["related"] = relatedList
		}
		return result, nil
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
				"source":    "local",
			}
		}

		localCount := len(resultList)
		communityCount := 0

		// Blend community results (consistent with knowledge_search)
		if opts.Cloud != nil {
			cloudDocs, cloudErr := opts.Cloud.DiscoverPublic(query, "experiment", limit)
			if cloudErr == nil && len(cloudDocs) > 0 {
				localIDs := make(map[string]bool, localCount)
				for _, r := range results {
					localIDs[r.ID] = true
				}
				for _, cd := range cloudDocs {
					cdID, _ := cd["id"].(string)
					if localIDs[cdID] {
						continue
					}
					resultList = append(resultList, map[string]any{
						"id":        cdID,
						"title":     cd["title"],
						"type":      cd["type"],
						"domain":    cd["domain"],
						"rrf_score": float64(0),
						"source":    "community",
					})
					communityCount++
				}
			}
		}

		response := map[string]any{
			"results": resultList,
			"count":   len(resultList),
		}
		if communityCount > 0 {
			response["local_count"] = localCount
			response["community_count"] = communityCount
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
				"source":    "local",
			}
		}

		localCount := len(resultList)
		communityCount := 0

		if opts.Cloud != nil {
			cloudDocs, cloudErr := opts.Cloud.DiscoverPublic(context, "pattern", limit)
			if cloudErr == nil && len(cloudDocs) > 0 {
				localIDs := make(map[string]bool, localCount)
				for _, r := range results {
					localIDs[r.ID] = true
				}
				for _, cd := range cloudDocs {
					cdID, _ := cd["id"].(string)
					if localIDs[cdID] {
						continue
					}
					resultList = append(resultList, map[string]any{
						"id":     cdID,
						"title":  cd["title"],
						"type":   cd["type"],
						"domain": cd["domain"],
						"source": "community",
					})
					communityCount++
				}
			}
		}

		response := map[string]any{
			"results": resultList,
			"count":   len(resultList),
		}
		if communityCount > 0 {
			response["local_count"] = localCount
			response["community_count"] = communityCount
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

func knowledgeDeleteNativeHandler(opts *KnowledgeNativeOpts) mcp.HandlerFunc {
	return func(rawArgs json.RawMessage) (any, error) {
		params := parseParams(rawArgs)
		docID, _ := params["doc_id"].(string)
		if docID == "" {
			return map[string]any{"error": "doc_id is required"}, nil
		}

		// Store.Delete handles: documents table + FTS5 + markdown file
		deleted, err := opts.Store.Delete(docID)
		if err != nil {
			return map[string]any{"error": fmt.Sprintf("delete failed: %v", err)}, nil
		}
		if !deleted {
			return map[string]any{"error": "document not found", "doc_id": docID}, nil
		}

		// Also remove vector embedding (shares same DB, separate table)
		opts.Store.DB().Exec("DELETE FROM knowledge_vectors WHERE doc_id = ?", docID)

		// Cloud soft-delete (async)
		if opts.Cloud != nil {
			go func() {
				if delErr := opts.Cloud.DeleteDocument(docID); delErr != nil {
					fmt.Fprintf(os.Stderr, "c4: knowledge cloud delete: %v\n", delErr)
				}
			}()
		}

		return map[string]any{
			"deleted": true,
			"doc_id":  docID,
		}, nil
	}
}

func knowledgeDiscoverNativeHandler(opts *KnowledgeNativeOpts) mcp.HandlerFunc {
	return func(rawArgs json.RawMessage) (any, error) {
		if opts.Cloud == nil {
			return map[string]any{"error": "cloud not configured — discover requires cloud connection"}, nil
		}

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

		docs, err := opts.Cloud.DiscoverPublic(query, docType, limit)
		if err != nil {
			return map[string]any{"error": fmt.Sprintf("discover failed: %v", err)}, nil
		}

		return map[string]any{
			"results": docs,
			"count":   len(docs),
		}, nil
	}
}

func knowledgeIngestNativeHandler(opts *KnowledgeNativeOpts) mcp.HandlerFunc {
	return func(rawArgs json.RawMessage) (any, error) {
		params := parseParams(rawArgs)
		filePath, _ := params["file_path"].(string)
		if filePath == "" {
			return map[string]any{"error": "file_path is required"}, nil
		}

		title, _ := params["title"].(string)
		visibility, _ := params["visibility"].(string)
		maxTokens := 512
		if mt, ok := params["max_tokens"].(float64); ok && mt > 0 {
			maxTokens = int(mt)
		}

		var tags []string
		if rawTags, ok := params["tags"]; ok {
			tags = toStringSliceAny(rawTags)
		}

		ingestOpts := knowledge.IngestOpts{
			Title:      title,
			Tags:       tags,
			Visibility: visibility,
			MaxTokens:  maxTokens,
		}

		result, err := knowledge.Ingest(opts.Store, opts.Searcher, filePath, ingestOpts)
		if err != nil {
			return map[string]any{"error": fmt.Sprintf("ingest failed: %v", err)}, nil
		}

		return map[string]any{
			"success":     true,
			"doc_id":      result.DocID,
			"title":       result.Title,
			"chunk_count": result.ChunkCount,
			"body_length": result.BodyLen,
		}, nil
	}
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

func knowledgeStatsNativeHandler(opts *KnowledgeNativeOpts) mcp.HandlerFunc {
	return func(rawArgs json.RawMessage) (any, error) {
		// Count by type
		allDocs, err := opts.Store.List("", "", 10000)
		if err != nil {
			return map[string]any{"error": fmt.Sprintf("list failed: %v", err)}, nil
		}

		typeCounts := map[string]int{}
		visCounts := map[string]int{}
		for _, d := range allDocs {
			dt, _ := d["type"].(string)
			typeCounts[dt]++
		}

		// Read visibility from docs (need full read for visibility field)
		for _, d := range allDocs {
			docID, _ := d["id"].(string)
			doc, _ := opts.Store.Get(docID)
			if doc != nil {
				vis := doc.Visibility
				if vis == "" {
					vis = "team"
				}
				visCounts[vis]++
			}
		}

		stats := map[string]any{
			"total":          len(allDocs),
			"by_type":        typeCounts,
			"by_visibility":  visCounts,
		}

		// Vector store stats
		if opts.Searcher != nil && opts.Searcher.VectorStore() != nil {
			vs := opts.Searcher.VectorStore()
			vectorCount := vs.Count()
			stats["vector_count"] = vectorCount
			stats["vector_dimension"] = vs.Dimension()
			stats["has_real_embedder"] = vs.HasRealEmbedder()
			modelCounts, _ := vs.CountByModel()
			if len(modelCounts) > 0 {
				stats["vector_by_model"] = modelCounts
			}
			// Documents without embeddings (pending re-embed)
			unembedded := len(allDocs) - vectorCount
			if unembedded < 0 {
				unembedded = 0
			}
			if unembedded > 0 {
				stats["unembedded"] = unembedded
			}

			// Pairwise similarity distribution (Phase 3)
			if vectorCount >= 2 {
				avg, maxSim, minSim, pairs := opts.Searcher.PairwiseSimilarityStats(100)
				if pairs > 0 {
					stats["similarity"] = map[string]any{
						"avg_pairwise":  math.Round(avg*1000) / 1000,
						"max_pairwise":  math.Round(maxSim*1000) / 1000,
						"min_pairwise":  math.Round(minSim*1000) / 1000,
						"pairs_sampled": pairs,
					}
				}
			}
		}

		// Pattern count + distillation readiness hint
		stats["pattern_count"] = typeCounts["pattern"]
		if len(allDocs) >= 50 {
			stats["distillation_hint"] = fmt.Sprintf("%d documents accumulated — pattern analysis possible", len(allDocs))
		}

		return stats, nil
	}
}

func knowledgeReindexNativeHandler(opts *KnowledgeNativeOpts) mcp.HandlerFunc {
	return func(rawArgs json.RawMessage) (any, error) {
		count, err := opts.Store.RebuildIndex()
		if err != nil {
			return map[string]any{"error": fmt.Sprintf("reindex failed: %v", err)}, nil
		}

		result := map[string]any{
			"success":  true,
			"indexed":  count,
			"docs_dir": opts.Store.DocsDir(),
		}

		// Re-embed all documents if vector store with real embedder is available
		if opts.Searcher != nil && opts.Searcher.VectorStore() != nil && opts.Searcher.VectorStore().HasRealEmbedder() {
			summaries, listErr := opts.Store.List("", "", 10000)
			if listErr == nil && len(summaries) > 0 {
				var ids []string
				var docs []*knowledge.Document
				for _, s := range summaries {
					docID, _ := s["id"].(string)
					if docID == "" {
						continue
					}
					doc, getErr := opts.Store.Get(docID)
					if getErr != nil || doc == nil {
						continue
					}
					ids = append(ids, docID)
					docs = append(docs, doc)
				}
				if len(ids) > 0 {
					if embErr := opts.Searcher.BatchIndexDocuments(ids, docs); embErr != nil {
						result["embed_error"] = embErr.Error()
					} else {
						result["embedded"] = len(ids)
					}
				}
			}
		}

		return result, nil
	}
}

func knowledgePublishNativeHandler(opts *KnowledgeNativeOpts) mcp.HandlerFunc {
	return func(rawArgs json.RawMessage) (any, error) {
		params := parseParams(rawArgs)
		docID, _ := params["doc_id"].(string)
		if docID == "" {
			return map[string]any{"error": "doc_id is required"}, nil
		}
		if opts.Cloud == nil {
			return map[string]any{"error": "cloud not configured — publish requires cloud connection"}, nil
		}

		doc, err := opts.Store.Get(docID)
		if err != nil || doc == nil {
			return map[string]any{"error": fmt.Sprintf("document not found: %s", docID)}, nil
		}

		if err := knowledge.PublishDocument(opts.Store, opts.Cloud, docID); err != nil {
			return map[string]any{"error": fmt.Sprintf("publish failed: %v", err)}, nil
		}

		strippedBody := knowledge.StripMetadata(doc.Body)
		return map[string]any{
			"success":    true,
			"doc_id":     docID,
			"title":      doc.Title,
			"type":       string(doc.Type),
			"visibility": "public",
			"stripped":   strippedBody != doc.Body,
		}, nil
	}
}
