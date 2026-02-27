//go:build !gpu

package gpuhandler

import (
	"github.com/changmin/c4-core/internal/daemon"
	"github.com/changmin/c4-core/internal/mcp"
)

// Register is a no-op stub when the gpu build tag is disabled.
func Register(_ *mcp.Registry, _ *daemon.Store, _ *daemon.Scheduler) {}
