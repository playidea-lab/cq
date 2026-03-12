//go:build research

package main

import (
	"context"
	"sync"
	"time"
)

// GateController is a one-shot gate that blocks until either the configured
// duration elapses or Release() is called. Each call to EnterGate() returns
// an independent channel, making re-entrant use safe.
type GateController struct {
	duration  time.Duration
	mu        sync.Mutex
	releaseCh chan struct{} // non-nil while a gate is active
}

// NewGateController creates a GateController with the given wait duration.
// A zero or negative duration makes every gate pass through immediately.
func NewGateController(d time.Duration) *GateController {
	return &GateController{duration: d}
}

// Duration returns the configured gate duration.
func (g *GateController) Duration() time.Duration {
	g.mu.Lock()
	d := g.duration
	g.mu.Unlock()
	return d
}

// EnterGate returns a channel that closes when the gate is released.
// Closing conditions: Release() call or duration expiry.
// ctx cancellation does NOT close the channel — gate is human-on-the-loop by design.
// Each call creates an independent channel. A second call releases the previous gate.
func (g *GateController) EnterGate(_ context.Context) <-chan struct{} {
	ch := make(chan struct{})
	d := g.duration
	if d <= 0 {
		close(ch)
		return ch
	}

	releaseCh := make(chan struct{})
	g.mu.Lock()
	old := g.releaseCh
	g.releaseCh = releaseCh
	g.mu.Unlock()
	// Release the previous gate if one was active (prevents goroutine leak).
	if old != nil {
		close(old)
	}

	go func() {
		t := time.NewTimer(d)
		defer t.Stop()
		defer func() {
			g.mu.Lock()
			if g.releaseCh == releaseCh {
				g.releaseCh = nil
			}
			g.mu.Unlock()
		}()
		select {
		case <-t.C:
			close(ch)
		case <-releaseCh:
			close(ch)
		}
	}()
	return ch
}

// Release immediately unblocks the active gate (if any).
// It is safe to call when no gate is active (no-op).
func (g *GateController) Release(reason string) {
	g.mu.Lock()
	ch := g.releaseCh
	g.releaseCh = nil
	g.mu.Unlock()
	if ch != nil {
		close(ch)
	}
}

// IsGating reports whether a gate is currently active.
func (g *GateController) IsGating() bool {
	g.mu.Lock()
	active := g.releaseCh != nil
	g.mu.Unlock()
	return active
}

// Stop releases the active gate. Alias for Release("stop").
func (g *GateController) Stop() {
	g.Release("stop")
}
