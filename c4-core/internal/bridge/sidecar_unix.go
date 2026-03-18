//go:build !windows

package bridge

import (
	"log"
	"os/exec"
	"syscall"
	"time"
)

// setSysProcAttr sets Setpgid so the sidecar runs in its own process group.
// This allows Stop/Restart to kill the entire tree (uv wrapper + python child)
// with a single signal to the negative PID (process group ID).
func setSysProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// killStaleGroup sends SIGTERM then SIGKILL to the process group identified by pid.
// Used by cleanupStaleSidecar to terminate orphaned process trees.
func killStaleGroup(pid int) {
	if err := syscall.Kill(-pid, syscall.SIGTERM); err != nil {
		log.Printf("c4: sidecar: SIGTERM to pgid %d failed: %v", pid, err)
	}
	time.Sleep(500 * time.Millisecond)
	_ = syscall.Kill(-pid, syscall.SIGKILL)
}

// isProcessAlive returns true if a process with the given pid exists.
func isProcessAlive(pid int) bool {
	return syscall.Kill(pid, 0) == nil
}

// killOrphan sends SIGTERM then SIGKILL to an individual orphan process.
func killOrphan(pid int) {
	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
		log.Printf("c4: sidecar: SIGTERM to orphan pid %d failed: %v", pid, err)
	}
	time.Sleep(200 * time.Millisecond)
	_ = syscall.Kill(pid, syscall.SIGKILL)
}

// killGroup sends SIGTERM or SIGKILL to an entire process group.
// Returns true if the signal was sent (always true on Unix).
func killGroup(pgid int, force bool) bool {
	if force {
		_ = syscall.Kill(-pgid, syscall.SIGKILL)
	} else {
		_ = syscall.Kill(-pgid, syscall.SIGTERM)
	}
	return true
}
