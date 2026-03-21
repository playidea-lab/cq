//go:build c1_messenger


package messengerhandler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/changmin/c4-core/internal/cloud"
	"github.com/changmin/c4-core/internal/mcp"
)

// C1Handler handles C1 (real-time collaboration) HTTP requests to Supabase.
type C1Handler struct {
	baseURL    string              // Supabase PostgREST URL (e.g., https://xxx.supabase.co/rest/v1)
	apiKey     string              // anon key
	tp         *cloud.TokenProvider
	projectID  string              // project ID for filtering
	httpClient *http.Client
}

// NewC1Handler creates a new C1Handler.
func NewC1Handler(baseURL, apiKey string, tp *cloud.TokenProvider, projectID string) *C1Handler {
	return &C1Handler{
		baseURL:   strings.TrimRight(baseURL, "/"),
		apiKey:    apiKey,
		tp:        tp,
		projectID: projectID,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

// c1MessageRow represents a message from c1_messages table.
type c1MessageRow struct {
	ID         string `json:"id"`
	ChannelID  string `json:"channel_id"`
	SenderName string `json:"sender_name"`
	Content    string `json:"content"`
	CreatedAt  string `json:"created_at"`
}

// c1ChannelRow represents a channel from c1_channels table.
type c1ChannelRow struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// c1ParticipantRow represents a participant from c1_participants table.
type c1ParticipantRow struct {
	AgentName  string `json:"agent_name"`
	ChannelID  string `json:"channel_id"`
	LastReadAt string `json:"last_read_at"`
}

// c1ChannelSummaryRow represents a summary from c1_channel_summaries table.
type c1ChannelSummaryRow struct {
	ChannelID     string `json:"channel_id"`
	Summary       string `json:"summary"`
	KeyDecisions  string `json:"key_decisions"`
	OpenQuestions string `json:"open_questions"`
}

// c1MemberRow represents a member from c1_members table.
type c1MemberRow struct {
	ID          string `json:"id"`
	ProjectID   string `json:"project_id"`
	MemberType  string `json:"member_type"`
	ExternalID  string `json:"external_id"`
	DisplayName string `json:"display_name"`
	Avatar      string `json:"avatar"`
	Status      string `json:"status"`
	StatusText  string `json:"status_text"`
	LastSeenAt  string `json:"last_seen_at"`
	CreatedAt   string `json:"created_at"`
}

// httpGet performs a GET request and decodes JSON response.
// Retries once on 401 Unauthorized after refreshing the token.
func (h *C1Handler) httpGet(table, filter string, dest any) error {
	u := h.baseURL + "/" + table
	if filter != "" {
		u += "?" + filter
	}

	for attempt := 0; attempt < 2; attempt++ {
		req, err := http.NewRequest("GET", u, nil)
		if err != nil {
			return err
		}
		h.setHeaders(req)

		resp, err := h.httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("GET %s: %w", table, err)
		}

		if resp.StatusCode == http.StatusUnauthorized && attempt == 0 {
			resp.Body.Close()
			if _, refreshErr := h.tp.Refresh(); refreshErr == nil {
				continue
			} else {
				return fmt.Errorf("GET %s: 401 unauthorized, token refresh: %w", table, refreshErr)
			}
		}

		if resp.StatusCode >= 400 {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
			resp.Body.Close()
			return fmt.Errorf("GET %s: %d %s", table, resp.StatusCode, string(body))
		}

		var decodeErr error
		if dest != nil {
			decodeErr = json.NewDecoder(resp.Body).Decode(dest)
		}
		resp.Body.Close()
		if decodeErr != nil {
			return fmt.Errorf("decode %s: %w", table, decodeErr)
		}
		return nil
	}
	return nil
}

// setHeaders sets required headers for Supabase PostgREST.
func (h *C1Handler) setHeaders(req *http.Request) {
	req.Header.Set("apikey", h.apiKey)
	req.Header.Set("Content-Type", "application/json")
	if token := h.tp.Token(); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
}

// escapeLikePattern escapes special characters in PostgREST LIKE patterns.
// Backslash-escapes % (any characters) and _ (any single character).
func escapeLikePattern(s string) string {
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return s
}

// resolveChannelID returns channel ID for a given channel name.
// Returns empty string if not found.
func (h *C1Handler) resolveChannelID(channelName string) (string, error) {
	var channels []c1ChannelRow
	filter := fmt.Sprintf("project_id=eq.%s&name=eq.%s&select=id",
		url.QueryEscape(h.projectID), url.QueryEscape(channelName))
	if err := h.httpGet("c1_channels", filter, &channels); err != nil {
		return "", err
	}
	if len(channels) == 0 {
		return "", nil
	}
	return channels[0].ID, nil
}

// Search searches messages with FTS query, optional channel and date filter.
func (h *C1Handler) Search(query, channelName, since string, limit int) (map[string]any, error) {
	// Build filter
	filters := []string{
		fmt.Sprintf("project_id=eq.%s", url.QueryEscape(h.projectID)),
		fmt.Sprintf("tsv=fts(english).%s", url.QueryEscape(query)),
	}

	// Optional channel filter
	if channelName != "" {
		channelID, err := h.resolveChannelID(channelName)
		if err != nil {
			return nil, fmt.Errorf("resolve channel: %w", err)
		}
		if channelID == "" {
			return nil, fmt.Errorf("channel not found: %s", channelName)
		}
		filters = append(filters, fmt.Sprintf("channel_id=eq.%s", url.QueryEscape(channelID)))
	}

	// Optional date filter
	if since != "" {
		filters = append(filters, fmt.Sprintf("created_at=gte.%s", url.QueryEscape(since)))
	}

	// Add select and ordering
	filterStr := strings.Join(filters, "&") + "&select=id,channel_id,sender_name,content,created_at&order=created_at.desc"
	if limit <= 0 {
		limit = 50
	}
	filterStr += fmt.Sprintf("&limit=%d", limit)

	var messages []c1MessageRow
	if err := h.httpGet("c1_messages", filterStr, &messages); err != nil {
		return nil, err
	}

	// Convert to result format
	results := make([]map[string]any, 0, len(messages))
	for _, m := range messages {
		results = append(results, map[string]any{
			"id":          m.ID,
			"channel_id":  m.ChannelID,
			"sender_name": m.SenderName,
			"content":     m.Content,
			"created_at":  m.CreatedAt,
		})
	}

	return map[string]any{
		"count":   len(results),
		"results": results,
	}, nil
}

// CheckMentions finds messages mentioning an agent, after their last_read_at.
func (h *C1Handler) CheckMentions(agentName string) ([]map[string]any, error) {
	// First get participant record to find last_read_at
	var participants []c1ParticipantRow
	filter := fmt.Sprintf("project_id=eq.%s&agent_name=eq.%s&select=channel_id,last_read_at",
		url.QueryEscape(h.projectID), url.QueryEscape(agentName))
	if err := h.httpGet("c1_participants", filter, &participants); err != nil {
		return nil, fmt.Errorf("get participant: %w", err)
	}

	// Build mention filter with escaped LIKE pattern
	escapedAgentName := escapeLikePattern(agentName)
	mentionPattern := fmt.Sprintf("*@%s*", escapedAgentName)
	filters := []string{
		fmt.Sprintf("project_id=eq.%s", url.QueryEscape(h.projectID)),
		fmt.Sprintf("content=like.%s", url.QueryEscape(mentionPattern)),
	}

	// If participant exists, filter by last_read_at
	if len(participants) > 0 && participants[0].LastReadAt != "" {
		filters = append(filters, fmt.Sprintf("created_at=gt.%s", url.QueryEscape(participants[0].LastReadAt)))
	}

	filterStr := strings.Join(filters, "&") + "&select=id,channel_id,sender_name,content,created_at&order=created_at.desc&limit=50"

	var messages []c1MessageRow
	if err := h.httpGet("c1_messages", filterStr, &messages); err != nil {
		return nil, err
	}

	// Get channel names for the messages
	channelIDs := make(map[string]bool)
	for _, m := range messages {
		channelIDs[m.ChannelID] = true
	}

	channelIDList := make([]string, 0, len(channelIDs))
	for id := range channelIDs {
		channelIDList = append(channelIDList, url.QueryEscape(id))
	}

	channelMap := make(map[string]string) // id -> name
	if len(channelIDList) > 0 {
		var channels []c1ChannelRow
		channelFilter := fmt.Sprintf("id=in.(%s)&select=id,name", strings.Join(channelIDList, ","))
		if err := h.httpGet("c1_channels", channelFilter, &channels); err == nil {
			for _, ch := range channels {
				channelMap[ch.ID] = ch.Name
			}
		}
	}

	// Format results
	results := make([]map[string]any, 0, len(messages))
	for _, m := range messages {
		channelName := channelMap[m.ChannelID]
		if channelName == "" {
			channelName = m.ChannelID
		}
		results = append(results, map[string]any{
			"channel_name": channelName,
			"message":      m.Content,
			"sender_name":  m.SenderName,
			"created_at":   m.CreatedAt,
		})
	}

	return results, nil
}

// GetBriefing returns channel summaries and recent messages for a project.
func (h *C1Handler) GetBriefing() (map[string]any, error) {
	// Get all channels for this project
	var channels []c1ChannelRow
	channelFilter := fmt.Sprintf("project_id=eq.%s&select=id,name", url.QueryEscape(h.projectID))
	if err := h.httpGet("c1_channels", channelFilter, &channels); err != nil {
		return nil, fmt.Errorf("get channels: %w", err)
	}

	channelMap := make(map[string]string) // id -> name
	channelIDs := make([]string, 0, len(channels))
	for _, ch := range channels {
		channelMap[ch.ID] = ch.Name
		channelIDs = append(channelIDs, ch.ID)
	}

	// Get summaries for all channels
	summaries := make([]map[string]any, 0)
	if len(channelIDs) > 0 {
		// URL-encode each channel ID for the in.(...) filter
		encodedIDs := make([]string, 0, len(channelIDs))
		for _, id := range channelIDs {
			encodedIDs = append(encodedIDs, url.QueryEscape(id))
		}
		var summaryRows []c1ChannelSummaryRow
		summaryFilter := fmt.Sprintf("channel_id=in.(%s)&select=channel_id,summary,key_decisions,open_questions",
			strings.Join(encodedIDs, ","))
		if err := h.httpGet("c1_channel_summaries", summaryFilter, &summaryRows); err == nil {
			for _, s := range summaryRows {
				summaries = append(summaries, map[string]any{
					"channel_name":   channelMap[s.ChannelID],
					"summary":        s.Summary,
					"key_decisions":  s.KeyDecisions,
					"open_questions": s.OpenQuestions,
				})
			}
		} else {
			fmt.Fprintf(os.Stderr, "c4: c1: failed to get channel summaries: %v\n", err)
		}
	}

	// Get recent 10 messages across all channels
	messageFilter := fmt.Sprintf("project_id=eq.%s&select=channel_id,sender_name,content,created_at&order=created_at.desc&limit=10",
		url.QueryEscape(h.projectID))
	var messages []c1MessageRow
	if err := h.httpGet("c1_messages", messageFilter, &messages); err != nil {
		return nil, fmt.Errorf("get recent messages: %w", err)
	}

	recentMessages := make([]map[string]any, 0, len(messages))
	for _, m := range messages {
		channelName := channelMap[m.ChannelID]
		if channelName == "" {
			channelName = m.ChannelID
		}
		recentMessages = append(recentMessages, map[string]any{
			"channel_name": channelName,
			"sender_name":  m.SenderName,
			"content":      m.Content,
			"created_at":   m.CreatedAt,
		})
	}

	return map[string]any{
		"channel_summaries": summaries,
		"recent_messages":   recentMessages,
	}, nil
}

// EnsureMember upserts a member record and returns the member ID.
func (h *C1Handler) EnsureMember(memberType, externalID, displayName string) (string, error) {
	// Try to find existing member
	var members []c1MemberRow
	filter := fmt.Sprintf("project_id=eq.%s&member_type=eq.%s&external_id=eq.%s&select=id",
		url.QueryEscape(h.projectID), url.QueryEscape(memberType), url.QueryEscape(externalID))
	if err := h.httpGet("c1_members", filter, &members); err != nil {
		return "", fmt.Errorf("lookup member: %w", err)
	}
	if len(members) > 0 {
		return members[0].ID, nil
	}

	// Create new member
	payload := map[string]any{
		"project_id":   h.projectID,
		"member_type":  memberType,
		"external_id":  externalID,
		"display_name": displayName,
		"status":       "online",
		"last_seen_at":  time.Now().UTC().Format(time.RFC3339),
	}
	var rows []struct {
		ID string `json:"id"`
	}
	if err := h.httpPostReturn("c1_members", payload, &rows); err != nil {
		return "", fmt.Errorf("create member: %w", err)
	}
	if len(rows) > 0 {
		return rows[0].ID, nil
	}
	return "", fmt.Errorf("member created but no ID returned")
}

// SendMessage sends a message to a channel as a member (agent or user).
func (h *C1Handler) SendMessage(channelName, content, senderName string, threadID string, metadata map[string]any) (map[string]any, error) {
	if channelName == "" {
		return nil, fmt.Errorf("channel_name is required")
	}
	if content == "" {
		return nil, fmt.Errorf("content is required")
	}

	// Resolve channel (auto-create if needed)
	channelID, err := h.resolveChannelID(channelName)
	if err != nil {
		return nil, fmt.Errorf("resolve channel: %w", err)
	}
	if channelID == "" {
		return nil, fmt.Errorf("channel not found: %s", channelName)
	}

	// Determine sender info
	if senderName == "" {
		senderName = "c4-agent"
	}

	// Ensure member exists
	memberID, err := h.EnsureMember("agent", senderName, senderName)
	if err != nil {
		return nil, fmt.Errorf("ensure member: %w", err)
	}

	// Build message payload
	payload := map[string]any{
		"channel_id":  channelID,
		"project_id":  h.projectID,
		"sender_type": "agent",
		"sender_id":   senderName,
		"sender_name": senderName,
		"member_id":   memberID,
		"content":     content,
	}
	if threadID != "" {
		payload["thread_id"] = threadID
	}
	if metadata != nil {
		payload["metadata"] = metadata
	}

	if err := h.httpPost("c1_messages", payload); err != nil {
		return nil, fmt.Errorf("send message: %w", err)
	}

	return map[string]any{
		"status":     "sent",
		"channel":    channelName,
		"member_id":  memberID,
		"sender":     senderName,
	}, nil
}

// UpdatePresence updates a member's status and status_text.
func (h *C1Handler) UpdatePresence(memberType, externalID, status, statusText string) error {
	// Validate status
	switch status {
	case "online", "working", "idle", "offline":
		// valid
	default:
		return fmt.Errorf("invalid status: %s (must be online/working/idle/offline)", status)
	}

	payload := map[string]any{
		"status":       status,
		"status_text":  statusText,
		"last_seen_at": time.Now().UTC().Format(time.RFC3339),
	}

	filter := fmt.Sprintf("project_id=eq.%s&member_type=eq.%s&external_id=eq.%s",
		url.QueryEscape(h.projectID), url.QueryEscape(memberType), url.QueryEscape(externalID))

	if err := h.httpPatch("c1_members", filter, payload); err != nil {
		return fmt.Errorf("update presence: %w", err)
	}
	return nil
}

// httpPatch performs a PATCH request to Supabase PostgREST.
func (h *C1Handler) httpPatch(table, filter string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	u := h.baseURL + "/" + table
	if filter != "" {
		u += "?" + filter
	}

	req, err := http.NewRequest("PATCH", u, bytes.NewReader(body))
	if err != nil {
		return err
	}
	h.setHeaders(req)

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("PATCH %s: %w", table, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
		return fmt.Errorf("PATCH %s: %d %s", table, resp.StatusCode, string(respBody))
	}
	return nil
}

// ClaimMessage atomically claims a c1_messages row via Supabase RPC.
// Returns true if the claim succeeded (no other worker claimed it first).
func (h *C1Handler) ClaimMessage(messageID, workerID string) (bool, error) {
	payload := map[string]any{
		"p_message_id": messageID,
		"p_worker_id":  workerID,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return false, fmt.Errorf("marshal: %w", err)
	}

	// Supabase RPC: POST /rest/v1/rpc/claim_message
	rpcURL := h.baseURL + "/rpc/claim_message"
	req, err := http.NewRequest("POST", rpcURL, bytes.NewReader(body))
	if err != nil {
		return false, err
	}
	h.setHeaders(req)
	req.Header.Set("Prefer", "return=representation")

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("RPC claim_message: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
		return false, fmt.Errorf("RPC claim_message: %d %s", resp.StatusCode, string(respBody))
	}

	// The function returns SETOF c1_messages; if the row was claimed, we get a non-empty array.
	var rows []json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&rows); err != nil {
		return false, fmt.Errorf("decode claim_message response: %w", err)
	}
	return len(rows) > 0, nil
}

