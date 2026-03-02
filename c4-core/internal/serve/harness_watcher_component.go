package serve

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/changmin/c4-core/internal/c1push"
	"github.com/changmin/c4-core/internal/harness"
)

// HarnessWatcherConfig holds configuration for HarnessWatcherComponent.
type HarnessWatcherConfig struct {
	// SupabaseURL is the Supabase project URL. If empty, the component is a no-op.
	SupabaseURL string
	// AnonKey is the Supabase anon key.
	AnonKey string
	// TenantID defaults to "default" if empty.
	TenantID string
}

// HarnessWatcherComponent watches ~/.claude/projects/**/*.jsonl and pushes new
// lines to c1_channels via c1push.Pusher. Activated only when cloud.url is set.
type HarnessWatcherComponent struct {
	cfg     HarnessWatcherConfig
	watcher *harness.JournalWatcher
	store   *harness.PositionStore
	cancel  context.CancelFunc
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
// No-op if SupabaseURL is empty.
func (h *HarnessWatcherComponent) Start(ctx context.Context) error {
	if h.cfg.SupabaseURL == "" {
		fmt.Fprintf(os.Stderr, "cq serve: [harness_watcher] cloud.url not configured — skipping\n")
		return nil
	}

	pusher := c1push.New(h.cfg.SupabaseURL, h.cfg.AnonKey)
	if pusher == nil {
		fmt.Fprintf(os.Stderr, "cq serve: [harness_watcher] missing supabase credentials — skipping\n")
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
	fmt.Fprintf(os.Stderr, "cq serve: [harness_watcher] stopped\n")
	return nil
}

// Health implements Component.
func (h *HarnessWatcherComponent) Health() ComponentHealth {
	if h.cfg.SupabaseURL == "" {
		return ComponentHealth{Status: "skipped", Detail: "cloud.url not configured"}
	}
	if h.watcher == nil {
		return ComponentHealth{Status: "error", Detail: "not started"}
	}
	return ComponentHealth{Status: "ok"}
}
