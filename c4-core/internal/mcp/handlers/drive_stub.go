//go:build !c0_drive

package handlers

import (
	"github.com/changmin/c4-core/internal/eventbus"
	"github.com/changmin/c4-core/internal/mcp"
)

// RegisterDriveHandlers is a no-op stub when c0_drive build tag is disabled.
func RegisterDriveHandlers(_ *mcp.Registry, _ any) {}

// RegisterDatasetHandlers is a no-op stub when c0_drive build tag is disabled.
func RegisterDatasetHandlers(_ *mcp.Registry, _ any) {}

// SetDriveEventBus is a no-op stub when c0_drive build tag is disabled.
func SetDriveEventBus(_ eventbus.Publisher) {}
