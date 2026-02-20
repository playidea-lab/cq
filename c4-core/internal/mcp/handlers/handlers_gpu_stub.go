//go:build !gpu

package handlers

import "github.com/changmin/c4-core/internal/mcp"

// RegisterGPUNativeHandlers is a no-op stub when the gpu build tag is disabled.
// GPU tools are not registered.
func RegisterGPUNativeHandlers(_ *mcp.Registry, _ any, _ any) {}

// registerGPUNative is a no-op when gpu build tag is disabled.
func registerGPUNative(_ *mcp.Registry, _ *NativeOpts) {}
