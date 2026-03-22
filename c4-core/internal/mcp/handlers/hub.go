//go:build hub

package handlers

import (
	"time"

	"github.com/changmin/c4-core/internal/eventbus"
	"github.com/changmin/c4-core/internal/hub"
	"github.com/changmin/c4-core/internal/mcp"
	"github.com/changmin/c4-core/internal/mcp/handlers/hubhandler"
)

// HubPoller is a type alias for hubhandler.HubPoller.
type HubPoller = hubhandler.HubPoller

// HubPollerOption is a type alias for hubhandler.HubPollerOption.
type HubPollerOption = hubhandler.HubPollerOption

// RegisterHubHandlers delegates to hubhandler.RegisterHubHandlers.
func RegisterHubHandlers(reg *mcp.Registry, hubClient *hub.Client) {
	hubhandler.RegisterHubHandlers(reg, hubClient)
}

// SetHubEventBus delegates to hubhandler.SetHubEventBus.
func SetHubEventBus(pub eventbus.Publisher, projectID string) {
	hubhandler.SetHubEventBus(pub, projectID)
}

// GetHubEventPub delegates to hubhandler.GetHubEventPub.
func GetHubEventPub() eventbus.Publisher {
	return hubhandler.GetHubEventPub()
}

// NewHubPoller delegates to hubhandler.NewHubPoller.
func NewHubPoller(client *hub.Client, pub eventbus.Publisher, interval time.Duration, opts ...HubPollerOption) *HubPoller {
	return hubhandler.NewHubPoller(client, pub, interval, opts...)
}

// WithMaxJobs delegates to hubhandler.WithMaxJobs.
func WithMaxJobs(n int) HubPollerOption {
	return hubhandler.WithMaxJobs(n)
}

// GetHubProjectID delegates to hubhandler.GetHubProjectID.
func GetHubProjectID() string {
	return hubhandler.GetHubProjectID()
}
