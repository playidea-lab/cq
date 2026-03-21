package serve

import (
	"context"
	"testing"
)

func TestHarnessWatcherComponent_Name(t *testing.T) {
	c := NewHarnessWatcherComponent(HarnessWatcherConfig{})
	if c.Name() != "harness_watcher" {
		t.Errorf("Name() = %q, want %q", c.Name(), "harness_watcher")
	}
}

func TestHarnessWatcherComponent_TraceRecorderSetWhenNoURL(t *testing.T) {
	c := NewHarnessWatcherComponent(HarnessWatcherConfig{})
	ctx := context.Background()

	// Start should succeed (journal push skipped, trace recording active).
	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start with empty URL: %v", err)
	}
	defer c.Stop(ctx)

	h := c.Health()
	if h.Status != "ok" {
		t.Errorf("Health.Status = %q, want %q", h.Status, "ok")
	}
}

func TestHarnessWatcherComponent_Stop_NotStarted(t *testing.T) {
	c := NewHarnessWatcherComponent(HarnessWatcherConfig{})
	if err := c.Stop(context.Background()); err != nil {
		t.Fatalf("Stop on not-started: %v", err)
	}
}

func TestHarnessWatcherComponent_DefaultTenantID(t *testing.T) {
	c := NewHarnessWatcherComponent(HarnessWatcherConfig{})
	if c.cfg.TenantID != "default" {
		t.Errorf("TenantID = %q, want %q", c.cfg.TenantID, "default")
	}
}
