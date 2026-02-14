package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/changmin/c4-core/internal/mcp"
)

// C1Handler provides HTTP client helpers for C1 Hub messaging operations.
type C1Handler struct {
	supabaseURL string
	supabaseKey string
	authToken   string
	projectID   string
	httpClient  *http.Client
}

// NewC1Handler creates a C1Handler with the given Supabase credentials.
func NewC1Handler(supabaseURL, supabaseKey, authToken, projectID string) *C1Handler {
	return &C1Handler{
		supabaseURL: strings.TrimRight(supabaseURL, "/"),
		supabaseKey: supabaseKey,
		authToken:   authToken,
		projectID:   projectID,
		httpClient:  &http.Client{Timeout: 30 * time.Second},
	}
}

// httpGet performs a GET request to the given path.
func (h *C1Handler) httpGet(path string) ([]byte, error) {
	url := h.supabaseURL + path
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("apikey", h.supabaseKey)
	req.Header.Set("Authorization", "Bearer "+h.authToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", path, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("GET %s: status %d: %s", path, resp.StatusCode, string(body))
	}

	return body, nil
}

// httpPost performs a POST request to the given path with JSON body.
func (h *C1Handler) httpPost(path string, body interface{}) ([]byte, error) {
	jsonData, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshaling body: %w", err)
	}

	url := h.supabaseURL + path
	req, err := http.NewRequest("POST", url, bytes.NewReader(jsonData))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("apikey", h.supabaseKey)
	req.Header.Set("Authorization", "Bearer "+h.authToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Prefer", "return=representation")

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("POST %s: %w", path, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("POST %s: status %d: %s", path, resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

// ListChannels lists all channels for a project.
func (h *C1Handler) ListChannels(args json.RawMessage) (interface{}, error) {
	var params struct {
		ProjectID string `json:"project_id"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}

	projectID := params.ProjectID
	if projectID == "" {
		projectID = h.projectID
	}
	if projectID == "" {
		return nil, fmt.Errorf("project_id is required")
	}

	path := fmt.Sprintf("/c1_channels?project_id=eq.%s&order=name&select=id,name,channel_type,metadata,created_at,updated_at", projectID)
	respBody, err := h.httpGet(path)
	if err != nil {
		return nil, err
	}

	var channels []map[string]interface{}
	if err := json.Unmarshal(respBody, &channels); err != nil {
		return nil, fmt.Errorf("unmarshaling channels: %w", err)
	}

	return map[string]interface{}{
		"project_id": projectID,
		"count":      len(channels),
		"channels":   channels,
	}, nil
}

// CreateChannel creates a new channel and adds the creator as a participant.
func (h *C1Handler) CreateChannel(args json.RawMessage) (interface{}, error) {
	var params struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		ChannelType string `json:"channel_type"`
		ProjectID   string `json:"project_id"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}

	if params.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if params.ChannelType == "" {
		params.ChannelType = "topic"
	}

	projectID := params.ProjectID
	if projectID == "" {
		projectID = h.projectID
	}
	if projectID == "" {
		return nil, fmt.Errorf("project_id is required")
	}

	// Build metadata with description if provided
	metadata := make(map[string]interface{})
	if params.Description != "" {
		metadata["description"] = params.Description
	}

	// Create channel
	channelBody := map[string]interface{}{
		"project_id":   projectID,
		"name":         params.Name,
		"channel_type": params.ChannelType,
		"metadata":     metadata,
	}

	respBody, err := h.httpPost("/c1_channels", channelBody)
	if err != nil {
		return nil, err
	}

	var channels []map[string]interface{}
	if err := json.Unmarshal(respBody, &channels); err != nil {
		return nil, fmt.Errorf("unmarshaling created channel: %w", err)
	}

	if len(channels) == 0 {
		return nil, fmt.Errorf("no channel returned after creation")
	}

	channel := channels[0]
	channelID, ok := channel["id"].(string)
	if !ok {
		return nil, fmt.Errorf("channel id not found in response")
	}

	// Add creator as participant (using "system" as default creator)
	participantBody := map[string]interface{}{
		"channel_id":     channelID,
		"participant_id": "system",
	}

	_, err = h.httpPost("/c1_participants", participantBody)
	if err != nil {
		// Log error but don't fail the whole operation
		return map[string]interface{}{
			"status":              "created",
			"channel":             channel,
			"participant_warning": fmt.Sprintf("failed to add creator: %v", err),
		}, nil
	}

	return map[string]interface{}{
		"status":  "created",
		"channel": channel,
	}, nil
}

// RegisterC1Handlers registers C1 Hub MCP tools.
func RegisterC1Handlers(reg *mcp.Registry, handler *C1Handler) {
	reg.Register(mcp.ToolSchema{
		Name:        "c1_list_channels",
		Description: "List all channels for a project in C1 Hub",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"project_id": map[string]interface{}{
					"type":        "string",
					"description": "Project UUID (optional, uses config default if not provided)",
				},
			},
		},
	}, handler.ListChannels)

	reg.Register(mcp.ToolSchema{
		Name:        "c1_create_channel",
		Description: "Create a new channel in C1 Hub and add creator as participant",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"name": map[string]interface{}{
					"type":        "string",
					"description": "Channel name",
				},
				"description": map[string]interface{}{
					"type":        "string",
					"description": "Channel description (stored in metadata)",
				},
				"channel_type": map[string]interface{}{
					"type":        "string",
					"description": "Channel type: topic, dm, or auto (default: topic)",
				},
				"project_id": map[string]interface{}{
					"type":        "string",
					"description": "Project UUID (optional, uses config default if not provided)",
				},
			},
			"required": []string{"name"},
		},
	}, handler.CreateChannel)
}
