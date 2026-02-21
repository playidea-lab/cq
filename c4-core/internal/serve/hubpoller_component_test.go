//go:build c5_hub

package serve

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/changmin/c4-core/internal/config"
)

func TestHubPollerComponent_Name(t *testing.T) {
	c := NewHubPollerComponent(config.HubConfig{}, nil, "")
	if c.Name() != "hubpoller" {
		t.Errorf("Name() = %q, want %q", c.Name(), "hubpoller")
	}
}

func TestHubPollerComponent_HealthBeforeStart(t *testing.T) {
	c := NewHubPollerComponent(config.HubConfig{}, nil, "")
	h := c.Health()
	if h.Status != "error" {
		t.Errorf("Health().Status = %q, want %q", h.Status, "error")
	}
	if h.Detail != "not started" {
		t.Errorf("Health().Detail = %q, want %q", h.Detail, "not started")
	}
}

func TestHubPollerComponent_StartNoURL(t *testing.T) {
	c := NewHubPollerComponent(config.HubConfig{Enabled: true}, nil, "")
	err := c.Start(context.Background())
	if err == nil {
		t.Fatal("expected error when URL is empty")
	}
	if err.Error() != "hub URL not configured" {
		t.Errorf("error = %q, want %q", err.Error(), "hub URL not configured")
	}
}

func TestHubPollerComponent_StartStop(t *testing.T) {
	cfg := config.HubConfig{
		Enabled: true,
		URL:     "http://127.0.0.1:19999",
	}

	c := NewHubPollerComponent(cfg, nil, "test-project")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := c.Start(ctx)
	if err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	// After start, health should be "ok" (retryCount is 0)
	h := c.Health()
	if h.Status != "ok" {
		t.Errorf("Health().Status after Start = %q, want %q", h.Status, "ok")
	}

	// Stop should not error
	if err := c.Stop(context.Background()); err != nil {
		t.Errorf("Stop() error: %v", err)
	}

	// Stop again should be safe (idempotent)
	if err := c.Stop(context.Background()); err != nil {
		t.Errorf("Stop() second call error: %v", err)
	}
}

func TestHubPollerComponent_DegradedAfterRetries(t *testing.T) {
	cfg := config.HubConfig{
		Enabled: true,
		URL:     "http://127.0.0.1:19999",
	}

	c := NewHubPollerComponent(cfg, nil, "test-project")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	defer c.Stop(context.Background())

	// Simulate consecutive health check failures by directly setting retryCount.
	// Calling checkHealth() would be slow due to hub client's retry+backoff.
	c.mu.Lock()
	c.retryCount = hubPollerDegradedThreshold
	c.lastCheckErr = nil
	c.mu.Unlock()

	h := c.Health()
	if h.Status != "degraded" {
		t.Errorf("Health().Status after %d failures = %q, want %q",
			hubPollerDegradedThreshold, h.Status, "degraded")
	}
	if h.Detail == "" {
		t.Error("Health().Detail should not be empty when degraded")
	}
}

func TestHubPollerComponent_DegradedWithError(t *testing.T) {
	cfg := config.HubConfig{
		Enabled: true,
		URL:     "http://127.0.0.1:19999",
	}

	c := NewHubPollerComponent(cfg, nil, "test-project")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	defer c.Stop(context.Background())

	// Simulate failures with an error message
	c.mu.Lock()
	c.retryCount = hubPollerDegradedThreshold
	c.lastCheckErr = fmt.Errorf("connection refused")
	c.mu.Unlock()

	h := c.Health()
	if h.Status != "degraded" {
		t.Errorf("Health().Status = %q, want %q", h.Status, "degraded")
	}
	if h.Detail == "" {
		t.Error("Health().Detail should contain error info")
	}
}

func TestHubPollerComponent_HealthRecovery(t *testing.T) {
	cfg := config.HubConfig{
		Enabled: true,
		URL:     "http://127.0.0.1:19999",
	}

	c := NewHubPollerComponent(cfg, nil, "test-project")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	defer c.Stop(context.Background())

	// Simulate degraded state
	c.mu.Lock()
	c.retryCount = hubPollerDegradedThreshold
	c.lastCheckErr = fmt.Errorf("health check failed")
	c.mu.Unlock()

	if c.Health().Status != "degraded" {
		t.Fatal("expected degraded status")
	}

	// Simulate recovery (as checkHealth would do on success)
	c.mu.Lock()
	c.retryCount = 0
	c.lastCheckErr = nil
	c.mu.Unlock()

	h := c.Health()
	if h.Status != "ok" {
		t.Errorf("Health().Status after recovery = %q, want %q", h.Status, "ok")
	}
}

func TestHubPollerComponent_ImplementsComponent(t *testing.T) {
	var _ Component = (*HubPollerComponent)(nil)
}

func TestHubPollerComponent_HealthLoopStopsOnCancel(t *testing.T) {
	cfg := config.HubConfig{
		Enabled: true,
		URL:     "http://127.0.0.1:19999",
	}

	c := NewHubPollerComponent(cfg, nil, "test-project")

	ctx, cancel := context.WithCancel(context.Background())

	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	// Cancel context -- health loop should exit
	cancel()

	// Give goroutines time to exit
	time.Sleep(50 * time.Millisecond)

	// Stop should still be safe
	if err := c.Stop(context.Background()); err != nil {
		t.Errorf("Stop() error after cancel: %v", err)
	}
}

func TestHubPollerComponent_BelowThreshold(t *testing.T) {
	cfg := config.HubConfig{
		Enabled: true,
		URL:     "http://127.0.0.1:19999",
	}

	c := NewHubPollerComponent(cfg, nil, "test-project")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	defer c.Stop(context.Background())

	// Set retryCount just below threshold -- should still be "ok"
	c.mu.Lock()
	c.retryCount = hubPollerDegradedThreshold - 1
	c.mu.Unlock()

	h := c.Health()
	if h.Status != "ok" {
		t.Errorf("Health().Status at threshold-1 = %q, want %q", h.Status, "ok")
	}
}
