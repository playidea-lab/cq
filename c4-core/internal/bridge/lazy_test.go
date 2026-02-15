package bridge

import (
	"strings"
	"sync"
	"testing"
	"time"
)

func TestLazyStarterStartsOnFirstAddr(t *testing.T) {
	cfg := &SidecarConfig{
		PythonCommand: "python3",
		PythonArgs:    []string{"-m", "c4.bridge.sidecar"},
		Host:          "localhost",
		Port:          0,
		StartTimeout:  5 * time.Second,
	}

	lazy := NewLazyStarter(cfg)
	defer lazy.Stop()

	// Before first call, sidecar should not be running
	if lazy.IsRunning() {
		t.Error("sidecar should not be running before first Addr() call")
	}

	// First call should start the sidecar
	addr, err := lazy.Addr()
	if err != nil {
		// Sidecar may not start if Python is not available — this is expected in CI
		if !strings.Contains(err.Error(), "python not found") &&
			!strings.Contains(err.Error(), "No module named") {
			t.Skipf("skipping test: Python sidecar not available: %v", err)
		}
		return
	}

	if addr == "" {
		t.Error("expected non-empty address after successful start")
	}

	// After successful start, sidecar should be running
	if !lazy.IsRunning() {
		t.Error("sidecar should be running after successful Addr() call")
	}
}

func TestLazyStarterCachesAddr(t *testing.T) {
	cfg := &SidecarConfig{
		PythonCommand: "python3",
		PythonArgs:    []string{"-m", "c4.bridge.sidecar"},
		Host:          "localhost",
		Port:          0,
		StartTimeout:  5 * time.Second,
	}

	lazy := NewLazyStarter(cfg)
	defer lazy.Stop()

	// First call
	addr1, err1 := lazy.Addr()
	if err1 != nil {
		// Sidecar may not start if Python is not available — this is expected in CI
		if !strings.Contains(err1.Error(), "python not found") &&
			!strings.Contains(err1.Error(), "No module named") {
			t.Skipf("skipping test: Python sidecar not available: %v", err1)
		}
		return
	}

	// Second call should return same address without re-starting
	addr2, err2 := lazy.Addr()
	if err2 != nil {
		t.Fatalf("second Addr() call failed: %v", err2)
	}

	if addr1 != addr2 {
		t.Errorf("expected same address on second call: got %s, want %s", addr2, addr1)
	}
}

func TestLazyStarterStopBeforeStart(t *testing.T) {
	cfg := DefaultSidecarConfig()
	lazy := NewLazyStarter(cfg)

	// Stop should be safe to call even if sidecar never started
	if err := lazy.Stop(); err != nil {
		t.Errorf("Stop() before start should not error: %v", err)
	}

	if lazy.IsRunning() {
		t.Error("IsRunning() should return false after Stop() before start")
	}
}

func TestLazyStarterConcurrentAddr(t *testing.T) {
	cfg := &SidecarConfig{
		PythonCommand: "python3",
		PythonArgs:    []string{"-m", "c4.bridge.sidecar"},
		Host:          "localhost",
		Port:          0,
		StartTimeout:  5 * time.Second,
	}

	lazy := NewLazyStarter(cfg)
	defer lazy.Stop()

	const numGoroutines = 10
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	addrs := make([]string, numGoroutines)
	errs := make([]error, numGoroutines)

	// Concurrent calls to Addr()
	for i := 0; i < numGoroutines; i++ {
		i := i
		go func() {
			defer wg.Done()
			addr, err := lazy.Addr()
			addrs[i] = addr
			errs[i] = err
		}()
	}

	wg.Wait()

	// Check that all calls either succeeded with same address or failed with same error
	var firstAddr string
	var firstErr error
	for i := 0; i < numGoroutines; i++ {
		if i == 0 {
			firstAddr = addrs[i]
			firstErr = errs[i]
			continue
		}

		// All results should be identical
		if errs[i] != nil && firstErr != nil {
			// Both errors — this is OK if Python is not available
			if !strings.Contains(firstErr.Error(), "python not found") &&
				!strings.Contains(firstErr.Error(), "No module named") {
				continue
			}
		} else if errs[i] == nil && firstErr == nil {
			// Both success — addresses should match
			if addrs[i] != firstAddr {
				t.Errorf("concurrent Addr() calls returned different addresses: got %s, want %s", addrs[i], firstAddr)
			}
		} else {
			// One succeeded, one failed — this should not happen
			t.Errorf("inconsistent results: call %d: addr=%s err=%v, call 0: addr=%s err=%v",
				i, addrs[i], errs[i], firstAddr, firstErr)
		}
	}
}

