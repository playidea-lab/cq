package serve

import (
	"sync/atomic"
	"testing"
	"time"
)

// Test_SafeGo_PanicRecovery verifies that safeGo restarts fn after a panic.
func Test_SafeGo_PanicRecovery(t *testing.T) {
	var callCount atomic.Int32

	safeGo("test-panic", func() {
		n := callCount.Add(1)
		if n == 1 {
			// First call: panic intentionally
			panic("intentional test panic")
		}
		// Second call: do nothing (fn returns normally)
	})

	// Wait up to 3 seconds for the second invocation to happen (1s sleep + startup time)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if callCount.Load() >= 2 {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}

	t.Fatalf("safeGo did not restart fn after panic within 3s; call count = %d", callCount.Load())
}

// Test_SafeGo_NormalReturn verifies that safeGo restarts fn after normal return too.
func Test_SafeGo_NormalReturn(t *testing.T) {
	var callCount atomic.Int32

	safeGo("test-normal", func() {
		callCount.Add(1)
		// Return immediately every time
	})

	// Wait up to 3 seconds for at least 2 calls (1s sleep between them)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if callCount.Load() >= 2 {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}

	t.Fatalf("safeGo did not restart fn after normal return within 3s; call count = %d", callCount.Load())
}
