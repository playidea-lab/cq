package knowledgehandler

import (
	"context"
	"encoding/json"
	"fmt"
	"math"

	"github.com/changmin/c4-core/internal/config"
	"github.com/changmin/c4-core/internal/knowledge"
	"github.com/changmin/c4-core/internal/mcp"
)

const knowledgeFeedResourceURI = "ui://cq/knowledge-feed"

func knowledgeSearchNativeHandler(opts *KnowledgeNativeOpts) mcp.HandlerFunc {
	return func(rawArgs json.RawMessage) (any, error) {
		params := parseParams(rawArgs)
		query, _ := params["query"].(string)
		if query == "" {
			return map[string]any{"error": "query is required"}, nil
		}

		docType, _ := params["doc_type"].(string)
		format, _ := params["format"].(string)
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
		cloudUsed := false

		// cloud-primary: try cloud semantic search first, fall back to local on failure
		if opts.CloudMode == config.CloudModePrimary && opts.CloudSearch != nil && opts.Searcher != nil && opts.Searcher.VectorStore() != nil {
			queryEmb, _, embedErr := opts.Searcher.VectorStore().EmbedText(context.Background(), query)
			if embedErr == nil && len(queryEmb) > 0 {
				cloudDocs, cloudErr := opts.CloudSearch.SemanticSearch(queryEmb, limit, 0.5)
				if cloudErr == nil {
					for _, cd := range cloudDocs {
						cdType, _ := cd["type"].(string)
						if docType != "" && cdType != docType {
							continue
						}
						results = append(results, knowledge.SearchResult{
							ID:     stringFromAny(cd["id"]),
							Title:  stringFromAny(cd["title"]),
							Type:   cdType,
							Domain: stringFromAny(cd["domain"]),
						})
					}
					cloudUsed = true
				}
				// on cloud error: fall through to local search below
			}
		}

		if !cloudUsed {
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
						ID:     stringFromAny(r["id"]),
						Title:  stringFromAny(r["title"]),
						Type:   stringFromAny(r["type"]),
						Domain: stringFromAny(r["domain"]),
					})
				}
			}
		}
		if err != nil {
			return map[string]any{"error": fmt.Sprintf("search failed: %v", err)}, nil
		}

		resultSource := "local"
		if cloudUsed {
			resultSource = "cloud"
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
				"source":           resultSource,
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
			"query":   query,
		}
		if communityCount > 0 {
			response["local_count"] = localCount
			response["community_count"] = communityCount
		}

		if format == "widget" {
			return map[string]any{
				"data": response,
				"_meta": map[string]any{
					"ui": map[string]any{
						"resourceUri": knowledgeFeedResourceURI,
					},
				},
			}, nil
		}

		return response, nil
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
			"total":         len(allDocs),
			"by_type":       typeCounts,
			"by_visibility": visCounts,
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
