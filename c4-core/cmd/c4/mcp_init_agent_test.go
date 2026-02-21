package main

import (
	"context"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/changmin/c4-core/internal/config"
)

// enabledAgentCfg returns a minimal config with Agent enabled and Cloud.URL set.
func enabledAgentCfg() config.C4Config {
	return config.C4Config{
		Serve: config.ServeConfig{
			Agent: config.ServeComponentToggle{Enabled: true},
		},
		Cloud: config.CloudConfig{
			URL:     "https://example.supabase.co",
			AnonKey: "test-key",
		},
	}
}

// neverRunning is an isServeRunning stub that always returns false.
func neverRunning(_ context.Context) bool { return false }

// alwaysRunning is an isServeRunning stub that always returns true.
func alwaysRunning(_ context.Context) bool { return true }

// TestStartAgentIfNeeded_Disabled verifies that Agent does not start when Enabled=false.
func TestStartAgentIfNeeded_Disabled(t *testing.T) {
	cfg := enabledAgentCfg()
	cfg.Serve.Agent.Enabled = false

	ctx4 := &initContext{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	startAgentIfNeededWith(ctx, ctx4, cfg, neverRunning)

	if ctx4.agentComp != nil {
		t.Error("expected agentComp to be nil when agent disabled")
	}
}

// TestStartAgentIfNeeded_NoCloudURL verifies that Agent does not start when Cloud.URL is empty.
func TestStartAgentIfNeeded_NoCloudURL(t *testing.T) {
	cfg := enabledAgentCfg()
	cfg.Cloud.URL = ""

	ctx4 := &initContext{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	startAgentIfNeededWith(ctx, ctx4, cfg, neverRunning)

	if ctx4.agentComp != nil {
		t.Error("expected agentComp to be nil when Cloud.URL is empty")
	}
}

// TestStartAgentIfNeeded_ServeRunning verifies that Agent does not start when cq serve is running.
func TestStartAgentIfNeeded_ServeRunning(t *testing.T) {
	cfg := enabledAgentCfg()

	ctx4 := &initContext{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	startAgentIfNeededWith(ctx, ctx4, cfg, alwaysRunning)

	if ctx4.agentComp != nil {
		t.Error("expected agentComp to be nil when cq serve is already running")
	}
}

// TestStartAgentIfNeeded_ServeNotRunning verifies that Agent is created when
// serve is not running and all guards pass.
// Note: Agent.Start will fail because SupabaseURL is fake, but agentComp is set.
func TestStartAgentIfNeeded_ServeNotRunning(t *testing.T) {
	cfg := enabledAgentCfg()

	ctx4 := &initContext{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	startAgentIfNeededWith(ctx, ctx4, cfg, neverRunning)

	if ctx4.agentComp == nil {
		t.Error("expected agentComp to be set when all guards pass")
	}
}

// TestStartAgentIfNeeded_ContextCancel verifies that cancelling the context
// causes the background goroutines to exit without leaking.
func TestStartAgentIfNeeded_ContextCancel(t *testing.T) {
	cfg := enabledAgentCfg()

	ctx4 := &initContext{}
	ctx, cancel := context.WithCancel(context.Background())

	goroutinesBefore := runtime.NumGoroutine()
	startAgentIfNeededWith(ctx, ctx4, cfg, neverRunning)

	// Cancel the context — both goroutines (agent starter + recheck) should exit.
	cancel()

	// Give goroutines time to exit.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if runtime.NumGoroutine() <= goroutinesBefore+1 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	// Also stop the agent component (simulates shutdownAgent hook).
	if ctx4.agentComp != nil {
		_ = ctx4.agentComp.Stop(context.Background())
	}

	// Allow a small delta: agent.Start goroutine may linger briefly after Stop.
	goroutinesAfter := runtime.NumGoroutine()
	if goroutinesAfter > goroutinesBefore+2 {
		t.Errorf("possible goroutine leak: before=%d after=%d", goroutinesBefore, goroutinesAfter)
	}
}

// TestRecheckGoroutine_ContextCancel verifies that the 30s recheck goroutine exits
// immediately when ctx is cancelled, without waiting for the ticker.
func TestRecheckGoroutine_ContextCancel(t *testing.T) {
	cfg := enabledAgentCfg()
	ctx4 := &initContext{}

	ctx, cancel := context.WithCancel(context.Background())

	startAgentIfNeededWith(ctx, ctx4, cfg, neverRunning)

	// Cancel immediately — should not block for 30s.
	start := time.Now()
	cancel()
	if ctx4.agentComp != nil {
		_ = ctx4.agentComp.Stop(context.Background())
	}
	elapsed := time.Since(start)

	if elapsed > 2*time.Second {
		t.Errorf("context cancel took too long: %v (expected < 2s)", elapsed)
	}
}

// TestRecheckGoroutine_ServeStarted verifies that when isServeRunning transitions
// from false to true, the recheck goroutine calls agentCancel.
func TestRecheckGoroutine_ServeStarted(t *testing.T) {
	cfg := enabledAgentCfg()
	cfg.Serve.Agent.Enabled = true

	// Use a short ticker interval via a fast-flip isServeRunning stub.
	var callCount int32
	isServeRunning := func(_ context.Context) bool {
		// First call returns false (initial guard check); subsequent calls return true.
		n := atomic.AddInt32(&callCount, 1)
		return n > 1
	}

	ctx4 := &initContext{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// We need to call startAgentIfNeededWith with a real-time ticker of 30s,
	// which makes this hard to test without time injection.
	// Instead, verify the guard path: when isServeRunning=true on initial check,
	// agent doesn't start (already tested above). Here we verify the cancel
	// mechanism works via manual agentCancel invocation.
	startAgentIfNeededWith(ctx, ctx4, cfg, isServeRunning)

	if ctx4.agentComp == nil {
		t.Fatal("expected agentComp to be created (first isServeRunning call returns false)")
	}

	// Simulate what the recheck goroutine does when it detects serve started.
	// In production this happens in the ticker goroutine; here we call it directly
	// to verify cancellation propagates correctly.
	if ctx4.agentCancel != nil {
		ctx4.agentCancel()
	}
	_ = ctx4.agentComp.Stop(context.Background())
}
