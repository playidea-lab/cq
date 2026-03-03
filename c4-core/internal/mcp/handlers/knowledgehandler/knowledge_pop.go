package knowledgehandler

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/changmin/c4-core/internal/knowledge"
	"github.com/changmin/c4-core/internal/llm"
	"github.com/changmin/c4-core/internal/mcp"
	"github.com/changmin/c4-core/internal/pop"
)

// POPOpts holds dependencies for POP MCP handlers.
type POPOpts struct {
	ProjectDir string
	Store      *knowledge.Store
	LLM        *llm.Gateway // nil if LLM gateway disabled
}

// RegisterPOPHandlers registers c4_pop_extract, c4_pop_status, c4_pop_reflect.
func RegisterPOPHandlers(reg *mcp.Registry, opts *POPOpts) {
	if opts == nil || opts.Store == nil {
		return
	}

	reg.Register(mcp.ToolSchema{
		Name:        "c4_pop_extract",
		Description: "Run a POP (Proactive Output Pipeline) extraction cycle: fetch recent messages, extract knowledge proposals via LLM, record them",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}, popExtractHandler(opts))

	reg.Register(mcp.ToolSchema{
		Name:        "c4_pop_status",
		Description: "Show POP pipeline status: last extraction time, gauge values, knowledge stats",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}, popStatusHandler(opts))

	reg.Register(mcp.ToolSchema{
		Name:        "c4_pop_reflect",
		Description: "Trigger a POP reflection: notify pending high-confidence proposals via CLI",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}, popReflectHandler(opts))
}

func popExtractHandler(opts *POPOpts) mcp.HandlerFunc {
	return func(_ json.RawMessage) (any, error) {
		engine := buildEngine(opts)
		if engine == nil {
			return map[string]any{"error": "POP engine unavailable (LLM or store missing)"}, nil
		}
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		err := engine.RunOnce(ctx)
		if err != nil {
			// ErrGaugeThresholdExceeded is a warning, not a hard failure
			if strings.Contains(err.Error(), "gauge threshold exceeded") {
				return map[string]any{
					"success": true,
					"warning": err.Error(),
				}, nil
			}
			return map[string]any{"error": fmt.Sprintf("POP extraction failed: %v", err)}, nil
		}
		return map[string]any{"success": true}, nil
	}
}

func popStatusHandler(opts *POPOpts) mcp.HandlerFunc {
	return func(_ json.RawMessage) (any, error) {
		statePath := pop.DefaultStatePath(opts.ProjectDir)
		gaugePath := defaultGaugePath(opts.ProjectDir)

		state, err := pop.Load(statePath)
		if err != nil {
			return map[string]any{"error": fmt.Sprintf("failed to load POP state: %v", err)}, nil
		}

		gt := pop.NewGaugeTracker(gaugePath)
		_ = gt.Load() // best-effort

		gauges := map[string]any{
			"merge_ambiguity":  gt.Get("merge_ambiguity"),
			"avg_fan_out":      gt.Get("avg_fan_out"),
			"contradictions":   gt.Get("contradictions"),
			"temporal_queries": gt.Get("temporal_queries"),
		}

		result := map[string]any{
			"last_extracted_at":    state.LastExtractedAt,
			"last_crystallized_at": state.LastCrystallizedAt,
			"gauges":               gauges,
		}

		// Knowledge stats (best-effort)
		if opts.Store != nil {
			if docs, sErr := opts.Store.List("", "", 10000); sErr == nil {
				typeCounts := map[string]int{}
				for _, d := range docs {
					dt, _ := d["type"].(string)
					typeCounts[dt]++
				}
				result["knowledge_stats"] = map[string]any{
					"total":   len(docs),
					"by_type": typeCounts,
				}
			}
		}

		return result, nil
	}
}

func popReflectHandler(opts *POPOpts) mcp.HandlerFunc {
	return func(_ json.RawMessage) (any, error) {
		var notifications []string
		notifier := &capturingNotifier{collected: &notifications}

		// Build a minimal engine with capturing notifier for reflection only
		stateFile := pop.DefaultStatePath(opts.ProjectDir)
		gaugeFile := defaultGaugePath(opts.ProjectDir)

		// No-op message source — reflection uses stored state, not new messages
		msgSrc := &noopMessageSource{}
		knowledgeAdapt := &knowledgeStoreAdapter{store: opts.Store}
		soulAdapt := &soulWriterAdapter{projectDir: opts.ProjectDir}
		var llmAdapt pop.LLMClient
		if opts.LLM != nil {
			llmAdapt = &llmClientAdapter{gw: opts.LLM}
		} else {
			llmAdapt = &noopLLMClient{}
		}

		engine := pop.NewEngine(msgSrc, knowledgeAdapt, soulAdapt, notifier, llmAdapt, stateFile, gaugeFile)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		_ = engine.RunOnce(ctx) // best-effort; errors logged internally

		return map[string]any{
			"reflected":     true,
			"notifications": notifications,
		}, nil
	}
}

