package main

import (
	"context"
	"encoding/json"
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
	cfg      config.ServeHypothesisSuggesterConfig
	gw       *llm.Gateway
	kStore   *knowledge.Store
	interval time.Duration
	mu       sync.Mutex
	lastCount int
	status   string
	cancel   context.CancelFunc
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

	newCount := len(docs) - last
	if newCount < h.cfg.Threshold {
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

	// Store expires_at in metadata_json field of the body as JSON so cleanup() can read it.
	expiresAt := time.Now().UTC().Add(h.cfg.TTL)
	expiresAtStr := expiresAt.Format(time.RFC3339)
	metaJSON, _ := json.Marshal(map[string]string{"expires_at": expiresAtStr})

	// Build the body: metadata_json line + insight
	body := string(metaJSON) + "\n" + insight

	meta := map[string]any{
		"hypothesis_status": "pending",
		"domain":            "hypothesis",
	}

	if _, err := h.kStore.Create(knowledge.TypeHypothesis, meta, body); err != nil {
		h.mu.Lock()
		h.status = "degraded"
		h.mu.Unlock()
		return
	}

	h.mu.Lock()
	h.lastCount = len(docs)
	h.mu.Unlock()
}

// cleanup marks expired pending hypotheses as expired.
// expires_at is read from the document body (first line is a JSON object with "expires_at").
func (h *hypothesisSuggester) cleanup() {
	docs, err := h.kStore.List(string(knowledge.TypeHypothesis), "", 100)
	if err != nil {
		return
	}
	now := time.Now().UTC()
	for _, d := range docs {
		status, _ := d["hypothesis_status"].(string)
		if status != "pending" {
			continue
		}
		docID, _ := d["id"].(string)
		if docID == "" {
			continue
		}
		// Read expires_at from document body (SSOT: Markdown file)
		doc, err := h.kStore.Get(docID)
		if err != nil || doc == nil {
			continue
		}
		expiresAt := parseExpiresAtFromBody(doc.Body)
		if expiresAt.IsZero() || !expiresAt.Before(now) {
			continue
		}
		expired := "expired"
		h.kStore.Update(docID, map[string]any{"hypothesis_status": expired}, &expired) //nolint:errcheck
	}
}

// parseExpiresAtFromBody extracts expires_at from the first line of the document body.
// The first line is expected to be a JSON object: {"expires_at":"<RFC3339>"}.
func parseExpiresAtFromBody(body string) time.Time {
	firstLine := body
	if idx := strings.Index(body, "\n"); idx >= 0 {
		firstLine = body[:idx]
	}
	firstLine = strings.TrimSpace(firstLine)
	var m map[string]string
	if err := json.Unmarshal([]byte(firstLine), &m); err == nil {
		if ea, ok := m["expires_at"]; ok {
			if t, err := time.Parse(time.RFC3339, ea); err == nil {
				return t
			}
		}
	}
	return time.Time{}
}
