package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/changmin/c4-core/internal/llm"
)

// ContextKeeper generates and maintains channel summaries using LLM.
// It is event-driven: triggered by task completions, checkpoints, or explicit requests.
type ContextKeeper struct {
	c1            *C1Handler
	gateway       *llm.Gateway
	minMessages   int    // minimum new messages before triggering summary update
	systemMemberID string // cached system member ID
}

// NewContextKeeper creates a ContextKeeper.
// gateway may be nil if LLM is not configured (summary updates will be skipped).
func NewContextKeeper(c1 *C1Handler, gateway *llm.Gateway) *ContextKeeper {
	return &ContextKeeper{
		c1:          c1,
		gateway:     gateway,
		minMessages: 5,
	}
}

// keeperSummaryRow represents a row from c1_channel_summaries for the keeper.
type keeperSummaryRow struct {
	ChannelID     string `json:"channel_id"`
	Summary       string `json:"summary"`
	KeyDecisions  string `json:"key_decisions"`  // JSON array as string
	OpenQuestions string `json:"open_questions"` // JSON array as string
	ActiveTasks   string `json:"active_tasks"`   // JSON array as string
	LastMessageID string `json:"last_message_id"`
	MessageCount  int    `json:"message_count"`
}

// keeperMessageRow includes sender info for summary generation.
type keeperMessageRow struct {
	ID         string `json:"id"`
	SenderName string `json:"sender_name"`
	Content    string `json:"content"`
	CreatedAt  string `json:"created_at"`
}

// summaryResult is the parsed LLM output for a channel summary.
type summaryResult struct {
	Summary       string   `json:"summary"`
	KeyDecisions  []string `json:"key_decisions"`
	OpenQuestions  []string `json:"open_questions"`
	ActiveTasks   []string `json:"active_tasks"`
}

// UpdateChannelSummary fetches recent messages since last summary and generates
// an incremental summary using LLM. Skips if fewer than minMessages new messages.
func (k *ContextKeeper) UpdateChannelSummary(channelID string) error {
	if k.gateway == nil {
		return nil // LLM not configured, skip silently
	}

	// 1. Get existing summary
	existing, err := k.getSummary(channelID)
	if err != nil {
		log.Printf("[keeper] get summary for %s: %v", channelID, err)
		// Not fatal — treat as empty summary
		existing = &keeperSummaryRow{ChannelID: channelID}
	}

	// 2. Get messages since last summary
	messages, err := k.getMessagesSince(channelID, existing.LastMessageID)
	if err != nil {
		return fmt.Errorf("get messages since: %w", err)
	}

	if len(messages) < k.minMessages {
		return nil // Not enough messages to update
	}

	// 3. Generate summary via LLM
	result, err := k.generateSummary(existing, messages)
	if err != nil {
		return fmt.Errorf("generate summary: %w", err)
	}

	// 4. Upsert summary
	lastMsgID := ""
	if len(messages) > 0 {
		lastMsgID = messages[len(messages)-1].ID
	}
	newCount := existing.MessageCount + len(messages)

	return k.upsertSummary(channelID, result, lastMsgID, newCount)
}

// EnsureChannel creates the channel if it doesn't exist, or returns the existing ID.
func (k *ContextKeeper) EnsureChannel(name, description, channelType string) (string, error) {
	// Try to resolve existing
	channelID, err := k.c1.resolveChannelID(name)
	if err != nil {
		return "", fmt.Errorf("resolve channel %s: %w", name, err)
	}
	if channelID != "" {
		return channelID, nil
	}

	// Create new channel
	payload := map[string]any{
		"project_id":   k.c1.projectID,
		"name":         name,
		"description":  description,
		"channel_type": channelType,
	}
	var rows []struct {
		ID string `json:"id"`
	}
	if err := k.c1.httpPostReturn("c1_channels", payload, &rows); err != nil {
		return "", fmt.Errorf("create channel %s: %w", name, err)
	}
	if len(rows) > 0 {
		return rows[0].ID, nil
	}
	return "", fmt.Errorf("channel %s created but no ID returned", name)
}

