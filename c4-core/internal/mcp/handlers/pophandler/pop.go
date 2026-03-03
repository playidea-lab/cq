package pophandler

import (
	"context"
	"encoding/json"
	"errors"
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

// Opts holds dependencies for POP MCP handlers.
type Opts struct {
	ProjectDir string
	Store      *knowledge.Store
	LLM        *llm.Gateway // nil if LLM gateway disabled
}

// Register registers c4_pop_extract, c4_pop_status, and c4_pop_reflect tools.
func Register(reg *mcp.Registry, opts *Opts) {
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
	}, extractHandler(opts))

	reg.Register(mcp.ToolSchema{
		Name:        "c4_pop_status",
		Description: "Show POP pipeline status: last extraction time, gauge values, knowledge stats",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}, statusHandler(opts))

	reg.Register(mcp.ToolSchema{
		Name:        "c4_pop_reflect",
		Description: "Reflect on pending high-confidence POP proposals",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}, reflectHandler(opts))
}

func extractHandler(opts *Opts) mcp.HandlerFunc {
	return func(_ json.RawMessage) (any, error) {
		engine := buildEngine(opts)
		if engine == nil {
			return map[string]any{"error": "POP engine unavailable (LLM or store missing)"}, nil
		}
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		err := engine.RunOnce(ctx)
		if err != nil {
			if errors.Is(err, pop.ErrGaugeThresholdExceeded) {
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

func statusHandler(opts *Opts) mcp.HandlerFunc {
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

		// Knowledge stats (best-effort; capped at 200 to avoid full-table scan)
		if opts.Store != nil {
			if docs, sErr := opts.Store.List("", "", 200); sErr == nil {
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

func reflectHandler(opts *Opts) mcp.HandlerFunc {
	return func(_ json.RawMessage) (any, error) {
		// Retrieve high-confidence proposals; capped at 200 to avoid full-table scan.
		docs, err := opts.Store.List("", "", 200)
		if err != nil {
			return map[string]any{"error": fmt.Sprintf("failed to list proposals: %v", err)}, nil
		}

		const highConfidenceThreshold = 0.8
		const reflectLimit = 5

		var proposals []map[string]any
		for _, d := range docs {
			conf, _ := d["confidence"].(float64)
			if conf < highConfidenceThreshold {
				continue
			}
			proposals = append(proposals, d)
			if len(proposals) >= reflectLimit {
				break
			}
		}

		return map[string]any{
			"proposals": proposals,
		}, nil
	}
}

// buildEngine creates a pop.Engine wired with real dependencies.
func buildEngine(opts *Opts) *pop.Engine {
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

// mapItemType converts a POP proposal item_type string to a validated
// knowledge.DocumentType. Only "pattern" maps to TypePattern; everything
// else (including unknown values) defaults to TypeInsight.
func mapItemType(itemType string) knowledge.DocumentType {
	if itemType == string(knowledge.TypePattern) {
		return knowledge.TypePattern
	}
	return knowledge.TypeInsight
}

func (a *knowledgeStoreAdapter) RecordProposal(_ context.Context, p pop.Proposal) (string, error) {
	metadata := map[string]any{
		"title":      p.Title,
		"visibility": p.Visibility,
		"confidence": p.Confidence,
	}
	return a.store.Create(mapItemType(p.ItemType), metadata, p.Content)
}

// soulWriterAdapter appends insights to soul-developer.md.
type soulWriterAdapter struct {
	projectDir string
}

const maxInsightBytes = 4096

func (s *soulWriterAdapter) AppendInsight(_ context.Context, userID, insight string) error {
	if userID == "" {
		userID = "default"
	}
	// Truncate LLM-generated content to prevent unbounded disk writes.
	if len(insight) > maxInsightBytes {
		insight = insight[:maxInsightBytes]
	}
	// Path traversal guard: ensure userID stays within .c4/souls/
	soulDir := filepath.Join(s.projectDir, ".c4", "souls", userID)
	soulsBase := filepath.Join(s.projectDir, ".c4", "souls")
	cleaned := filepath.Clean(soulDir)
	if !strings.HasPrefix(cleaned, soulsBase+string(filepath.Separator)) && cleaned != soulsBase {
		return errors.New("pop: soul path traversal detected")
	}

	if err := os.MkdirAll(cleaned, 0755); err != nil {
		return err
	}
	soulPath := filepath.Join(cleaned, "soul-developer.md")
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

// Compile-time interface assertions for all POP adapters.
var (
	_ pop.MessageSource  = (*noopMessageSource)(nil)
	_ pop.LLMClient      = (*noopLLMClient)(nil)
	_ pop.LLMClient      = (*llmClientAdapter)(nil)
	_ pop.KnowledgeStore = (*knowledgeStoreAdapter)(nil)
	_ pop.SoulWriter     = (*soulWriterAdapter)(nil)
	_ pop.Notifier       = (*cliNotifier)(nil)
)

// noopMessageSource returns no messages (used when no c1 integration is available).
type noopMessageSource struct{}

func (n *noopMessageSource) RecentMessages(_ context.Context, _ time.Time, _ int) ([]pop.Message, error) {
	return nil, nil
}
