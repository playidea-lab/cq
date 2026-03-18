//go:build !windows

package daemon

import (
	"os/exec"
	"syscall"
)

// setSysProcAttr configures the command to run in its own process group.
func setSysProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// killProcessGroup sends SIGKILL to the entire process group.
func (s *Scheduler) killProcessGroup(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	pgid, err := syscall.Getpgid(cmd.Process.Pid)
	if err == nil {
		syscall.Kill(-pgid, syscall.SIGKILL) //nolint:errcheck
	} else {
		cmd.Process.Kill() //nolint:errcheck
	}
}