// buildEngine creates a pop.Engine wired with real dependencies.
func buildEngine(opts *POPOpts) *pop.Engine {
	if opts.Store == nil {
		return nil
	}
	stateFile := pop.DefaultStatePath(opts.ProjectDir)
	gaugeFile := defaultGaugePath(opts.ProjectDir)

	msgSrc := &noopMessageSource{}
	knowledgeAdapt := &knowledgeStoreAdapter{store: opts.Store}
	soulAdapt := &soulWriterAdapter{projectDir: opts.ProjectDir}
	notifier := &cliNotifier{}

	var llmAdapt pop.LLMClient
	if opts.LLM != nil {
		llmAdapt = &llmClientAdapter{gw: opts.LLM}
	} else {
		llmAdapt = &noopLLMClient{}
	}

	return pop.NewEngine(msgSrc, knowledgeAdapt, soulAdapt, notifier, llmAdapt, stateFile, gaugeFile)
}

func defaultGaugePath(projectDir string) string {
	return filepath.Join(projectDir, ".c4", "pop", "gauge.json")
}

// =========================================================================
// Adapter implementations
// =========================================================================

// llmClientAdapter adapts llm.Gateway to pop.LLMClient.
type llmClientAdapter struct {
	gw *llm.Gateway
}

func (a *llmClientAdapter) Complete(ctx context.Context, prompt string) (string, error) {
	req := &llm.ChatRequest{
		Messages: []llm.Message{
			{Role: "user", Content: prompt},
		},
		MaxTokens: 2048,
	}
	resp, err := a.gw.Chat(ctx, "pop_extraction", req)
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}

// noopLLMClient is used when no LLM gateway is available.
type noopLLMClient struct{}

func (n *noopLLMClient) Complete(_ context.Context, _ string) (string, error) {
	return "[]", nil // empty proposals
}

// knowledgeStoreAdapter adapts knowledge.Store to pop.KnowledgeStore.
type knowledgeStoreAdapter struct {
	store *knowledge.Store
}

func (a *knowledgeStoreAdapter) RecordProposal(_ context.Context, p pop.Proposal) (string, error) {
	metadata := map[string]any{
		"title":      p.Title,
		"visibility": p.Visibility,
		"confidence": p.Confidence,
	}
	docType := knowledge.DocumentType(p.ItemType)
	if docType == "" {
		docType = knowledge.TypeInsight
	}
	return a.store.Create(docType, metadata, p.Content)
}

// soulWriterAdapter appends insights to soul-developer.md.
type soulWriterAdapter struct {
	projectDir string
}

func (s *soulWriterAdapter) AppendInsight(_ context.Context, userID, insight string) error {
	if userID == "" {
		userID = "default"
	}
	soulDir := filepath.Join(s.projectDir, ".c4", "souls", userID)
	if err := os.MkdirAll(soulDir, 0755); err != nil {
		return err
	}
	soulPath := filepath.Join(soulDir, "soul-developer.md")
	f, err := os.OpenFile(soulPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = fmt.Fprintf(f, "\n## POP Insight (%s)\n%s\n", time.Now().Format(time.RFC3339), insight)
	return err
}

// cliNotifier prints proposal notifications to stderr.
type cliNotifier struct{}

func (c *cliNotifier) Notify(_ context.Context, p pop.Proposal) error {
	fmt.Fprintf(os.Stderr, "cq: pop: [%s] %s (confidence=%.2f)\n", p.ItemType, p.Title, p.Confidence)
	return nil
}

// capturingNotifier collects notification messages for the reflect handler.
type capturingNotifier struct {
	collected *[]string
}

func (c *capturingNotifier) Notify(_ context.Context, p pop.Proposal) error {
	*c.collected = append(*c.collected, fmt.Sprintf("[%s] %s (confidence=%.2f)", p.ItemType, p.Title, p.Confidence))
	return nil
}

// noopMessageSource returns no messages (used when no c1 integration is available).
type noopMessageSource struct{}

func (n *noopMessageSource) RecentMessages(_ context.Context, _ time.Time, _ int) ([]pop.Message, error) {
	return nil, nil
}
