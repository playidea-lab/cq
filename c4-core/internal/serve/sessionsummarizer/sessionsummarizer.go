// Package sessionsummarizer provides a serve.Component that asynchronously
// summarizes unsummarized sessions via the LLM gateway and stores results
// in the knowledge store.
package sessionsummarizer

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/changmin/c4-core/internal/knowledge"
	"github.com/changmin/c4-core/internal/llm"
	"github.com/changmin/c4-core/internal/serve"
)

const (
	defaultPollInterval  = 60 * time.Second
	defaultMaxConcurrent = 2
	// approxCharsPerToken is a rough estimate for token truncation (4 chars ≈ 1 token).
	approxCharsPerToken = 4
	// maxInputTokens is the target token budget for the conversation sent to LLM.
	maxInputTokens = 8000
)

// DB is the minimal database interface required by the summarizer.
// Satisfied by *sql.DB.
type DB interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// Config holds configuration for SessionSummarizerComponent.
type Config struct {
	DB             DB
	KnowledgeStore *knowledge.Store
	LLMGateway     *llm.Gateway
	PollInterval   time.Duration // default 60s
	MaxConcurrent  int           // default 2
}

// SessionSummarizerComponent polls for unsummarized sessions every PollInterval
// and calls the LLM gateway to produce structured summaries stored as knowledge docs.
type SessionSummarizerComponent struct {
	cfg    Config
	cancel context.CancelFunc
	done   chan struct{}

	mu     sync.Mutex
	status string
	detail string
}

// compile-time assertion
var _ serve.Component = (*SessionSummarizerComponent)(nil)

// New creates a new SessionSummarizerComponent with production defaults.
func New(cfg Config) *SessionSummarizerComponent {
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = defaultPollInterval
	}
	if cfg.MaxConcurrent <= 0 {
		cfg.MaxConcurrent = defaultMaxConcurrent
	}
	return &SessionSummarizerComponent{cfg: cfg, status: "ok"}
}

// Name implements serve.Component.
func (s *SessionSummarizerComponent) Name() string { return "session-summarizer" }

// Start launches the background polling goroutine.
func (s *SessionSummarizerComponent) Start(ctx context.Context) error {
	ctx2, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	s.done = make(chan struct{})
	go s.loop(ctx2)
	return nil
}

// Stop cancels the background loop and waits for it to exit.
func (s *SessionSummarizerComponent) Stop(_ context.Context) error {
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
		if s.done != nil {
			<-s.done
		}
	}
	return nil
}

// Health implements serve.Component.
func (s *SessionSummarizerComponent) Health() serve.ComponentHealth {
	s.mu.Lock()
	defer s.mu.Unlock()
	return serve.ComponentHealth{Status: s.status, Detail: s.detail}
}

func (s *SessionSummarizerComponent) loop(ctx context.Context) {
	defer close(s.done)
	ticker := time.NewTicker(s.cfg.PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.poll(ctx)
		}
	}
}

// sessionRow represents a row from the sessions table.
type sessionRow struct {
	ID        string
	Tool      string
	Project   sql.NullString
	JSONLPath sql.NullString
	StartedAt sql.NullString
}

func (s *SessionSummarizerComponent) poll(ctx context.Context) {
	if s.cfg.DB == nil {
		return
	}

	rows, err := s.cfg.DB.QueryContext(ctx, `
		SELECT session_id, tool, project, jsonl_path, started_at
		FROM sessions
		WHERE summarized_at IS NULL
		  AND jsonl_path IS NOT NULL
		  AND jsonl_path != ''
		ORDER BY started_at ASC
		LIMIT 10
	`)
	if err != nil {
		s.setStatus("degraded", fmt.Sprintf("query sessions: %v", err))
		return
	}
	defer rows.Close()

	var sessions []sessionRow
	for rows.Next() {
		var r sessionRow
		if err := rows.Scan(&r.ID, &r.Tool, &r.Project, &r.JSONLPath, &r.StartedAt); err != nil {
			continue
		}
		sessions = append(sessions, r)
	}
	if err := rows.Err(); err != nil {
		s.setStatus("degraded", fmt.Sprintf("rows error: %v", err))
		return
	}

	if len(sessions) == 0 {
		s.setStatus("ok", "")
		return
	}

	// Bounded concurrency via semaphore channel.
	sem := make(chan struct{}, s.cfg.MaxConcurrent)
	var wg sync.WaitGroup
	for _, sess := range sessions {
		sem <- struct{}{}
		wg.Add(1)
		go func(sess sessionRow) {
			defer wg.Done()
			defer func() { <-sem }()
			if err := s.summarize(ctx, sess); err != nil {
				// Non-fatal: log and continue.
				fmt.Fprintf(os.Stderr, "session-summarizer: session %s failed: %v\n", sess.ID, err)
			}
		}(sess)
	}
	wg.Wait()
	s.setStatus("ok", "")
}

