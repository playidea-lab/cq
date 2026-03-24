//go:build !linux

package main

// isWSL2 always returns false on non-Linux platforms.
func isWSL2() bool { return false }

// wslExec is a no-op on non-Linux platforms.
func wslExec(_ string) error { return nil }

// wslExecOutput is a no-op on non-Linux platforms.
func wslExecOutput(_ string) (string, error) { return "", nil }

const wslTaskName = "CQ-Serve-WSL"

// registerWindowsTask is a no-op on non-Linux platforms.
func registerWindowsTask(_, _, _ string) error { return nil }

// unregisterWindowsTask is a no-op on non-Linux platforms.
func unregisterWindowsTask() error { return nil }

// checkWslConf always returns true on non-Linux platforms (skip warning).
func checkWslConf() bool { return true }
