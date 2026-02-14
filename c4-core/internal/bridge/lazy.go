package bridge

import (
	"fmt"
	"sync"
)

// LazyStarter wraps Sidecar with lazy initialization.
// The sidecar only starts when Addr() is first called.
// This improves startup time and prevents errors when Python/uv is not available
// but Go-native tools are sufficient.
type LazyStarter struct {
	mu      sync.Mutex
	sidecar *Sidecar
	cfg     *SidecarConfig
	started bool
	err     error
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

// Addr returns the address of the sidecar, starting it if necessary.
// On first call, it starts the sidecar. Subsequent calls return the cached address.
// Returns empty string and error if startup fails.
func (l *LazyStarter) Addr() (string, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Return cached result if already started (success or failure)
	if l.started {
		if l.err != nil {
			return "", l.err
		}
		return l.sidecar.Addr(), nil
	}

	// First call: start the sidecar
	l.started = true
	sidecar, err := StartSidecar(l.cfg)
	if err != nil {
		l.err = fmt.Errorf("lazy start: %w", err)
		return "", l.err
	}

	l.sidecar = sidecar
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
