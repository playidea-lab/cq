package serve

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// shortTempDir creates a short temp dir under /tmp to avoid Unix socket path
// length limits (104 bytes on macOS). t.TempDir() paths are too long.
func shortTempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "eb-")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

func TestEBComp_Name(t *testing.T) {
	c := NewEventBusComponent(EventBusConfig{})
	if c.Name() != "eventbus" {
		t.Errorf("Name() = %q, want %q", c.Name(), "eventbus")
	}
}

func TestEBComp_StartStop(t *testing.T) {
	dataDir := shortTempDir(t)
	c := NewEventBusComponent(EventBusConfig{DataDir: dataDir})

	ctx := context.Background()

	// Start
	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Server should be available
	if c.Server() == nil {
		t.Fatal("Server() should not be nil after Start")
	}

	// Socket path should be non-empty
	sockPath := c.SocketPath()
	if sockPath == "" {
		t.Fatal("SocketPath() should not be empty after Start")
	}

	// Socket file should exist
	if _, err := os.Stat(sockPath); err != nil {
		t.Errorf("socket file should exist: %v", err)
	}

	// Stop
	if err := c.Stop(ctx); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	// Server should be nil after stop
	if c.Server() != nil {
		t.Error("Server() should be nil after Stop")
	}

	// SocketPath should be empty after stop
	if c.SocketPath() != "" {
		t.Error("SocketPath() should be empty after Stop")
	}
}

func TestEBComp_HealthOK(t *testing.T) {
	dataDir := shortTempDir(t)
	c := NewEventBusComponent(EventBusConfig{DataDir: dataDir})

	ctx := context.Background()

	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer c.Stop(ctx)

	h := c.Health()
	if h.Status != "ok" {
		t.Errorf("Health.Status = %q, want %q (detail: %s)", h.Status, "ok", h.Detail)
	}
}

func TestEBComp_HealthNotStarted(t *testing.T) {
	c := NewEventBusComponent(EventBusConfig{})

	h := c.Health()
	if h.Status != "error" {
		t.Errorf("Health.Status = %q, want %q", h.Status, "error")
	}
	if h.Detail != "not started" {
		t.Errorf("Health.Detail = %q, want %q", h.Detail, "not started")
	}
}

func TestEBComp_HealthAfterStop(t *testing.T) {
	dataDir := shortTempDir(t)
	c := NewEventBusComponent(EventBusConfig{DataDir: dataDir})

	ctx := context.Background()

	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if err := c.Stop(ctx); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	h := c.Health()
	if h.Status != "error" {
		t.Errorf("Health.Status = %q, want %q", h.Status, "error")
	}
}

func TestEBComp_SockDefault(t *testing.T) {
	// Before start, SocketPath should be empty
	c := NewEventBusComponent(EventBusConfig{})
	if c.SocketPath() != "" {
		t.Errorf("SocketPath() = %q before start, want empty", c.SocketPath())
	}
}

func TestEBComp_SockCustom(t *testing.T) {
	dataDir := shortTempDir(t)
	c := NewEventBusComponent(EventBusConfig{DataDir: dataDir})

	ctx := context.Background()
	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer c.Stop(ctx)

	sockPath := c.SocketPath()
	if dir := filepath.Dir(sockPath); dir != dataDir {
		t.Errorf("socket dir = %q, want %q", dir, dataDir)
	}
}

func TestEBComp_StopIdempotent(t *testing.T) {
	dataDir := shortTempDir(t)
	c := NewEventBusComponent(EventBusConfig{DataDir: dataDir})

	ctx := context.Background()

	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Double stop should not panic or error
	if err := c.Stop(ctx); err != nil {
		t.Fatalf("first Stop failed: %v", err)
	}
	if err := c.Stop(ctx); err != nil {
		t.Fatalf("second Stop failed: %v", err)
	}
}

