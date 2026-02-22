//go:build windows

// Package session tracks active cq MCP sessions using PID lock files.
// Windows stub: PID liveness checks via kill(0) are not available on Windows.
// ActiveCount always returns 0 to avoid false positives.
package session

import (
	"fmt"
	"os"
)

// Monitor is a no-op session monitor for Windows.
type Monitor struct {
	lockDir string
	pid     int
}

// New creates a Monitor. On Windows the implementation is a stub.
func New(c4Dir string) (*Monitor, error) {
	return &Monitor{
		pid: os.Getpid(),
	}, nil
}

// Start is a no-op on Windows.
func (m *Monitor) Start() error {
	return nil
}

// Stop is a no-op on Windows.
func (m *Monitor) Stop() error {
	return nil
}

// ActiveCount always returns 0 on Windows (stub).
func (m *Monitor) ActiveCount() int {
	return 0
}

// lockFilePath returns the path that would be used on non-Windows platforms.
func (m *Monitor) lockFilePath() string {
	return fmt.Sprintf("%s/%d.lock", m.lockDir, m.pid)
}
