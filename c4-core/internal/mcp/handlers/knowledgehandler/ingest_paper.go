package knowledgehandler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/changmin/c4-core/internal/llm"
	"github.com/changmin/c4-core/internal/mcp"
)

// PaperKnowledgeWriter saves paper-lesson documents to the knowledge store.
// Satisfied by the handlers.KnowledgeWriter adapter.
type PaperKnowledgeWriter interface {
	CreateExperiment(metadata map[string]any, body string) (string, error)
}

// paperLesson is a single lesson extracted from a paper/text by the LLM.
type paperLesson struct {
	Title       string   `json:"title"`
	Warning     string   `json:"warning"`
	Tags        []string `json:"tags"`
	Severity    string   `json:"severity"`
	AppliesWhen string   `json:"applies_when"`
}

const (
	maxIngestChars = 8000
	ingestTimeout  = 90 * time.Second
)

var htmlTagRe = regexp.MustCompile(`<[^>]+>`)

const paperExtractionPrompt = `다음 텍스트에서 소프트웨어 개발/코드 리뷰 시 적용 가능한 핵심 교훈을 추출하세요.

각 교훈을 JSON 배열로 반환하세요 (다른 텍스트 없이 JSON만):
[
  {
    "title": "한 줄 요약",
    "warning": "워커에게 줄 구체적 경고 메시지",
    "tags": ["관련 태그들"],
    "severity": "high|medium|low",
    "applies_when": "이 교훈이 적용되는 조건"
  }
]

프로젝트 컨텍스트: %s

텍스트:
%s`

// IngestPaperHandler creates the MCP handler for c4_knowledge_ingest_paper.
// It extracts software development lessons from a paper/URL/text via LLM
// and saves them as paper-lesson documents in the knowledge store.
func IngestPaperHandler(llmGW *llm.Gateway, kw PaperKnowledgeWriter) mcp.HandlerFunc {
	return func(rawArgs json.RawMessage) (any, error) {
		var params map[string]any
		if err := json.Unmarshal(rawArgs, &params); err != nil {
			return nil, fmt.Errorf("invalid args: %w", err)
		}

		source, _ := params["source"].(string)
		if source == "" {
			return map[string]any{"error": "source is required"}, nil
		}
		ctx, _ := params["context"].(string)
		if ctx == "" {
			ctx = "일반 소프트웨어 프로젝트"
		}

		if llmGW == nil {
			return map[string]any{"error": "LLM gateway not configured"}, nil
		}
		if kw == nil {
			return map[string]any{"error": "knowledge writer not configured"}, nil
		}

		// Resolve source to text
		text, err := resolveSource(source)
		if err != nil {
			return map[string]any{"error": fmt.Sprintf("failed to resolve source: %v", err)}, nil
		}
		if strings.TrimSpace(text) == "" {
			return map[string]any{"error": "source resolved to empty text"}, nil
		}

		// Truncate to LLM context limit
		if len(text) > maxIngestChars {
			text = text[:maxIngestChars] + "\n...(truncated)"
		}

		// Build extraction prompt
		prompt := fmt.Sprintf(paperExtractionPrompt, ctx, text)

		// Call LLM for extraction
		ref := llmGW.Resolve("haiku", "")
		llmCtx, cancel := context.WithTimeout(context.Background(), ingestTimeout)
		defer cancel()

		resp, err := llmGW.Chat(llmCtx, "haiku", &llm.ChatRequest{
			Model:       ref.Model,
			Messages:    []llm.Message{{Role: "user", Content: prompt}},
			MaxTokens:   2000,
			Temperature: 0.3,
		})
		if err != nil {
			return map[string]any{"error": fmt.Sprintf("LLM extraction failed: %v", err)}, nil
		}

		// Parse JSON response
		lessons, err := parseLessons(resp.Content)
		if err != nil {
			return map[string]any{
				"error":        fmt.Sprintf("failed to parse LLM response as lessons: %v", err),
				"raw_response": resp.Content,
			}, nil
		}
		if len(lessons) == 0 {
			return map[string]any{"error": "LLM returned no lessons"}, nil
		}

		// Save each lesson to knowledge store
		var savedIDs []string
		var savedLessons []map[string]any
		for _, lesson := range lessons {
			tags := append(lesson.Tags, "paper-lesson")
			metadata := map[string]any{
				"title":        lesson.Title,
				"doc_type":     "paper-lesson",
				"source":       source,
				"tags":         tags,
				"severity":     lesson.Severity,
				"applies_when": lesson.AppliesWhen,
			}
			docID, saveErr := kw.CreateExperiment(metadata, lesson.Warning)
			if saveErr != nil {
				fmt.Fprintf(os.Stderr, "c4: ingest_paper: save lesson failed: %v\n", saveErr)
				continue
			}
			savedIDs = append(savedIDs, docID)
			savedLessons = append(savedLessons, map[string]any{
				"doc_id":       docID,
				"title":        lesson.Title,
				"severity":     lesson.Severity,
				"applies_when": lesson.AppliesWhen,
				"tags":         tags,
			})
		}

		return map[string]any{
			"success":       true,
			"lessons_count": len(savedIDs),
			"lessons":       savedLessons,
			"source":        source,
		}, nil
	}
}

// resolveSource fetches or reads the content of a source.
// Handles: HTTP(S) URLs, local file paths, and raw text.
func resolveSource(source string) (string, error) {
	if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") {
		return fetchURL(source)
	}
	if _, err := os.Stat(source); err == nil {
		data, err := os.ReadFile(source)
		if err != nil {
			return "", fmt.Errorf("read file: %w", err)
		}
		return string(data), nil
	}
	// Treat as raw text
	return source, nil
}

// fetchURL performs a simple HTTP GET and strips HTML tags.
func fetchURL(url string) (string, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return "", fmt.Errorf("http get: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	if err != nil {
		return "", fmt.Errorf("read body: %w", err)
	}

	text := htmlTagRe.ReplaceAllString(string(body), " ")
	// Collapse whitespace
	text = strings.Join(strings.Fields(text), " ")
	return text, nil
}

// parseLessons parses the LLM JSON response into a slice of paperLesson.
// It tolerates a JSON array wrapped in markdown code fences.
func parseLessons(content string) ([]paperLesson, error) {
	// Strip markdown code fences if present
	s := strings.TrimSpace(content)
	if idx := strings.Index(s, "["); idx >= 0 {
		s = s[idx:]
	}
	if idx := strings.LastIndex(s, "]"); idx >= 0 {
		s = s[:idx+1]
	}

	var lessons []paperLesson
	if err := json.Unmarshal([]byte(s), &lessons); err != nil {
		return nil, fmt.Errorf("json unmarshal: %w", err)
	}
	return lessons, nil
}