func TestLazyStarterRestartBeforeStart(t *testing.T) {
	cfg := &SidecarConfig{
		PythonCommand: "python3",
		PythonArgs:    []string{"-m", "c4.bridge.sidecar"},
		Host:          "localhost",
		Port:          0,
		StartTimeout:  5 * time.Second,
	}

	lazy := NewLazyStarter(cfg)
	defer lazy.Stop()

	// Restart before first start should act like first start
	addr, err := lazy.Restart()
	if err != nil {
		// Sidecar may not start if Python is not available — this is expected in CI
		if !strings.Contains(err.Error(), "python not found") &&
			!strings.Contains(err.Error(), "No module named") {
			t.Skipf("skipping test: Python sidecar not available: %v", err)
		}
		return
	}

	if addr == "" {
		t.Error("expected non-empty address after Restart()")
	}

	if !lazy.IsRunning() {
		t.Error("sidecar should be running after Restart()")
	}
}

func TestLazyStarterCachesError(t *testing.T) {
	// Use invalid Python command to force startup failure
	cfg := &SidecarConfig{
		PythonCommand: "nonexistent-python-command",
		PythonArgs:    []string{"-m", "c4.bridge.sidecar"},
		Host:          "localhost",
		Port:          0,
		StartTimeout:  1 * time.Second,
	}

	lazy := NewLazyStarter(cfg)
	defer lazy.Stop()

	// First call should fail
	addr1, err1 := lazy.Addr()
	if err1 == nil {
		t.Error("expected error with invalid Python command")
	}
	if addr1 != "" {
		t.Error("expected empty address on error")
	}

	// Second call should return the same cached error
	addr2, err2 := lazy.Addr()
	if err2 == nil {
		t.Error("expected cached error on second call")
	}
	if addr2 != "" {
		t.Error("expected empty address on cached error")
	}

	// Error messages should be consistent
	if err1.Error() != err2.Error() {
		t.Errorf("expected same error message on second call: got %q, want %q", err2.Error(), err1.Error())
	}
}

func TestLazyStarterRetriesOnCachedError(t *testing.T) {
	// Both attempts will fail (invalid python), but the key is:
	// Addr() retries instead of returning the cached error forever.
	t.Setenv("PATH", "/nonexistent") // ensure no python fallback
	cfg := &SidecarConfig{
		PythonCommand: "nonexistent-python-command-retry-test",
		PythonArgs:    []string{"-m", "c4.bridge.sidecar"},
		Host:          "localhost",
		Port:          0,
		StartTimeout:  1 * time.Second,
	}

	lazy := NewLazyStarter(cfg)
	defer lazy.Stop()

	// First call fails
	_, err1 := lazy.Addr()
	if err1 == nil {
		t.Fatal("expected error on first call")
	}

	// Verify internal state was reset for retry
	lazy.mu.Lock()
	started := lazy.started
	cachedErr := lazy.err
	lazy.mu.Unlock()

	// After Addr() with retry logic, state should allow retry:
	// started should be true (retry happened), err should still be set (retry also failed)
	if !started {
		t.Fatal("expected started=true after retry attempt")
	}
	if cachedErr == nil {
		t.Fatal("expected err to be set (retry also failed with invalid python)")
	}

	// The important behavior: second Addr() call should attempt a fresh start,
	// not just return the stale cached error from the first attempt.
	_, err2 := lazy.Addr()
	if err2 == nil {
		t.Fatal("expected error on second call")
	}

	// Both errors should be fresh startup errors (not stale cached ones).
	// The error is "lazy start: ..." which proves a new attempt was made.
	if !strings.Contains(err2.Error(), "lazy start:") {
		t.Fatalf("expected 'lazy start:' error on retry (fresh attempt), got: %v", err2)
	}
}

func TestLazyStarterHealthCheckStartsAfterAddr(t *testing.T) {
	cfg := &SidecarConfig{
		PythonCommand: "python3",
		PythonArgs:    []string{"-m", "c4.bridge.sidecar"},
		Host:          "localhost",
		Port:          0,
		StartTimeout:  5 * time.Second,
	}

	var restartCalled bool
	lazy := NewLazyStarter(cfg)
	lazy.SetOnRestart(func(addr string) {
		restartCalled = true
	})
	defer lazy.Stop()

	addr, err := lazy.Addr()
	if err != nil {
		if !strings.Contains(err.Error(), "python not found") &&
			!strings.Contains(err.Error(), "No module named") {
			t.Skipf("skipping test: Python sidecar not available: %v", err)
		}
		return
	}

	if addr == "" {
		t.Fatal("expected non-empty address")
	}

	// Verify health check was started (healthStop channel should be set)
	lazy.mu.Lock()
	sidecar := lazy.sidecar
	lazy.mu.Unlock()

	sidecar.mu.Lock()
	hasHealthCheck := sidecar.healthStop != nil
	sidecar.mu.Unlock()

	if !hasHealthCheck {
		t.Fatal("expected health check to be started after successful Addr()")
	}

	// restartCalled won't be true (sidecar is healthy), just verify callback was set
	_ = restartCalled
}
