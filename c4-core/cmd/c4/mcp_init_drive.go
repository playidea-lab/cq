//go:build c0_drive

package main

import (
	"fmt"
	"os"

	"github.com/changmin/c4-core/internal/drive"
	"github.com/changmin/c4-core/internal/eventbus"
	"github.com/changmin/c4-core/internal/mcp/handlers"
)

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
	handlers.RegisterDriveHandlers(ctx.reg, driveClient)
	fmt.Fprintln(os.Stderr, "cq: drive enabled (6 tools)")
	return nil
}

// wireDriveEventBus wires the eventbus to Drive components.
func wireDriveEventBus(_ *initContext, ebClient *eventbus.Client) {
	handlers.SetDriveEventBus(ebClient)
}
