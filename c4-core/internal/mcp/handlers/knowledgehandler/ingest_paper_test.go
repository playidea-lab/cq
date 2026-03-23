package knowledgehandler

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/changmin/c4-core/internal/llm"
	"github.com/changmin/c4-core/internal/mcp"
)

// mockPaperKnowledgeWriter captures CreateExperiment calls for assertions.
type mockPaperKnowledgeWriter struct {
	calls []paperWriteCall
	err   error
}

type paperWriteCall struct {
	metadata map[string]any
	body     string
}

func (m *mockPaperKnowledgeWriter) CreateExperiment(metadata map[string]any, body string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	m.calls = append(m.calls, paperWriteCall{metadata: metadata, body: body})
	return "doc-" + metadata["title"].(string)[:4], nil
}

func callIngestPaper(t *testing.T, reg *mcp.Registry, args map[string]any) map[string]any {
	t.Helper()
	rawArgs, _ := json.Marshal(args)
	result, err := reg.Call("c4_knowledge_ingest_paper", json.RawMessage(rawArgs))
	if err != nil {
		t.Fatalf("Call c4_knowledge_ingest_paper: %v", err)
	}
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", result)
	}
	return m
}

func setupIngestPaperTest(t *testing.T, mockResp string, mockErr error) (*mcp.Registry, *mockPaperKnowledgeWriter) {
	t.Helper()

	mock := llm.NewMockProvider("mock")
	if mockErr != nil {
		mock.Err = mockErr
	} else {
		mock.Response = &llm.ChatResponse{
			Content:      mockResp,
			Model:        "mock-model",
			FinishReason: "stop",
		}
	}

	gw := llm.NewGateway(llm.RoutingTable{Default: mock.Name()})
	gw.Register(mock)

	kw := &mockPaperKnowledgeWriter{}
	reg := mcp.NewRegistry()
	reg.Register(mcp.ToolSchema{Name: "c4_knowledge_ingest_paper"}, IngestPaperHandler(gw, kw))

	return reg, kw
}

func TestIngestPaper_HappyPath(t *testing.T) {
	mockJSON := `[
		{"title":"테스트 교훈 1","warning":"경고 메시지 1","tags":["go","testing"],"severity":"high","applies_when":"테스트 작성 시"},
		{"title":"테스트 교훈 2","warning":"경고 메시지 2","tags":["review"],"severity":"medium","applies_when":"코드 리뷰 시"}
	]`
	reg, kw := setupIngestPaperTest(t, mockJSON, nil)

	result := callIngestPaper(t, reg, map[string]any{
		"source":  "Go에서 항상 에러를 처리해야 한다. 무시하면 나중에 큰 버그가 된다.",
		"context": "Go 프로젝트",
	})

	if _, ok := result["error"]; ok {
		t.Fatalf("unexpected error: %v", result)
	}
	if result["success"] != true {
		t.Errorf("expected success=true, got %v", result["success"])
	}

	count, _ := result["lessons_count"].(int)
	if count != 2 {
		t.Errorf("expected lessons_count=2, got %v", result["lessons_count"])
	}

	if len(kw.calls) != 2 {
		t.Fatalf("expected 2 CreateExperiment calls, got %d", len(kw.calls))
	}

	// Verify first lesson metadata
	call0 := kw.calls[0]
	if call0.metadata["doc_type"] != "paper-lesson" {
		t.Errorf("expected doc_type=paper-lesson, got %v", call0.metadata["doc_type"])
	}
	if call0.metadata["title"] != "테스트 교훈 1" {
		t.Errorf("expected title='테스트 교훈 1', got %v", call0.metadata["title"])
	}
	if call0.body != "경고 메시지 1" {
		t.Errorf("expected body='경고 메시지 1', got %v", call0.body)
	}
	if call0.metadata["severity"] != "high" {
		t.Errorf("expected severity=high, got %v", call0.metadata["severity"])
	}

	// Verify tags include "paper-lesson"
	tags0 := toStringSliceAny(call0.metadata["tags"])
	hasPaperLesson := false
	for _, tag := range tags0 {
		if tag == "paper-lesson" {
			hasPaperLesson = true
		}
	}
	if !hasPaperLesson {
		t.Errorf("expected 'paper-lesson' in tags, got %v", tags0)
	}
}

