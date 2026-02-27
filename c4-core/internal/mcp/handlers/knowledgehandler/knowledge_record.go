package knowledgehandler

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/changmin/c4-core/internal/knowledge"
	"github.com/changmin/c4-core/internal/mcp"
)

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

		// Async cloud push — include embedding if available so Supabase pgvector is populated.
		if opts.Cloud != nil {
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
