// Package c1push provides a thin PostgREST HTTP client for pushing messages
// to c1_channels and c1_messages tables in Supabase.
// It is intentionally standalone (~50 lines) to avoid importing c5/ (separate Go module).
package c1push

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// Platform identifies the source platform for a channel.
type Platform string

const (
	PlatformClaudeCode Platform = "claude_code"
	PlatformCodexCLI   Platform = "codex_cli" // Phase 2
	PlatformCursor     Platform = "cursor"     // Phase 2
)

// PushMessage is a single message to be appended to a channel.
type PushMessage struct {
	SenderName string `json:"sender_name"`
	SenderType string `json:"sender_type"`
	Content    string `json:"content"`
}

// Pusher is a PostgREST client for c1_channels and c1_messages.
type Pusher struct {
	supabaseURL string
	anonKey     string
	httpClient  *http.Client
}

// New creates a Pusher. Returns nil if supabaseURL or anonKey is empty.
func New(supabaseURL, anonKey string) *Pusher {
	if supabaseURL == "" || anonKey == "" {
		return nil
	}
	return &Pusher{
		supabaseURL: supabaseURL,
		anonKey:     anonKey,
		httpClient:  &http.Client{Timeout: 10 * time.Second},
	}
}

// EnsureChannel creates or retrieves a channel by (tenant_id, platform, name).
// Returns the channel UUID. Uses resolution=ignore-duplicates + fallback GET for idempotency.
func (p *Pusher) EnsureChannel(ctx context.Context, tenantID, projectID, name string, platform Platform) (string, error) {
	if tenantID == "" {
		tenantID = "default"
	}

	// Try POST with ignore-duplicates first.
	row := map[string]string{
		"tenant_id":    tenantID,
		"name":         name,
		"channel_type": "session",
		"platform":     string(platform),
	}
	if projectID != "" {
		row["project_id"] = projectID
	}
	body, _ := json.Marshal([]map[string]string{row})

	endpoint := p.supabaseURL + "/rest/v1/c1_channels"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("c1push: ensure channel: %w", err)
	}
	p.setHeaders(req)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Prefer", "return=representation,resolution=ignore-duplicates")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("c1push: ensure channel: http: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	if resp.StatusCode == http.StatusConflict || (resp.StatusCode >= 200 && resp.StatusCode < 300 && len(respBody) <= 2) {
		// 409 or empty 2xx (ON CONFLICT DO NOTHING) — fall back to GET.
		return p.findChannel(ctx, tenantID, string(platform), name)
	}
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("c1push: ensure channel: server error %d: %s", resp.StatusCode, string(respBody))
	}

	var rows []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(respBody, &rows); err == nil && len(rows) > 0 {
		return rows[0].ID, nil
	}
	return p.findChannel(ctx, tenantID, string(platform), name)
}

// AppendMessages inserts msgs into c1_messages for the given channel UUID.
func (p *Pusher) AppendMessages(ctx context.Context, channelID string, msgs []PushMessage) error {
	if len(msgs) == 0 {
		return nil
	}
	type row struct {
		ChannelID  string `json:"channel_id"`
		SenderName string `json:"sender_name"`
		SenderType string `json:"sender_type"`
		Content    string `json:"content"`
	}
	rows := make([]row, len(msgs))
	for i, m := range msgs {
		rows[i] = row{
			ChannelID:  channelID,
			SenderName: m.SenderName,
			SenderType: m.SenderType,
			Content:    m.Content,
		}
	}
	body, err := json.Marshal(rows)
	if err != nil {
		return fmt.Errorf("c1push: append: marshal: %w", err)
	}

	endpoint := p.supabaseURL + "/rest/v1/c1_messages"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("c1push: append: %w", err)
	}
	p.setHeaders(req)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Prefer", "return=minimal")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("c1push: append: http: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("c1push: append: status %d: %s", resp.StatusCode, string(b))
	}
	return nil
}

// findChannel looks up a channel by (tenant_id, platform, name) and returns its UUID.
func (p *Pusher) findChannel(ctx context.Context, tenantID, platform, name string) (string, error) {
	params := url.Values{
		"tenant_id": {"eq." + tenantID},
		"platform":  {"eq." + platform},
		"name":      {"eq." + name},
		"select":    {"id"},
		"limit":     {"1"},
	}
	endpoint := p.supabaseURL + "/rest/v1/c1_channels?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", fmt.Errorf("c1push: find channel: %w", err)
	}
	p.setHeaders(req)
	req.Header.Set("Accept", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("c1push: find channel: http: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("c1push: find channel: status %d", resp.StatusCode)
	}
	var rows []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &rows); err != nil || len(rows) == 0 {
		return "", nil
	}
	return rows[0].ID, nil
}

func (p *Pusher) setHeaders(req *http.Request) {
	req.Header.Set("apikey", p.anonKey)
	req.Header.Set("Authorization", "Bearer "+p.anonKey)
}