// ensureSystemMember returns the cached system member ID, creating it on first call.
func (k *ContextKeeper) ensureSystemMember() string {
	if k.systemMemberID != "" {
		return k.systemMemberID
	}
	memberID, err := k.c1.EnsureMember("system", "c4-engine", "C4 System")
	if err != nil {
		log.Printf("[keeper] failed to ensure system member: %v", err)
		return ""
	}
	k.systemMemberID = memberID
	return memberID
}

// AutoPost posts a system message to a channel (by name), auto-creating if needed.
func (k *ContextKeeper) AutoPost(channelName, content string) error {
	channelID, err := k.EnsureChannel(channelName, "Auto-created by C4 engine", "updates")
	if err != nil {
		return fmt.Errorf("ensure channel %s: %w", channelName, err)
	}

	// Ensure system member exists
	memberID := k.ensureSystemMember()

	// Post system message
	payload := map[string]any{
		"channel_id":  channelID,
		"project_id":  k.c1.projectID,
		"sender_type": "system",
		"sender_id":   "c4-engine",
		"sender_name": "system",
		"content":     content,
	}
	if memberID != "" {
		payload["member_id"] = memberID
	}
	if err := k.c1.httpPost("c1_messages", payload); err != nil {
		return fmt.Errorf("auto-post to %s: %w", channelName, err)
	}

	return nil
}

// EnsureSystemChannels creates the standard system channels for a project
// and removes legacy channels (worker-* per-worker channels, #updates duplicate).
func (k *ContextKeeper) EnsureSystemChannels() error {
	// --- Step 1: Delete legacy channels ---
	k.deleteLegacyChannels()

	// --- Step 1b: Reset stale agent presence (online/working → offline) ---
	k.resetStaleAgentPresence()

	// --- Step 2: Ensure canonical system channels ---
	channels := []struct {
		name        string
		description string
		channelType string
	}{
		{"general", "General discussion", "topic"},
		{"tasks", "Task events (created, completed, blocked)", "auto"},
		{"events", "EventBus event summaries", "auto"},
		{"knowledge", "Knowledge recorded events", "auto"},
		{"cq", "Shared worker dispatch channel (@cq mentions)", "worker"},
	}
	for _, ch := range channels {
		id, err := k.EnsureChannel(ch.name, ch.description, ch.channelType)
		if err != nil {
			fmt.Fprintf(os.Stderr, "cq: [keeper] failed to ensure channel %s: %v\n", ch.name, err)
		} else {
			fmt.Fprintf(os.Stderr, "cq: [keeper] channel %s ok (id=%s)\n", ch.name, id)
		}
	}
	return nil
}

// deleteLegacyChannels removes old-style per-worker channels and name duplicates.
func (k *ContextKeeper) deleteLegacyChannels() {
	type channelRow struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	var rows []channelRow
	filter := fmt.Sprintf("project_id=eq.%s&select=id,name", url.QueryEscape(k.c1.projectID))
	if err := k.c1.httpGet("c1_channels", filter, &rows); err != nil {
		fmt.Fprintf(os.Stderr, "cq: [keeper] list channels for cleanup: %v\n", err)
		return
	}
	for _, ch := range rows {
		// Delete old per-worker channels (worker-*)
		// Delete channels whose name starts with '#' (e.g. #updates duplicate)
		if strings.HasPrefix(ch.Name, "worker-") || strings.HasPrefix(ch.Name, "#") {
			if err := k.c1.httpDelete("c1_channels", fmt.Sprintf("id=eq.%s", ch.ID)); err != nil {
				fmt.Fprintf(os.Stderr, "cq: [keeper] delete legacy channel %q: %v\n", ch.Name, err)
			} else {
				fmt.Fprintf(os.Stderr, "cq: [keeper] deleted legacy channel %q\n", ch.Name)
			}
		}
	}
}

