//go:build hub

package main

import (
	"context"
	"os"

	"github.com/changmin/c4-core/internal/hub"
	"github.com/changmin/c4-core/internal/serve"
)

// runWatchdog starts the Watchdog supervisor in place of the normal serve loop.
// It re-invokes the current binary with the same arguments minus --watchdog,
// supervising it and restarting on crash with exponential backoff.
func runWatchdog(ctx context.Context, hubClientAny any, extraArgs []string) error {
	var hubClient *hub.Client
	if c, ok := hubClientAny.(*hub.Client); ok {
		hubClient = c
	}

	hostname, _ := os.Hostname()
	wd := serve.NewWatchdog(extraArgs, hubClient, hostname)
	return wd.Run(ctx)
}
