//go:build !windows

package main

import (
	"os/exec"
	"syscall"
)

// setDetachedProcess configures the command to run in a new session,
// detached from the parent's controlling terminal (Unix: Setsid).
func setDetachedProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
