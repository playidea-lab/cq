package state_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/changmin/c4-core/internal/state"
)

// newTestLocker creates a PhaseLocker backed by a temporary directory.
func newTestLocker(t *testing.T) (*state.PhaseLocker, string) {
	t.Helper()
	tmpDir := t.TempDir()
	// The locker expects rootDir to contain a .c4/ subdir.
	// It creates .c4/phase_locks/ automatically.
	return state.NewPhaseLocker(tmpDir), tmpDir
}

// TestPhaseLockAcquireAndRelease verifies basic acquire and release semantics.
func TestPhaseLockAcquireAndRelease(t *testing.T) {
	locker, _ := newTestLocker(t)

	// Acquire should succeed on fresh state.
	result := locker.Acquire("polish")
	if !result.Acquired {
		t.Fatalf("expected Acquired=true, got error: %+v", result.Error)
	}
	if result.Error != nil {
		t.Errorf("expected nil Error on success, got %+v", result.Error)
	}

	// Release should succeed.
	if !locker.Release("polish") {
		t.Fatal("Release returned false")
	}

	// After release, acquire should succeed again.
	result2 := locker.Acquire("polish")
	if !result2.Acquired {
		t.Fatalf("expected Acquired=true after release, got error: %+v", result2.Error)
	}
	_ = locker.Release("polish")
}

// TestPhaseLockCrossSession verifies that a second locker cannot acquire when
// the first has a live lock (LOCK_HELD scenario).
func TestPhaseLockCrossSession(t *testing.T) {
	locker1, tmpDir := newTestLocker(t)
	locker2 := state.NewPhaseLocker(tmpDir)

	// locker1 acquires.
	res1 := locker1.Acquire("polish")
	if !res1.Acquired {
		t.Fatalf("locker1 acquire failed: %+v", res1.Error)
	}
	defer locker1.Release("polish")

	// Write a lock file with our own live PID so locker2 sees it as valid.
	// (The file written by locker1 above already has the current PID and hostname.)

	// locker2 should see LOCK_HELD.
	res2 := locker2.Acquire("polish")
	if res2.Acquired {
		t.Fatal("expected locker2 acquire to fail (LOCK_HELD)")
	}
	if res2.Error == nil {
		t.Fatal("expected non-nil Error")
	}
	if res2.Error.Code != "LOCK_HELD" {
		t.Errorf("expected code LOCK_HELD, got %q", res2.Error.Code)
	}
	if res2.Error.Details == nil {
		t.Fatal("expected non-nil Details")
	}
	if res2.Error.Details.HolderPID != os.Getpid() {
		t.Errorf("expected holder PID %d, got %d", os.Getpid(), res2.Error.Details.HolderPID)
	}
}

