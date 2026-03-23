package cloud

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/changmin/c4-core/internal/knowledge"
)

var _ knowledge.CloudSyncer = (*KnowledgeCloudClient)(nil)

// KnowledgeCloudClient handles cloud sync for knowledge documents.
// Uses the same PostgREST REST API pattern as CloudStore.
type KnowledgeCloudClient struct {
	baseURL       string // Supabase PostgREST URL
	apiKey        string
	tokenProvider *TokenProvider
	projectID     string
	httpClient    *http.Client
}

// cloudDocRow maps to the c4_documents Supabase table.
type cloudDocRow struct {
	DocID           string    `json:"doc_id"`
	ProjectID       string    `json:"project_id,omitempty"`
	DocType         string    `json:"doc_type"`
	Title           string    `json:"title"`
	Domain          string    `json:"domain"`
	Tags            string    `json:"tags"`                        // JSON array string
	Body            string    `json:"body"`
	Metadata        string    `json:"metadata"`                    // JSON object string
	ContentHash     string    `json:"content_hash"`
	Version         int       `json:"version"`
	CreatedBy       string    `json:"created_by"`
	Visibility      string    `json:"visibility,omitempty"`        // private, team, public
	CreatedByUserID string    `json:"created_by_user_id,omitempty"`
	CreatedAt       string    `json:"created_at,omitempty"`
	UpdatedAt       string    `json:"updated_at,omitempty"`
	Embedding       []float32 `json:"embedding,omitempty"` // pgvector: uploaded when available
}

// NewKnowledgeCloudClient creates a new knowledge cloud sync client.
func NewKnowledgeCloudClient(baseURL, apiKey string, tp *TokenProvider, projectID string) *KnowledgeCloudClient {
	return &KnowledgeCloudClient{
		baseURL:       strings.TrimRight(baseURL, "/"),
		apiKey:        apiKey,
		tokenProvider: tp,
		projectID:     projectID,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

// SyncDocument upserts a knowledge document to the cloud.
// Extracts doc_type, title, content, tags, domain from MCP tool params.
func (k *KnowledgeCloudClient) SyncDocument(params map[string]any, docID string) error {
	if docID == "" {
		return fmt.Errorf("empty doc_id")
	}

	docType, _ := params["doc_type"].(string)
	title, _ := params["title"].(string)
	domain, _ := params["domain"].(string)

	// Content field: MCP schema uses "content", Python uses "body"
	body, _ := params["content"].(string)
	if body == "" {
		body, _ = params["body"].(string)
	}

	// Tags: may be []any from JSON
	tagsJSON := "[]"
	if rawTags, ok := params["tags"]; ok {
		if b, err := json.Marshal(rawTags); err == nil {
			tagsJSON = string(b)
		}
	}

	// Metadata: optional extra fields
	metadataJSON := "{}"
	if rawMeta, ok := params["metadata"]; ok {
		if b, err := json.Marshal(rawMeta); err == nil {
			metadataJSON = string(b)
		}
	}

	// Content hash
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(body)))[:16]

	visibility, _ := params["visibility"].(string)
	if visibility == "" {
		visibility = "team"
	}

	// Version: use provided value (from SyncAfterUpdate) or default to 1 (new docs)
	version := 1
	if v, ok := params["version"]; ok {
		switch vv := v.(type) {
		case int:
			version = vv
		case float64:
			version = int(vv)
		}
	}

	// Embedding: passed via params["_embedding"] by SyncAfterRecord when available.
	// pgvector accepts JSON float32 array directly via PostgREST.
	var embedding []float32
	if raw, ok := params["_embedding"]; ok {
		switch v := raw.(type) {
		case []float32:
			embedding = v
		case []any:
			embedding = make([]float32, len(v))
			for i, f := range v {
				if fv, ok2 := f.(float64); ok2 {
					embedding[i] = float32(fv)
				}
			}
		}
	}

	row := cloudDocRow{
		DocID:       docID,
		ProjectID:   k.projectID,
		DocType:     docType,
		Title:       title,
		Domain:      domain,
		Tags:        tagsJSON,
		Body:        body,
		Metadata:    metadataJSON,
		ContentHash: hash,
		Version:     version,
		Visibility:  visibility,
		CreatedBy:   "",
		Embedding:   embedding,
	}

	return k.post("c4_documents", row)
}

