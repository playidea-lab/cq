//go:build !windows

package session_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/changmin/c4-core/internal/session"
)

// newMonitorForTest creates a Monitor using t.TempDir() as c4Dir and sets the
// working directory to a deterministic test directory so that the sha256 key
// is reproducible within the test.
func newMonitorForTest(t *testing.T) *session.Monitor {
	t.Helper()
	c4Dir := t.TempDir()
	m, err := session.New(c4Dir)
	if err != nil {
		t.Fatalf("session.New: %v", err)
	}
	return m
}

// TestMonitorStart verifies that Start() creates the <pid>.lock file.
func TestMonitorStart(t *testing.T) {
	c4Dir := t.TempDir()
	m, err := session.New(c4Dir)
	if err != nil {
		t.Fatalf("session.New: %v", err)
	}

	if err := m.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Find the lock file anywhere under c4Dir/sessions/.
	lockPath := findLockFile(t, c4Dir, os.Getpid())
	if lockPath == "" {
		t.Fatalf("expected <pid>.lock file under %s/sessions/, not found", c4Dir)
	}
}

// TestMonitorStop verifies that Stop() removes the <pid>.lock file.
func TestMonitorStop(t *testing.T) {
	c4Dir := t.TempDir()
	m, err := session.New(c4Dir)
	if err != nil {
		t.Fatalf("session.New: %v", err)
	}

	if err := m.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	lockPath := findLockFile(t, c4Dir, os.Getpid())
	if lockPath == "" {
		t.Fatalf("expected <pid>.lock file after Start, not found")
	}

	if err := m.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Fatalf("expected lock file %s to be removed after Stop, but it still exists", lockPath)
	}
}

// TestMonitorStopIdempotent verifies that calling Stop when no lock file exists
// returns nil without error.
func TestMonitorStopIdempotent(t *testing.T) {
	c4Dir := t.TempDir()
	m, err := session.New(c4Dir)
	if err != nil {
		t.Fatalf("session.New: %v", err)
	}

	// Stop without a prior Start — must not error.
	if err := m.Stop(); err != nil {
		t.Fatalf("Stop (no prior Start): %v", err)
	}
}

// TestActiveCountExcludesDeadPIDs verifies that ActiveCount() filters out lock
// files for PIDs that no longer exist.
func TestActiveCountExcludesDeadPIDs(t *testing.T) {
	c4Dir := t.TempDir()
	m, err := session.New(c4Dir)
	if err != nil {
		t.Fatalf("session.New: %v", err)
	}

	if err := m.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Inject a stale lock file for a PID that almost certainly does not exist.
	// PID 1 on macOS/Linux is init/launchd and is always alive, so we use a
	// large number that is very unlikely to be an active process.
	lockDir := findLockDir(t, c4Dir)
	stalePID := 2000000 // outside the typical PID range
	stalePath := filepath.Join(lockDir, fmt.Sprintf("%d.lock", stalePID))
	if err := os.WriteFile(stalePath, []byte(`{"pid":2000000,"dir":"","started_at":""}`), 0644); err != nil {
		t.Fatalf("write stale lock: %v", err)
	}

	count := m.ActiveCount()
	// The stale lock file should be removed and not counted.
	// ActiveCount should be exactly 1 (our own process).
	if count != 1 {
		t.Fatalf("expected ActiveCount == 1, got %d", count)
	}

	// Stale file should have been cleaned up.
	if _, err := os.Stat(stalePath); !os.IsNotExist(err) {
		t.Fatalf("stale lock file %s was not removed by ActiveCount", stalePath)
	}
}

// TestActiveCountTwoMonitors verifies that two Monitors for the same directory
// both show up in ActiveCount.
func TestActiveCountTwoMonitors(t *testing.T) {
	c4Dir := t.TempDir()

	// Both monitors use the same c4Dir (and the same working directory), so they
	// share the same lockDir.  To simulate two concurrent sessions we create a
	// second monitor that uses a fake PID that we know is alive (our own PID
	// incremented by zero — we still write the file manually because the
	// Monitor uses os.Getpid() internally).
	m1, err := session.New(c4Dir)
	if err != nil {
		t.Fatalf("session.New (m1): %v", err)
	}
	if err := m1.Start(); err != nil {
		t.Fatalf("m1.Start: %v", err)
	}

	// Simulate a second live session by writing its lock file directly.
	lockDir := findLockDir(t, c4Dir)
	// Use PID 1 (init/launchd) which is guaranteed to exist on Unix.
	secondPID := 1
	lockPath2 := filepath.Join(lockDir, fmt.Sprintf("%d.lock", secondPID))
	if err := os.WriteFile(lockPath2, []byte(`{"pid":1,"dir":"","started_at":""}`), 0644); err != nil {
		t.Fatalf("write second lock: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(lockPath2) })

	count := m1.ActiveCount()
	if count != 2 {
		t.Fatalf("expected ActiveCount == 2 (own + PID 1), got %d", count)
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// findLockFile walks c4Dir/sessions/ and returns the path of the first lock
// file whose base name (without .lock) equals pid, or "" if not found.
func findLockFile(t *testing.T, c4Dir string, pid int) string {
	t.Helper()
	sessionsDir := filepath.Join(c4Dir, "sessions")
	target := strconv.Itoa(pid) + ".lock"

	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		subEntries, _ := os.ReadDir(filepath.Join(sessionsDir, e.Name()))
		for _, se := range subEntries {
			if se.Name() == target {
				return filepath.Join(sessionsDir, e.Name(), target)
			}
		}
	}
	return ""
}

// findLockDir returns the first subdirectory under c4Dir/sessions/ or "" if
// none exists.
func findLockDir(t *testing.T, c4Dir string) string {
	t.Helper()
	sessionsDir := filepath.Join(c4Dir, "sessions")
	entries, err := os.ReadDir(sessionsDir)
	if err != nil || len(entries) == 0 {
		t.Fatalf("no sessions subdirectory found under %s", sessionsDir)
	}
	for _, e := range entries {
		if e.IsDir() {
			return filepath.Join(sessionsDir, e.Name())
		}
	}
	t.Fatalf("no directory entries under %s", sessionsDir)
	return ""
}
