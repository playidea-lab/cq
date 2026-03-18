//go:build windows

package daemon

import "os/exec"

// setSysProcAttr is a no-op on Windows; process groups are not used.
func setSysProcAttr(_ *exec.Cmd) {}

// killProcessGroup kills the process directly on Windows (no process groups).
func (s *Scheduler) killProcessGroup(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	cmd.Process.Kill() //nolint:errcheck
}
