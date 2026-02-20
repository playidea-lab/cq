//go:build !c5_hub

package handlers

import (
	"context"
	"time"

	"github.com/changmin/c4-core/internal/eventbus"
	"github.com/changmin/c4-core/internal/mcp"
)

// RegisterHubHandlers is a no-op stub when c5_hub build tag is disabled.
func RegisterHubHandlers(_ *mcp.Registry, _ any) {}

// SetHubEventBus is a no-op stub when c5_hub build tag is disabled.
func SetHubEventBus(_ eventbus.Publisher, _ string) {}

// GetHubEventPub returns a no-op publisher when c5_hub build tag is disabled.
func GetHubEventPub() eventbus.Publisher {
	return eventbus.NoopPublisher{}
}

// HubPoller is a placeholder type when c5_hub build tag is disabled.
type HubPoller struct{}

// NewHubPoller returns nil when c5_hub is disabled.
func NewHubPoller(_ any, _ eventbus.Publisher, _ time.Duration, _ ...HubPollerOption) *HubPoller {
	return nil
}

// HubPollerOption is a placeholder when c5_hub build tag is disabled.
type HubPollerOption func(*HubPoller)

// WithMaxJobs is a no-op stub when c5_hub build tag is disabled.
func WithMaxJobs(_ int) HubPollerOption { return func(*HubPoller) {} }

// SetProjectID is a no-op when c5_hub is disabled.
func (p *HubPoller) SetProjectID(_ string) {}

// Start is a no-op when c5_hub is disabled.
func (p *HubPoller) Start(_ context.Context) {}

// WorkerDeps is a placeholder type when c5_hub build tag is disabled.
type WorkerDeps struct {
	HubClient     any
	ShutdownStore any
	Keeper        *ContextKeeper
}

// RegisterWorkerHandlers is a no-op stub when c5_hub build tag is disabled.
func RegisterWorkerHandlers(_ *mcp.Registry, _ *WorkerDeps) {}
