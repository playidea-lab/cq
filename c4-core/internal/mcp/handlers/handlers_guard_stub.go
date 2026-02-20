//go:build !c6_guard

package handlers

import (
	"github.com/changmin/c4-core/internal/guard"
	"github.com/changmin/c4-core/internal/mcp"
)

// RegisterGuardHandlers is a no-op stub when c6_guard build tag is disabled.
func RegisterGuardHandlers(_ *mcp.Registry, _ *guard.Engine) {}
