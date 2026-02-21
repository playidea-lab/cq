// Package serve provides a component lifecycle manager for the cq serve command.
// Components (EventBus, EventSink, HubPoller, etc.) register via the Component
// interface and are started/stopped by the Manager.
package serve

import "context"

// ComponentHealth represents the health status of a single component.
type ComponentHealth struct {
	Status string `json:"status"` // "ok", "degraded", "error"
	Detail string `json:"detail,omitempty"`
}

// Component is the interface that all managed services must implement.
type Component interface {
	// Name returns a human-readable identifier for this component.
	Name() string

	// Start initializes and runs the component. It should return promptly;
	// long-running work must be launched in a goroutine that respects ctx.
	Start(ctx context.Context) error

	// Stop gracefully shuts down the component.
	Stop(ctx context.Context) error

	// Health returns the current health status.
	Health() ComponentHealth
}
