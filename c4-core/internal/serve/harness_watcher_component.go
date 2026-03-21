package serve

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/changmin/c4-core/internal/channelpush"
	"github.com/changmin/c4-core/internal/harness"
	"github.com/changmin/c4-core/internal/knowledge"
	"github.com/changmin/c4-core/internal/observe"
)

// HarnessWatcherConfig holds configuration for HarnessWatcherComponent.
type HarnessWatcherConfig struct {
	// SupabaseURL is the Supabase project URL. If empty, journal push is skipped.
	SupabaseURL string
	// AnonKey is the Supabase anon key.
	AnonKey string
	// TenantID defaults to "default" if empty.
	TenantID string
	// DB is an optional *sql.DB for TraceCollector persistence.
	// If nil, trace steps are still recorded in-process but not persisted.
	DB *sql.DB
	// KnowledgeStore is an optional knowledge store for recording session LLM
	// usage reports on Stop(). If nil, the report is skipped.
	KnowledgeStore *knowledge.Store
}

// HarnessWatcherComponent watches ~/.claude/projects/**/*.jsonl and pushes new
// lines to c1_channels via channelpush.Pusher. Activated only when cloud.url is set.
// It always installs an observe.TraceCollector so LLM usage from harness journals
// is captured regardless of cloud connectivity.
type HarnessWatcherComponent struct {
	cfg     HarnessWatcherConfig
	watcher *harness.JournalWatcher
	store   *harness.PositionStore
	cancel  context.CancelFunc
	tc      *observe.TraceCollector
}

// NewHarnessWatcherComponent creates a HarnessWatcherComponent.
func NewHarnessWatcherComponent(cfg HarnessWatcherConfig) *HarnessWatcherComponent {
	if cfg.TenantID == "" {
		cfg.TenantID = "default"
	}
	return &HarnessWatcherComponent{cfg: cfg}
}

// Name implements Component.
func (h *HarnessWatcherComponent) Name() string { return "harness_watcher" }

// Start implements Component.
// Always installs a TraceCollector for LLM usage capture.
// Journal push to Supabase is skipped if SupabaseURL is empty.
func (h *HarnessWatcherComponent) Start(ctx context.Context) error {
	// Always set up TraceCollector for LLM usage capture from harness journals.
	tc := observe.NewTraceCollector()
	if h.cfg.DB != nil {
		tc.SetDB(h.cfg.DB)
	}
	h.tc = tc
	harness.SetTraceRecorder(tc)

	if h.cfg.SupabaseURL == "" {
		fmt.Fprintf(os.Stderr, "cq serve: [harness_watcher] cloud.url not configured — journal push skipped, trace recording active\n")
		return nil
	}

	pusher := channelpush.New(h.cfg.SupabaseURL, h.cfg.AnonKey)
	if pusher == nil {
		fmt.Fprintf(os.Stderr, "cq serve: [harness_watcher] missing supabase credentials — journal push skipped, trace recording active\n")
		return nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("harness_watcher: home dir: %w", err)
	}
	dbPath := filepath.Join(home, ".c4", "harness_positions.db")

	store, err := harness.NewPositionStore(dbPath)
	if err != nil {
		return fmt.Errorf("harness_watcher: position store: %w", err)
	}
	h.store = store

	watcher := harness.NewJournalWatcher(pusher, store, h.cfg.TenantID)
	h.watcher = watcher

	watchCtx, cancel := context.WithCancel(ctx)
	h.cancel = cancel

	if err := watcher.Start(watchCtx); err != nil {
		cancel()
		store.Close()
		return fmt.Errorf("harness_watcher: start: %w", err)
	}

	fmt.Fprintf(os.Stderr, "cq serve: [harness_watcher] started (tenant=%s)\n", h.cfg.TenantID)
	return nil
}

// Stop implements Component.
func (h *HarnessWatcherComponent) Stop(ctx context.Context) error {
	if h.cancel != nil {
		h.cancel()
	}
	if h.watcher != nil {
		_ = h.watcher.Stop(ctx)
	}
	if h.store != nil {
		_ = h.store.Close()
	}
	// Record LLM usage report to knowledge store before closing TraceCollector.
	if h.tc != nil && h.cfg.DB != nil && h.cfg.KnowledgeStore != nil {
		h.recordLLMUsageReport()
	}
	// Clear global trace recorder and flush pending writes.
	harness.SetTraceRecorder(nil)
	if h.tc != nil {
		h.tc.Close()
		h.tc = nil
	}
	fmt.Fprintf(os.Stderr, "cq serve: [harness_watcher] stopped\n")
	return nil
}

// recordLLMUsageReport queries TraceAnalyzer and writes a session LLM usage
// summary to the knowledge store. Failures are non-fatal (logged only).
func (h *HarnessWatcherComponent) recordLLMUsageReport() {
	analyzer := observe.NewTraceAnalyzer(h.cfg.DB)
	stats, err := analyzer.StatsByTaskType()
	if err != nil {
		fmt.Fprintf(os.Stderr, "cq serve: [harness_watcher] llm_usage_report: query failed: %v\n", err)
		return
	}
	if len(stats) == 0 {
		return
	}

	date := time.Now().UTC().Format("2006-01-02")
	title := fmt.Sprintf("LLM 사용 리포트 %s", date)

	// Build markdown summary.
	body := fmt.Sprintf("# %s\n\n", title)
	var totalCalls int64
	var totalCost float64
	for taskType, models := range stats {
		body += fmt.Sprintf("## task_type: %s\n\n", taskType)
		body += "| 모델 | 호출 수 | 성공률 | 평균 비용 | 평균 레이턴시 |\n"
		body += "|------|---------|--------|-----------|---------------|\n"
		for _, m := range models {
			body += fmt.Sprintf("| %s | %d | %.1f%% | $%.6f | %.0fms |\n",
				m.Model, m.Count, m.SuccessRate*100, m.AvgCost, m.AvgLatency)
			totalCalls += m.Count
			totalCost += m.AvgCost * float64(m.Count)
		}
		body += "\n"
	}
	body += fmt.Sprintf("---\n합계: %d 호출, 총 비용 $%.6f\n", totalCalls, totalCost)

	_, err = h.cfg.KnowledgeStore.Create(knowledge.TypeInsight, map[string]any{
		"title":  title,
		"domain": "observe",
		"tags":   []string{"llm-usage", "session-report"},
	}, body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cq serve: [harness_watcher] llm_usage_report: knowledge record failed: %v\n", err)
		return
	}
	fmt.Fprintf(os.Stderr, "cq serve: [harness_watcher] llm_usage_report: recorded (%d task types, %d calls)\n", len(stats), totalCalls)
}

// Health implements Component.
func (h *HarnessWatcherComponent) Health() ComponentHealth {
	if h.tc == nil {
		return ComponentHealth{Status: "error", Detail: "not started"}
	}
	if h.cfg.SupabaseURL == "" {
		return ComponentHealth{Status: "ok", Detail: "trace recording active (journal push disabled: cloud.url not configured)"}
	}
	if h.watcher == nil {
		return ComponentHealth{Status: "ok", Detail: "trace recording active (journal push not started)"}
	}
	return ComponentHealth{Status: "ok"}
}
