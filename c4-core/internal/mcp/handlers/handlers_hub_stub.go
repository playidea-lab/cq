//go:build !c5_hub

package handlers

import (
	"github.com/changmin/c4-core/internal/eventbus"
	"github.com/changmin/c4-core/internal/mcp"
	"github.com/changmin/c4-core/internal/mcp/handlers/c1handler"
	"github.com/changmin/c4-core/internal/mcp/handlers/hubhandler"
)

// HubPoller is a type alias for hubhandler.HubPoller.
type HubPoller = hubhandler.HubPoller

// HubPollerOption is a type alias for hubhandler.HubPollerOption.
type HubPollerOption = hubhandler.HubPollerOption

// RegisterHubHandlers is a no-op stub when c5_hub build tag is disabled.
func RegisterHubHandlers(_ *mcp.Registry, _ any) {}

// SetHubEventBus is a no-op stub when c5_hub build tag is disabled.
func SetHubEventBus(_ eventbus.Publisher, _ string) {}

// GetHubEventPub delegates to hubhandler.GetHubEventPub.
func GetHubEventPub() eventbus.Publisher {
	return hubhandler.GetHubEventPub()
}

// GetHubProjectID delegates to hubhandler.GetHubProjectID.
func GetHubProjectID() string {
	return hubhandler.GetHubProjectID()
}

// WorkerDeps is a placeholder type when c5_hub build tag is disabled.
type WorkerDeps struct {
	HubClient     any
	ShutdownStore any
	Keeper        *c1handler.ContextKeeper
}

// RegisterWorkerHandlers is a no-op stub when c5_hub build tag is disabled.
func RegisterWorkerHandlers(_ *mcp.Registry, _ *WorkerDeps) {}
