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

var _ Provider = (*CQProxyProvider)(nil)

// CQProxyProvider relays LLM requests to the CQ Edge Function using Bearer JWT auth.
// It reuses anthropicRequest/anthropicResponse wire types (same package) since the
// Edge Function exposes an Anthropic-compatible Messages API endpoint.
type CQProxyProvider struct {
	baseURL   string
	tokenFunc func() string
	client    *http.Client
}

// NewCQProxyProvider creates a new CQProxyProvider.
// tokenFunc is called per-request to supply a fresh Bearer JWT; nil disables the provider.
func NewCQProxyProvider(baseURL string, tokenFunc func() string) *CQProxyProvider {
	return &CQProxyProvider{
		baseURL:   strings.TrimRight(baseURL, "/"),
		tokenFunc: tokenFunc,
		client:    &http.Client{Timeout: 120 * time.Second},
	}
}

func (p *CQProxyProvider) Name() string { return "cq-proxy" }

func (p *CQProxyProvider) IsAvailable() bool { return p.tokenFunc != nil }

// Models returns only haiku models — lightweight models suitable for proxy routing.
func (p *CQProxyProvider) Models() []ModelInfo {
	var models []ModelInfo
	for _, m := range Catalog {
		if strings.Contains(m.ID, "haiku") {
			models = append(models, m)
		}
	}
	return models
}

func (p *CQProxyProvider) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	apiReq := anthropicRequest{
		Model:     req.Model,
		MaxTokens: req.MaxTokens,
		Messages:  req.Messages,
	}
	if apiReq.MaxTokens == 0 {
		apiReq.MaxTokens = 4096
	}

	if req.System != "" {
		apiReq.System, _ = json.Marshal(req.System)
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
	httpReq.Header.Set("Authorization", "Bearer "+p.tokenFunc())

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
			return nil, fmt.Errorf("cq-proxy error (%d): %s", resp.StatusCode, errResp.Error.Message)
		}
		return nil, fmt.Errorf("cq-proxy error (%d): %s", resp.StatusCode, string(respBody))
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
			InputTokens:  apiResp.Usage.InputTokens,
			OutputTokens: apiResp.Usage.OutputTokens,
		},
	}, nil
}
