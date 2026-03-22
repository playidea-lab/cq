//go:build !hub

package hubhandler

import (
	"context"
	"time"

	"github.com/changmin/c4-core/internal/eventbus"
	"github.com/changmin/c4-core/internal/mcp"
)

// RegisterHubHandlers is a no-op stub when hub build tag is disabled.
func RegisterHubHandlers(_ *mcp.Registry, _ any) {}

// SetHubEventBus is a no-op stub when hub build tag is disabled.
func SetHubEventBus(_ eventbus.Publisher, _ string) {}

// GetHubEventPub returns a no-op publisher when hub build tag is disabled.
func GetHubEventPub() eventbus.Publisher {
	return eventbus.NoopPublisher{}
}

// GetHubProjectID returns an empty string when hub build tag is disabled.
func GetHubProjectID() string { return "" }

// HubPoller is a placeholder type when hub build tag is disabled.
type HubPoller struct{}

// NewHubPoller returns nil when hub is disabled.
func NewHubPoller(_ any, _ eventbus.Publisher, _ time.Duration, _ ...HubPollerOption) *HubPoller {
	return nil
}

// HubPollerOption is a placeholder when hub build tag is disabled.
type HubPollerOption func(*HubPoller)

// WithMaxJobs is a no-op stub when hub build tag is disabled.
func WithMaxJobs(_ int) HubPollerOption { return func(*HubPoller) {} }

// SetProjectID is a no-op when hub is disabled.
func (p *HubPoller) SetProjectID(_ string) {}

// Start is a no-op when hub is disabled.
func (p *HubPoller) Start(_ context.Context) {}
