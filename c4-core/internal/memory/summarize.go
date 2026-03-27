package memory

import (
	"context"
	"fmt"
	"strings"

	"github.com/changmin/c4-core/internal/llm"
)

const (
	// approxCharsPerToken is a rough char-to-token estimate (4 chars ~ 1 token).
	approxCharsPerToken = 4
	// maxInputTokens is the token budget for conversation text sent to the LLM.
	maxInputTokens = 8000
)

// Summarizer extracts knowledge from conversation sessions via LLM.
type Summarizer struct {
	LLM   *llm.Gateway // LLM gateway for summarization calls
	Model string       // optional model hint (empty = let gateway choose)
}

// Summarize takes a Session and returns a Markdown summary string.
func (s *Summarizer) Summarize(ctx context.Context, sess Session) (string, error) {
	conv := formatConversation(sess)
	if strings.TrimSpace(conv) == "" {
		return "", fmt.Errorf("empty conversation for session %s", sess.ID)
	}

	conv = truncateToTokens(conv, maxInputTokens)

	project := sess.Project
	if project == "" {
		project = "unknown"
	}
	date := sess.StartedAt.Format("2006-01-02")
	prompt := buildImportPrompt(project, sess.Source, date, conv)

	if s.LLM == nil {
		// Fallback: return truncated conversation as-is when no LLM is available.
		return fmt.Sprintf("## 세션: %s (%s, %s)\n\n%s", project, sess.Source, date, conv), nil
	}

	req := &llm.ChatRequest{
		Messages: []llm.Message{
			{Role: "user", Content: prompt},
		},
		MaxTokens: 1024,
	}
	if s.Model != "" {
		req.Model = s.Model
	}

	resp, err := s.LLM.Chat(ctx, "memory_import", req)
	if err != nil {
		return "", fmt.Errorf("LLM summarization failed: %w", err)
	}

	summary := strings.TrimSpace(resp.Content)
	if summary == "" {
		return "", fmt.Errorf("LLM returned empty summary for session %s", sess.ID)
	}
	return summary, nil
}

// formatConversation converts session turns into a plain text conversation.
func formatConversation(sess Session) string {
	var sb strings.Builder
	for _, t := range sess.Turns {
		text := strings.TrimSpace(t.Content)
		if text == "" {
			continue
		}
		prefix := "User"
		if t.Role == "assistant" {
			prefix = "Assistant"
		}
		sb.WriteString(prefix)
		sb.WriteString(": ")
		sb.WriteString(text)
		sb.WriteString("\n\n")
	}
	return sb.String()
}

// truncateToTokens truncates text to approximately maxTokens tokens,
// keeping the most recent (tail) portion which is typically most relevant.
func truncateToTokens(text string, maxTokens int) string {
	maxChars := maxTokens * approxCharsPerToken
	if len(text) <= maxChars {
		return text
	}
	truncated := text[len(text)-maxChars:]
	if idx := strings.Index(truncated, "\n"); idx >= 0 && idx < 200 {
		truncated = "[이전 대화 생략]\n\n" + truncated[idx+1:]
	} else {
		truncated = "[이전 대화 생략]\n\n" + truncated
	}
	return truncated
}

// buildImportPrompt constructs the LLM prompt for session summarization.
func buildImportPrompt(project, source, date, conversation string) string {
	return fmt.Sprintf(`다음은 %s 프로젝트에서 %s를 사용한 %s 날의 AI 대화 세션입니다.

아래 형식으로 핵심 내용을 요약해 주세요. 대화가 한국어이면 한국어로, 영어이면 영어로 작성하세요.

## 주제
- (이 세션의 주요 주제/목표)

## 결정사항
- (내린 주요 기술적/설계적 결정사항)

## 발견/학습
- (새로 알게 된 사실이나 인사이트)

## 미해결
- (완료하지 못했거나 향후 해결이 필요한 문제)

---
대화 내용:
%s`, project, source, date, conversation)
}
