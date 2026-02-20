//go:build gpu

package handlers

import "github.com/changmin/c4-core/internal/mcp"

// registerGPUNative registers GPU tools using the real Go-native implementation.
func registerGPUNative(reg *mcp.Registry, opts *NativeOpts) {
	if opts != nil {
		RegisterGPUNativeHandlers(reg, opts.GPUStore, opts.GPUScheduler)
	} else {
		RegisterGPUNativeHandlers(reg, nil, nil)
	}
}
