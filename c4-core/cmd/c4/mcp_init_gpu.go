//go:build gpu

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/changmin/c4-core/internal/daemon"
)

func init() {
	registerPreStoreHook(initGPU)
	registerShutdownHook(shutdownGPU)
}

// initGPU creates the daemon store, GPU monitor, and scheduler.
// Runs as a pre-store hook so ctx.daemonStore and ctx.scheduler are available
// for NativeOpts before handler registration.
func initGPU(ctx *initContext) error {
	daemonDBPath := filepath.Join(ctx.projectDir, ".c4", "daemon.db")
	ds, dsErr := daemon.NewStore(daemonDBPath)
	if dsErr != nil {
		fmt.Fprintf(os.Stderr, "cq: daemon store init failed (job scheduler unavailable): %v\n", dsErr)
		return nil
	}
	ctx.daemonStore = ds

	daemonDataDir := filepath.Join(ctx.projectDir, ".c4", "daemon")
	gpuMon := daemon.NewGpuMonitor()
	gpuCount := 0
	if gpus, gpuErr := gpuMon.GetAllGPUs(); gpuErr == nil {
		gpuCount = len(gpus)
	}
	sched := daemon.NewScheduler(ds, daemon.SchedulerConfig{
		DataDir:  daemonDataDir,
		GPUCount: gpuCount,
	})
	schedCtx, schedCancel := context.WithCancel(context.Background())
	ctx.schedulerCancel = schedCancel
	ctx.scheduler = sched
	sched.Start(schedCtx)
	fmt.Fprintf(os.Stderr, "cq: daemon scheduler started (gpus=%d)\n", gpuCount)
	return nil
}

// shutdownGPU cleans up GPU/daemon resources.
func shutdownGPU(ctx *initContext) {
	if ctx.scheduler != nil {
		ctx.scheduler.Stop()
	}
	if ctx.schedulerCancel != nil {
		ctx.schedulerCancel()
	}
	if ctx.daemonStore != nil {
		_ = ctx.daemonStore.Close()
	}
}