func TestIngestPaper_EmptySource(t *testing.T) {
	reg, _ := setupIngestPaperTest(t, "[]", nil)

	result := callIngestPaper(t, reg, map[string]any{})

	if result["error"] != "source is required" {
		t.Errorf("expected error='source is required', got %v", result["error"])
	}
}

func TestIngestPaper_LLMError(t *testing.T) {
	reg, _ := setupIngestPaperTest(t, "", errors.New("api quota exceeded"))

	result := callIngestPaper(t, reg, map[string]any{
		"source": "Some paper text about software patterns.",
	})

	errStr, ok := result["error"].(string)
	if !ok || errStr == "" {
		t.Errorf("expected error string, got %v", result)
	}
	if !contains(errStr, "LLM extraction failed") {
		t.Errorf("expected 'LLM extraction failed' in error, got %v", errStr)
	}
}

func TestIngestPaper_InvalidJSON(t *testing.T) {
	reg, _ := setupIngestPaperTest(t, "This is not JSON at all.", nil)

	result := callIngestPaper(t, reg, map[string]any{
		"source": "Some paper text about software patterns.",
	})

	errStr, ok := result["error"].(string)
	if !ok || errStr == "" {
		t.Errorf("expected error string, got %v", result)
	}
	if !contains(errStr, "parse LLM response") {
		t.Errorf("expected 'parse LLM response' in error, got %v", errStr)
	}
}

func TestIngestPaper_MarkdownWrappedJSON(t *testing.T) {
	mockJSON := "```json\n[{\"title\":\"교훈 A\",\"warning\":\"경고 A\",\"tags\":[\"go\"],\"severity\":\"low\",\"applies_when\":\"조건 A\"}]\n```"
	reg, kw := setupIngestPaperTest(t, mockJSON, nil)

	result := callIngestPaper(t, reg, map[string]any{
		"source": "Some text about Go best practices.",
	})

	if _, ok := result["error"]; ok {
		t.Fatalf("unexpected error with markdown-wrapped JSON: %v", result)
	}
	count2, _ := result["lessons_count"].(int)
	if count2 != 1 {
		t.Errorf("expected lessons_count=1, got %v", result["lessons_count"])
	}
	if len(kw.calls) != 1 {
		t.Errorf("expected 1 CreateExperiment call, got %d", len(kw.calls))
	}
}

func TestIngestPaper_NoLLM(t *testing.T) {
	kw := &mockPaperKnowledgeWriter{}
	reg := mcp.NewRegistry()
	reg.Register(mcp.ToolSchema{Name: "c4_knowledge_ingest_paper"}, IngestPaperHandler(nil, kw))

	result := callIngestPaper(t, reg, map[string]any{
		"source": "some text",
	})

	if result["error"] != "LLM gateway not configured" {
		t.Errorf("expected LLM gateway error, got %v", result["error"])
	}
}

func TestIngestPaper_SourceIncluded(t *testing.T) {
	mockJSON := `[{"title":"교훈","warning":"경고","tags":[],"severity":"medium","applies_when":"항상"}]`
	reg, kw := setupIngestPaperTest(t, mockJSON, nil)

	result := callIngestPaper(t, reg, map[string]any{
		"source": "raw text content",
	})

	if _, ok := result["error"]; ok {
		t.Fatalf("unexpected error: %v", result)
	}
	if result["source"] != "raw text content" {
		t.Errorf("expected source='raw text content', got %v", result["source"])
	}
	// metadata source should be set
	if len(kw.calls) > 0 && kw.calls[0].metadata["source"] != "raw text content" {
		t.Errorf("expected metadata source='raw text content', got %v", kw.calls[0].metadata["source"])
	}
}

// contains is a helper for substring check.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