// SearchDocuments performs a full-text search on cloud knowledge documents.
func (k *KnowledgeCloudClient) SearchDocuments(query string, docType string, limit int) ([]map[string]any, error) {
	if limit <= 0 {
		limit = 10
	}

	filter := "project_id=eq." + url.QueryEscape(k.projectID) + "&deleted_at=is.null"
	if query != "" {
		// PostgreSQL tsvector FTS via PostgREST
		filter += "&tsv=fts.english." + url.QueryEscape(toTSQuery(query))
	}
	if docType != "" {
		filter += "&doc_type=eq." + url.QueryEscape(docType)
	}
	filter += fmt.Sprintf("&order=updated_at.desc&limit=%d", limit)
	filter += "&select=doc_id,doc_type,title,domain,tags,visibility,content_hash,version,created_at,updated_at"

	var rows []map[string]any
	if err := k.get("c4_documents", filter, &rows); err != nil {
		return nil, err
	}
	return rows, nil
}

// GetDocument fetches a single knowledge document from the cloud.
func (k *KnowledgeCloudClient) GetDocument(docID string) (map[string]any, error) {
	filter := "project_id=eq." + url.QueryEscape(k.projectID) + "&doc_id=eq." + url.QueryEscape(docID)

	var rows []map[string]any
	if err := k.get("c4_documents", filter, &rows); err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("document not found: %s", docID)
	}
	return rows[0], nil
}

// ListDocuments lists knowledge documents with optional type filter.
func (k *KnowledgeCloudClient) ListDocuments(docType string, limit int) ([]map[string]any, error) {
	if limit <= 0 {
		limit = 50
	}

	filter := "project_id=eq." + url.QueryEscape(k.projectID) + "&deleted_at=is.null"
	if docType != "" {
		filter += "&doc_type=eq." + url.QueryEscape(docType)
	}
	filter += fmt.Sprintf("&order=updated_at.desc&limit=%d", limit)
	filter += "&select=doc_id,doc_type,title,domain,tags,visibility,version,content_hash,created_at,updated_at"

	var rows []map[string]any
	if err := k.get("c4_documents", filter, &rows); err != nil {
		return nil, err
	}
	return rows, nil
}

// DeleteDocument soft-deletes a knowledge document from the cloud (sets deleted_at).
func (k *KnowledgeCloudClient) DeleteDocument(docID string) error {
	filter := "project_id=eq." + url.QueryEscape(k.projectID) + "&doc_id=eq." + url.QueryEscape(docID)
	patch := map[string]any{
		"deleted_at": time.Now().UTC().Format(time.RFC3339),
	}
	return k.patch("c4_documents", filter, patch)
}

// UpdateDocument updates a knowledge document in the cloud.
func (k *KnowledgeCloudClient) UpdateDocument(docID string, updates map[string]any) error {
	filter := "project_id=eq." + url.QueryEscape(k.projectID) + "&doc_id=eq." + url.QueryEscape(docID)
	return k.patch("c4_documents", filter, updates)
}

// DiscoverPublic searches for public documents across all projects (no project_id filter).
// Used for cross-project knowledge discovery.
func (k *KnowledgeCloudClient) DiscoverPublic(query string, docType string, limit int) ([]map[string]any, error) {
	if limit <= 0 {
		limit = 10
	}

	filter := "visibility=eq.public&deleted_at=is.null"
	if query != "" {
		filter += "&tsv=fts.english." + url.QueryEscape(toTSQuery(query))
	}
	if docType != "" {
		filter += "&doc_type=eq." + url.QueryEscape(docType)
	}
	filter += fmt.Sprintf("&order=updated_at.desc&limit=%d", limit)
	filter += "&select=doc_id,project_id,doc_type,title,domain,tags,visibility,content_hash,version,created_at,updated_at"

	var rows []map[string]any
	if err := k.get("c4_documents", filter, &rows); err != nil {
		return nil, err
	}
	return rows, nil
}

