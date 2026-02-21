package serve

import (
	"context"
	"fmt"
	"os"
	"sync"
)

// Manager manages the lifecycle of registered components.
// Components are started in registration order and stopped in reverse order.
type Manager struct {
	mu         sync.Mutex
	components []Component
	started    []Component // tracks successfully started components for ordered shutdown
}

// NewManager creates a new component manager.
func NewManager() *Manager {
	return &Manager{}
}

// Register adds a component to the manager.
// Components are started in the order they are registered.
func (m *Manager) Register(c Component) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.components = append(m.components, c)
}

// StartAll starts all registered components in order.
// If a component fails to start, previously started components are stopped
// in reverse order and the error is returned.
func (m *Manager) StartAll(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.started = m.started[:0]

	for _, c := range m.components {
		fmt.Fprintf(os.Stderr, "cq serve: starting %s\n", c.Name())
		if err := c.Start(ctx); err != nil {
			// Rollback: stop already-started components in reverse
			for i := len(m.started) - 1; i >= 0; i-- {
				_ = m.started[i].Stop(ctx)
			}
			m.started = nil
			return fmt.Errorf("starting %s: %w", c.Name(), err)
		}
		m.started = append(m.started, c)
	}

	return nil
}

// StopAll stops all started components in reverse order.
// All components are attempted even if some fail; the first error is returned.
func (m *Manager) StopAll(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var firstErr error
	for i := len(m.started) - 1; i >= 0; i-- {
		c := m.started[i]
		fmt.Fprintf(os.Stderr, "cq serve: stopping %s\n", c.Name())
		if err := c.Stop(ctx); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	m.started = nil
	return firstErr
}

// HealthMap returns the health status of all registered components.
func (m *Manager) HealthMap() map[string]ComponentHealth {
	m.mu.Lock()
	defer m.mu.Unlock()

	result := make(map[string]ComponentHealth, len(m.components))
	for _, c := range m.components {
		result[c.Name()] = c.Health()
	}
	return result
}

// ComponentCount returns the number of registered components.
func (m *Manager) ComponentCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.components)
}
