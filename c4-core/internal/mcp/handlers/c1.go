package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/changmin/c4-core/internal/mcp"
)

// C1Handler handles C1 (real-time collaboration) HTTP requests to Supabase.
type C1Handler struct {
	baseURL    string       // Supabase PostgREST URL (e.g., https://xxx.supabase.co/rest/v1)
	apiKey     string       // anon key
	authToken  string       // user's JWT
	projectID  string       // project ID for filtering
	httpClient *http.Client
}

// NewC1Handler creates a new C1Handler.
func NewC1Handler(baseURL, apiKey, authToken, projectID string) *C1Handler {
	return &C1Handler{
		baseURL:   strings.TrimRight(baseURL, "/"),
		apiKey:    apiKey,
		authToken: authToken,
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

// httpGet performs a GET request and decodes JSON response.
func (h *C1Handler) httpGet(table, filter string, dest any) error {
	u := h.baseURL + "/" + table
	if filter != "" {
		u += "?" + filter
	}

	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return err
	}
	h.setHeaders(req)

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("GET %s: %w", table, err)
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

// setHeaders sets required headers for Supabase PostgREST.
func (h *C1Handler) setHeaders(req *http.Request) {
	req.Header.Set("apikey", h.apiKey)
	req.Header.Set("Content-Type", "application/json")
	if h.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+h.authToken)
	}
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

// intOr returns a if a > 0, otherwise b.
func intOr(a, b int) int {
	if a > 0 {
		return a
	}
	return b
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

	// Build mention filter
	mentionPattern := fmt.Sprintf("*@%s*", agentName)
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
		channelIDList = append(channelIDList, id)
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
		var summaryRows []c1ChannelSummaryRow
		summaryFilter := fmt.Sprintf("channel_id=in.(%s)&select=channel_id,summary,key_decisions,open_questions",
			strings.Join(channelIDs, ","))
		if err := h.httpGet("c1_channel_summaries", summaryFilter, &summaryRows); err == nil {
			for _, s := range summaryRows {
				summaries = append(summaries, map[string]any{
					"channel_name":   channelMap[s.ChannelID],
					"summary":        s.Summary,
					"key_decisions":  s.KeyDecisions,
					"open_questions": s.OpenQuestions,
				})
			}
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
}
