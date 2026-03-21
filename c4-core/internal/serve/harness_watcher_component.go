package serve

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"github.com/changmin/c4-core/internal/c1push"
	"github.com/changmin/c4-core/internal/harness"
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
}

// HarnessWatcherComponent watches ~/.claude/projects/**/*.jsonl and pushes new
// lines to c1_channels via c1push.Pusher. Activated only when cloud.url is set.
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

	pusher := c1push.New(h.cfg.SupabaseURL, h.cfg.AnonKey)
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
	// Clear global trace recorder and flush pending writes.
	harness.SetTraceRecorder(nil)
	if h.tc != nil {
		h.tc.Close()
		h.tc = nil
	}
	fmt.Fprintf(os.Stderr, "cq serve: [harness_watcher] stopped\n")
	return nil
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
