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

const defaultOllamaBaseURL = "http://localhost:11434"

var (
	_ Provider      = (*OllamaProvider)(nil)
	_ EmbedProvider = (*OllamaProvider)(nil)
)

// OllamaProvider implements the Provider interface for the Ollama local API.
type OllamaProvider struct {
	baseURL string
	client  *http.Client
}

// NewOllamaProvider creates a new Ollama provider.
// If baseURL is empty, the default localhost URL is used.
func NewOllamaProvider(baseURL string) *OllamaProvider {
	if baseURL == "" {
		baseURL = defaultOllamaBaseURL
	}
	return &OllamaProvider{
		baseURL: strings.TrimRight(baseURL, "/"),
		client:  &http.Client{Timeout: 300 * time.Second},
	}
}

func (p *OllamaProvider) Name() string { return "ollama" }

func (p *OllamaProvider) IsAvailable() bool { return p.baseURL != "" }

func (p *OllamaProvider) Models() []ModelInfo {
	var models []ModelInfo
	for _, m := range Catalog {
		if strings.Contains(m.ID, ":") {
			models = append(models, m)
		}
	}
	return models
}

// ollamaRequest is the request body for the Ollama Chat API.
type ollamaRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Stream   bool      `json:"stream"`
}

// ollamaResponse is the response from the Ollama Chat API.
type ollamaResponse struct {
	Model   string `json:"model"`
	Message struct {
		Content string `json:"content"`
	} `json:"message"`
	Done           bool `json:"done"`
	EvalCount      int  `json:"eval_count"`
	PromptEvalCount int `json:"prompt_eval_count"`
}

// ollamaErrorResponse is the error response from the Ollama API.
type ollamaErrorResponse struct {
	Error string `json:"error"`
}

// ollamaEmbedRequest is the request body for the Ollama Embed API.
type ollamaEmbedRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

// ollamaEmbedResponse is the response from the Ollama Embed API.
type ollamaEmbedResponse struct {
	Model      string      `json:"model"`
	Embeddings [][]float32 `json:"embeddings"`
}

// Embed generates embeddings using the Ollama /api/embed endpoint.
// Model defaults to "nomic-embed-text" (768 dims) if not specified.
func (p *OllamaProvider) Embed(ctx context.Context, texts []string, model string) (*EmbedResponse, error) {
	if model == "" {
		model = "nomic-embed-text"
	}

	apiReq := ollamaEmbedRequest{
		Model: model,
		Input: texts,
	}

	body, err := json.Marshal(apiReq)
	if err != nil {
		return nil, fmt.Errorf("marshal embed request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/api/embed", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create embed request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("embed http request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read embed response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp ollamaErrorResponse
		if json.Unmarshal(respBody, &errResp) == nil && errResp.Error != "" {
			return nil, fmt.Errorf("ollama embed API error (%d): %s", resp.StatusCode, errResp.Error)
		}
		return nil, fmt.Errorf("ollama embed API error (%d): %s", resp.StatusCode, string(respBody))
	}

	var apiResp ollamaEmbedResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("unmarshal embed response: %w", err)
	}

	return &EmbedResponse{
		Embeddings: apiResp.Embeddings,
		Model:      apiResp.Model,
		Usage:      TokenUsage{InputTokens: len(texts)},
	}, nil
}

func (p *OllamaProvider) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	// System prompt goes as a system role message prepended to the conversation
	messages := make([]Message, 0, len(req.Messages)+1)
	if req.System != "" {
		messages = append(messages, Message{Role: "system", Content: req.System})
	}
	messages = append(messages, req.Messages...)

	apiReq := ollamaRequest{
		Model:    req.Model,
		Messages: messages,
		Stream:   false,
	}

	body, err := json.Marshal(apiReq)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp ollamaErrorResponse
		if json.Unmarshal(respBody, &errResp) == nil && errResp.Error != "" {
			return nil, fmt.Errorf("ollama API error (%d): %s", resp.StatusCode, errResp.Error)
		}
		return nil, fmt.Errorf("ollama API error (%d): %s", resp.StatusCode, string(respBody))
	}

	var apiResp ollamaResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	finishReason := ""
	if apiResp.Done {
		finishReason = "stop"
	}

	return &ChatResponse{
		Content:      apiResp.Message.Content,
		Model:        apiResp.Model,
		FinishReason: finishReason,
		Usage: TokenUsage{
			InputTokens:  apiResp.PromptEvalCount,
			OutputTokens: apiResp.EvalCount,
		},
	}, nil
}
