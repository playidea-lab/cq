//go:build research

package handlers

import "github.com/changmin/c4-core/internal/mcp"

// registerResearchNative registers research tools using the Go-native implementation.
// Falls back to proxy if ResearchStore is nil.
func registerResearchNative(reg *mcp.Registry, proxy *BridgeProxy, opts *NativeOpts) {
	if opts != nil && opts.ResearchStore != nil {
		RegisterResearchNativeHandlers(reg, opts.ResearchStore)
	} else {
		RegisterResearchProxyHandlers(reg, proxy)
	}
}
