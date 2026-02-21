package serve

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// DefaultPIDPath returns the default path for the serve PID file.
func DefaultPIDPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".c4", "serve", "serve.pid")
}

// DefaultHealthURL is the default health endpoint for cq serve.
const DefaultHealthURL = "http://localhost:4140/health"

// IsServeRunning checks if cq serve is running by:
// 1. Reading the PID file at ~/.c4/serve/serve.pid
// 2. Checking if the process is alive (signal 0)
// 3. Confirming via HTTP GET to localhost:4140/health
//
// Returns true only if both PID is alive AND health endpoint responds 200.
// Stale PID files (process dead) are handled gracefully (returns false).
func IsServeRunning() bool {
	return isServeRunningWith(DefaultPIDPath(), DefaultHealthURL)
}

// isServeRunningWith is the testable inner function.
func isServeRunningWith(pidPath, healthURL string) bool {
	if pidPath == "" {
		return false
	}

	// Step 1: Read PID file
	data, err := os.ReadFile(pidPath)
	if err != nil {
		return false // no PID file → not running
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 0 {
		return false // invalid PID
	}

	// Step 2: Check if process is alive
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	if err := proc.Signal(syscall.Signal(0)); err != nil {
		return false // process dead → stale PID file
	}

	// Step 3: HTTP health check
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(healthURL)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	// Accept both 200 (ok) and 503 (degraded) — a degraded serve is still running
	return resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusServiceUnavailable
}

// StatusMessage returns a human-readable string for stderr logging.
func StatusMessage(component string) string {
	return fmt.Sprintf("cq: serve running, skipping %s (managed by serve)", component)
}
