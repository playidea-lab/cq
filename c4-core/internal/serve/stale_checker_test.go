package serve

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/changmin/c4-core/internal/config"
	"github.com/changmin/c4-core/internal/mcp/handlers"
)

// --- mock implementations ---

type mockStaleStore struct {
	mu         sync.Mutex
	staleTasks []handlers.Task
	staleErr   error
	resetCalls []string
	resetErr   error
}

func (m *mockStaleStore) StaleTasks(_ int) ([]handlers.Task, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.staleTasks, m.staleErr
}

func (m *mockStaleStore) ResetTask(taskID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.resetCalls = append(m.resetCalls, taskID)
	return m.resetErr
}

func (m *mockStaleStore) getResetCalls() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]string, len(m.resetCalls))
	copy(cp, m.resetCalls)
	return cp
}

type mockPublisher struct {
	mu     sync.Mutex
	events []publishedEvent
}

type publishedEvent struct {
	evType    string
	source    string
	data      json.RawMessage
	projectID string
}

func (p *mockPublisher) PublishAsync(evType, source string, data json.RawMessage, projectID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.events = append(p.events, publishedEvent{evType, source, data, projectID})
}

func (p *mockPublisher) getEvents() []publishedEvent {
	p.mu.Lock()
	defer p.mu.Unlock()
	cp := make([]publishedEvent, len(p.events))
	copy(cp, p.events)
	return cp
}

// controlledTicker wraps a manually-driven channel as a *time.Ticker.
// The returned send channel can be used to trigger ticks in tests.
type controlledTicker struct {
	ticker *time.Ticker
	send   chan time.Time
}

func newControlledTicker() *controlledTicker {
	ch := make(chan time.Time, 1)
	// Create a ticker with a very long interval so it won't fire on its own.
	t := time.NewTicker(24 * time.Hour)
	t.C = ch // replace the channel
	return &controlledTicker{ticker: t, send: ch}
}

func (c *controlledTicker) tick() {
	c.send <- time.Now()
}

func (c *controlledTicker) factory() tickerFn {
	return func(_ time.Duration) *time.Ticker {
		return c.ticker
	}
}

// --- tests ---

// TestStaleChecker_DetectsAndResets verifies that stale tasks are reset and events are published.
func TestStaleChecker_DetectsAndResets(t *testing.T) {
	store := &mockStaleStore{
		staleTasks: []handlers.Task{
			{ID: "T-001-0", WorkerID: "w1", Status: "in_progress"},
			{ID: "T-002-0", WorkerID: "w2", Status: "in_progress"},
		},
	}
	pub := &mockPublisher{}

	ct := newControlledTicker()
	sc := newStaleCheckerWithTicker(store, pub, config.StaleCheckerConfig{
		Enabled:          true,
		ThresholdMinutes: 30,
		IntervalSeconds:  60,
	}, ct.factory())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := sc.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer sc.Stop(ctx)

	// Give goroutine time to reach the select.
	time.Sleep(10 * time.Millisecond)

	// Trigger one check cycle.
	ct.tick()

	// Give the goroutine time to process.
	time.Sleep(50 * time.Millisecond)

	resetCalls := store.getResetCalls()
	if len(resetCalls) != 2 {
		t.Errorf("ResetTask called %d times, want 2", len(resetCalls))
	}

	events := pub.getEvents()
	if len(events) != 2 {
		t.Errorf("PublishAsync called %d times, want 2", len(events))
	}
	for _, ev := range events {
		if ev.evType != "task.stale" {
			t.Errorf("event type = %q, want %q", ev.evType, "task.stale")
		}
		if ev.source != "c4.stale_checker" {
			t.Errorf("event source = %q, want %q", ev.source, "c4.stale_checker")
		}
		var payload map[string]any
		if err := json.Unmarshal(ev.data, &payload); err != nil {
			t.Errorf("unmarshal event data: %v", err)
		}
		if _, ok := payload["task_id"]; !ok {
			t.Error("event payload missing task_id")
		}
	}
}

