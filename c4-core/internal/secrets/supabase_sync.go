package secrets

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// SupabaseSync implements CloudSyncer against the c4_secrets PostgREST table.
// Secrets are stored as plaintext values (the caller — secrets.Store — is
// responsible for encryption at rest in the local SQLite layer).
// The Supabase row contains the raw plaintext so that different devices sharing
// the same Supabase project can sync without sharing a master key.
//
// Table schema (migration 00032_c4_secrets.sql):
//
//	c4_secrets(project_id TEXT, key TEXT, ciphertext TEXT, nonce TEXT, updated_at BIGINT)
//	PRIMARY KEY (project_id, key)
//
// For this sync layer we repurpose "ciphertext" as the plaintext value and
// "nonce" as an empty string sentinel, keeping the schema flexible for future
// server-side encryption.
type SupabaseSync struct {
	baseURL    string // e.g. "https://abc.supabase.co"
	anonKey    string
	httpClient *http.Client
}

// NewSupabaseSync creates a SupabaseSync. baseURL is the Supabase project URL.
func NewSupabaseSync(baseURL, anonKey string) *SupabaseSync {
	return &SupabaseSync{
		baseURL:    strings.TrimRight(baseURL, "/"),
		anonKey:    anonKey,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// secretRow mirrors the c4_secrets table columns used for sync.
type secretRow struct {
	ProjectID  string `json:"project_id"`
	Key        string `json:"key"`
	Ciphertext string `json:"ciphertext"` // stores plaintext value for now
	Nonce      string `json:"nonce"`
	UpdatedAt  int64  `json:"updated_at"`
}

func (s *SupabaseSync) restURL(path string) string {
	return s.baseURL + "/rest/v1/" + path
}

func (s *SupabaseSync) do(ctx context.Context, method, url string, body []byte, extraHeaders map[string]string) ([]byte, int, error) {
	var r io.Reader
	if body != nil {
		r = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, r)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("apikey", s.anonKey)
	req.Header.Set("Authorization", "Bearer "+s.anonKey)
	for k, v := range extraHeaders {
		req.Header.Set(k, v)
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1MB limit
	return data, resp.StatusCode, err
}

// Set upserts a secret value into Supabase.
func (s *SupabaseSync) Set(ctx context.Context, projectID, key, value string) error {
	row := secretRow{
		ProjectID:  projectID,
		Key:        key,
		Ciphertext: value,
		Nonce:      "",
		UpdatedAt:  time.Now().Unix(),
	}
	body, err := json.Marshal(row)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	headers := map[string]string{
		"Prefer": "resolution=merge-duplicates",
	}
	data, status, err := s.do(ctx, http.MethodPost, s.restURL("c4_secrets"), body, headers)
	if err != nil {
		return fmt.Errorf("supabase set: %w", err)
	}
	if status >= 300 {
		return fmt.Errorf("supabase set: status %d: %s", status, string(data))
	}
	return nil
}

// Get fetches a secret value from Supabase.
func (s *SupabaseSync) Get(ctx context.Context, projectID, key string) (string, error) {
	u := fmt.Sprintf("%s?project_id=eq.%s&key=eq.%s&select=ciphertext&limit=1",
		s.restURL("c4_secrets"), projectID, key)
	data, status, err := s.do(ctx, http.MethodGet, u, nil, map[string]string{"Accept": "application/json"})
	if err != nil {
		return "", fmt.Errorf("supabase get: %w", err)
	}
	if status >= 300 {
		return "", fmt.Errorf("supabase get: status %d: %s", status, string(data))
	}
	var rows []struct {
		Ciphertext string `json:"ciphertext"`
	}
	if err := json.Unmarshal(data, &rows); err != nil {
		return "", fmt.Errorf("supabase get: decode: %w", err)
	}
	if len(rows) == 0 {
		return "", ErrNotFound
	}
	return rows[0].Ciphertext, nil
}

// ListKeys returns all secret keys for the given project.
func (s *SupabaseSync) ListKeys(ctx context.Context, projectID string) ([]string, error) {
	u := fmt.Sprintf("%s?project_id=eq.%s&select=key&order=key",
		s.restURL("c4_secrets"), projectID)
	data, status, err := s.do(ctx, http.MethodGet, u, nil, map[string]string{"Accept": "application/json"})
	if err != nil {
		return nil, fmt.Errorf("supabase list: %w", err)
	}
	if status >= 300 {
		return nil, fmt.Errorf("supabase list: status %d: %s", status, string(data))
	}
	var rows []struct {
		Key string `json:"key"`
	}
	if err := json.Unmarshal(data, &rows); err != nil {
		return nil, fmt.Errorf("supabase list: decode: %w", err)
	}
	keys := make([]string, len(rows))
	for i, r := range rows {
		keys[i] = r.Key
	}
	return keys, nil
}

// Delete removes a secret from Supabase.
func (s *SupabaseSync) Delete(ctx context.Context, projectID, key string) error {
	u := fmt.Sprintf("%s?project_id=eq.%s&key=eq.%s",
		s.restURL("c4_secrets"), projectID, key)
	data, status, err := s.do(ctx, http.MethodDelete, u, nil, nil)
	if err != nil {
		return fmt.Errorf("supabase delete: %w", err)
	}
	if status >= 300 {
		return fmt.Errorf("supabase delete: status %d: %s", status, string(data))
	}
	return nil
}

// compile-time assertion: SupabaseSync must implement CloudSyncer.
var _ CloudSyncer = (*SupabaseSync)(nil)

// ErrSupabaseNotConfigured is returned when Supabase URL or key is empty.
var ErrSupabaseNotConfigured = errors.New("supabase not configured (missing url or anon key)")

// NewSupabaseSyncFromConfig creates a SupabaseSync if url and key are non-empty,
// or returns (nil, ErrSupabaseNotConfigured) for graceful local-only fallback.
func NewSupabaseSyncFromConfig(supabaseURL, anonKey string) (*SupabaseSync, error) {
	if supabaseURL == "" || anonKey == "" {
		return nil, ErrSupabaseNotConfigured
	}
	return NewSupabaseSync(supabaseURL, anonKey), nil
}
