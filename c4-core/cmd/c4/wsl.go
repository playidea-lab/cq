//go:build linux

package main

import (
	"os"
	"os/exec"
	"strings"
)

// isWSL2 returns true if the current process is running inside WSL2.
// It checks /proc/version for "microsoft" or "WSL" (case-insensitive).
func isWSL2() bool {
	data, err := os.ReadFile("/proc/version")
	if err != nil {
		return false
	}
	lower := strings.ToLower(string(data))
	return strings.Contains(lower, "microsoft") || strings.Contains(lower, "wsl")
}

// wslExec runs a Windows command via powershell.exe (available in WSL2).
// cmd is passed as the -Command argument to powershell.exe.
func wslExec(cmd string) error {
	return exec.Command("powershell.exe", "-Command", cmd).Run()
}
