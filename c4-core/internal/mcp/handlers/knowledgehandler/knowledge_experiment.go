package knowledgehandler

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/changmin/c4-core/internal/webcontent"
	"github.com/changmin/c4-core/internal/knowledge"
	"github.com/changmin/c4-core/internal/llm"
	"github.com/changmin/c4-core/internal/mcp"
	"github.com/changmin/c4-core/internal/mcp/apps"
)

const experimentCompareResourceURI = "ui://cq/experiment-compare"

// RegisterExperimentCompareWidget registers the experiment-compare HTML widget in the resource store.
// Call this when the apps ResourceStore is available.
func RegisterExperimentCompareWidget(rs *apps.ResourceStore, html string) {
	if rs != nil && html != "" {
		rs.Register(experimentCompareResourceURI, html)
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
			var embedding []float32
			if opts.Searcher != nil {
				if vs := opts.Searcher.VectorStore(); vs != nil {
					embedding = vs.Get(docID)
				}
			}
			go func() {
				if syncErr := knowledge.SyncAfterRecord(opts.Cloud, params, docID, embedding); syncErr != nil {
					fmt.Fprintf(os.Stderr, "c4: knowledge cloud sync: %v\n", syncErr)
				}
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
					ID:    stringFromAny(r["id"]),
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

		format, _ := params["format"].(string)
		if format == "widget" {
			return map[string]any{
				"data": response,
				"_meta": map[string]any{
					"ui": map[string]any{
						"resourceUri": experimentCompareResourceURI,
					},
				},
			}, nil
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
					ID:    stringFromAny(r["id"]),
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
			Cloud:      opts.Cloud,
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
				llmCtx, llmCancel := context.WithTimeout(context.Background(), 60*time.Second)
				resp, err := opts.LLM.Chat(llmCtx, "scout", &llm.ChatRequest{
					Model:       ref.Model,
					Messages:    []llm.Message{{Role: "user", Content: prompt}},
					MaxTokens:   200,
					Temperature: 0.3,
				})
				llmCancel()
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
			"clusters":           clusterResults,
			"total_clusters":     len(clusters),
			"total_docs_covered": totalDocsCovered,
			"dry_run":            dryRun,
		}, nil
	}
}
