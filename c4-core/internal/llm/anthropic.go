package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	defaultAnthropicBaseURL = "https://api.anthropic.com"
	anthropicAPIVersion     = "2023-06-01"
	anthropicBetaCaching    = "prompt-caching-2024-07-31"
)

var _ Provider = (*AnthropicProvider)(nil)

// AnthropicProvider implements the Provider interface for Anthropic's Messages API.
type AnthropicProvider struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

// NewAnthropicProvider creates a new Anthropic provider.
// If baseURL is empty, the default Anthropic API URL is used.
func NewAnthropicProvider(apiKey, baseURL string) *AnthropicProvider {
	if baseURL == "" {
		baseURL = defaultAnthropicBaseURL
	}
	return &AnthropicProvider{
		apiKey:  apiKey,
		baseURL: strings.TrimRight(baseURL, "/"),
		client:  &http.Client{Timeout: 120 * time.Second},
	}
}

func (p *AnthropicProvider) Name() string { return "anthropic" }

func (p *AnthropicProvider) IsAvailable() bool { return p.apiKey != "" }

func (p *AnthropicProvider) Models() []ModelInfo {
	var models []ModelInfo
	for _, m := range Catalog {
		if strings.HasPrefix(m.ID, "claude-") {
			models = append(models, m)
		}
	}
	return models
}

// anthropicCacheControl specifies the cache control type for a content block.
type anthropicCacheControl struct {
	Type string `json:"type"` // "ephemeral"
}

// anthropicSystemBlock is a content block for the system prompt (supports cache_control).
type anthropicSystemBlock struct {
	Type         string                 `json:"type"`
	Text         string                 `json:"text"`
	CacheControl *anthropicCacheControl `json:"cache_control,omitempty"`
}

// anthropicTool is the wire format for a tool in the Anthropic Messages API.
// It mirrors Tool but adds an optional cache_control field.
type anthropicTool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	InputSchema map[string]any         `json:"input_schema,omitempty"`
	CacheControl *anthropicCacheControl `json:"cache_control,omitempty"`
}

// anthropicRequest is the request body for the Anthropic Messages API.
// System is json.RawMessage to support both plain string and content block array.
type anthropicRequest struct {
	Model     string          `json:"model"`
	MaxTokens int             `json:"max_tokens"`
	System    json.RawMessage `json:"system,omitempty"`
	Messages  []Message       `json:"messages"`
	Tools     []anthropicTool `json:"tools,omitempty"`
}

// anthropicResponse is the response from the Anthropic Messages API.
type anthropicResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Model      string `json:"model"`
	StopReason string `json:"stop_reason"`
	Usage struct {
		InputTokens              int `json:"input_tokens"`
		OutputTokens             int `json:"output_tokens"`
		CacheCreationInputTokens int `json:"cache_creation_input_tokens"` // Claude 3.x
		CacheReadInputTokens     int `json:"cache_read_input_tokens"`
		CacheCreation            struct {                                 // Claude 4.x
			Ephemeral5mInputTokens int `json:"ephemeral_5m_input_tokens"`
			Ephemeral1hInputTokens int `json:"ephemeral_1h_input_tokens"`
		} `json:"cache_creation"`
	} `json:"usage"`
}

// anthropicErrorResponse is the error response from the Anthropic API.
type anthropicErrorResponse struct {
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

func (p *AnthropicProvider) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	apiReq := anthropicRequest{
		Model:     req.Model,
		MaxTokens: req.MaxTokens,
		Messages:  req.Messages,
	}
	if apiReq.MaxTokens == 0 {
		apiReq.MaxTokens = 4096
	}

	// System prompt: plain string or content block with cache_control
	if req.System != "" {
		if req.CacheSystemPrompt {
			blocks := []anthropicSystemBlock{{
				Type:         "text",
				Text:         req.System,
				CacheControl: &anthropicCacheControl{Type: "ephemeral"},
			}}
			apiReq.System, _ = json.Marshal(blocks)
		} else {
			apiReq.System, _ = json.Marshal(req.System)
		}
	}

	// Tools: convert to wire format, optionally attaching cache_control to the last tool.
	if len(req.Tools) > 0 {
		tools := make([]anthropicTool, len(req.Tools))
		for i, t := range req.Tools {
			tools[i] = anthropicTool{
				Name:        t.Name,
				Description: t.Description,
				InputSchema: t.InputSchema,
			}
		}
		if req.CacheTools {
			tools[len(tools)-1].CacheControl = &anthropicCacheControl{Type: "ephemeral"}
		}
		apiReq.Tools = tools
	}

	body, err := json.Marshal(apiReq)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", p.apiKey)
	httpReq.Header.Set("anthropic-version", anthropicAPIVersion)
	if req.CacheSystemPrompt || req.CacheTools {
		httpReq.Header.Set("anthropic-beta", anthropicBetaCaching)
	}
	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<22)) // 4 MiB max
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp anthropicErrorResponse
		if json.Unmarshal(respBody, &errResp) == nil && errResp.Error.Message != "" {
			return nil, fmt.Errorf("anthropic API error (%d): %s", resp.StatusCode, errResp.Error.Message)
		}
		return nil, fmt.Errorf("anthropic API error (%d): %s", resp.StatusCode, string(respBody))
	}

	var apiResp anthropicResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	var content string
	for _, c := range apiResp.Content {
		if c.Type == "text" {
			content += c.Text
		}
	}

	return &ChatResponse{
		Content:      content,
		Model:        apiResp.Model,
		FinishReason: apiResp.StopReason,
		Usage: TokenUsage{
			InputTokens:      apiResp.Usage.InputTokens,
			OutputTokens:     apiResp.Usage.OutputTokens,
			CacheReadTokens: apiResp.Usage.CacheReadInputTokens,
			// Claude 3.x: CacheCreationInputTokens; Claude 4.x: CacheCreation.Ephemeral* fields.
			// Sum all to support both API versions.
			CacheWriteTokens: apiResp.Usage.CacheCreationInputTokens +
				apiResp.Usage.CacheCreation.Ephemeral5mInputTokens +
				apiResp.Usage.CacheCreation.Ephemeral1hInputTokens,
		},
	}, nil
}
