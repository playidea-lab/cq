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

// SupabaseStore persists conversation history in Supabase via PostgREST.
// Messages are stored in c1_messages; channels in c1_channels; members in c1_members.
// Migration 00028_unified_conversation.sql must be applied before use.
type SupabaseStore struct {
	supabaseURL string
	supabaseKey string
	httpClient  *http.Client
}

// c1MessageRow is the JSON row shape for c1_messages insert/select.
type c1MessageRow struct {
	ChannelID  string `json:"channel_id,omitempty"`
	ProjectID  string `json:"project_id,omitempty"` // nullable UUID; required for conv_to_knowledge trigger
	SenderName string `json:"sender_name,omitempty"`
	SenderType string `json:"sender_type,omitempty"`
	Content    string `json:"content"`
}

// c1ChannelRow is the JSON row shape for c1_channels insert/select.
type c1ChannelRow struct {
	ID          string `json:"id,omitempty"`
	TenantID    string `json:"tenant_id"`
	Name        string `json:"name"`
	ChannelType string `json:"channel_type"`
	Platform    string `json:"platform"`
}

// c1MemberRow is the JSON row shape for c1_members insert/select.
type c1MemberRow struct {
	ID          string `json:"id,omitempty"`
	TenantID    string `json:"tenant_id"`
	MemberType  string `json:"member_type"`
	ExternalID  string `json:"external_id"`
	DisplayName string `json:"display_name"`
	Platform    string `json:"platform"`
	PlatformID  string `json:"platform_id"`
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

// Get fetches the most recent limit messages for channelID (c1_channels UUID), oldest-first.
func (s *SupabaseStore) Get(ctx context.Context, channelID string, limit int) ([]Message, error) {
	params := url.Values{
		"channel_id": {"eq." + channelID},
		"order":      {"created_at.desc"},
		"select":     {"sender_type,content"},
	}
	if limit > 0 {
		params.Set("limit", strconv.Itoa(limit))
	}
	endpoint := s.supabaseURL + "/rest/v1/c1_messages?" + params.Encode()

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

	var rows []struct {
		SenderType string `json:"sender_type"`
		Content    string `json:"content"`
	}
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, fmt.Errorf("conversation: supabase: decode: %w", err)
	}

	// Rows arrived newest-first; reverse to oldest-first for LLM context.
	for i, j := 0, len(rows)-1; i < j; i, j = i+1, j-1 {
		rows[i], rows[j] = rows[j], rows[i]
	}

	msgs := make([]Message, len(rows))
	for i, r := range rows {
		role := "assistant"
		if r.SenderType == "user" {
			role = "user"
		}
		msgs[i] = Message{Role: role, Content: r.Content}
	}
	return msgs, nil
}