// TestStaleChecker_NoStaleTasks verifies that ResetTask is not called when there are no stale tasks.
func TestStaleChecker_NoStaleTasks(t *testing.T) {
	store := &mockStaleStore{staleTasks: nil}
	pub := &mockPublisher{}

	ct := newControlledTicker()
	sc := newStaleCheckerWithTicker(store, pub, config.StaleCheckerConfig{
		Enabled:          true,
		ThresholdMinutes: 30,
		IntervalSeconds:  60,
	}, ct.factory())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := sc.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer sc.Stop(ctx)

	time.Sleep(10 * time.Millisecond)
	ct.tick()
	time.Sleep(50 * time.Millisecond)

	if calls := store.getResetCalls(); len(calls) != 0 {
		t.Errorf("ResetTask called %d times, want 0", len(calls))
	}
	if evs := pub.getEvents(); len(evs) != 0 {
		t.Errorf("PublishAsync called %d times, want 0", len(evs))
	}
}

// TestStaleChecker_NilPublisher verifies that ResetTask is still called when publisher is nil.
func TestStaleChecker_NilPublisher(t *testing.T) {
	store := &mockStaleStore{
		staleTasks: []handlers.Task{
			{ID: "T-010-0", WorkerID: "w3", Status: "in_progress"},
		},
	}

	ct := newControlledTicker()
	sc := newStaleCheckerWithTicker(store, nil, config.StaleCheckerConfig{
		Enabled:          true,
		ThresholdMinutes: 30,
		IntervalSeconds:  60,
	}, ct.factory())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := sc.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer sc.Stop(ctx)

	// Must not panic even with nil publisher.
	time.Sleep(10 * time.Millisecond)
	ct.tick()
	time.Sleep(50 * time.Millisecond)

	if calls := store.getResetCalls(); len(calls) != 1 || calls[0] != "T-010-0" {
		t.Errorf("ResetTask calls = %v, want [T-010-0]", calls)
	}
}

// TestStaleChecker_Health verifies health transitions: not started → error, started → ok, stopped → ok.
func TestStaleChecker_Health(t *testing.T) {
	store := &mockStaleStore{}
	sc := NewStaleChecker(store, nil, config.StaleCheckerConfig{
		Enabled:          true,
		ThresholdMinutes: 30,
		IntervalSeconds:  60,
	})

	// Before Start: health should be error.
	h := sc.Health()
	if h.Status != "error" {
		t.Errorf("before Start: Status = %q, want %q", h.Status, "error")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := sc.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// After Start: health should be ok.
	h = sc.Health()
	if h.Status != "ok" {
		t.Errorf("after Start: Status = %q, want %q", h.Status, "ok")
	}

	if err := sc.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	// After Stop: health should still be ok (started flag remains true).
	h = sc.Health()
	if h.Status != "ok" {
		t.Errorf("after Stop: Status = %q, want %q", h.Status, "ok")
	}
}

// TestStaleChecker_StaleTasks_Error verifies that StaleTasks errors are handled gracefully.
func TestStaleChecker_StaleTasks_Error(t *testing.T) {
	store := &mockStaleStore{staleErr: errors.New("db error")}

	ct := newControlledTicker()
	sc := newStaleCheckerWithTicker(store, nil, config.StaleCheckerConfig{
		Enabled:          true,
		ThresholdMinutes: 30,
		IntervalSeconds:  60,
	}, ct.factory())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := sc.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer sc.Stop(ctx)

	time.Sleep(10 * time.Millisecond)
	// Should not panic on error.
	ct.tick()
	time.Sleep(50 * time.Millisecond)

	if calls := store.getResetCalls(); len(calls) != 0 {
		t.Errorf("ResetTask should not be called on StaleTasks error; got %d calls", len(calls))
	}
}
