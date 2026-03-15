package suggestpoller

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/changmin/c4-core/internal/knowledge"
	"github.com/changmin/c4-core/internal/serve"
)

// LLMCaller is an interface for calling an LLM with a single prompt.
// This is intentionally simpler than the eventbus LLMCaller — no system/user split.
type LLMCaller interface {
	Call(ctx context.Context, prompt string) (string, error)
}

// C1Notifier is an optional interface for sending C1 Messenger notifications.
// Pass nil to disable notifications.
type C1Notifier interface {
	SendMessage(ctx context.Context, msg string) error
}

// HypothesisResult holds the parsed LLM response.
type HypothesisResult struct {
	Insight   string `json:"insight"`
	YAMLDraft string `json:"yaml_draft"`
	DocID     string `json:"doc_id"`
}

// Watermark records the watermark for the suggest poller.
type Watermark struct {
	LastAnalyzedAt time.Time `json:"last_analyzed_at"`
	LastCount      int       `json:"last_count"`
}

// Config holds configuration for KnowledgeSuggestPoller.
type Config struct {
	Store                  *knowledge.Store
	LLMCaller              LLMCaller
	C1                     C1Notifier // nil = disabled
	NewExperimentThreshold int        // default 5
	PollInterval           time.Duration
	WatermarkPath          string // .c4/suggest_poller_watermark.json
}

// KnowledgeSuggestPoller monitors experiment documents and triggers LLM analysis
// when enough new experiments have accumulated since the last analysis.
// It implements serve.Component.
type KnowledgeSuggestPoller struct {
	cfg    Config
	cancel context.CancelFunc
	done   chan struct{}

	mu        sync.Mutex
	status    string
	detail    string
	analyzing bool // true while runAnalysis() is executing
}

// compile-time interface assertion
var _ serve.Component = (*KnowledgeSuggestPoller)(nil)

func New(cfg Config) *KnowledgeSuggestPoller {
	if cfg.NewExperimentThreshold <= 0 {
		cfg.NewExperimentThreshold = 5
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 5 * time.Minute
	}
	return &KnowledgeSuggestPoller{cfg: cfg, status: "ok"}
}

func (p *KnowledgeSuggestPoller) Name() string { return "knowledge-suggest-poller" }

func (p *KnowledgeSuggestPoller) Start(ctx context.Context) error {
	ctx2, cancel := context.WithCancel(ctx)
	p.cancel = cancel
	p.done = make(chan struct{})
	go p.loop(ctx2)
	return nil
}

func (p *KnowledgeSuggestPoller) Stop(_ context.Context) error {
	if p.cancel != nil {
		p.cancel()
		p.cancel = nil
		if p.done != nil {
			<-p.done
		}
	}
	return nil
}

func (p *KnowledgeSuggestPoller) Health() serve.ComponentHealth {
	p.mu.Lock()
	defer p.mu.Unlock()
	return serve.ComponentHealth{Status: p.status, Detail: p.detail}
}

func (p *KnowledgeSuggestPoller) loop(ctx context.Context) {
	defer close(p.done)
	ticker := time.NewTicker(p.cfg.PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.poll(ctx)
		}
	}
}

func (p *KnowledgeSuggestPoller) poll(ctx context.Context) {
	wm := p.loadWatermark()

	docs, err := p.cfg.Store.List(string(knowledge.TypeExperiment), "", 200)
	if err != nil {
		fmt.Fprintf(os.Stderr, "knowledge-suggest-poller: list experiments: %v\n", err)
		p.mu.Lock()
		p.status = "degraded"
		p.detail = err.Error()
		p.mu.Unlock()
		return
	}

	newCount := countNewSince(docs, wm.LastAnalyzedAt)
	if newCount < p.cfg.NewExperimentThreshold {
		return
	}

	if _, err := p.runAnalysis(ctx, docs, ""); err != nil {
		fmt.Fprintf(os.Stderr, "knowledge-suggest-poller: analysis failed: %v\n", err)
		p.mu.Lock()
		p.status = "degraded"
		p.detail = err.Error()
		p.mu.Unlock()
		return
	}

	p.mu.Lock()
	p.status = "ok"
	p.detail = ""
	p.mu.Unlock()
}

// Trigger manually triggers analysis for experiments matching tag (empty = recent).
// Returns an error if analysis is already running.
func (p *KnowledgeSuggestPoller) Trigger(ctx context.Context, tag string, limit int) (*HypothesisResult, error) {
	p.mu.Lock()
	if p.analyzing {
		p.mu.Unlock()
		return nil, fmt.Errorf("analysis already in progress")
	}
	p.mu.Unlock()

	if limit <= 0 {
		limit = 20
	}
	docs, err := p.cfg.Store.List(string(knowledge.TypeExperiment), "", limit)
	if err != nil {
		return nil, fmt.Errorf("list experiments: %w", err)
	}

	return p.runAnalysis(ctx, docs, tag)
}

