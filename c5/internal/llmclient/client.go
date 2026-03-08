// Package llmclient provides an OpenAI-compatible chat completion client.
// It supports Gemini, OpenAI, and Ollama backends via the /v1/chat/completions endpoint,
// and the official Anthropic Messages API.
package llmclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// chatProvider is the internal abstraction for a chat backend.
type chatProvider interface {
	chatWithHistory(ctx context.Context, system string, history []Message, userMsg string) (string, error)
}

// Message is an exported chat message for multi-turn conversations.
type Message struct {
	Role    string // "user" or "assistant"
	Content string
}

// Client is the public handle for chat completion.
type Client struct {
	p chatProvider
}

// New creates a Client backed by an OpenAI-compatible endpoint.
// baseURL should be the provider's OpenAI-compatible endpoint prefix
// (e.g. "https://generativelanguage.googleapis.com/v1beta/openai" for Gemini,
// "http://localhost:11434/v1" for Ollama, "https://api.openai.com/v1" for OpenAI).
// If maxTokens <= 0, 4096 is used.
func New(baseURL, apiKey, model string, maxTokens int) *Client {
	if maxTokens <= 0 {
		maxTokens = 4096
	}
	return &Client{p: &openAIProvider{
		baseURL: baseURL,
		apiKey:  apiKey,
		model:   model,
		maxTok:  maxTokens,
		client:  &http.Client{Timeout: 60 * time.Second},
	}}
}

// NewAnthropic creates a Client backed by the official Anthropic Messages API.
// If maxTokens <= 0, 4096 is used.
func NewAnthropic(apiKey, model string, maxTokens int) *Client {
	if maxTokens <= 0 {
		maxTokens = 4096
	}
	return &Client{p: &anthropicProvider{
		apiKey: apiKey,
		model:  model,
		maxTok: maxTokens,
		client: &http.Client{Timeout: 60 * time.Second},
	}}
}

// Chat sends a chat completion request with the given system and user messages.
func (c *Client) Chat(ctx context.Context, system, userMsg string) (string, error) {
	return c.ChatWithHistory(ctx, system, nil, userMsg)
}

// ChatWithHistory sends a multi-turn chat completion request.
func (c *Client) ChatWithHistory(ctx context.Context, system string, history []Message, userMsg string) (string, error) {
	return c.p.chatWithHistory(ctx, system, history, userMsg)
}

// --- OpenAI-compatible provider ---

type openAIProvider struct {
	baseURL string
	apiKey  string
	model   string
	maxTok  int
	client  *http.Client
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model     string        `json:"model"`
	Messages  []chatMessage `json:"messages"`
	MaxTokens int           `json:"max_tokens,omitempty"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Code    any    `json:"code"`
	} `json:"error,omitempty"`
}

func (o *openAIProvider) chatWithHistory(ctx context.Context, system string, history []Message, userMsg string) (string, error) {
	if o.apiKey == "" {
		return "", fmt.Errorf("llmclient: apiKey is required")
	}

	var messages []chatMessage
	if system != "" {
		messages = append(messages, chatMessage{Role: "system", Content: system})
	}
	for _, m := range history {
		messages = append(messages, chatMessage{Role: m.Role, Content: m.Content})
	}
	messages = append(messages, chatMessage{Role: "user", Content: userMsg})

	reqBody := chatRequest{
		Model:     o.model,
		Messages:  messages,
		MaxTokens: o.maxTok,
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("llmclient: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.baseURL+"/chat/completions", bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("llmclient: create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+o.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := o.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("llmclient: http: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<22))
	if err != nil {
		return "", fmt.Errorf("llmclient: read body: %w", err)
	}

	var cr chatResponse
	if err := json.Unmarshal(body, &cr); err != nil {
		return "", fmt.Errorf("llmclient: decode response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		errBody := body
		if len(errBody) > 512 {
			errBody = errBody[:512]
		}
		return "", fmt.Errorf("llmclient: unexpected status %d: %s", resp.StatusCode, string(errBody))
	}
	if cr.Error != nil {
		return "", fmt.Errorf("llmclient: API error: %s", cr.Error.Message)
	}
	if len(cr.Choices) == 0 {
		return "", fmt.Errorf("llmclient: no choices in response")
	}

	return cr.Choices[0].Message.Content, nil
}

// --- Anthropic provider ---

type anthropicProvider struct {
	apiKey string
	model  string
	maxTok int
	client *http.Client
}

type anthropicRequest struct {
	Model     string        `json:"model"`
	MaxTokens int           `json:"max_tokens"`
	System    string        `json:"system,omitempty"`
	Messages  []chatMessage `json:"messages"`
}

type anthropicResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	StopReason string `json:"stop_reason"`
	Type       string `json:"type"`
	Error      *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (a *anthropicProvider) chatWithHistory(ctx context.Context, system string, history []Message, userMsg string) (string, error) {
	if a.apiKey == "" {
		return "", fmt.Errorf("llmclient: apiKey is required")
	}

	var messages []chatMessage
	for _, m := range history {
		messages = append(messages, chatMessage{Role: m.Role, Content: m.Content})
	}
	messages = append(messages, chatMessage{Role: "user", Content: userMsg})

	reqBody := anthropicRequest{
		Model:     a.model,
		MaxTokens: a.maxTok,
		System:    system,
		Messages:  messages,
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("llmclient: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.anthropic.com/v1/messages", bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("llmclient: create request: %w", err)
	}
	req.Header.Set("x-api-key", a.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("llmclient: http: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<22))
	if err != nil {
		return "", fmt.Errorf("llmclient: read body: %w", err)
	}

	var ar anthropicResponse
	if err := json.Unmarshal(body, &ar); err != nil {
		return "", fmt.Errorf("llmclient: decode response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		if ar.Error != nil {
			return "", fmt.Errorf("llmclient: API error: %s", ar.Error.Message)
		}
		errBody := body
		if len(errBody) > 512 {
			errBody = errBody[:512]
		}
		return "", fmt.Errorf("llmclient: unexpected status %d: %s", resp.StatusCode, string(errBody))
	}
	if len(ar.Content) == 0 {
		return "", fmt.Errorf("llmclient: no content in response")
	}

	return ar.Content[0].Text, nil
}
