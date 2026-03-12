//go:build research

package main

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type mockNotifier struct {
	mu    sync.Mutex
	calls []string // event names
	err   error
}

func (m *mockNotifier) Notify(_ context.Context, _, _, event string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, event)
	return m.err
}

func (m *mockNotifier) callCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.calls)
}

// TestNotifyBridge_Dedup: same event called twice within cooldown → only 1 call sent.
func TestNotifyBridge_Dedup(t *testing.T) {
	mn := &mockNotifier{}
	b := NewNotifyBridge(mn, 1*time.Hour)

	b.Emit(context.Background(), EventDebateComplete, "title", "msg")
	b.Emit(context.Background(), EventDebateComplete, "title", "msg")

	if got := mn.callCount(); got != 1 {
		t.Errorf("expected 1 call, got %d", got)
	}
}

// TestNotifyBridge_SameEventDifferentTitle: same event, different title → cooldown key=event → 1 call.
func TestNotifyBridge_SameEventDifferentTitle(t *testing.T) {
	mn := &mockNotifier{}
	b := NewNotifyBridge(mn, 1*time.Hour)

	b.Emit(context.Background(), EventHypothesisRegistered, "title-A", "msg-A")
	b.Emit(context.Background(), EventHypothesisRegistered, "title-B", "msg-B")

	if got := mn.callCount(); got != 1 {
		t.Errorf("expected 1 call (cooldown keyed by event only), got %d", got)
	}
}

// TestNotifyBridge_DifferentEvents: two different events → each sent independently.
func TestNotifyBridge_DifferentEvents(t *testing.T) {
	mn := &mockNotifier{}
	b := NewNotifyBridge(mn, 1*time.Hour)

	b.Emit(context.Background(), EventBudgetWarning, "budget", "80%")
	b.Emit(context.Background(), EventGateEntered, "gate", "waiting")

	if got := mn.callCount(); got != 2 {
		t.Errorf("expected 2 calls for different events, got %d", got)
	}
}

// TestNotifyBridge_NonFatal: notifier returns error → Emit returns normally, no panic.
func TestNotifyBridge_NonFatal(t *testing.T) {
	mn := &mockNotifier{err: errors.New("dooray down")}
	b := NewNotifyBridge(mn, 1*time.Hour)

	// Must not panic.
	b.Emit(context.Background(), EventAutoContinued, "auto", "resumed")

	if got := mn.callCount(); got != 1 {
		t.Errorf("expected notifier to be called once, got %d", got)
	}
}

// TestNotifyBridge_CooldownExpiry: cooldown=1ms → after sleep, same event is sent again.
func TestNotifyBridge_CooldownExpiry(t *testing.T) {
	mn := &mockNotifier{}
	b := NewNotifyBridge(mn, 1*time.Millisecond)

	b.Emit(context.Background(), EventDebateComplete, "t", "m")
	time.Sleep(5 * time.Millisecond)
	b.Emit(context.Background(), EventDebateComplete, "t", "m")

	if got := mn.callCount(); got != 2 {
		t.Errorf("expected 2 calls after cooldown expiry, got %d", got)
	}
}
