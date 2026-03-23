//go:build c0_drive

package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/changmin/c4-core/internal/drive"
	"github.com/changmin/c4-core/internal/eventbus"
	"github.com/changmin/c4-core/internal/mcp/handlers/drivehandler"
)

// driveUploaderAdapter adapts *drive.Client to the DriveUploader interface.
type driveUploaderAdapter struct {
	client *drive.Client
}

func (a *driveUploaderAdapter) Upload(localPath, drivePath string, metadata json.RawMessage) error {
	_, err := a.client.Upload(localPath, drivePath, metadata)
	return err
}

func init() {
	registerInitHook(initDrive)
	registerEBWireHook(wireDriveEventBus)
}

// initDrive registers Drive handlers if cloud is enabled.
func initDrive(ctx *initContext) error {
	if ctx.cfgMgr == nil || !ctx.cfgMgr.GetConfig().Cloud.Enabled {
		return nil
	}
	cloudCfg := ctx.cfgMgr.GetConfig().Cloud
	if cloudCfg.URL == "" || cloudCfg.AnonKey == "" {
		return nil
	}
	driveClient := drive.NewClient(cloudCfg.URL, cloudCfg.AnonKey, ctx.cloudTP, ctx.cloudProjectID, cloudCfg.BucketName)
	drivehandler.RegisterDriveHandlers(ctx.reg, driveClient)
	drivehandler.RegisterDatasetHandlers(ctx.reg, drive.NewDatasetClient(driveClient), ctx.projectDir)
	// Wire Drive into Knowledge handlers for experiment artifact auto-upload.
	// knowledgeOpts is a shared pointer — handlers already captured it via closure.
	if ctx.knowledgeOpts != nil {
		ctx.knowledgeOpts.Drive = &driveUploaderAdapter{client: driveClient}
	}
	fmt.Fprintln(os.Stderr, "cq: drive enabled (9 tools)")
	return nil
}

// wireDriveEventBus wires the eventbus to Drive components.
func wireDriveEventBus(_ *initContext, ebClient *eventbus.Client) {
	drivehandler.SetDriveEventBus(ebClient)
}
