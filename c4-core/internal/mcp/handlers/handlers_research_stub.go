//go:build !research

package handlers

import "github.com/changmin/c4-core/internal/mcp"

// RegisterResearchNativeHandlers is a no-op stub when the research build tag is disabled.
func RegisterResearchNativeHandlers(_ *mcp.Registry, _ any) {}

// RegisterResearchProxyHandlers is a no-op stub when the research build tag is disabled.
func RegisterResearchProxyHandlers(_ *mcp.Registry, _ *BridgeProxy) {}

// SetResearchEventBus is a no-op stub when the research build tag is disabled.
func SetResearchEventBus(_ any, _ string) {}

// registerResearchNative is a no-op when research build tag is disabled.
func registerResearchNative(_ *mcp.Registry, _ *BridgeProxy, _ *NativeOpts) {}
