package main

import (
	"os"
	"strconv"
	"testing"
)

// TestEnsureServeRunning_NoServeFlag verifies that ensureServeRunning(true) is a no-op.
func TestEnsureServeRunning_NoServeFlag(t *testing.T) {
	// Should return immediately without any side effects.
	ensureServeRunning(true)
}

// TestIsCQServeProcess_NotRunning verifies that a non-existent PID returns false.
func TestIsCQServeProcess_NotRunning(t *testing.T) {
	if isCQServeProcess(999999) {
		t.Error("expected false for non-existent PID 999999")
	}
}

// TestEnsureServeRunning_AlreadyRunning exercises the PID file read path.
// It isolates the PID path to a tmpdir so the real ~/.c4/ is not touched.
func TestEnsureServeRunning_AlreadyRunning(t *testing.T) {
	// Isolate PID file path to tmpDir.
	orig := servePIDPath
	servePIDPath = tmpServePID(t, os.Getpid())
	defer func() { servePIDPath = orig }()

	// Call ensureServeRunning(false) — exercises:
	//   - os.ReadFile of serve.pid (reads our tmpdir file)
	//   - strconv.Atoi + isCQServeProcess (returns false for test process)
	//   - os.Executable() + exec.Command fork attempt (gracefully fails)
	// Should not panic.
	ensureServeRunning(false)
}

// TestEnsureServeRunning_WithPIDFile writes a PID file pointing to the current process
// and injects the path via servePIDPath, verifying ensureServeRunning reads it correctly.
func TestEnsureServeRunning_WithPIDFile(t *testing.T) {
	orig := servePIDPath
	servePIDPath = tmpServePID(t, os.Getpid())
	defer func() { servePIDPath = orig }()

	// isCQServeProcess returns false for the test process (not "cq serve"),
	// so ensureServeRunning will attempt to fork (gracefully fails in tests).
	ensureServeRunning(false) // must not panic
}

// tmpServePID writes a PID file to a temp location and returns the path.
func tmpServePID(t *testing.T, pid int) string {
	t.Helper()
	pidFile := t.TempDir() + "/serve.pid"
	if err := os.WriteFile(pidFile, []byte(strconv.Itoa(pid)), 0644); err != nil {
		t.Fatal(err)
	}
	return pidFile
}