// Append inserts msgs into c1_messages for the given channel UUID.
// projectID is forwarded to c1_messages.project_id so the conv_to_knowledge
// DB trigger can record assistant replies into c4_documents (trigger fires only
// when project_id IS NOT NULL). platform is accepted for interface compatibility.
func (s *SupabaseStore) Append(ctx context.Context, channelID, _ string, projectID string, msgs []Message) error {
	if len(msgs) == 0 {
		return nil
	}
	rows := make([]c1MessageRow, len(msgs))
	for i, m := range msgs {
		senderType := "system"
		senderName := "c5-assistant"
		if m.Role == "user" {
			senderType = "user"
			senderName = "user"
		}
		rows[i] = c1MessageRow{
			ChannelID:  channelID,
			ProjectID:  projectID, // empty → omitempty → NULL in DB (trigger skips)
			SenderName: senderName,
			SenderType: senderType,
			Content:    m.Content,
		}
	}

	body, err := json.Marshal(rows)
	if err != nil {
		return fmt.Errorf("conversation: supabase: marshal: %w", err)
	}

	endpoint := s.supabaseURL + "/rest/v1/c1_messages"
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
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("conversation: supabase: insert status %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

// EnsureChannel creates or retrieves a channel by (tenant_id, platform, name).
// Returns the channel's UUID string. The partial unique index
// uniq_c1_channels_bot (migration 00028) prevents duplicates for bot/event channels.
func (s *SupabaseStore) EnsureChannel(ctx context.Context, ch Channel) (string, error) {
	tenant := ch.TenantID
	if tenant == "" {
		tenant = "default"
	}
	chType := ch.ChannelType
	if chType == "" {
		chType = "bot"
	}

	// Try to find existing channel first.
	if id, err := s.findChannel(ctx, tenant, ch.Platform, ch.Name); err == nil && id != "" {
		return id, nil
	}

	// Create the channel; ignore-duplicates handles the race between GET and POST.
	row := c1ChannelRow{
		TenantID:    tenant,
		Name:        ch.Name,
		ChannelType: chType,
		Platform:    ch.Platform,
	}
	body, err := json.Marshal([]c1ChannelRow{row})
	if err != nil {
		return "", fmt.Errorf("conversation: ensure channel: marshal: %w", err)
	}

	endpoint := s.supabaseURL + "/rest/v1/c1_channels"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("conversation: ensure channel: create request: %w", err)
	}
	s.setHeaders(req)
	req.Header.Set("Content-Type", "application/json")
	// return=representation so we get the generated UUID back.
	// resolution=ignore-duplicates maps to ON CONFLICT DO NOTHING.
	req.Header.Set("Prefer", "return=representation,resolution=ignore-duplicates")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("conversation: ensure channel: http: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	if resp.StatusCode == http.StatusConflict {
		// 409: partial unique index conflict (race between GET and POST).
		// resolution=ignore-duplicates does not cover partial indexes on all
		// PostgREST versions — fall back to GET to return the existing UUID.
		return s.findChannel(ctx, tenant, ch.Platform, ch.Name)
	}
	if resp.StatusCode >= 400 {
		// RLS violation, schema error, or other server-side rejection.
		return "", fmt.Errorf("conversation: ensure channel: server error %d: %s", resp.StatusCode, string(respBody))
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		var rows []c1ChannelRow
		if err := json.Unmarshal(respBody, &rows); err == nil && len(rows) > 0 {
			return rows[0].ID, nil
		}
	}

	// 2xx but empty (ON CONFLICT DO NOTHING fired) — fetch the existing row.
	return s.findChannel(ctx, tenant, ch.Platform, ch.Name)
}

// findChannel looks up a channel by (tenant_id, platform, name) and returns its UUID.
func (s *SupabaseStore) findChannel(ctx context.Context, tenantID, platform, name string) (string, error) {
	params := url.Values{
		"tenant_id": {"eq." + tenantID},
		"platform":  {"eq." + platform},
		"name":      {"eq." + name},
		"select":    {"id"},
		"limit":     {"1"},
	}
	endpoint := s.supabaseURL + "/rest/v1/c1_channels?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", fmt.Errorf("conversation: find channel: create request: %w", err)
	}
	s.setHeaders(req)
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("conversation: find channel: http: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("conversation: find channel: status %d", resp.StatusCode)
	}
	var rows []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &rows); err != nil || len(rows) == 0 {
		return "", nil
	}
	return rows[0].ID, nil
}

// ListChannels returns channels for the given tenant (and optional project).
func (s *SupabaseStore) ListChannels(ctx context.Context, tenantID, projectID string) ([]Channel, error) {
	if tenantID == "" {
		tenantID = "default"
	}
	params := url.Values{
		"tenant_id": {"eq." + tenantID},
		"select":    {"id,tenant_id,name,channel_type,platform"},
		"limit":     {"500"},
	}
	if projectID != "" {
		params.Set("project_id", "eq."+projectID)
	}
	endpoint := s.supabaseURL + "/rest/v1/c1_channels?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("conversation: list channels: %w", err)
	}
	s.setHeaders(req)
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("conversation: list channels: http: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("conversation: list channels: status %d", resp.StatusCode)
	}
	var rows []c1ChannelRow
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, fmt.Errorf("conversation: list channels: decode: %w", err)
	}
	out := make([]Channel, len(rows))
	for i, r := range rows {
		out[i] = Channel{TenantID: r.TenantID, Name: r.Name, ChannelType: r.ChannelType, Platform: r.Platform}
	}
	return out, nil
}

