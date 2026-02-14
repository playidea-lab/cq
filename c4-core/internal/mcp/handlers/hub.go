package handlers

import (
	"github.com/changmin/c4-core/internal/hub"
	"github.com/changmin/c4-core/internal/mcp"
)

// RegisterHubHandlers registers c4_hub_* MCP tools.
func RegisterHubHandlers(reg *mcp.Registry, hubClient *hub.Client) {
	registerHubJobHandlers(reg, hubClient)
	registerHubDAGHandlers(reg, hubClient)
	registerHubInfraHandlers(reg, hubClient)
	registerHubEdgeHandlers(reg, hubClient)
}
