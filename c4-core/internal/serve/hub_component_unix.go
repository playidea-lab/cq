//go:build !windows

package serve

import (
	"os"
	"syscall"
)

// stopProcessGraceful sends SIGTERM to request graceful shutdown.
func stopProcessGraceful(proc *os.Process) error {
	return proc.Signal(syscall.SIGTERM)
}

// stopProcessForce sends SIGKILL to force-kill the process.
func stopProcessForce(proc *os.Process) error {
	return proc.Signal(syscall.SIGKILL)
}
