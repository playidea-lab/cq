// Package knowledge provides a client for querying project knowledge via Supabase PostgREST.
package knowledge

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const maxBodyRunes = 2000

// SearchResult holds a single knowledge document returned by a search query.
type SearchResult struct {
	DocID  string `json:"doc_id"`
	Title  string `json:"title"`
	Domain string `json:"domain"`
	Body   string `json:"body"`
}

// Client queries the Supabase PostgREST API for c4_documents.
type Client struct {
	supabaseURL string
	supabaseKey string
	httpClient  *http.Client
}

// New creates a Client. supabaseURL is the project URL (e.g. "https://xxx.supabase.co")
// and supabaseKey is the service-role key.
func New(supabaseURL, supabaseKey string) *Client {
	return &Client{
		supabaseURL: supabaseURL,
		supabaseKey: supabaseKey,
		httpClient:  &http.Client{Timeout: 10 * time.Second},
	}
}

// Search queries c4_documents for documents matching query within the given projectID.
// Returns at most limit results. Body text is truncated to maxBodyRunes runes to
// conserve LLM context. Returns nil, nil when the client is not configured.
func (c *Client) Search(ctx context.Context, projectID, query string, limit int) ([]SearchResult, error) {
	if c.supabaseURL == "" || c.supabaseKey == "" {
		return nil, nil
	}

	// Truncate query to 100 runes to limit PostgREST filter injection surface.
	if utf8.RuneCountInString(query) > 100 {
		query = string([]rune(query)[:100])
	}
	// Strip PostgREST or-filter metacharacters to prevent filter injection.
	query = strings.NewReplacer("(", "", ")", "", ",", " ", "*", "").Replace(query)
	orFilter := fmt.Sprintf("(title.ilike.*%s*,body.ilike.*%s*)", query, query)
	params := url.Values{
		"project_id": {"eq." + projectID},
		"deleted_at": {"is.null"},
		"or":         {orFilter},
		"select":     {"doc_id,title,domain,body"},
		"order":      {"updated_at.desc"},
		"limit":      {strconv.Itoa(limit)},
	}
	endpoint := c.supabaseURL + "/rest/v1/c4_documents?" + params.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("knowledge: create request: %w", err)
	}
	req.Header.Set("apikey", c.supabaseKey)
	req.Header.Set("Authorization", "Bearer "+c.supabaseKey)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("knowledge: http: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<22)) // 4 MiB safety cap
	if err != nil {
		return nil, fmt.Errorf("knowledge: read body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("knowledge: unexpected status %d: %s", resp.StatusCode, string(body))
	}

	var results []SearchResult
	if err := json.Unmarshal(body, &results); err != nil {
		return nil, fmt.Errorf("knowledge: decode response: %w", err)
	}

	// Truncate body to maxBodyRunes runes for LLM context efficiency.
	for i := range results {
		if utf8.RuneCountInString(results[i].Body) > maxBodyRunes {
			runes := []rune(results[i].Body)
			results[i].Body = string(runes[:maxBodyRunes])
		}
	}

	return results, nil
}
