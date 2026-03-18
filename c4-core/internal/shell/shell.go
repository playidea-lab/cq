// Package shell provides cross-platform shell command helpers.
package shell

import (
	"context"
	"os/exec"
	"runtime"
)

// Command returns an *exec.Cmd that runs cmd via the system shell.
// On Unix/macOS it uses sh(1); on Windows it uses Git Bash (bash.exe).
func Command(ctx context.Context, cmd string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		return exec.CommandContext(ctx, "bash.exe", "-c", cmd)
	}
	return exec.CommandContext(ctx, "sh", "-c", cmd)
}
