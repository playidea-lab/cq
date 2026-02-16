package knowledge

import (
	"encoding/json"
	"fmt"
	"regexp"
)

// CloudSyncer abstracts cloud knowledge operations.
// Implemented by cloud.KnowledgeCloudClient.
type CloudSyncer interface {
	SyncDocument(params map[string]any, docID string) error
	SearchDocuments(query string, docType string, limit int) ([]map[string]any, error)
	ListDocuments(docType string, limit int) ([]map[string]any, error)
	GetDocument(docID string) (map[string]any, error)
	DeleteDocument(docID string) error
	DiscoverPublic(query string, docType string, limit int) ([]map[string]any, error)
}

// PullResult holds the result of a cloud pull operation.
type PullResult struct {
	Pulled     int      `json:"pulled"`
	Updated    int      `json:"updated"`
	Skipped    int      `json:"skipped"`
	Deleted    int      `json:"deleted"`
	Errors     []string `json:"errors"`
	PulledIDs  []string `json:"pulled_ids"`
	UpdatedIDs []string `json:"updated_ids"`
	SkippedIDs []string `json:"skipped_ids"`
	DeletedIDs []string `json:"deleted_ids"`
}

// Pull downloads documents from cloud to local store.
// force=true overwrites existing local docs regardless of version.
func Pull(store *Store, cloud CloudSyncer, docType string, limit int, force bool) (*PullResult, error) {
	if cloud == nil {
		return nil, fmt.Errorf("cloud not configured")
	}
	if limit <= 0 {
		limit = 50
	}

	// 1. List cloud docs (lightweight — no body)
	cloudDocs, err := cloud.ListDocuments(docType, limit)
	if err != nil {
		return nil, fmt.Errorf("cloud list: %w", err)
	}

	result := &PullResult{}

	for _, cdoc := range cloudDocs {
		docID, _ := cdoc["doc_id"].(string)
		if docID == "" {
			continue
		}

		// 2. Check local existence
		localDoc, localErr := store.Get(docID)
		localExists := localErr == nil && localDoc != nil

		if localExists && !force {
			// Compare version: cloud newer → update, otherwise skip
			cloudVer := toFloat(cdoc["version"])
			localVer := float64(localDoc.Version)
			if cloudVer <= localVer {
				result.Skipped++
				result.SkippedIDs = append(result.SkippedIDs, docID)
				continue
			}
		}

		// 3. Fetch full doc from cloud (includes body)
		fullDoc, getErr := cloud.GetDocument(docID)
		if getErr != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", docID, getErr))
			continue
		}

		// 4. Extract fields
		dt, _ := fullDoc["doc_type"].(string)
		if dt == "" {
			dt = "experiment"
		}

		tags := extractTags(fullDoc["tags"])

		body, _ := fullDoc["body"].(string)
		title, _ := fullDoc["title"].(string)
		domain, _ := fullDoc["domain"].(string)

		metadata := map[string]any{
			"id":     docID,
			"title":  title,
			"domain": domain,
			"tags":   tags,
		}

		// 5. Create or update local doc
		if localExists {
			bodyPtr := &body
			_, updateErr := store.Update(docID, metadata, bodyPtr)
			if updateErr != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", docID, updateErr))
				continue
			}
			result.Updated++
			result.UpdatedIDs = append(result.UpdatedIDs, docID)
		} else {
			_, createErr := store.Create(DocumentType(dt), metadata, body)
			if createErr != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", docID, createErr))
				continue
			}
			result.Pulled++
			result.PulledIDs = append(result.PulledIDs, docID)
		}
	}

	// 6. Delete local docs that no longer exist in cloud.
	// Only when not filtering by docType, to avoid accidentally deleting
	// docs of other types that were simply excluded from the cloud query.
	if docType == "" {
		cloudIDs := make(map[string]bool, len(cloudDocs))
		for _, cd := range cloudDocs {
			if id, _ := cd["doc_id"].(string); id != "" {
				cloudIDs[id] = true
			}
		}

		localDocs, listErr := store.List("", "", limit)
		if listErr == nil {
			for _, ld := range localDocs {
				localID, _ := ld["id"].(string)
				if localID != "" && !cloudIDs[localID] {
					if _, delErr := store.Delete(localID); delErr == nil {
						result.Deleted++
						result.DeletedIDs = append(result.DeletedIDs, localID)
					} else {
						result.Errors = append(result.Errors, fmt.Sprintf("delete %s: %v", localID, delErr))
					}
				}
			}
		}
	}

	return result, nil
}

// SyncAfterRecord pushes a newly recorded document to cloud (async-safe).
func SyncAfterRecord(cloud CloudSyncer, params map[string]any, docID string) error {
	if cloud == nil {
		return nil
	}
	return cloud.SyncDocument(params, docID)
}

// SyncAfterUpdate pushes updated document fields to cloud.
func SyncAfterUpdate(cloud CloudSyncer, docID string, doc *Document) error {
	if cloud == nil || doc == nil {
		return nil
	}
	params := map[string]any{
		"doc_type":   string(doc.Type),
		"title":      doc.Title,
		"content":    doc.Body,
		"domain":     doc.Domain,
		"tags":       doc.Tags,
		"visibility": doc.Visibility,
	}
	return cloud.SyncDocument(params, docID)
}

