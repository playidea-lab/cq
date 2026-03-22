//go:build windows

package serve

import "os"

// stopProcessGraceful on Windows falls back to Process.Kill since
// SIGTERM is not supported.
func stopProcessGraceful(proc *os.Process) error {
	return proc.Kill()
}

// stopProcessForce on Windows uses Process.Kill.
func stopProcessForce(proc *os.Process) error {
	return proc.Kill()
}
