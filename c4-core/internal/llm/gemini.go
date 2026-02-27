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

const defaultGeminiBaseURL = "https://generativelanguage.googleapis.com"

var _ Provider = (*GeminiProvider)(nil)

// GeminiProvider implements the Provider interface for Google's Gemini GenerateContent API.
type GeminiProvider struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

// NewGeminiProvider creates a new Gemini provider.
// If baseURL is empty, the default Gemini API URL is used.
func NewGeminiProvider(apiKey, baseURL string) *GeminiProvider {
	if baseURL == "" {
		baseURL = defaultGeminiBaseURL
	}
	return &GeminiProvider{
		apiKey:  apiKey,
		baseURL: strings.TrimRight(baseURL, "/"),
		client:  &http.Client{Timeout: 120 * time.Second},
	}
}

func (p *GeminiProvider) Name() string { return "gemini" }

func (p *GeminiProvider) IsAvailable() bool { return p.apiKey != "" }

func (p *GeminiProvider) Models() []ModelInfo {
	var models []ModelInfo
	for _, m := range Catalog {
		if strings.HasPrefix(m.ID, "gemini-") {
			models = append(models, m)
		}
	}
	return models
}

// geminiRequest is the request body for the Gemini GenerateContent API.
type geminiRequest struct {
	Contents          []geminiContent          `json:"contents"`
	SystemInstruction *geminiSystemInstruction `json:"systemInstruction,omitempty"`
	GenerationConfig  *geminiGenerationConfig  `json:"generationConfig,omitempty"`
}

type geminiContent struct {
	Role  string       `json:"role"`
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text string `json:"text"`
}

type geminiSystemInstruction struct {
	Parts []geminiPart `json:"parts"`
}

type geminiGenerationConfig struct {
	MaxOutputTokens int     `json:"maxOutputTokens,omitempty"`
	Temperature     float64 `json:"temperature,omitempty"`
}

// geminiResponse is the response from the Gemini GenerateContent API.
type geminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
		FinishReason string `json:"finishReason"`
	} `json:"candidates"`
	UsageMetadata struct {
		PromptTokenCount     int `json:"promptTokenCount"`
		CandidatesTokenCount int `json:"candidatesTokenCount"`
	} `json:"usageMetadata"`
}

// geminiErrorResponse is the error response from the Gemini API.
type geminiErrorResponse struct {
	Error struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
	} `json:"error"`
}

func (p *GeminiProvider) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	// Map messages: "assistant" -> "model" for Gemini
	contents := make([]geminiContent, 0, len(req.Messages))
	for _, m := range req.Messages {
		role := m.Role
		if role == "assistant" {
			role = "model"
		}
		contents = append(contents, geminiContent{
			Role:  role,
			Parts: []geminiPart{{Text: m.Content}},
		})
	}

	apiReq := geminiRequest{Contents: contents}

	if req.System != "" {
		apiReq.SystemInstruction = &geminiSystemInstruction{
			Parts: []geminiPart{{Text: req.System}},
		}
	}

	if req.MaxTokens > 0 || req.Temperature > 0 {
		apiReq.GenerationConfig = &geminiGenerationConfig{
			MaxOutputTokens: req.MaxTokens,
			Temperature:     req.Temperature,
		}
	}

	body, err := json.Marshal(apiReq)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	// Gemini uses API key as query parameter, model name in URL path
	url := fmt.Sprintf("%s/v1beta/models/%s:generateContent?key=%s", p.baseURL, req.Model, p.apiKey)

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
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
		var errResp geminiErrorResponse
		if json.Unmarshal(respBody, &errResp) == nil && errResp.Error.Message != "" {
			return nil, fmt.Errorf("gemini API error (%d): %s", resp.StatusCode, errResp.Error.Message)
		}
		return nil, fmt.Errorf("gemini API error (%d): %s", resp.StatusCode, string(respBody))
	}

	var apiResp geminiResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	var content, finishReason string
	if len(apiResp.Candidates) > 0 {
		finishReason = apiResp.Candidates[0].FinishReason
		for _, part := range apiResp.Candidates[0].Content.Parts {
			content += part.Text
		}
	}

	return &ChatResponse{
		Content:      content,
		Model:        req.Model,
		FinishReason: finishReason,
		Usage: TokenUsage{
			InputTokens:  apiResp.UsageMetadata.PromptTokenCount,
			OutputTokens: apiResp.UsageMetadata.CandidatesTokenCount,
		},
	}, nil
}