// resetStaleAgentPresence sets all online/working agent members to offline at startup.
// This cleans up stale presence from sessions that exited without calling c4_worker_shutdown.
func (k *ContextKeeper) resetStaleAgentPresence() {
	type memberRow struct {
		ID string `json:"id"`
	}
	var rows []memberRow
	filter := fmt.Sprintf(
		"project_id=eq.%s&member_type=eq.agent&status=in.(online,working,idle)&select=id",
		url.QueryEscape(k.c1.projectID),
	)
	if err := k.c1.httpGet("c1_members", filter, &rows); err != nil {
		fmt.Fprintf(os.Stderr, "cq: [keeper] list stale agents: %v\n", err)
		return
	}
	if len(rows) == 0 {
		return
	}
	// PATCH all stale agents to offline
	patch := map[string]any{"status": "offline", "status_text": "Session ended (auto-reset)"}
	patchBody, _ := json.Marshal(patch)
	patchFilter := fmt.Sprintf(
		"project_id=eq.%s&member_type=eq.agent&status=in.(online,working,idle)",
		url.QueryEscape(k.c1.projectID),
	)
	u := k.c1.baseURL + "/c1_members?" + patchFilter
	req, err := http.NewRequest("PATCH", u, bytes.NewReader(patchBody))
	if err != nil {
		fmt.Fprintf(os.Stderr, "cq: [keeper] patch stale agents: %v\n", err)
		return
	}
	k.c1.setHeaders(req)
	resp, err := k.c1.httpClient.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cq: [keeper] patch stale agents: %v\n", err)
		return
	}
	resp.Body.Close()
	fmt.Fprintf(os.Stderr, "cq: [keeper] reset %d stale agent(s) to offline\n", len(rows))
}

// getSummary retrieves the current summary for a channel.
func (k *ContextKeeper) getSummary(channelID string) (*keeperSummaryRow, error) {
	var rows []keeperSummaryRow
	filter := fmt.Sprintf("channel_id=eq.%s&select=channel_id,summary,key_decisions,open_questions,active_tasks,last_message_id,message_count",
		url.QueryEscape(channelID))
	if err := k.c1.httpGet("c1_channel_summaries", filter, &rows); err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return &keeperSummaryRow{ChannelID: channelID}, nil
	}
	return &rows[0], nil
}

// getMessagesSince returns messages after lastMessageID, ordered by created_at ASC.
func (k *ContextKeeper) getMessagesSince(channelID, lastMessageID string) ([]keeperMessageRow, error) {
	var messages []keeperMessageRow
	filters := []string{
		fmt.Sprintf("channel_id=eq.%s", url.QueryEscape(channelID)),
		"select=id,sender_name,content,created_at",
		"order=created_at.asc",
		"limit=100",
	}
	if lastMessageID != "" {
		// Get the created_at of the last message to filter by date
		// This is simpler than filtering by ID ordering
		var lastMsgs []keeperMessageRow
		lastFilter := fmt.Sprintf("id=eq.%s&select=id,sender_name,content,created_at", url.QueryEscape(lastMessageID))
		if err := k.c1.httpGet("c1_messages", lastFilter, &lastMsgs); err == nil && len(lastMsgs) > 0 {
			filters = append(filters, fmt.Sprintf("created_at=gt.%s", url.QueryEscape(lastMsgs[0].CreatedAt)))
		}
	}

	filter := strings.Join(filters, "&")
	if err := k.c1.httpGet("c1_messages", filter, &messages); err != nil {
		return nil, err
	}
	return messages, nil
}

