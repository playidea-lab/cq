// Package llmclient provides an OpenAI-compatible chat completion client.
// It supports Gemini, OpenAI, and Ollama backends via the /v1/chat/completions endpoint.
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

// Client is a minimal OpenAI-compatible chat completion client.
type Client struct {
	baseURL string
	apiKey  string
	model   string
	maxTok  int
	client  *http.Client
}

// New creates a Client. baseURL should be the provider's OpenAI-compatible endpoint prefix
// (e.g. "https://generativelanguage.googleapis.com/v1beta/openai" for Gemini,
// "http://localhost:11434/v1" for Ollama, "https://api.openai.com/v1" for OpenAI).
// If model is empty, "gemini-3.0-flash" is used. If maxTokens <= 0, 4096 is used.
func New(baseURL, apiKey, model string, maxTokens int) *Client {
	if model == "" {
		model = "gemini-3.0-flash"
	}
	if maxTokens <= 0 {
		maxTokens = 4096
	}
	return &Client{
		baseURL: baseURL,
		apiKey:  apiKey,
		model:   model,
		maxTok:  maxTokens,
		client:  &http.Client{Timeout: 60 * time.Second},
	}
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

// Chat sends a chat completion request with the given system and user messages.
// Returns an error if apiKey is empty or if the API returns a non-200 status or error body.
func (c *Client) Chat(ctx context.Context, system, userMsg string) (string, error) {
	if c.apiKey == "" {
		return "", fmt.Errorf("llmclient: apiKey is required")
	}

	var messages []chatMessage
	if system != "" {
		messages = append(messages, chatMessage{Role: "system", Content: system})
	}
	messages = append(messages, chatMessage{Role: "user", Content: userMsg})

	reqBody := chatRequest{
		Model:     c.model,
		Messages:  messages,
		MaxTokens: c.maxTok,
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("llmclient: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("llmclient: create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("llmclient: http: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<22)) // 4 MiB safety cap
	if err != nil {
		return "", fmt.Errorf("llmclient: read body: %w", err)
	}

	var cr chatResponse
	if err := json.Unmarshal(body, &cr); err != nil {
		return "", fmt.Errorf("llmclient: decode response: %w", err)
	}

	if cr.Error != nil {
		return "", fmt.Errorf("llmclient: API error: %s", cr.Error.Message)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("llmclient: unexpected status %d: %s", resp.StatusCode, string(body))
	}
	if len(cr.Choices) == 0 {
		return "", fmt.Errorf("llmclient: no choices in response")
	}

	return cr.Choices[0].Message.Content, nil
}
