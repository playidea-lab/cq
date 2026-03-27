//go:build llm_gateway

package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/changmin/c4-core/internal/llm"
)

func init() {
	captureSessionSummarizeFn = captureSessionSummarizeLLM
}

// captureSessionSummarizeLLM calls the LLM gateway to produce a 2-line session summary.
// Timeout is 5 seconds; on any failure returns "".
func captureSessionSummarizeLLM(jsonlPath, project, date string) string {
	conv := captureSessionExtractConversation(jsonlPath)
	if strings.TrimSpace(conv) == "" {
		return ""
	}
	conv = captureSessionTruncate(conv, 8000)

	prompt := fmt.Sprintf(`다음 AI 대화 세션을 2줄로 요약하세요. 핵심 결정사항과 구현 내용만.

프로젝트: %s (%s)

대화:
%s

요약 (2줄):`, project, date, conv)

	gw, err := buildCaptureSessionGateway()
	if err != nil {
		return ""
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := gw.Chat(ctx, "session_capture", &llm.ChatRequest{
		MaxTokens: 128,
		Messages: []llm.Message{
			{Role: "user", Content: prompt},
		},
	})
	if err != nil {
		return ""
	}
	return strings.TrimSpace(resp.Content)
}

// buildCaptureSessionGateway builds a minimal LLM gateway for session capture.
// Returns error if no providers are available (e.g. no API key, no cloud session).
func buildCaptureSessionGateway() (*llm.Gateway, error) {
	result, err := buildLLMGateway()
	if err != nil {
		fmt.Fprintf(os.Stderr, "cq: captureSession: LLM gateway unavailable: %v\n", err)
		return nil, err
	}
	return result.gateway, nil
}