// cqMentionRow is a minimal message row for @cq mention polling.
type cqMentionRow struct {
	ID        string `json:"id"`
	Content   string `json:"content"`
	SenderName string `json:"sender_name"`
	CreatedAt string `json:"created_at"`
}

// PollCqMentions returns unclaimed messages from the #cq channel that mention @cq.
func (h *C1Handler) PollCqMentions(channelID string, limit int) ([]cqMentionRow, error) {
	if limit <= 0 {
		limit = 10
	}
	filter := fmt.Sprintf(
		"channel_id=eq.%s&content=like.%s&claimed_by=is.null&select=id,content,sender_name,created_at&order=created_at.asc&limit=%d",
		url.QueryEscape(channelID),
		url.QueryEscape("*@cq*"),
		limit,
	)
	var rows []cqMentionRow
	if err := h.httpGet("c1_messages", filter, &rows); err != nil {
		return nil, err
	}
	return rows, nil
}

// RegisterC1Handlers registers c1_* MCP tools.
func RegisterC1Handlers(reg *mcp.Registry, handler *C1Handler) {
	// c1_search — Full-text search across messages
	reg.Register(mcp.ToolSchema{
		Name:        "c1_search",
		Description: "Search C1 messages with full-text query, optional channel and date filter",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query":        map[string]any{"type": "string", "description": "Search query (full-text)"},
				"channel_name": map[string]any{"type": "string", "description": "Optional channel name filter"},
				"since":        map[string]any{"type": "string", "description": "Optional ISO 8601 date filter (e.g. 2026-02-14T00:00:00Z)"},
				"limit":        map[string]any{"type": "integer", "description": "Max results (default: 50)"},
			},
			"required": []string{"query"},
		},
	}, func(raw json.RawMessage) (any, error) {
		var args struct {
			Query       string `json:"query"`
			ChannelName string `json:"channel_name"`
			Since       string `json:"since"`
			Limit       int    `json:"limit"`
		}
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("parse args: %w", err)
		}
		if args.Query == "" {
			return nil, fmt.Errorf("query is required")
		}
		return handler.Search(args.Query, args.ChannelName, args.Since, args.Limit)
	})

	// c1_check_mentions — Find mentions of an agent
	reg.Register(mcp.ToolSchema{
		Name:        "c1_check_mentions",
		Description: "Check for messages mentioning an agent (after their last_read_at)",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"agent_name": map[string]any{"type": "string", "description": "Agent name to check mentions for"},
			},
			"required": []string{"agent_name"},
		},
	}, func(raw json.RawMessage) (any, error) {
		var args struct {
			AgentName string `json:"agent_name"`
		}
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("parse args: %w", err)
		}
		if args.AgentName == "" {
			return nil, fmt.Errorf("agent_name is required")
		}
		return handler.CheckMentions(args.AgentName)
	})

	// c1_get_briefing — Get project briefing with summaries and recent messages
	reg.Register(mcp.ToolSchema{
		Name:        "c1_get_briefing",
		Description: "Get project briefing: channel summaries and recent messages",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}, func(raw json.RawMessage) (any, error) {
		return handler.GetBriefing()
	})

	// c1_send_message — Send a message to a channel as an agent
	reg.Register(mcp.ToolSchema{
		Name:        "c1_send_message",
		Description: "Send a message to a C1 channel as an agent. Auto-creates agent member record.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"channel_name": map[string]any{"type": "string", "description": "Target channel name (e.g. general, tasks)"},
				"content":      map[string]any{"type": "string", "description": "Message content (markdown supported)"},
				"agent_name":   map[string]any{"type": "string", "description": "Sender agent name (default: c4-agent)"},
				"thread_id":    map[string]any{"type": "string", "description": "Optional thread ID for threaded replies"},
			},
			"required": []string{"channel_name", "content"},
		},
	}, func(raw json.RawMessage) (any, error) {
		var args struct {
			ChannelName string         `json:"channel_name"`
			Content     string         `json:"content"`
			AgentName   string         `json:"agent_name"`
			ThreadID    string         `json:"thread_id"`
			Metadata    map[string]any `json:"metadata"`
		}
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("parse args: %w", err)
		}
		return handler.SendMessage(args.ChannelName, args.Content, args.AgentName, args.ThreadID, args.Metadata)
	})

	// c1_update_presence — Update agent presence status
	reg.Register(mcp.ToolSchema{
		Name:        "c1_update_presence",
		Description: "Update an agent's presence status (online/working/idle/offline) with optional status text",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"agent_name":  map[string]any{"type": "string", "description": "Agent name"},
				"status":      map[string]any{"type": "string", "enum": []string{"online", "working", "idle", "offline"}, "description": "Presence status"},
				"status_text": map[string]any{"type": "string", "description": "Status text (e.g. 'Working on T-003-0')"},
			},
			"required": []string{"agent_name", "status"},
		},
	}, func(raw json.RawMessage) (any, error) {
		var args struct {
			AgentName  string `json:"agent_name"`
			Status     string `json:"status"`
			StatusText string `json:"status_text"`
		}
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("parse args: %w", err)
		}
		if args.AgentName == "" {
			return nil, fmt.Errorf("agent_name is required")
		}
		if err := handler.UpdatePresence("agent", args.AgentName, args.Status, args.StatusText); err != nil {
			return nil, err
		}
		return map[string]any{"status": "updated", "agent": args.AgentName, "presence": args.Status}, nil
	})
}