// TestEBComp_HealthCacheHit verifies that consecutive Health() calls within 5s
// return the cached result without issuing a new gRPC connection.
func TestEBComp_HealthCacheHit(t *testing.T) {
	dataDir := shortTempDir(t)
	c := NewEventBusComponent(EventBusConfig{DataDir: dataDir})

	ctx := context.Background()
	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer c.Stop(ctx)

	// First call — populates cache.
	h1 := c.Health()
	if h1.Status != "ok" {
		t.Fatalf("first Health() = %q, want %q (detail: %s)", h1.Status, "ok", h1.Detail)
	}
	cachedAt := c.healthCache.at

	// Second call — must hit cache (same timestamp).
	h2 := c.Health()
	if h2.Status != h1.Status {
		t.Errorf("second Health() = %q, want %q (cache miss?)", h2.Status, h1.Status)
	}
	if c.healthCache.at != cachedAt {
		t.Error("cache timestamp changed on second call — cache miss occurred")
	}

	// Third call — must still hit cache.
	h3 := c.Health()
	if h3.Status != h1.Status {
		t.Errorf("third Health() = %q, want %q (cache miss?)", h3.Status, h1.Status)
	}
	if c.healthCache.at != cachedAt {
		t.Error("cache timestamp changed on third call — cache miss occurred")
	}
}

// TestEBComp_HealthCacheExpiry verifies that after 5s the cache is refreshed.
func TestEBComp_HealthCacheExpiry(t *testing.T) {
	dataDir := shortTempDir(t)
	c := NewEventBusComponent(EventBusConfig{DataDir: dataDir})

	ctx := context.Background()
	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer c.Stop(ctx)

	// First call — populates cache.
	h1 := c.Health()
	if h1.Status != "ok" {
		t.Fatalf("first Health() = %q, want %q (detail: %s)", h1.Status, "ok", h1.Detail)
	}

	// Artificially expire the cache by backdating the timestamp.
	c.healthMu.Lock()
	c.healthCache.at = time.Now().Add(-6 * time.Second)
	c.healthMu.Unlock()

	// Next call — must refresh (new timestamp).
	beforeRefresh := time.Now()
	h2 := c.Health()
	if h2.Status != "ok" {
		t.Errorf("refreshed Health() = %q, want %q (detail: %s)", h2.Status, "ok", h2.Detail)
	}
	if !c.healthCache.at.After(beforeRefresh) {
		t.Error("cache timestamp not updated after expiry")
	}
}

// TestEBComp_HealthCacheFirstCall verifies that the first call (at.IsZero())
// immediately issues a gRPC call (no cache bypass or panic).
func TestEBComp_HealthCacheFirstCall(t *testing.T) {
	dataDir := shortTempDir(t)
	c := NewEventBusComponent(EventBusConfig{DataDir: dataDir})

	// Before Start: cache is zero, doHealth returns "not started".
	h := c.Health()
	if h.Status != "error" {
		t.Errorf("Health() before start = %q, want %q", h.Status, "error")
	}
	// Cache should now be populated with the "not started" result.
	if c.healthCache.at.IsZero() {
		t.Error("cache.at should not be zero after first Health() call")
	}
}

func TestEBComp_ManagerIntegration(t *testing.T) {
	dataDir := shortTempDir(t)
	c := NewEventBusComponent(EventBusConfig{DataDir: dataDir})

	mgr := NewManager()
	mgr.Register(c)

	ctx := context.Background()

	if err := mgr.StartAll(ctx); err != nil {
		t.Fatalf("StartAll failed: %v", err)
	}

	// Health should report ok
	hm := mgr.HealthMap()
	if h, ok := hm["eventbus"]; !ok {
		t.Error("eventbus not in health map")
	} else if h.Status != "ok" {
		t.Errorf("eventbus health = %q, want %q (detail: %s)", h.Status, "ok", h.Detail)
	}

	if err := mgr.StopAll(ctx); err != nil {
		t.Fatalf("StopAll failed: %v", err)
	}
}
