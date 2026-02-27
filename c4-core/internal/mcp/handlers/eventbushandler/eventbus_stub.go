//go:build !c3_eventbus

package eventbushandler

import (
	"net/http"

	"github.com/changmin/c4-core/internal/eventbus"
	"github.com/changmin/c4-core/internal/mcp"
)

// RegisterEventBusHandlers is a no-op stub when c3_eventbus build tag is disabled.
func RegisterEventBusHandlers(_ *mcp.Registry, _ *eventbus.Client) {}

// StartEventSinkServer is a no-op stub when c3_eventbus build tag is disabled.
// Returns nil server (disabled).
func StartEventSinkServer(_ int, _ string, _ eventbus.Publisher) (*http.Server, error) {
	return nil, nil
}