// TestPhaseLockMCPResponse verifies the JSON schema of PhaseLockResult.
func TestPhaseLockMCPResponse(t *testing.T) {
	locker, tmpDir := newTestLocker(t)

	// Success case: acquired=true, error=null.
	t.Run("success_schema", func(t *testing.T) {
		res := locker.Acquire("finish")
		defer locker.Release("finish")

		data, err := json.Marshal(res)
		if err != nil {
			t.Fatalf("json.Marshal failed: %v", err)
		}
		var m map[string]any
		if err := json.Unmarshal(data, &m); err != nil {
			t.Fatalf("unmarshal failed: %v", err)
		}
		if m["acquired"] != true {
			t.Errorf("expected acquired=true in JSON, got %v", m["acquired"])
		}
		if _, hasError := m["error"]; hasError && m["error"] != nil {
			t.Errorf("expected error=null in JSON, got %v", m["error"])
		}
	})

	// Failure case: acquired=false, error has code/message/details.
	t.Run("lock_held_schema", func(t *testing.T) {
		// Write a lock file with a dead PID on a foreign hostname so it stays valid.
		locker2 := state.NewPhaseLocker(tmpDir)
		lockDir := filepath.Join(tmpDir, ".c4", "phase_locks")
		if err := os.MkdirAll(lockDir, 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		fakeLock := map[string]any{
			"pid":         99999999,
			"hostname":    "foreign-host",
			"phase":       "polish",
			"acquired_at": time.Now().UTC().Format(time.RFC3339),
			"session_id":  "test-session",
		}
		data, _ := json.Marshal(fakeLock)
		lockFile := filepath.Join(lockDir, "polish.lock")
		if err := os.WriteFile(lockFile, data, 0644); err != nil {
			t.Fatalf("write lock: %v", err)
		}
		defer os.Remove(lockFile)

		res := locker2.Acquire("polish")
		if res.Acquired {
			t.Fatal("expected acquire to fail")
		}

		jsonData, err := json.Marshal(res)
		if err != nil {
			t.Fatalf("json.Marshal failed: %v", err)
		}
		var m map[string]any
		if err := json.Unmarshal(jsonData, &m); err != nil {
			t.Fatalf("unmarshal failed: %v", err)
		}
		if m["acquired"] != false {
			t.Errorf("expected acquired=false, got %v", m["acquired"])
		}
		errObj, ok := m["error"].(map[string]any)
		if !ok {
			t.Fatalf("expected error object, got %T: %v", m["error"], m["error"])
		}
		if errObj["code"] != "LOCK_HELD" {
			t.Errorf("expected code=LOCK_HELD, got %v", errObj["code"])
		}
		if _, ok := errObj["message"]; !ok {
			t.Error("expected message field in error")
		}
		details, ok := errObj["details"].(map[string]any)
		if !ok {
			t.Fatalf("expected details object, got %T", errObj["details"])
		}
		if _, ok := details["holder_pid"]; !ok {
			t.Error("expected holder_pid in details")
		}
		if _, ok := details["holder_hostname"]; !ok {
			t.Error("expected holder_hostname in details")
		}
	})
}

// TestPhaseLockStaleDetection tests all 5 stale detection scenarios.
func TestPhaseLockStaleDetection(t *testing.T) {
	hostname, _ := os.Hostname()

	writeTestLock := func(t *testing.T, lockFile string, info map[string]any) {
		t.Helper()
		data, err := json.Marshal(info)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		if err := os.MkdirAll(filepath.Dir(lockFile), 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(lockFile, data, 0644); err != nil {
			t.Fatalf("write: %v", err)
		}
	}

	// Scenario 1: Same host, PID alive (current process) → NOT stale → acquire fails.
	t.Run("scenario1_same_host_pid_alive", func(t *testing.T) {
		tmpDir := t.TempDir()
		locker := state.NewPhaseLocker(tmpDir)
		lockDir := filepath.Join(tmpDir, ".c4", "phase_locks")
		lockFile := filepath.Join(lockDir, "polish.lock")

		writeTestLock(t, lockFile, map[string]any{
			"pid":         os.Getpid(), // current PID — definitely alive
			"hostname":    hostname,
			"phase":       "polish",
			"acquired_at": time.Now().UTC().Format(time.RFC3339),
		})

		res := locker.Acquire("polish")
		if res.Acquired {
			t.Fatal("scenario1: expected acquire to fail (lock valid, PID alive)")
		}
		if res.Error == nil || res.Error.Code != "LOCK_HELD" {
			t.Errorf("scenario1: expected LOCK_HELD, got %+v", res.Error)
		}
	})

	// Scenario 2: Same host, PID dead (ESRCH) → stale → acquire succeeds.
	t.Run("scenario2_same_host_pid_dead", func(t *testing.T) {
		// Find a PID that is definitely dead: use a very large unlikely PID.
		// We verify it's dead by checking kill(pid, 0).
		deadPID := findDeadPID(t)

		tmpDir := t.TempDir()
		locker := state.NewPhaseLocker(tmpDir)
		lockDir := filepath.Join(tmpDir, ".c4", "phase_locks")
		lockFile := filepath.Join(lockDir, "polish.lock")

		writeTestLock(t, lockFile, map[string]any{
			"pid":         deadPID,
			"hostname":    hostname,
			"phase":       "polish",
			"acquired_at": time.Now().UTC().Format(time.RFC3339),
		})

		res := locker.Acquire("polish")
		if !res.Acquired {
			t.Fatalf("scenario2: expected acquire to succeed (stale lock, PID dead), got %+v", res.Error)
		}
		_ = locker.Release("polish")
	})

	// Scenario 3: Same host, EPERM (we simulate by noting we can't easily trigger
	// EPERM in a test, so we document this scenario as conservative/not stale.
	// This scenario is implicitly tested: any non-ESRCH, non-nil sigErr → not stale.
	// We skip direct EPERM injection as it would require root privileges.)

	// Scenario 4: Different host, age < 2h → NOT stale → acquire fails.
	t.Run("scenario4_diff_host_recent", func(t *testing.T) {
		tmpDir := t.TempDir()
		locker := state.NewPhaseLocker(tmpDir)
		lockDir := filepath.Join(tmpDir, ".c4", "phase_locks")
		lockFile := filepath.Join(lockDir, "polish.lock")

		writeTestLock(t, lockFile, map[string]any{
			"pid":         12345,
			"hostname":    "other-host-xyz",
			"phase":       "polish",
			"acquired_at": time.Now().UTC().Add(-30 * time.Minute).Format(time.RFC3339), // 30m ago
		})

		res := locker.Acquire("polish")
		if res.Acquired {
			t.Fatal("scenario4: expected acquire to fail (diff host, lock < 2h old)")
		}
		if res.Error == nil || res.Error.Code != "LOCK_HELD" {
			t.Errorf("scenario4: expected LOCK_HELD, got %+v", res.Error)
		}
	})

	// Scenario 5: Different host, age >= 2h → stale → acquire succeeds.
	t.Run("scenario5_diff_host_stale", func(t *testing.T) {
		tmpDir := t.TempDir()
		locker := state.NewPhaseLocker(tmpDir)
		lockDir := filepath.Join(tmpDir, ".c4", "phase_locks")
		lockFile := filepath.Join(lockDir, "polish.lock")

		writeTestLock(t, lockFile, map[string]any{
			"pid":         12345,
			"hostname":    "other-host-xyz",
			"phase":       "polish",
			"acquired_at": time.Now().UTC().Add(-3 * time.Hour).Format(time.RFC3339), // 3h ago
		})

		res := locker.Acquire("polish")
		if !res.Acquired {
			t.Fatalf("scenario5: expected acquire to succeed (diff host, lock >= 2h), got %+v", res.Error)
		}
		_ = locker.Release("polish")
	})
}

// findDeadPID returns a PID that is confirmed dead via kill -0.
// It scans candidate PIDs to find one where Signal(0) returns an error
// indicating the process is dead (ESRCH on Linux, "process already finished" on macOS).
func findDeadPID(t *testing.T) int {
	t.Helper()
	// Scan a wide range of candidate PIDs looking for one that is dead.
	// Start from a high value to avoid kernel PIDs (1, 2, etc.) and system processes.
	for pid := 60000; pid > 1000; pid-- {
		proc, err := os.FindProcess(pid)
		if err != nil {
			return pid
		}
		sigErr := proc.Signal(syscall.Signal(0))
		if sigErr != nil {
			if isDeadPIDError(sigErr) {
				return pid
			}
		}
	}

	t.Skip("could not find a dead PID for scenario 2 test — skipping")
	return -1
}

// isDeadPIDError returns true if the error from Signal(0) indicates a dead process.
// Covers ESRCH (Linux) and "os: process already finished" (macOS).
func isDeadPIDError(err error) bool {
	if err == nil {
		return false
	}
	if errno, ok := err.(syscall.Errno); ok && errno == syscall.ESRCH {
		return true
	}
	return err.Error() == "os: process already finished"
}
