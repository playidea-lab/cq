//go:build !c7_observe

package handlers

import "github.com/changmin/c4-core/internal/mcp"

// RegisterObserveTraceHandlers is a no-op stub when c7_observe build tag is disabled.
func RegisterObserveTraceHandlers(_ *mcp.Registry) {}
