//go:build !cdp

package cdphandler

import "github.com/changmin/c4-core/internal/mcp"

// RegisterCDPHandlers is a no-op stub when the cdp build tag is disabled.
// CDP and WebMCP tools are not registered.
func RegisterCDPHandlers(_ *mcp.Registry, _ any) {}