// EnsureParticipant creates or retrieves a member by (tenant_id, platform, platform_id).
// Returns the member's UUID string. The partial unique index
// uniq_c1_members_platform (migration 00028) prevents duplicates.
func (s *SupabaseStore) EnsureParticipant(ctx context.Context, p Participant) (string, error) {
	tenant := p.TenantID
	if tenant == "" {
		tenant = "default"
	}
	if p.PlatformID == "" {
		return "", nil // no-op if no platform identity
	}

	// Try to find existing member.
	if id, err := s.findParticipant(ctx, tenant, p.Platform, p.PlatformID); err == nil && id != "" {
		return id, nil
	}

	memberType := p.MemberType
	if memberType == "" {
		memberType = "user"
	}
	row := c1MemberRow{
		TenantID:    tenant,
		MemberType:  memberType,
		ExternalID:  p.ExternalID,
		DisplayName: p.DisplayName,
		Platform:    p.Platform,
		PlatformID:  p.PlatformID,
	}
	body, err := json.Marshal([]c1MemberRow{row})
	if err != nil {
		return "", fmt.Errorf("conversation: ensure participant: marshal: %w", err)
	}

	endpoint := s.supabaseURL + "/rest/v1/c1_members"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("conversation: ensure participant: create request: %w", err)
	}
	s.setHeaders(req)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Prefer", "return=representation,resolution=ignore-duplicates")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("conversation: ensure participant: http: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	if resp.StatusCode == http.StatusConflict {
		// 409: partial unique index conflict (race between GET and POST).
		// resolution=ignore-duplicates does not cover partial indexes on all
		// PostgREST versions — fall back to GET to return the existing UUID.
		return s.findParticipant(ctx, tenant, p.Platform, p.PlatformID)
	}
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("conversation: ensure participant: server error %d: %s", resp.StatusCode, string(respBody))
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		var rows []c1MemberRow
		if err := json.Unmarshal(respBody, &rows); err == nil && len(rows) > 0 {
			return rows[0].ID, nil
		}
	}

	// 2xx but empty — fetch the existing row.
	return s.findParticipant(ctx, tenant, p.Platform, p.PlatformID)
}

// findParticipant looks up a member by (tenant_id, platform, platform_id) and returns its UUID.
func (s *SupabaseStore) findParticipant(ctx context.Context, tenantID, platform, platformID string) (string, error) {
	params := url.Values{
		"tenant_id":   {"eq." + tenantID},
		"platform":    {"eq." + platform},
		"platform_id": {"eq." + platformID},
		"select":      {"id"},
		"limit":       {"1"},
	}
	endpoint := s.supabaseURL + "/rest/v1/c1_members?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", fmt.Errorf("conversation: find participant: create request: %w", err)
	}
	s.setHeaders(req)
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("conversation: find participant: http: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("conversation: find participant: status %d", resp.StatusCode)
	}
	var rows []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &rows); err != nil || len(rows) == 0 {
		return "", nil
	}
	return rows[0].ID, nil
}

// Compile-time interface assertion.
var _ Store = (*SupabaseStore)(nil)

// Cleanup is a no-op for SupabaseStore; expiry is handled by database policies.
func (s *SupabaseStore) Cleanup() {}

func (s *SupabaseStore) setHeaders(req *http.Request) {
	req.Header.Set("apikey", s.supabaseKey)
	req.Header.Set("Authorization", "Bearer "+s.supabaseKey)
}
