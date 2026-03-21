//go:build research

package orchestrator

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// LoopEvent represents a named event from the research loop.
type LoopEvent = string

const (
	EventDebateComplete       LoopEvent = "debate_complete"
	EventHypothesisRegistered LoopEvent = "hypothesis_registered"
	EventBudgetWarning        LoopEvent = "budget_warning"
	EventGateEntered          LoopEvent = "gate_entered"
	EventAutoContinued        LoopEvent = "auto_continued"
)

// Notifier is the interface for sending notifications.
// The c4_notify MCP tool implements this.
type Notifier interface {
	Notify(ctx context.Context, title, message, event string) error
}

// NotifyBridge sends loop events via an injected Notifier with per-event cooldown.
// Non-fatal: errors are logged but never returned to the caller.
type NotifyBridge struct {
	notifier     Notifier
	cooldown     time.Duration
	mu           sync.Mutex
	lastNotified map[string]time.Time
}

// NewNotifyBridge creates a bridge with the given notifier and cooldown duration.
// If notifier is nil, all Emit calls are no-ops.
func NewNotifyBridge(n Notifier, cooldown time.Duration) *NotifyBridge {
	return &NotifyBridge{
		notifier:     n,
		cooldown:     cooldown,
		lastNotified: make(map[string]time.Time),
	}
}

// Emit sends a notification for the event if the cooldown has elapsed.
// Cooldown key is the event string alone (title/message do not affect dedup).
// Always non-fatal: errors are logged via slog.WarnContext, never returned.
func (b *NotifyBridge) Emit(ctx context.Context, event, title, message string) {
	if b.notifier == nil {
		return
	}
	now := time.Now()
	b.mu.Lock()
	last, ok := b.lastNotified[event]
	if ok && now.Sub(last) < b.cooldown {
		b.mu.Unlock()
		return // still in cooldown
	}
	// Reserve the slot before releasing the lock to prevent concurrent sends.
	b.lastNotified[event] = now
	b.mu.Unlock()

	if err := b.notifier.Notify(ctx, title, message, event); err != nil {
		slog.WarnContext(ctx, "notify bridge: failed to send notification",
			"event", event,
			"error", err,
		)
		// Roll back reservation so a retry can be attempted after cooldown.
		b.mu.Lock()
		if t, ok := b.lastNotified[event]; ok && t.Equal(now) {
			delete(b.lastNotified, event)
		}
		b.mu.Unlock()
	}
}
