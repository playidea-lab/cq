//go:build llm_gateway

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/changmin/c4-core/internal/llm"
)

func init() {
	sessionCloseSummarizeFn = sessionCloseSummarizeLLM
}

// sessionCloseSummarizeLLM calls the LLM gateway for a combined summary + persona extraction.
func sessionCloseSummarizeLLM(jsonlPath, project, date string) *sessionCloseResult {
	conv := captureSessionExtractConversation(jsonlPath)
	if strings.TrimSpace(conv) == "" {
		return nil
	}
	conv = captureSessionTruncate(conv, 8000)

	prompt := buildSessionClosePrompt(project, date, conv)

	gw, err := buildSessionCloseGateway()
	if err != nil {
		fmt.Fprintf(os.Stderr, "cq: session close gateway build failed: %v\n", err)
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := gw.Chat(ctx, "session_summarize", &llm.ChatRequest{
		Model:     "cq-proxy/claude-haiku-4-5-20251001",
		MaxTokens: 1024,
		Messages: []llm.Message{
			{Role: "user", Content: prompt},
		},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "cq: session close LLM failed: %v\n", err)
		return nil
	}

	return parseSessionCloseResponse(resp.Content)
}

// buildSessionClosePrompt creates the combined summarization + persona extraction prompt.
// Extracts deep knowledge: why decisions were made, what failed, quantitative results.
func buildSessionClosePrompt(project, date, conversation string) string {
	return fmt.Sprintf(`다음은 %s 프로젝트의 %s AI 대화 세션입니다.

아래 3가지를 추출해 주세요. **표면적 요약이 아니라 실질 지식을 추출하세요.**

## 1. 세션 요약
이 세션에서 한 일과 결과. 반드시 포함:
- **왜**: 이 작업을 한 동기/맥락
- **결과**: 구체적 산출물 (수치, 파일명, 상태 변화)
- **실패한 접근**: 시도했지만 안 된 것과 그 이유 (있다면)
- **발견**: 예상과 달랐던 것, 새로 알게 된 사실
5-10줄로 작성.

## 2. 결정사항
기술적/설계적 결정을 "무엇을 + 왜" 형식으로. 대안이 있었다면 포함.
결정사항이 없으면 빈 배열.
` + "```json" + `
{"decisions": ["X 대신 Y 선택: Z 때문에", "A 방식 채택: B 제약 조건으로 인해"]}
` + "```" + `

## 3. 선호
사용자가 표현한 작업 방식 선호도를 JSON 배열로.
(코딩 스타일은 제외 — rules/CLAUDE.md 영역)
선호가 없으면 빈 배열.
` + "```json" + `
{"preferences": ["선호1", "선호2"]}
` + "```" + `

---
대화 내용:
%s`, project, date, conversation)
}

// parseSessionCloseResponse parses the LLM response into structured data.
func parseSessionCloseResponse(content string) *sessionCloseResult {
	result := &sessionCloseResult{}

	// Extract summary: text before first ```json block
	lines := strings.Split(content, "\n")
	var summaryLines []string
	inJSON := false
	jsonBlocks := []string{}
	var currentJSON strings.Builder

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```json") {
			inJSON = true
			currentJSON.Reset()
			continue
		}
		if inJSON && strings.HasPrefix(trimmed, "```") {
			inJSON = false
			jsonBlocks = append(jsonBlocks, currentJSON.String())
			continue
		}
		if inJSON {
			currentJSON.WriteString(line)
			currentJSON.WriteString("\n")
		} else if len(jsonBlocks) == 0 {
			// Before first JSON block = summary area
			if trimmed != "" && !strings.HasPrefix(trimmed, "## ") && !strings.HasPrefix(trimmed, "#") {
				summaryLines = append(summaryLines, line)
			}
		}
	}

	result.Summary = strings.TrimSpace(strings.Join(summaryLines, "\n"))

	// Parse JSON blocks
	for _, block := range jsonBlocks {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}
		var data map[string][]string
		if err := json.Unmarshal([]byte(block), &data); err != nil {
			continue
		}
		if d, ok := data["decisions"]; ok {
			result.Decisions = d
		}
		if p, ok := data["preferences"]; ok {
			result.Preferences = p
		}
	}

	return result
}

// buildSessionCloseGateway builds a minimal LLM gateway for session close.
func buildSessionCloseGateway() (*llm.Gateway, error) {
	r, err := buildLLMGateway()
	if err != nil {
		fmt.Fprintf(os.Stderr, "cq: sessionClose: LLM gateway unavailable: %v\n", err)
		return nil, err
	}
	return r.gateway, nil
}
