package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/changmin/c4-core/internal/config"
	"github.com/changmin/c4-core/internal/knowledge"
	"github.com/changmin/c4-core/internal/llm"
	"github.com/changmin/c4-core/internal/serve"
)

// hypothesisSuggester is a serve.Component that watches for new experiments
// and generates TypeHypothesis docs when the threshold is reached.
type hypothesisSuggester struct {
	cfg       config.ServeHypothesisSuggesterConfig
	gw        *llm.Gateway
	kStore    *knowledge.Store
	interval  time.Duration
	mu        sync.Mutex
	lastCount int
	status    string
	cancel    context.CancelFunc
}

// compile-time assertion
var _ serve.Component = (*hypothesisSuggester)(nil)

func registerHypothesisSuggesterComponent(mgr *serve.Manager, cfg config.C4Config, gw *llm.Gateway, kStore *knowledge.Store) {
	if !cfg.Serve.HypothesisSuggester.Enabled {
		return
	}
	if kStore == nil {
		fmt.Fprintf(os.Stderr, "cq serve: hypothesis suggester skipped (no knowledge store)\n")
		return
	}
	c := newHypothesisSuggester(cfg.Serve.HypothesisSuggester, gw, kStore)
	mgr.Register(c)
	fmt.Fprintf(os.Stderr, "cq serve: registered hypothesis-suggester\n")
}

func newHypothesisSuggester(cfg config.ServeHypothesisSuggesterConfig, gw *llm.Gateway, kStore *knowledge.Store) *hypothesisSuggester {
	interval := 30 * time.Second
	if cfg.Interval != "" {
		if d, err := time.ParseDuration(cfg.Interval); err == nil {
			interval = d
		}
	}
	threshold := cfg.Threshold
	if threshold <= 0 {
		threshold = 5
	}
	ttl := cfg.TTL
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	return &hypothesisSuggester{
		cfg: config.ServeHypothesisSuggesterConfig{
			Enabled:   cfg.Enabled,
			Threshold: threshold,
			Interval:  cfg.Interval,
			TTL:       ttl,
		},
		gw:       gw,
		kStore:   kStore,
		interval: interval,
		status:   "ok",
	}
}

func (h *hypothesisSuggester) Name() string { return "hypothesis-suggester" }

func (h *hypothesisSuggester) Start(ctx context.Context) error {
	if h.cfg.TTL <= 0 {
		return fmt.Errorf("hypothesis-suggester: TTL must be > 0")
	}
	cctx, cancel := context.WithCancel(ctx)
	h.mu.Lock()
	h.cancel = cancel
	h.mu.Unlock()

	go func() {
		ticker := time.NewTicker(h.interval)
		defer ticker.Stop()
		for {
			select {
			case <-cctx.Done():
				return
			case <-ticker.C:
				h.poll(cctx)
				h.cleanup()
			}
		}
	}()
	return nil
}

func (h *hypothesisSuggester) Stop(_ context.Context) error {
	h.mu.Lock()
	if h.cancel != nil {
		h.cancel()
	}
	h.mu.Unlock()
	return nil
}

func (h *hypothesisSuggester) Health() serve.ComponentHealth {
	h.mu.Lock()
	s := h.status
	h.mu.Unlock()
	return serve.ComponentHealth{Status: s}
}

// poll checks for new experiments and generates a hypothesis if threshold is met.
func (h *hypothesisSuggester) poll(ctx context.Context) {
	docs, err := h.kStore.List(string(knowledge.TypeExperiment), "", 50)
	if err != nil {
		h.mu.Lock()
		h.status = "degraded"
		h.mu.Unlock()
		return
	}

	h.mu.Lock()
	last := h.lastCount
	h.mu.Unlock()

	currentCount := len(docs)
	if currentCount < last {
		// Docs were deleted; reset baseline without triggering hypothesis generation.
		h.mu.Lock()
		h.lastCount = currentCount
		h.mu.Unlock()
		return
	}
	if currentCount-last < h.cfg.Threshold {
		return
	}

	// Generate hypothesis via LLM (optional — skip gracefully if gw is nil or fails)
	insight := ""
	if h.gw != nil {
		var sb strings.Builder
		for _, d := range docs {
			title, _ := d["title"].(string)
			sb.WriteString(title)
			sb.WriteString("\n")
		}
		resp, llmErr := h.gw.Chat(ctx, "hypothesis", &llm.ChatRequest{
			Messages: []llm.Message{
				{Role: "user", Content: "Analyze these experiments and propose a hypothesis:\n" + sb.String()},
			},
			MaxTokens: 512,
		})
		if llmErr == nil {
			insight = resp.Content
		}
	}

	// Store expires_at in frontmatter metadata so readHypMeta() and cleanup() can find it.
	expiresAtStr := time.Now().UTC().Add(h.cfg.TTL).Format(time.RFC3339)

	// Use both "status" and "hypothesis_status" so CLI (runSuggestList/Approve) and
	// cleanup() read the same canonical field regardless of creation path.
	meta := map[string]any{
		"title":             "Hypothesis (auto-generated)",
		"status":            "pending",
		"hypothesis_status": "pending",
		"domain":            "hypothesis",
		"expires_at":        expiresAtStr,
	}

	if _, err := h.kStore.Create(knowledge.TypeHypothesis, meta, insight); err != nil {
		h.mu.Lock()
		h.status = "degraded"
		h.mu.Unlock()
		return
	}

	h.mu.Lock()
	h.lastCount = currentCount
	h.mu.Unlock()
}

// cleanup marks expired pending hypotheses as expired.
// expires_at is read from frontmatter (unified schema with poll() and hypothesis_suggest.go).
func (h *hypothesisSuggester) cleanup() {
	docs, err := h.kStore.List(string(knowledge.TypeHypothesis), "", 100)
	if err != nil {
		return
	}
	now := time.Now().UTC()
	for _, d := range docs {
		// Check both fields: poll() sets hypothesis_status, MCP handler sets status.
		hypStatus, _ := d["hypothesis_status"].(string)
		docStatus, _ := d["status"].(string)
		if hypStatus != "pending" && docStatus != "pending" {
			continue
		}
		docID, _ := d["id"].(string)
		if docID == "" {
			continue
		}
		// Read expires_at from frontmatter (same schema as poll() and hypothesis_suggest.go).
		expiresAtStr, _ := readHypMeta(h.kStore, docID)
		if expiresAtStr == "" {
			continue
		}
		t, parseErr := time.Parse(time.RFC3339, expiresAtStr)
		if parseErr != nil || !t.Before(now) {
			continue
		}
		if _, updateErr := h.kStore.Update(docID, map[string]any{
			"status":            "expired",
			"hypothesis_status": "expired",
		}, nil); updateErr != nil {
			fmt.Fprintf(os.Stderr, "hypothesis-suggester: cleanup update failed for %s: %v\n", docID, updateErr)
		}
	}
}
