//go:build c0_drive

package handlers

import (
	"github.com/changmin/c4-core/internal/drive"
	"github.com/changmin/c4-core/internal/eventbus"
	"github.com/changmin/c4-core/internal/mcp"
	"github.com/changmin/c4-core/internal/mcp/handlers/drivehandler"
)

// RegisterDriveHandlers delegates to drivehandler.RegisterDriveHandlers.
func RegisterDriveHandlers(reg *mcp.Registry, driveClient *drive.Client) {
	drivehandler.RegisterDriveHandlers(reg, driveClient)
}

// RegisterDatasetHandlers delegates to drivehandler.RegisterDatasetHandlers.
func RegisterDatasetHandlers(reg *mcp.Registry, client *drive.DatasetClient) {
	drivehandler.RegisterDatasetHandlers(reg, client)
}

// SetDriveEventBus delegates to drivehandler.SetDriveEventBus.
func SetDriveEventBus(pub eventbus.Publisher) {
	drivehandler.SetDriveEventBus(pub)
}
