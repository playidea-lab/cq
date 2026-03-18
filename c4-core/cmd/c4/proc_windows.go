//go:build windows

package main

import (
	"os/exec"
	"syscall"
)

// setDetachedProcess configures the command to run detached from the parent
// console (Windows: CREATE_NEW_PROCESS_GROUP).
func setDetachedProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}
}
