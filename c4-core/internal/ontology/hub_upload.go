package ontology

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// pendingUploadFile is the path (relative to project root) where failed
// uploads are queued for later retry.
const pendingUploadFile = ".c4/pending-uploads.json"

// collectivePatternRow maps to the collective_patterns Supabase table.
// The unique constraint is (domain, path, value).
type collectivePatternRow struct {
	Domain           string   `json:"domain"`
	Path             string   `json:"path"`
	Value            string   `json:"value"`
	Frequency        int      `json:"frequency"`
	Confidence       string   `json:"confidence"`
	Tags             []string `json:"tags"`
	ContributorCount int      `json:"contributor_count"`
}

// HubUploader uploads anonymized ontology patterns to the Supabase
// collective_patterns table using the PostgREST REST API.
//
// Authentication uses a static API key (anon key) plus a Bearer token
// supplied via tokenFn. This mirrors the pattern in cloud.KnowledgeCloudClient.
type HubUploader struct {
	baseURL    string // Supabase PostgREST base URL (e.g. https://xxx.supabase.co/rest/v1)
	apiKey     string // Supabase anon key
	tokenFn    func() string
	projectRoot string // local project root for pending queue
	httpClient *http.Client
}

// NewHubUploader creates a HubUploader.
// tokenFn returns the current Bearer token (can be a static closure).
// projectRoot is used to locate .c4/pending-uploads.json.
func NewHubUploader(baseURL, apiKey string, tokenFn func() string, projectRoot string) *HubUploader {
	return &HubUploader{
		baseURL:     strings.TrimRight(baseURL, "/"),
		apiKey:      apiKey,
		tokenFn:     tokenFn,
		projectRoot: projectRoot,
		httpClient:  &http.Client{Timeout: 15 * time.Second},
	}
}

// Upload sends all patterns to the Hub. Patterns that fail to upload are
// appended to .c4/pending-uploads.json for later retry.
//
// Returns the number of patterns successfully uploaded.
func (u *HubUploader) Upload(patterns []AnonPattern) (int, error) {
	if len(patterns) == 0 {
		return 0, nil
	}

	var (
		uploaded int
		failed   []AnonPattern
	)

	for _, p := range patterns {
		if err := u.upsert(p); err != nil {
			failed = append(failed, p)
		} else {
			uploaded++
		}
	}

	if len(failed) > 0 {
		if qErr := u.enqueue(failed); qErr != nil {
			return uploaded, fmt.Errorf("upload partial (%d failed); enqueue error: %w", len(failed), qErr)
		}
		return uploaded, fmt.Errorf("%d pattern(s) failed upload and queued to %s", len(failed), pendingUploadFile)
	}

	return uploaded, nil
}

// upsert posts a single pattern row to collective_patterns with merge-duplicate
// resolution on (domain, path, value).
func (u *HubUploader) upsert(p AnonPattern) error {
	tags := p.Tags
	if tags == nil {
		tags = []string{}
	}
	row := collectivePatternRow{
		Domain:           p.Domain,
		Path:             p.Path,
		Value:            p.Value,
		Frequency:        p.Frequency,
		Confidence:       p.Confidence,
		Tags:             tags,
		ContributorCount: 1,
	}

	data, err := json.Marshal(row)
	if err != nil {
		return fmt.Errorf("marshal pattern: %w", err)
	}

	req, err := http.NewRequest("POST", u.baseURL+"/rest/v1/collective_patterns", strings.NewReader(string(data)))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("apikey", u.apiKey)
	req.Header.Set("Authorization", "Bearer "+u.tokenFn())
	// On conflict (domain,path,value): increment frequency and contributor_count.
	req.Header.Set("Prefer", "return=minimal,resolution=merge-duplicates")

	resp, err := u.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("POST collective_patterns: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("POST collective_patterns: %d %s", resp.StatusCode, string(body))
	}
	return nil
}

// enqueue appends failed patterns to .c4/pending-uploads.json.
// Existing entries are preserved; new entries are appended.
func (u *HubUploader) enqueue(patterns []AnonPattern) error {
	queuePath := filepath.Join(u.projectRoot, pendingUploadFile)

	if err := os.MkdirAll(filepath.Dir(queuePath), 0755); err != nil {
		return fmt.Errorf("create pending dir: %w", err)
	}

	existing := readPendingQueue(queuePath)
	existing = append(existing, patterns...)

	data, err := json.MarshalIndent(existing, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal pending queue: %w", err)
	}
	return os.WriteFile(queuePath, data, 0644)
}

// readPendingQueue reads the existing pending queue. Returns nil on error or
// if the file does not exist.
func readPendingQueue(path string) []AnonPattern {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var q []AnonPattern
	if err := json.Unmarshal(data, &q); err != nil {
		return nil
	}
	return q
}