// runAnalysis calls the LLM, parses the result, and stores a TypeHypothesis document.
// It uses p.mu to prevent concurrent execution.
func (p *KnowledgeSuggestPoller) runAnalysis(ctx context.Context, docs []map[string]any, tag string) (*HypothesisResult, error) {
	p.mu.Lock()
	if p.analyzing {
		p.mu.Unlock()
		return nil, fmt.Errorf("analysis already in progress")
	}
	p.analyzing = true
	p.mu.Unlock()

	defer func() {
		p.mu.Lock()
		p.analyzing = false
		p.mu.Unlock()
	}()

	if p.cfg.LLMCaller == nil {
		return nil, fmt.Errorf("no LLM caller configured")
	}

	prompt := BuildAnalysisPrompt(docs)
	resp, err := p.cfg.LLMCaller.Call(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("LLM call failed: %w", err)
	}

	result, err := ParseAnalysisResponse(resp)
	if err != nil {
		return nil, fmt.Errorf("malformed LLM response: %w", err)
	}

	// Build metadata for TypeHypothesis
	label := tag
	if label == "" {
		label = "recent"
	}
	meta := map[string]any{
		"title":      "Suggestion: " + label,
		"expires_at": time.Now().Add(24 * time.Hour).Format(time.RFC3339),
		"status":     "pending",
		"yaml_draft": result.YAMLDraft,
	}
	if tag != "" {
		meta["tags"] = []string{tag}
	}

	docID, err := p.cfg.Store.Create(knowledge.TypeHypothesis, meta, result.Insight)
	if err != nil {
		return nil, fmt.Errorf("store hypothesis: %w", err)
	}
	result.DocID = docID

	// Notify C1 if configured.
	if p.cfg.C1 != nil {
		msg := fmt.Sprintf("suggest-poller: new hypothesis %s — %s", docID, label)
		if notifyErr := p.cfg.C1.SendMessage(ctx, msg); notifyErr != nil {
			fmt.Fprintf(os.Stderr, "knowledge-suggest-poller: c1 notify: %v\n", notifyErr)
		}
	}

	// Advance watermark after successful analysis.
	wm := Watermark{
		LastAnalyzedAt: time.Now(),
		LastCount:      len(docs),
	}
	if err := p.saveWatermark(wm); err != nil {
		fmt.Fprintf(os.Stderr, "knowledge-suggest-poller: save watermark: %v\n", err)
	}

	return result, nil
}

// BuildAnalysisPrompt builds the LLM prompt from experiment documents.
func BuildAnalysisPrompt(docs []map[string]any) string {
	var sb strings.Builder
	sb.WriteString("당신은 ML 실험 분석가입니다. 실험 결과를 분석하고 다음 실험을 제안해주세요.\n")
	sb.WriteString("아래 형식의 JSON으로만 응답하세요:\n")
	sb.WriteString(`{"insight": "분석 텍스트", "yaml_draft": "run: python train.py\n..."}`)
	sb.WriteString("\n\n다음 실험 결과를 분석하고 다음 실험을 제안해주세요:\n")
	for _, doc := range docs {
		title, _ := doc["title"].(string)
		id, _ := doc["id"].(string)
		sb.WriteString(fmt.Sprintf("- [%s] %s\n", id, title))
	}
	return sb.String()
}

// ParseAnalysisResponse parses the JSON response from the LLM.
func ParseAnalysisResponse(resp string) (*HypothesisResult, error) {
	// Find the first '{' to handle any leading text.
	start := strings.Index(resp, "{")
	if start < 0 {
		return nil, fmt.Errorf("no JSON object found in response")
	}
	resp = resp[start:]
	var result HypothesisResult
	if err := json.Unmarshal([]byte(resp), &result); err != nil {
		return nil, fmt.Errorf("json unmarshal: %w", err)
	}
	if result.Insight == "" {
		return nil, fmt.Errorf("insight field is empty")
	}
	return &result, nil
}

// countNewSince returns the number of documents created after cutoff.
func countNewSince(docs []map[string]any, cutoff time.Time) int {
	if cutoff.IsZero() {
		return len(docs)
	}
	n := 0
	for _, doc := range docs {
		createdAt, _ := doc["created_at"].(string)
		t, err := time.Parse(time.RFC3339, createdAt)
		if err != nil {
			continue
		}
		if t.After(cutoff) {
			n++
		}
	}
	return n
}

func (p *KnowledgeSuggestPoller) loadWatermark() Watermark {
	data, err := os.ReadFile(p.cfg.WatermarkPath)
	if err != nil {
		return Watermark{}
	}
	var wm Watermark
	if err := json.Unmarshal(data, &wm); err != nil {
		return Watermark{}
	}
	return wm
}

func (p *KnowledgeSuggestPoller) saveWatermark(wm Watermark) error {
	data, err := json.Marshal(wm)
	if err != nil {
		return err
	}
	dir := filepath.Dir(p.cfg.WatermarkPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "suggest_poller_wm_*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, werr := tmp.Write(data); werr != nil {
		tmp.Close()
		os.Remove(tmpName)
		return werr
	}
	if serr := tmp.Sync(); serr != nil {
		tmp.Close()
		os.Remove(tmpName)
		return serr
	}
	tmp.Close()
	return os.Rename(tmpName, p.cfg.WatermarkPath)
}