// generateSummary calls LLM to produce an incremental summary.
func (k *ContextKeeper) generateSummary(existing *keeperSummaryRow, messages []keeperMessageRow) (*summaryResult, error) {
	// Format messages for prompt
	var msgLines []string
	for _, m := range messages {
		msgLines = append(msgLines, fmt.Sprintf("[%s] %s: %s", m.CreatedAt, m.SenderName, m.Content))
	}

	prompt := fmt.Sprintf(`You are a project context summarizer. Update the existing summary with new messages.

Existing summary:
%s

New messages:
%s

Return a JSON object with these fields:
- "summary": Updated 2-3 sentence summary of the channel's current state
- "key_decisions": Array of key decisions made (keep existing + add new)
- "open_questions": Array of unresolved questions (remove resolved, add new)
- "active_tasks": Array of task IDs currently being discussed

Return ONLY valid JSON, no markdown fences.`, existing.Summary, strings.Join(msgLines, "\n"))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := k.gateway.Chat(ctx, "summary", &llm.ChatRequest{
		Messages:    []llm.Message{{Role: "user", Content: prompt}},
		MaxTokens:   500,
		Temperature: 0.3,
	})
	if err != nil {
		return nil, fmt.Errorf("llm chat: %w", err)
	}

	// Parse JSON response
	var result summaryResult
	content := strings.TrimSpace(resp.Content)
	// Strip markdown fences if present
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return nil, fmt.Errorf("parse llm response: %w (content: %s)", err, content)
	}
	return &result, nil
}

// upsertSummary writes or updates the channel summary in Supabase.
func (k *ContextKeeper) upsertSummary(channelID string, result *summaryResult, lastMessageID string, messageCount int) error {
	keyDecisions, _ := json.Marshal(result.KeyDecisions)
	openQuestions, _ := json.Marshal(result.OpenQuestions)
	activeTasks, _ := json.Marshal(result.ActiveTasks)

	payload := map[string]any{
		"channel_id":      channelID,
		"summary":         result.Summary,
		"key_decisions":   json.RawMessage(keyDecisions),
		"open_questions":  json.RawMessage(openQuestions),
		"active_tasks":    json.RawMessage(activeTasks),
		"last_message_id": lastMessageID,
		"message_count":   messageCount,
	}

	return k.c1.httpUpsert("c1_channel_summaries", "channel_id", payload)
}

// httpPostReturn performs a POST request and decodes the response body.
func (h *C1Handler) httpPostReturn(table string, payload any, dest any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	u := h.baseURL + "/" + table
	req, err := http.NewRequest("POST", u, bytes.NewReader(body))
	if err != nil {
		return err
	}
	h.setHeaders(req)
	req.Header.Set("Prefer", "return=representation")

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("POST %s: %w", table, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("POST %s: %d %s", table, resp.StatusCode, string(respBody))
	}

	if dest != nil {
		if err := json.NewDecoder(resp.Body).Decode(dest); err != nil {
			return fmt.Errorf("decode %s response: %w", table, err)
		}
	}
	return nil
}

// httpPost performs a POST request to Supabase PostgREST.
func (h *C1Handler) httpPost(table string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	u := h.baseURL + "/" + table
	req, err := http.NewRequest("POST", u, bytes.NewReader(body))
	if err != nil {
		return err
	}
	h.setHeaders(req)

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("POST %s: %w", table, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("POST %s: %d %s", table, resp.StatusCode, string(respBody))
	}
	return nil
}

// httpDelete performs a DELETE request to Supabase PostgREST with a filter.
func (h *C1Handler) httpDelete(table, filter string) error {
	u := h.baseURL + "/" + table + "?" + filter
	req, err := http.NewRequest("DELETE", u, nil)
	if err != nil {
		return err
	}
	h.setHeaders(req)

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("DELETE %s: %w", table, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("DELETE %s: %d %s", table, resp.StatusCode, string(respBody))
	}
	return nil
}

// httpUpsert performs an upsert (POST with on_conflict) to Supabase PostgREST.
func (h *C1Handler) httpUpsert(table, conflictColumn string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	u := h.baseURL + "/" + table
	req, err := http.NewRequest("POST", u, bytes.NewReader(body))
	if err != nil {
		return err
	}
	h.setHeaders(req)
	req.Header.Set("Prefer", fmt.Sprintf("resolution=merge-duplicates,return=minimal"))
	// PostgREST uses the table's unique/pk constraint for on_conflict

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("UPSERT %s: %w", table, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("UPSERT %s: %d %s", table, resp.StatusCode, string(respBody))
	}
	return nil
}