func (s *SessionSummarizerComponent) summarize(ctx context.Context, sess sessionRow) error {
	// Read conversation from JSONL file.
	conv, err := readConversation(sess.JSONLPath.String)
	if err != nil {
		// Mark as summarized with a note so we don't retry broken files indefinitely.
		return s.markSummarized(ctx, sess.ID, "")
	}
	if strings.TrimSpace(conv) == "" {
		return s.markSummarized(ctx, sess.ID, "")
	}

	// Truncate to ~8K tokens if necessary.
	conv = truncateToTokens(conv, maxInputTokens)

	project := sess.Project.String
	if project == "" {
		project = "unknown"
	}
	date := sess.StartedAt.String
	if date == "" {
		date = time.Now().UTC().Format("2006-01-02")
	} else if len(date) > 10 {
		date = date[:10]
	}

	prompt := buildSummarizationPrompt(project, sess.Tool, date, conv)

	// Call LLM gateway; if unavailable, skip silently (retry next cycle).
	if s.cfg.LLMGateway == nil {
		return nil
	}

	resp, err := s.cfg.LLMGateway.Chat(ctx, "session_summarize", &llm.ChatRequest{
		Messages: []llm.Message{
			{Role: "user", Content: prompt},
		},
		MaxTokens: 1024,
	})
	if err != nil {
		// Skip silently; will retry next poll cycle.
		return nil
	}

	summary := strings.TrimSpace(resp.Content)
	if summary == "" {
		return s.markSummarized(ctx, sess.ID, "")
	}

	// Store in knowledge store.
	var docID string
	if s.cfg.KnowledgeStore != nil {
		title := fmt.Sprintf("세션 요약: %s (%s, %s)", project, sess.Tool, date)
		meta := map[string]any{
			"title":  title,
			"domain": "session",
			"tags":   []string{"session", sess.Tool},
		}
		id, createErr := s.cfg.KnowledgeStore.Create(knowledge.TypeInsight, meta, summary)
		if createErr != nil {
			fmt.Fprintf(os.Stderr, "session-summarizer: knowledge store create failed: %v\n", createErr)
		} else {
			docID = id
		}
	}

	return s.markSummarized(ctx, sess.ID, docID)
}

func (s *SessionSummarizerComponent) markSummarized(ctx context.Context, sessionID, docID string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.cfg.DB.ExecContext(ctx, `
		UPDATE sessions
		SET summarized_at = ?, summary_doc_id = ?
		WHERE session_id = ?
	`, now, docID, sessionID)
	return err
}

func (s *SessionSummarizerComponent) setStatus(status, detail string) {
	s.mu.Lock()
	s.status = status
	s.detail = detail
	s.mu.Unlock()
}

// jsonlEntry is a minimal representation of a JSONL conversation entry.
type jsonlEntry struct {
	Type    string `json:"type"`
	Role    string `json:"role"`
	Message *struct {
		Role    string `json:"role"`
		Content any    `json:"content"` // string or []map[string]any
	} `json:"message"`
	// Some formats use top-level role + content directly.
	Content any `json:"content"`
}

// readConversation reads a JSONL file and extracts human/assistant turns as plain text.
func readConversation(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open jsonl: %w", err)
	}
	defer f.Close()

	var sb strings.Builder
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1<<20), 1<<20) // 1 MiB line buffer
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var entry jsonlEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}

		role := entry.Role
		var contentRaw any = entry.Content

		if entry.Message != nil {
			if entry.Message.Role != "" {
				role = entry.Message.Role
			}
			contentRaw = entry.Message.Content
		}

		if role == "" {
			continue
		}
		if role != "user" && role != "assistant" {
			continue
		}

		text := extractText(contentRaw)
		if text == "" {
			continue
		}
		prefix := "User"
		if role == "assistant" {
			prefix = "Assistant"
		}
		sb.WriteString(prefix)
		sb.WriteString(": ")
		sb.WriteString(text)
		sb.WriteString("\n\n")
	}
	return sb.String(), scanner.Err()
}

// extractText converts a content field (string or []block) to plain text.
func extractText(content any) string {
	switch v := content.(type) {
	case string:
		return strings.TrimSpace(v)
	case []any:
		var parts []string
		for _, block := range v {
			if m, ok := block.(map[string]any); ok {
				if t, ok := m["text"].(string); ok && t != "" {
					parts = append(parts, strings.TrimSpace(t))
				}
			}
		}
		return strings.Join(parts, " ")
	}
	return ""
}

// truncateToTokens truncates text to approximately maxTokens tokens.
// If the text is longer, it keeps the last part (most recent messages are most relevant).
func truncateToTokens(text string, maxTokens int) string {
	maxChars := maxTokens * approxCharsPerToken
	if len(text) <= maxChars {
		return text
	}
	// Keep the last maxChars characters (most recent conversation).
	truncated := text[len(text)-maxChars:]
	// Try to start on a clean line boundary.
	if idx := strings.Index(truncated, "\n"); idx >= 0 && idx < 200 {
		truncated = "[이전 대화 생략]\n\n" + truncated[idx+1:]
	} else {
		truncated = "[이전 대화 생략]\n\n" + truncated
	}
	return truncated
}

// buildSummarizationPrompt constructs the LLM prompt for session summarization.
// Designed to extract actionable knowledge, not just surface-level summaries.
func buildSummarizationPrompt(project, tool, date, conversation string) string {
	return fmt.Sprintf(`다음은 %s 프로젝트에서 %s 도구를 사용한 %s 날의 AI 대화 세션입니다.

**표면적 요약이 아니라 실질 지식을 추출하세요.** 아래 형식으로:

## 세션 요약: %s (%s, %s)

### 결정사항과 근거
- (무엇을 결정했는지 + 왜 그렇게 했는지. 대안이 있었다면 왜 버렸는지)

### 실험/구현 결과
- (구체적 수치, 성능, 상태 변화. "구현했다"가 아니라 "X를 Y로 바꿔서 Z 결과")

### 실패한 접근
- (시도했지만 안 된 것과 그 이유. 없으면 생략)

### 발견
- (예상과 달랐던 것, 새로 알게 된 사실. 다음에 같은 문제를 만나면 참고할 것)

### 미해결
- (완료하지 못한 것과 다음 단계)

---
대화 내용:
%s`, project, tool, date, project, tool, date, conversation)
}
