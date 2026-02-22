//go:build !windows

// Package session tracks active cq MCP sessions using PID lock files.
// Each session writes a <pid>.lock file into a per-project subdirectory under
// {c4Dir}/sessions/{sha256(absDir)[:16]}/. Because the OS reclaims file
// descriptors on process exit the lock files serve as lightweight presence
// indicators; stale files from dead processes are cleaned up automatically
// during ActiveCount().
package session

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// lockInfo is the JSON schema written into each <pid>.lock file.
type lockInfo struct {
	PID       int    `json:"pid"`
	Dir       string `json:"dir"`
	StartedAt string `json:"started_at"`
}

// Monitor tracks active sessions for a single project directory using PID
// lock files stored under {c4Dir}/sessions/{dirHash}/.
type Monitor struct {
	lockDir string
	pid     int
}

// New creates a Monitor whose lock files are stored under:
//
//	{c4Dir}/sessions/{sha256(absDir)[:16]}/
//
// c4Dir is typically ~/.c4 for global sessions or the project's .c4/
// directory.  absDir must be an absolute path identifying the project.
func New(c4Dir string) (*Monitor, error) {
	// We use the current working directory as the project identifier so that
	// callers only need to pass c4Dir.
	absDir, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("session.New: cannot resolve working directory: %w", err)
	}

	hash := sha256.Sum256([]byte(absDir))
	dirKey := fmt.Sprintf("%x", hash[:8]) // 16 hex chars from 8 bytes

	lockDir := filepath.Join(c4Dir, "sessions", dirKey)

	return &Monitor{
		lockDir: lockDir,
		pid:     os.Getpid(),
	}, nil
}

// Start creates the <pid>.lock file for this process.
// It is idempotent: calling Start twice is safe.
func (m *Monitor) Start() error {
	if err := os.MkdirAll(m.lockDir, 0755); err != nil {
		return fmt.Errorf("session.Monitor.Start: cannot create lock dir: %w", err)
	}

	info := lockInfo{
		PID:       m.pid,
		Dir:       m.lockDir,
		StartedAt: time.Now().UTC().Format(time.RFC3339),
	}

	data, err := json.Marshal(info)
	if err != nil {
		return fmt.Errorf("session.Monitor.Start: cannot marshal lock info: %w", err)
	}

	lockPath := m.lockFilePath()
	if err := os.WriteFile(lockPath, data, 0644); err != nil {
		return fmt.Errorf("session.Monitor.Start: cannot write lock file: %w", err)
	}

	return nil
}

// Stop removes the <pid>.lock file for this process.
// It is idempotent: calling Stop when no lock file exists returns nil.
func (m *Monitor) Stop() error {
	lockPath := m.lockFilePath()
	err := os.Remove(lockPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("session.Monitor.Stop: cannot remove lock file: %w", err)
	}
	return nil
}

// ActiveCount returns the number of live sessions for this project directory.
// Stale lock files (processes that no longer exist) are silently removed.
func (m *Monitor) ActiveCount() int {
	entries, err := os.ReadDir(m.lockDir)
	if err != nil {
		// Directory does not exist yet → no active sessions.
		return 0
	}

	count := 0
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".lock") {
			continue
		}

		pidStr := strings.TrimSuffix(name, ".lock")
		pid, err := strconv.Atoi(pidStr)
		if err != nil {
			// Unexpected file name; skip.
			continue
		}

		if isAlive(pid) {
			count++
		} else {
			// Remove stale lock file silently.
			_ = os.Remove(filepath.Join(m.lockDir, name))
		}
	}

	return count
}

// lockFilePath returns the absolute path to this process's lock file.
func (m *Monitor) lockFilePath() string {
	return filepath.Join(m.lockDir, fmt.Sprintf("%d.lock", m.pid))
}

// isAlive reports whether the given PID refers to a running process on this
// host by sending signal 0 (no actual signal delivered).
func isAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = proc.Signal(syscall.Signal(0))
	if err == nil {
		return true
	}
	// EPERM means the process exists but we lack permission to signal it.
	if errno, ok := err.(syscall.Errno); ok && errno == syscall.EPERM {
		return true
	}
	return false
}
