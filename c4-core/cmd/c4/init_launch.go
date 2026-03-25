package main

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// servePIDPath overrides the default PID file location; used in tests for path isolation.
var servePIDPath string

// ensureServeRunning checks if cq serve is running and starts it in the background if not.
// If noServe is true, it skips the check entirely.
// Failures are non-fatal: a warning is printed and execution continues.
func ensureServeRunning(noServe bool) {
	if noServe {
		return
	}

	pidPath := servePIDPath
	if pidPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return
		}
		pidPath = filepath.Join(home, ".c4", "serve", "serve.pid")
	}
	data, err := os.ReadFile(pidPath)
	if err == nil {
		pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
		if err == nil && isCQServeProcess(pid) {
			// Already running — nothing to do.
			return
		}
	}

	// Not running — start cq serve in background.
	cqPath, err := os.Executable()
	if err != nil {
		return
	}

	cmd := exec.Command(cqPath, "serve")
	setDetachedProcess(cmd)
	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "cq: warn: could not start serve: %v\n", err)
		return
	}
	pid := cmd.Process.Pid
	// Release resources; we don't wait for this background process.
	_ = cmd.Process.Release()

	// Wait up to 2s for serve to become healthy.
	// 4140 is cq serve's default listen port (matches serve command default).
	const serveHealthURL = "http://127.0.0.1:4140/health"
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(serveHealthURL)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				fmt.Fprintf(os.Stderr, "cq: serve started (pid=%d)\n", pid)
				return
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	fmt.Fprintf(os.Stderr, "cq: warn: serve started (pid=%d) but health check timed out (%s)\n", pid, serveHealthURL)
}

// isCQServeProcess returns true if the given PID is a running "cq serve" process.
func isCQServeProcess(pid int) bool {
	switch runtime.GOOS {
	case "linux":
		data, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
		if err != nil {
			return false
		}
		// cmdline fields are NUL-separated
		parts := strings.Split(string(data), "\x00")
		hasCQ, hasServe := false, false
		for _, p := range parts {
			base := filepath.Base(p)
			if base == "cq" || strings.HasSuffix(p, "/cq") {
				hasCQ = true
			}
			if p == "serve" {
				hasServe = true
			}
		}
		return hasCQ && hasServe
	case "darwin":
		out, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "command=").Output()
		if err != nil {
			return false
		}
		parts := strings.Fields(strings.TrimSpace(string(out)))
		if len(parts) < 2 {
			return false
		}
		return filepath.Base(parts[0]) == "cq" && parts[1] == "serve"
	default:
		// Unsupported OS — assume not running to trigger a start attempt.
		return false
	}
}

const onboardingMsg = "만들고 싶은 게 있으면 말씀해주세요. /c4-plan으로 시작합니다."
const telegramChannelPlugin = "plugin:telegram@claude-plugins-official"

// telegramChannelConfigured returns true when ~/.claude/channels/telegram/.env
// contains a TELEGRAM_BOT_TOKEN line, meaning the user has set up the telegram plugin.
func telegramChannelConfigured() bool {
	if os.Getenv("CQ_NO_TELEGRAM") != "" {
		return false
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	data, err := os.ReadFile(filepath.Join(home, ".claude", "channels", "telegram", ".env"))
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "TELEGRAM_BOT_TOKEN=") && len(line) > len("TELEGRAM_BOT_TOKEN=") {
			return true
		}
	}
	return false
}

// buildLaunchArgs returns [tool, ...baseArgs] with --append-system-prompt appended when firstRun is true.
func buildLaunchArgs(firstRun bool, tool string, baseArgs []string) []string {
	args := append([]string{tool}, baseArgs...)
	if firstRun {
		args = append(args, "--append-system-prompt", onboardingMsg)
	}
	if tool == "claude" && telegramChannelConfigured() {
		args = append(args, "--channels", telegramChannelPlugin)
	}
	return args
}

// isFirstRun returns true when ~/.c4/first_run does not exist.
// Returns false on any error other than ErrNotExist (e.g. permission denied, HOME unset).
func isFirstRun() bool {
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	_, err = os.Stat(filepath.Join(home, ".c4", "first_run"))
	if err == nil {
		return false
	}
	return os.IsNotExist(err)
}

// markFirstRun creates ~/.c4/first_run to record that onboarding has occurred.
func markFirstRun() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	dir := filepath.Join(home, ".c4")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(dir, "first_run"), os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	return f.Close()
}

// launchTool launches the specified AI coding tool, replacing the current process.
func launchTool(tool, dir string) error {
	var toolCmd string
	var baseArgs []string

	switch tool {
	case "claude":
		toolCmd = "claude"
		baseArgs = nil
	case "codex":
		toolCmd = "codex"
		baseArgs = nil
	case "cursor":
		toolCmd = "cursor"
		baseArgs = []string{dir}
	case "gemini":
		toolCmd = "gemini"
		baseArgs = nil
	default:
		return fmt.Errorf("unknown tool: %s (supported: claude, codex, cursor, gemini)", tool)
	}

	toolPath, err := exec.LookPath(toolCmd)
	if err != nil {
		return fmt.Errorf("%s not found in PATH (install it first): %w", toolCmd, err)
	}

	first := isFirstRun()
	toolArgs := buildLaunchArgs(first, toolCmd, baseArgs)

	fmt.Fprintf(os.Stderr, "cq: launching %s...\n", tool)
	if first {
		if err := markFirstRun(); err != nil {
			fmt.Fprintf(os.Stderr, "cq: warning: markFirstRun: %v\n", err)
		}
	}
	return syscall.Exec(toolPath, toolArgs, os.Environ())
}
