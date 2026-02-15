package bridge

import (
	"fmt"
	"sync"
	"time"
)

// LazyStarter wraps Sidecar with lazy initialization.
// The sidecar only starts when Addr() is first called.
// This improves startup time and prevents errors when Python/uv is not available
// but Go-native tools are sufficient.
type LazyStarter struct {
	mu        sync.Mutex
	sidecar   *Sidecar
	cfg       *SidecarConfig
	started   bool
	err       error
	onRestart func(string) // callback when sidecar restarts via health check
}

// NewLazyStarter creates a LazyStarter with the given config.
// The sidecar will not start until the first call to Addr().
func NewLazyStarter(cfg *SidecarConfig) *LazyStarter {
	if cfg == nil {
		cfg = DefaultSidecarConfig()
	}
	return &LazyStarter{
		cfg: cfg,
	}
}

// SetOnRestart sets a callback that is invoked when the sidecar restarts
// via health check. Typically used to update the proxy address.
func (l *LazyStarter) SetOnRestart(fn func(string)) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.onRestart = fn
}

// Addr returns the address of the sidecar, starting it if necessary.
// On first call, it starts the sidecar. Subsequent calls return the cached address.
// If a previous start failed, retries once before returning the error.
func (l *LazyStarter) Addr() (string, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	// If previously started successfully, return cached address
	if l.started && l.err == nil && l.sidecar != nil {
		return l.sidecar.Addr(), nil
	}

	// If previous start failed, clear state to retry
	if l.started && l.err != nil {
		l.started = false
		l.err = nil
		l.sidecar = nil
	}

	// Start the sidecar
	l.started = true
	sidecar, err := StartSidecar(l.cfg)
	if err != nil {
		l.err = fmt.Errorf("lazy start: %w", err)
		return "", l.err
	}

	l.sidecar = sidecar

	// Start health check after successful start
	l.sidecar.StartHealthCheck(30*time.Second, l.onRestart)

	return l.sidecar.Addr(), nil
}

// Stop stops the sidecar if it was started.
// Safe to call even if sidecar never started.
func (l *LazyStarter) Stop() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.sidecar == nil {
		return nil
	}
	return l.sidecar.Stop()
}

// IsRunning returns true if the sidecar was started and is still running.
func (l *LazyStarter) IsRunning() bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.sidecar == nil {
		return false
	}
	return l.sidecar.IsRunning()
}

// Restart restarts the sidecar if it was started.
// Returns the new address. Implements Restarter interface.
func (l *LazyStarter) Restart() (string, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.sidecar == nil {
		// Not started yet — treat as first start
		l.started = true
		sidecar, err := StartSidecar(l.cfg)
		if err != nil {
			l.err = fmt.Errorf("lazy restart: %w", err)
			return "", l.err
		}
		l.sidecar = sidecar
		l.err = nil
		l.sidecar.StartHealthCheck(30*time.Second, l.onRestart)
		return l.sidecar.Addr(), nil
	}

	// Delegate to sidecar's restart
	newAddr, err := l.sidecar.Restart()
	if err != nil {
		l.err = err
		return "", err
	}
	l.err = nil
	return newAddr, nil
}
