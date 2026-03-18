//go:build windows

package bridge

import (
	"log"
	"os/exec"
)

// setSysProcAttr is a no-op on Windows; process groups are not used.
func setSysProcAttr(cmd *exec.Cmd) {}

// killStaleGroup kills the process by PID on Windows (no process group support).
// pid here is the process PID recorded in the PID file.
func killStaleGroup(pid int) {
	log.Printf("c4: sidecar: Windows stub: cannot kill process group for pid %d", pid)
}

// isProcessAlive always returns false on Windows in this stub.
// The PID file cleanup path is skipped when this returns false.
func isProcessAlive(pid int) bool {
	return false
}

// killOrphan is a no-op on Windows; pgrep-based orphan cleanup is Unix-only.
func killOrphan(pid int) {}

// killGroup is a no-op on Windows; process groups via negative PID are Unix-only.
// Returns false so callers fall back to cmd.Process.Kill() for termination.
func killGroup(pgid int, force bool) bool { return false }
