package serve

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/changmin/c4-core/internal/config"
	"github.com/changmin/c4-core/internal/eventbus"
	"github.com/changmin/c4-core/internal/mcp/handlers"
)

// StaleTaskStore is the data access interface required by StaleCheckerComponent.
// It is satisfied by *handlers.SQLiteStore.
type StaleTaskStore interface {
	StaleTasks(minMinutes int) ([]handlers.Task, error)
	ResetTask(taskID string) error
}

// tickerFn is a function that produces a *time.Ticker; used for clock injection in tests.
type tickerFn func(d time.Duration) *time.Ticker

// StaleCheckerComponent implements the Component interface.
// It periodically scans for tasks stuck in in_progress and resets them to pending.
type StaleCheckerComponent struct {
	store     StaleTaskStore
	pub       eventbus.Publisher
	cfg       config.StaleCheckerConfig
	newTicker tickerFn

	mu      sync.Mutex
	cancel  context.CancelFunc
	started bool
}

// NewStaleChecker creates a StaleCheckerComponent with production defaults.
func NewStaleChecker(store StaleTaskStore, pub eventbus.Publisher, cfg config.StaleCheckerConfig) *StaleCheckerComponent {
	return newStaleCheckerWithTicker(store, pub, cfg, time.NewTicker)
}

// newStaleCheckerWithTicker creates a StaleCheckerComponent with a custom ticker factory.
// Used in tests to avoid real-time delays.
func newStaleCheckerWithTicker(store StaleTaskStore, pub eventbus.Publisher, cfg config.StaleCheckerConfig, newTick tickerFn) *StaleCheckerComponent {
	if cfg.ThresholdMinutes <= 0 {
		cfg.ThresholdMinutes = 30
	}
	if cfg.IntervalSeconds <= 0 {
		cfg.IntervalSeconds = 60
	}
	if newTick == nil {
		newTick = time.NewTicker
	}
	return &StaleCheckerComponent{
		store:     store,
		pub:       pub,
		cfg:       cfg,
		newTicker: newTick,
	}
}

// Name implements Component.
func (s *StaleCheckerComponent) Name() string { return "stale_checker" }

// Start launches the background check loop.
func (s *StaleCheckerComponent) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)

	s.mu.Lock()
	s.cancel = cancel
	s.started = true
	s.mu.Unlock()

	go s.run(ctx)
	return nil
}

// Stop cancels the background loop.
func (s *StaleCheckerComponent) Stop(_ context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	return nil
}

// Health implements Component.
// Returns "error" before Start is called, "ok" after Start (including after Stop).
func (s *StaleCheckerComponent) Health() ComponentHealth {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.started {
		return ComponentHealth{Status: "error", Detail: "not started"}
	}
	return ComponentHealth{Status: "ok"}
}

// run is the background goroutine that drives periodic stale-task checks.
func (s *StaleCheckerComponent) run(ctx context.Context) {
	ticker := s.newTicker(time.Duration(s.cfg.IntervalSeconds) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.check()
		}
	}
}

// check queries for stale tasks and resets each one.
func (s *StaleCheckerComponent) check() {
	tasks, err := s.store.StaleTasks(s.cfg.ThresholdMinutes)
	if err != nil {
		slog.Warn("stale_checker: StaleTasks failed", "err", err)
		return
	}

	for _, t := range tasks {
		if err := s.store.ResetTask(t.ID); err != nil {
			slog.Warn("stale_checker: ResetTask failed", "task_id", t.ID, "err", err)
			continue
		}
		slog.Info("stale_checker: reset stale task", "task_id", t.ID, "worker_id", t.WorkerID)

		if s.pub != nil {
			payload, _ := json.Marshal(map[string]any{
				"task_id":       t.ID,
				"worker_id":     t.WorkerID,
				"stale_minutes": s.cfg.ThresholdMinutes,
			})
			s.pub.PublishAsync("task.stale", "c4.stale_checker", json.RawMessage(payload), "")
		}
	}
}
