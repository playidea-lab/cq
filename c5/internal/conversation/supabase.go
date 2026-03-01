package conversation

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// SupabaseStore persists conversation history in a Supabase conversations table
// via the PostgREST REST API.
//
// Expected table schema:
//
//	create table conversations (
//	  id         uuid default gen_random_uuid() primary key,
//	  channel_id text      not null,
//	  platform   text      not null default '',
//	  project_id text      not null default '',
//	  role       text      not null,
//	  content    text      not null,
//	  created_at timestamptz not null default now()
//	);
type SupabaseStore struct {
	supabaseURL string
	supabaseKey string
	httpClient  *http.Client
}

// conversationRow is the JSON row shape for both insert and select.
type conversationRow struct {
	ChannelID string `json:"channel_id"`
	Platform  string `json:"platform,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
	Role      string `json:"role"`
	Content   string `json:"content"`
}

// NewSupabaseStore creates a SupabaseStore. Returns nil if supabaseURL or
// supabaseKey is empty — callers should fall back to MemoryStore in that case.
func NewSupabaseStore(supabaseURL, supabaseKey string) *SupabaseStore {
	if supabaseURL == "" || supabaseKey == "" {
		return nil
	}
	return &SupabaseStore{
		supabaseURL: supabaseURL,
		supabaseKey: supabaseKey,
		httpClient:  &http.Client{Timeout: 10 * time.Second},
	}
}

// Get fetches the most recent limit messages for channelID, oldest-first.
func (s *SupabaseStore) Get(ctx context.Context, channelID string, limit int) ([]Message, error) {
	params := url.Values{
		"channel_id": {"eq." + channelID},
		"order":      {"created_at.desc"},
		"select":     {"role,content"},
	}
	if limit > 0 {
		params.Set("limit", strconv.Itoa(limit))
	}
	endpoint := s.supabaseURL + "/rest/v1/conversations?" + params.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("conversation: supabase: create request: %w", err)
	}
	s.setHeaders(req)
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("conversation: supabase: http: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20)) // 4 MiB
	if err != nil {
		return nil, fmt.Errorf("conversation: supabase: read body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("conversation: supabase: unexpected status %d: %s", resp.StatusCode, string(body))
	}

	var rows []conversationRow
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, fmt.Errorf("conversation: supabase: decode: %w", err)
	}

	// Rows arrived newest-first; reverse to oldest-first for LLM context.
	for i, j := 0, len(rows)-1; i < j; i, j = i+1, j-1 {
		rows[i], rows[j] = rows[j], rows[i]
	}

	msgs := make([]Message, len(rows))
	for i, r := range rows {
		msgs[i] = Message{Role: r.Role, Content: r.Content}
	}
	return msgs, nil
}

// Append inserts msgs into the conversations table.
func (s *SupabaseStore) Append(ctx context.Context, channelID, platform, projectID string, msgs []Message) error {
	if len(msgs) == 0 {
		return nil
	}
	rows := make([]conversationRow, len(msgs))
	for i, m := range msgs {
		rows[i] = conversationRow{
			ChannelID: channelID,
			Platform:  platform,
			ProjectID: projectID,
			Role:      m.Role,
			Content:   m.Content,
		}
	}

	body, err := json.Marshal(rows)
	if err != nil {
		return fmt.Errorf("conversation: supabase: marshal: %w", err)
	}

	endpoint := s.supabaseURL + "/rest/v1/conversations"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("conversation: supabase: create request: %w", err)
	}
	s.setHeaders(req)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Prefer", "return=minimal")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("conversation: supabase: http: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body) //nolint:errcheck

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("conversation: supabase: insert status %d", resp.StatusCode)
	}
	return nil
}

// Compile-time interface assertion.
var _ Store = (*SupabaseStore)(nil)

// Cleanup is a no-op for SupabaseStore; expiry is handled by database policies.
func (s *SupabaseStore) Cleanup() {}

func (s *SupabaseStore) setHeaders(req *http.Request) {
	req.Header.Set("apikey", s.supabaseKey)
	req.Header.Set("Authorization", "Bearer "+s.supabaseKey)
}
