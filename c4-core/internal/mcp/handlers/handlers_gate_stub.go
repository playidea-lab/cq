//go:build !c8_gate

package handlers

import (
	"github.com/changmin/c4-core/internal/gate"
	"github.com/changmin/c4-core/internal/mcp"
)

// RegisterGateHandlers is a no-op stub when c8_gate build tag is disabled.
func RegisterGateHandlers(_ *mcp.Registry, _ *gate.WebhookManager, _ *gate.Scheduler, _ *gate.SlackConnector, _ *gate.GitHubConnector) {
}