// SemanticSearch calls the c4_knowledge_search_semantic PostgreSQL RPC via PostgREST.
// embedding must be a non-nil float32 slice matching the pgvector dimension.
// Falls back to empty result (not error) so callers can fall back to local search.
func (k *KnowledgeCloudClient) SemanticSearch(embedding []float32, limit int, similarityThreshold float32) ([]map[string]any, error) {
	if len(embedding) == 0 {
		return nil, fmt.Errorf("embedding must be non-empty")
	}
	if limit <= 0 {
		limit = 10
	}
	if similarityThreshold <= 0 {
		similarityThreshold = 0.5
	}

	payload := map[string]any{
		"query_embedding":      embedding,
		"match_count":          limit,
		"similarity_threshold": similarityThreshold,
		"filter_project_id":    k.projectID,
		"filter_visibility":    "team",
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("semantic search marshal: %w", err)
	}

	rpcURL := k.baseURL + "/rpc/c4_knowledge_search_semantic"

	var results []map[string]any
	for attempt := 0; attempt < 2; attempt++ {
		req, err := http.NewRequest("POST", rpcURL, strings.NewReader(string(data)))
		if err != nil {
			return nil, err
		}
		k.setHeaders(req)

		resp, err := k.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("semantic search RPC: %w", err)
		}

		if resp.StatusCode == http.StatusUnauthorized && attempt == 0 {
			resp.Body.Close()
			if _, err := k.tokenProvider.Refresh(); err == nil {
				continue
			}
		}

		defer resp.Body.Close()

		if resp.StatusCode >= 400 {
			body, _ := io.ReadAll(resp.Body)
			return nil, fmt.Errorf("semantic search RPC: %d %s", resp.StatusCode, string(body))
		}

		if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
			return nil, fmt.Errorf("semantic search decode: %w", err)
		}
		return results, nil
	}
	return results, nil
}

// toTSQuery converts a multi-word query into PostgreSQL tsquery format.
// "embedding search" → "embedding & search" (AND).
func toTSQuery(q string) string {
	words := strings.Fields(q)
	if len(words) <= 1 {
		return q
	}
	return strings.Join(words, " & ")
}

// =========================================================================
// PostgREST HTTP helpers (same pattern as CloudStore)
// =========================================================================

func (k *KnowledgeCloudClient) get(table, filter string, dest any) error {
	reqURL := k.baseURL + "/" + table
	if filter != "" {
		reqURL += "?" + filter
	}

	for attempt := 0; attempt < 2; attempt++ {
		req, err := http.NewRequest("GET", reqURL, nil)
		if err != nil {
			return err
		}
		k.setHeaders(req)

		resp, err := k.httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("GET %s: %w", table, err)
		}

		if resp.StatusCode == http.StatusUnauthorized && attempt == 0 {
			resp.Body.Close()
			if _, err := k.tokenProvider.Refresh(); err == nil {
				continue
			}
		}

		defer resp.Body.Close()

		if resp.StatusCode >= 400 {
			body, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("GET %s: %d %s", table, resp.StatusCode, string(body))
		}

		if dest != nil {
			if err := json.NewDecoder(resp.Body).Decode(dest); err != nil {
				return fmt.Errorf("decode %s: %w", table, err)
			}
		}
		return nil
	}
	return nil
}

func (k *KnowledgeCloudClient) post(table string, body any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	for attempt := 0; attempt < 2; attempt++ {
		req, err := http.NewRequest("POST", k.baseURL+"/"+table, strings.NewReader(string(data)))
		if err != nil {
			return err
		}
		k.setHeaders(req)
		req.Header.Set("Prefer", "return=minimal,resolution=merge-duplicates")

		resp, err := k.httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("POST %s: %w", table, err)
		}

		if resp.StatusCode == http.StatusUnauthorized && attempt == 0 {
			resp.Body.Close()
			if _, err := k.tokenProvider.Refresh(); err == nil {
				continue
			}
		}

		defer resp.Body.Close()

		if resp.StatusCode >= 400 {
			respBody, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("POST %s: %d %s", table, resp.StatusCode, string(respBody))
		}
		return nil
	}
	return nil
}

func (k *KnowledgeCloudClient) patch(table, filter string, body any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	reqURL := k.baseURL + "/" + table + "?" + filter
	for attempt := 0; attempt < 2; attempt++ {
		req, err := http.NewRequest("PATCH", reqURL, strings.NewReader(string(data)))
		if err != nil {
			return err
		}
		k.setHeaders(req)
		req.Header.Set("Prefer", "return=minimal")

		resp, err := k.httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("PATCH %s: %w", table, err)
		}

		if resp.StatusCode == http.StatusUnauthorized && attempt == 0 {
			resp.Body.Close()
			if _, err := k.tokenProvider.Refresh(); err == nil {
				continue
			}
		}

		defer resp.Body.Close()

		if resp.StatusCode >= 400 {
			respBody, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("PATCH %s: %d %s", table, resp.StatusCode, string(respBody))
		}
		return nil
	}
	return nil
}

func (k *KnowledgeCloudClient) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("apikey", k.apiKey)
	req.Header.Set("Authorization", "Bearer "+k.tokenProvider.Token())
}
