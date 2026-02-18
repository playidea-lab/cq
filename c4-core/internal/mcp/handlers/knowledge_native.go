package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strings"

	"github.com/changmin/c4-core/internal/c2/webcontent"
	"github.com/changmin/c4-core/internal/eventbus"
	"github.com/changmin/c4-core/internal/knowledge"
	"github.com/changmin/c4-core/internal/llm"
	"github.com/changmin/c4-core/internal/mcp"
)

// KnowledgeNativeOpts holds dependencies for native knowledge handlers.
type KnowledgeNativeOpts struct {
	Store    *knowledge.Store
	Searcher *knowledge.Searcher
	Cloud    knowledge.CloudSyncer   // nil if cloud disabled
	Usage    *knowledge.UsageTracker // nil if usage tracking disabled
	LLM      *llm.Gateway             // nil if LLM gateway disabled (distill unavailable)
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

	// 14. c4_knowledge_distill — LLM-based pattern extraction from similar document clusters
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

		// Find related documents (best-effort, uses same embedding as IndexDocument)
		var relatedList []map[string]any
		if opts.Searcher != nil && embedWarning == "" {
			doc, _ := opts.Store.Get(docID)
			searchText := ""
			if doc != nil {
				searchText = knowledge.DocumentToText(doc)
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

		if knowledgeEventPub != nil {
			payload, _ := json.Marshal(map[string]any{"doc_id": docID, "doc_type": docType, "title": title})
			knowledgeEventPub.PublishAsync("knowledge.recorded", "c4.knowledge", payload, knowledgeProjectID)
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

		// Track usage: cite=true → ActionCite (higher boost), else ActionView
		if opts.Usage != nil {
			cite, _ := params["cite"].(bool)
			if cite {
				opts.Usage.Record(docID, knowledge.ActionCite)
			} else {
				opts.Usage.Record(docID, knowledge.ActionView)
			}
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
		if knowledgeEventPub != nil {
			payload, _ := json.Marshal(map[string]any{"query": query, "doc_type": docType, "result_count": len(results)})
			knowledgeEventPub.PublishAsync("knowledge.searched", "c4.knowledge", payload, knowledgeProjectID)
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
					cdID, _ := cd["doc_id"].(string)
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
		// Find related documents (best-effort, uses same embedding as IndexDocument)
		var relatedList []map[string]any
		if opts.Searcher != nil && embedWarning2 == "" {
			doc, _ := opts.Store.Get(docID)
			searchText := ""
			if doc != nil {
				searchText = knowledge.DocumentToText(doc)
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
		if knowledgeEventPub != nil {
			payload, _ := json.Marshal(map[string]any{"doc_id": docID, "doc_type": "experiment", "title": title})
			knowledgeEventPub.PublishAsync("knowledge.recorded", "c4.knowledge", payload, knowledgeProjectID)
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

		// Track search_hit usage
		if opts.Usage != nil {
			for _, r := range results {
				opts.Usage.Record(r.ID, knowledge.ActionSearchHit)
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
					cdID, _ := cd["doc_id"].(string)
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

		// Track search_hit usage
		if opts.Usage != nil {
			for _, r := range results {
				opts.Usage.Record(r.ID, knowledge.ActionSearchHit)
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
					cdID, _ := cd["doc_id"].(string)
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

		// Also remove vector embeddings including chunks (doc_id-chunk-N)
		opts.Store.DB().Exec("DELETE FROM knowledge_vectors WHERE doc_id = ? OR doc_id LIKE ?", docID, docID+"-chunk-%")

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
		urlStr, _ := params["url"].(string)

		if filePath != "" && urlStr != "" {
			return map[string]any{"error": "provide either file_path or url, not both"}, nil
		}
		if filePath == "" && urlStr == "" {
			return map[string]any{"error": "file_path or url is required"}, nil
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

		// URL path: fetch web content, then ingest as text
		if urlStr != "" {
			fetchResult, err := webcontent.Fetch(urlStr, nil)
			if err != nil {
				return map[string]any{"error": fmt.Sprintf("fetch failed: %v", err)}, nil
			}

			if ingestOpts.Title == "" {
				ingestOpts.Title = fetchResult.Title
			}
			ingestOpts.Tags = append(ingestOpts.Tags, "source:url")

			result, err := knowledge.IngestText(opts.Store, opts.Searcher, fetchResult.Content, ingestOpts)
			if err != nil {
				return map[string]any{"error": fmt.Sprintf("ingest failed: %v", err)}, nil
			}

			return map[string]any{
				"success":     true,
				"doc_id":      result.DocID,
				"title":       result.Title,
				"chunk_count": result.ChunkCount,
				"body_length": result.BodyLen,
				"source_url":  urlStr,
				"method":      fetchResult.Method,
			}, nil
		}

		// File path: existing behavior
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

		// Embedding coverage
		if opts.Searcher != nil && opts.Searcher.VectorStore() != nil {
			vectorCount := opts.Searcher.VectorStore().Count()
			total := len(allDocs)
			if total > 0 {
				pct := float64(vectorCount) / float64(total) * 100
				stats["embedding_coverage"] = fmt.Sprintf("%.0f%%", pct)
			}
		}

		// Usage stats
		if opts.Usage != nil {
			stats["usage"] = opts.Usage.GetStats()
		}

		// Pattern count + structured distillation info
		stats["pattern_count"] = typeCounts["pattern"]
		if opts.Searcher != nil && opts.Searcher.VectorStore() != nil && opts.Searcher.VectorStore().Count() >= 2 {
			clusters := opts.Searcher.FindClusters(0.7, 3)
			largestCluster := 0
			for _, c := range clusters {
				if len(c) > largestCluster {
					largestCluster = len(c)
				}
			}
			stats["distillation"] = map[string]any{
				"cluster_count":   len(clusters),
				"largest_cluster": largestCluster,
				"hint":            fmt.Sprintf("%d clusters (largest=%d docs) — run c4_knowledge_distill", len(clusters), largestCluster),
			}
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

const distillPrompt = `You are a knowledge distillation assistant. Given these related documents, extract the common pattern or principle in 1-2 concise sentences.

Documents:
%s

Pattern:`

func knowledgeDistillNativeHandler(opts *KnowledgeNativeOpts) mcp.HandlerFunc {
	return func(rawArgs json.RawMessage) (any, error) {
		params := parseParams(rawArgs)

		threshold := 0.7
		if t, ok := params["threshold"].(float64); ok && t > 0 && t <= 1.0 {
			threshold = t
		}
		minCluster := 3
		if mc, ok := params["min_cluster"].(float64); ok && mc >= 2 {
			minCluster = int(mc)
		}
		dryRun := true
		if dr, ok := params["dry_run"].(bool); ok {
			dryRun = dr
		}

		clusters := opts.Searcher.FindClusters(threshold, minCluster)
		if len(clusters) == 0 {
			return map[string]any{
				"clusters":       []any{},
				"total_clusters": 0,
				"message":        "no clusters found at this threshold",
			}, nil
		}

		totalDocsCovered := 0
		var clusterResults []map[string]any

		for _, cluster := range clusters {
			totalDocsCovered += len(cluster)

			// Collect document bodies (max 5 per cluster)
			maxDocs := 5
			if len(cluster) < maxDocs {
				maxDocs = len(cluster)
			}
			var docTexts []string
			for i := 0; i < maxDocs; i++ {
				doc, _ := opts.Store.Get(cluster[i])
				if doc != nil {
					text := doc.Title
					if doc.Body != "" {
						body := doc.Body
						if len(body) > 300 {
							body = body[:300] + "..."
						}
						text += ": " + body
					}
					docTexts = append(docTexts, fmt.Sprintf("- %s", text))
				}
			}

			clusterResult := map[string]any{
				"size": len(cluster),
				"docs": cluster,
			}

			// Call LLM for pattern extraction
			if len(docTexts) > 0 {
				prompt := fmt.Sprintf(distillPrompt, strings.Join(docTexts, "\n"))
				ref := opts.LLM.Resolve("scout", "")
				resp, err := opts.LLM.Chat(context.Background(), "scout", &llm.ChatRequest{
					Model:       ref.Model,
					Messages:    []llm.Message{{Role: "user", Content: prompt}},
					MaxTokens:   200,
					Temperature: 0.3,
				})
				if err != nil {
					clusterResult["llm_error"] = err.Error()
				} else {
					pattern := strings.TrimSpace(resp.Content)
					clusterResult["suggested_pattern"] = pattern

					// Create pattern document if not dry_run
					if !dryRun && pattern != "" {
						metadata := map[string]any{
							"title":      fmt.Sprintf("Distilled: %s", truncate(pattern, 60)),
							"tags":       []string{"auto-distilled"},
							"visibility": "team",
						}
						patID, createErr := opts.Store.Create(knowledge.TypePattern, metadata, pattern)
						if createErr == nil {
							clusterResult["created_pattern_id"] = patID
							// Index for search
							if opts.Searcher != nil {
								doc, _ := opts.Store.Get(patID)
								if doc != nil {
									opts.Searcher.IndexDocument(patID, doc)
								}
							}
						}
					}
				}
			}

			clusterResults = append(clusterResults, clusterResult)
		}

		return map[string]any{
			"clusters":          clusterResults,
			"total_clusters":    len(clusters),
			"total_docs_covered": totalDocsCovered,
			"dry_run":           dryRun,
		}, nil
	}
}

// truncate is defined in twin.go — reused here for distill pattern titles
