package knowledge

import (
	"encoding/json"
	"fmt"
)

// CloudSyncer abstracts cloud knowledge operations.
// Implemented by cloud.KnowledgeCloudClient.
type CloudSyncer interface {
	SyncDocument(params map[string]any, docID string) error
	SearchDocuments(query string, docType string, limit int) ([]map[string]any, error)
	ListDocuments(docType string, limit int) ([]map[string]any, error)
	GetDocument(docID string) (map[string]any, error)
}

// PullResult holds the result of a cloud pull operation.
type PullResult struct {
	Pulled     int      `json:"pulled"`
	Updated    int      `json:"updated"`
	Skipped    int      `json:"skipped"`
	Errors     []string `json:"errors"`
	PulledIDs  []string `json:"pulled_ids"`
	UpdatedIDs []string `json:"updated_ids"`
	SkippedIDs []string `json:"skipped_ids"`
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

	return result, nil
}

// SyncAfterRecord pushes a newly recorded document to cloud (async-safe).
func SyncAfterRecord(cloud CloudSyncer, params map[string]any, docID string) error {
	if cloud == nil {
		return nil
	}
	return cloud.SyncDocument(params, docID)
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
