package ontology

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// HubDownloader fetches collective ontology patterns from the Supabase
// collective_patterns table and merges them into a user's L1 ontology.
type HubDownloader struct {
	baseURL    string
	apiKey     string
	tokenFn    func() string
	httpClient *http.Client
}

// NewHubDownloader creates a HubDownloader.
func NewHubDownloader(baseURL, apiKey string, tokenFn func() string) *HubDownloader {
	return &HubDownloader{
		baseURL:    strings.TrimRight(baseURL, "/"),
		apiKey:     apiKey,
		tokenFn:    tokenFn,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// Download fetches top-50 collective patterns for the given domain,
// ordered by frequency descending, confidence HIGH first.
func (d *HubDownloader) Download(domain string) ([]collectivePatternRow, error) {
	if domain == "" {
		return nil, nil
	}

	// PostgREST query: filter by domain, order by frequency desc, limit 50
	url := fmt.Sprintf("%s/rest/v1/collective_patterns?domain=eq.%s&order=frequency.desc&limit=50",
		d.baseURL, domain)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("apikey", d.apiKey)
	req.Header.Set("Authorization", "Bearer "+d.tokenFn())
	req.Header.Set("Accept", "application/json")

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("hub download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("hub download HTTP %d: %s", resp.StatusCode, string(body))
	}

	var rows []collectivePatternRow
	if err := json.NewDecoder(resp.Body).Decode(&rows); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return rows, nil
}

// SeedFromHub downloads collective patterns and merges them into the user's L1 ontology.
// Existing nodes are not overwritten (merge via Updater.AddOrUpdate).
// Returns the number of nodes added.
func (d *HubDownloader) SeedFromHub(username, domain string) (int, error) {
	if username == "" || username == GlobalUsername {
		return 0, nil
	}

	rows, err := d.Download(domain)
	if err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 0, nil
	}

	o, err := Load(username)
	if err != nil {
		return 0, fmt.Errorf("load ontology: %w", err)
	}

	updater := NewUpdater(o)
	added := 0
	for _, r := range rows {
		node := Node{
			Label:          r.Path,
			Description:    r.Value,
			Tags:           r.Tags,
			Frequency:      r.Frequency,
			NodeConfidence: Confidence(r.Confidence),
			Scope:          "collective",
			SourceRole:     "hub",
		}
		updater.AddOrUpdate(r.Path, node)
		added++
	}

	if err := Save(username, o); err != nil {
		return 0, fmt.Errorf("save ontology: %w", err)
	}
	return added, nil
}
