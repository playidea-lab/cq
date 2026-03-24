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

// wslExecOutput runs a Windows command and returns stdout.
func wslExecOutput(cmd string) (string, error) {
	out, err := exec.Command("powershell.exe", "-Command", cmd).CombinedOutput()
	return string(out), err
}

const wslTaskName = "CQ-Serve-WSL"

// registerWindowsTask registers a Windows Task Scheduler task that starts
// WSL + cq serve on Windows boot. This ensures the worker reconnects to
// relay even after a full Windows reboot.
func registerWindowsTask(execPath, configPath, dir string) error {
	// Build the wsl.exe command that will run inside the task
	wslCmd := "wsl.exe --exec " + execPath + " serve"
	if configPath != "" {
		wslCmd += " --config " + configPath
	}
	if dir != "" {
		wslCmd += " --dir " + dir
	}

	// Remove existing task first (idempotent)
	_ = wslExec("Unregister-ScheduledTask -TaskName '" + wslTaskName + "' -Confirm:$false -ErrorAction SilentlyContinue")

	// Register new task: run at Windows startup, run whether user is logged on or not
	ps := "$action = New-ScheduledTaskAction -Execute 'wsl.exe' -Argument '--exec " + execPath + " serve"
	if configPath != "" {
		ps += " --config " + configPath
	}
	if dir != "" {
		ps += " --dir " + dir
	}
	ps += "'; "
	ps += "$trigger = New-ScheduledTaskTrigger -AtStartup; "
	ps += "$settings = New-ScheduledTaskSettingsSet -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries -StartWhenAvailable -RestartCount 3 -RestartInterval (New-TimeSpan -Minutes 1); "
	ps += "Register-ScheduledTask -TaskName '" + wslTaskName + "' -Action $action -Trigger $trigger -Settings $settings -Description 'CQ Serve auto-start via WSL2' -RunLevel Limited"

	return wslExec(ps)
}

// unregisterWindowsTask removes the CQ-Serve-WSL scheduled task.
func unregisterWindowsTask() error {
	return wslExec("Unregister-ScheduledTask -TaskName '" + wslTaskName + "' -Confirm:$false -ErrorAction SilentlyContinue")
}

// checkWslConf checks if /etc/wsl.conf has systemd=true enabled.
// Returns true if systemd is enabled, false otherwise.
func checkWslConf() bool {
	data, err := os.ReadFile("/etc/wsl.conf")
	if err != nil {
		return false
	}
	lower := strings.ToLower(string(data))
	return strings.Contains(lower, "systemd=true") || strings.Contains(lower, "systemd = true")
}