// SyncAfterDelete soft-deletes a document in the cloud.
func SyncAfterDelete(cloud CloudSyncer, docID string) error {
	if cloud == nil {
		return nil
	}
	return cloud.DeleteDocument(docID)
}

// PushChanges pushes all local documents to cloud.
// Compares content_hash to avoid unnecessary uploads.
func PushChanges(store *Store, cloud CloudSyncer, limit int) (*PushResult, error) {
	if cloud == nil {
		return nil, fmt.Errorf("cloud not configured")
	}
	if limit <= 0 {
		limit = 100
	}

	// List local docs
	localDocs, err := store.List("", "", limit)
	if err != nil {
		return nil, fmt.Errorf("list local: %w", err)
	}

	// List cloud docs for hash comparison
	cloudDocs, err := cloud.ListDocuments("", limit)
	if err != nil {
		return nil, fmt.Errorf("cloud list: %w", err)
	}

	cloudHashes := make(map[string]string, len(cloudDocs))
	for _, cd := range cloudDocs {
		id, _ := cd["doc_id"].(string)
		hash, _ := cd["content_hash"].(string)
		if id != "" {
			cloudHashes[id] = hash
		}
	}

	result := &PushResult{}

	for _, ldoc := range localDocs {
		docID, _ := ldoc["id"].(string)
		if docID == "" {
			continue
		}

		// Read full doc for hash + body
		doc, err := store.Get(docID)
		if err != nil || doc == nil {
			continue
		}

		localHash := contentHash(docToMarkdown(doc))

		// Skip if cloud already has same content
		if cloudHash, exists := cloudHashes[docID]; exists && cloudHash == localHash {
			result.Skipped++
			continue
		}

		// Push to cloud
		params := map[string]any{
			"doc_type":   string(doc.Type),
			"title":      doc.Title,
			"content":    doc.Body,
			"domain":     doc.Domain,
			"tags":       doc.Tags,
			"visibility": doc.Visibility,
		}
		if err := cloud.SyncDocument(params, docID); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", docID, err))
			continue
		}
		result.Pushed++
		result.PushedIDs = append(result.PushedIDs, docID)
	}

	return result, nil
}

// PushResult holds the result of a push operation.
type PushResult struct {
	Pushed    int      `json:"pushed"`
	Skipped   int      `json:"skipped"`
	Errors    []string `json:"errors"`
	PushedIDs []string `json:"pushed_ids"`
}

// PublishDocument publishes a local document to the community pool.
// Strips project-specific metadata (task_id, file paths) and sets visibility=public.
func PublishDocument(store *Store, cloud CloudSyncer, docID string) error {
	if cloud == nil {
		return fmt.Errorf("cloud not configured")
	}

	doc, err := store.Get(docID)
	if err != nil || doc == nil {
		return fmt.Errorf("document not found: %s", docID)
	}

	// Strip project-specific metadata
	body := StripMetadata(doc.Body)

	params := map[string]any{
		"doc_type":   string(doc.Type),
		"title":      doc.Title,
		"content":    body,
		"domain":     doc.Domain,
		"tags":       doc.Tags,
		"visibility": "public",
		"metadata": map[string]any{
			"source": "community",
		},
	}

	// Push with community project context
	if err := cloud.SyncDocument(params, docID); err != nil {
		return fmt.Errorf("cloud publish: %w", err)
	}

	// Mark local document as published
	vis := "public"
	store.Update(docID, map[string]any{"visibility": vis}, nil)
	return nil
}

// Pre-compiled regexes for metadata stripping.
var (
	reTaskID    = regexp.MustCompile(`\b[TRC]P?-\d{1,4}(-\d+)?\b`)
	reCommitSHA = regexp.MustCompile(`\b[0-9a-f]{8,40}\b`)
	reAbsPath   = regexp.MustCompile(`/(?:Users|home|var|tmp|opt|etc)/[^\s,)]+`)
)

// StripMetadata removes project-specific identifiers from document body.
// Strips: task IDs (T-NNN, R-NNN), file paths, git SHAs (8+ hex chars).
func StripMetadata(body string) string {
	body = reTaskID.ReplaceAllString(body, "[task]")
	body = reCommitSHA.ReplaceAllString(body, "[commit]")
	body = reAbsPath.ReplaceAllString(body, "[path]")
	return body
}

// extractTags handles tags in various formats (JSON string, []any, []string).
func extractTags(v any) []string {
	if v == nil {
		return nil
	}
	switch t := v.(type) {
	case []string:
		return t
	case []any:
		var tags []string
		for _, item := range t {
			if s, ok := item.(string); ok {
				tags = append(tags, s)
			}
		}
		return tags
	case string:
		// JSON array string
		var tags []string
		if json.Unmarshal([]byte(t), &tags) == nil {
			return tags
		}
		return nil
	}
	return nil
}
