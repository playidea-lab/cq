// Package serve provides a component lifecycle framework for long-running
// background services in the C4 engine (e.g., Supabase Realtime listener,
// agent dispatcher).
//
// Each component implements the Component interface and is managed by
// a ComponentManager that handles ordered startup and shutdown.
package serve

import (
	"context"
	"fmt"
	"sync"
)

// Status represents the lifecycle state of a component.
type Status string

const (
	StatusStopped  Status = "stopped"
	StatusStarting Status = "starting"
	StatusRunning  Status = "running"
	StatusDegraded Status = "degraded" // running with reduced functionality
	StatusStopping Status = "stopping"
	StatusFailed   Status = "failed"
)

// Component is the interface that long-running background services implement.
type Component interface {
	// Name returns a human-readable identifier for the component.
	Name() string

	// Start initializes the component and begins its work.
	// The context is cancelled when the component should stop.
	// Start should return quickly; long-running work should be spawned in goroutines.
	Start(ctx context.Context) error

	// Stop gracefully shuts down the component.
	// It should release all resources and stop all goroutines.
	Stop() error

	// Status returns the current lifecycle state.
	Status() Status
}

// ComponentManager manages a set of components with ordered startup and shutdown.
type ComponentManager struct {
	mu         sync.Mutex
	components []Component
	ctx        context.Context
	cancel     context.CancelFunc
	running    bool
}

// NewComponentManager creates a new manager.
func NewComponentManager() *ComponentManager {
	return &ComponentManager{}
}

// Add registers a component. Must be called before Start.
func (m *ComponentManager) Add(c Component) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.components = append(m.components, c)
}

// Start starts all components in registration order.
// If any component fails to start, previously started components are stopped.
func (m *ComponentManager) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.running {
		return fmt.Errorf("component manager already running")
	}

	m.ctx, m.cancel = context.WithCancel(ctx)

	for i, c := range m.components {
		if err := c.Start(m.ctx); err != nil {
			// Roll back: stop already-started components in reverse order
			for j := i - 1; j >= 0; j-- {
				m.components[j].Stop()
			}
			m.cancel()
			return fmt.Errorf("start %s: %w", c.Name(), err)
		}
	}
	m.running = true
	return nil
}

// Stop stops all components in reverse registration order.
func (m *ComponentManager) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return nil
	}

	m.cancel()

	var firstErr error
	for i := len(m.components) - 1; i >= 0; i-- {
		if err := m.components[i].Stop(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	m.running = false
	return firstErr
}

// Statuses returns the current status of all registered components.
func (m *ComponentManager) Statuses() map[string]Status {
	m.mu.Lock()
	defer m.mu.Unlock()

	result := make(map[string]Status, len(m.components))
	for _, c := range m.components {
		result[c.Name()] = c.Status()
	}
	return result
}
