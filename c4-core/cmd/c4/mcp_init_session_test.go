package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/changmin/c4-core/internal/config"
	"github.com/changmin/c4-core/internal/session"
)

// sessionCfgWithLimit returns a SessionsConfig with the given limit and enabled flag.
func sessionCfgWithLimit(limit int, enabled bool) config.SessionsConfig {
	return config.SessionsConfig{Limit: limit, Enabled: enabled}
}

// makeInitContextWithSessions builds a minimal initContext whose cfgMgr has the
// given sessions config applied.
func makeInitContextWithSessions(t *testing.T, projectDir string, limit int, enabled bool) *initContext {
	t.Helper()

	tmpRoot := t.TempDir()
	_ = os.MkdirAll(filepath.Join(tmpRoot, ".c4"), 0755)

	mgr, err := config.New(tmpRoot)
	if err != nil {
		t.Fatalf("config.New: %v", err)
	}
	mgr.Set("sessions.limit", limit)
	mgr.Set("sessions.enabled", enabled)

	return &initContext{
		projectDir: projectDir,
		cfgMgr:     mgr,
	}
}

// TestInitSession_Disabled verifies that the warning is not emitted when sessions.enabled=false.
func TestInitSession_Disabled(t *testing.T) {
	tmpDir := t.TempDir()
	ctx := makeInitContextWithSessions(t, tmpDir, 2, false)

	output := captureStderr(func() {
		_ = initSession(ctx)
	})

	if output != "" {
		t.Errorf("expected no stderr output when sessions.enabled=false, got: %q", output)
	}
	if ctx.sessionMonitor != nil {
		t.Error("expected sessionMonitor to be nil when sessions.enabled=false")
	}
}

// TestInitSession_NoWarning_UnderLimit verifies that no warning is emitted when
// active sessions are within the configured limit.
func TestInitSession_NoWarning_UnderLimit(t *testing.T) {
	tmpDir := t.TempDir()
	// limit=4, this process = 1 active session → no warning.
	ctx := makeInitContextWithSessions(t, tmpDir, 4, true)

	output := captureStderr(func() {
		_ = initSession(ctx)
	})

	if sessionContains(output, "초과") {
		t.Errorf("expected no warning when sessions (1) <= limit (4), got: %q", output)
	}

	if ctx.sessionMonitor != nil {
		_ = ctx.sessionMonitor.Stop()
	}
}

// TestInitSession_Warning_OverLimit verifies that the cloud-upgrade warning is emitted
// when active session count exceeds the configured limit.
func TestInitSession_Warning_OverLimit(t *testing.T) {
	tmpDir := t.TempDir()
	c4Dir := filepath.Join(tmpDir, ".c4")
	_ = os.MkdirAll(c4Dir, 0755)

	// Pre-create the lock directory by starting a throwaway monitor.
	// This makes the sessions subdirectory (sha256(os.Getwd())[:16]) exist before
	// initSession runs so we can inject extra lock files.
	preMonitor, err := session.New(c4Dir)
	if err != nil {
		t.Fatalf("session.New (pre): %v", err)
	}
	if err := preMonitor.Start(); err != nil {
		t.Fatalf("preMonitor.Start: %v", err)
	}
	t.Cleanup(func() { _ = preMonitor.Stop() })

	// Find the lock subdirectory created by preMonitor.
	sessionsDir := filepath.Join(c4Dir, "sessions")
	entries, err := os.ReadDir(sessionsDir)
	if err != nil || len(entries) == 0 {
		t.Fatalf("sessions dir not found or empty after preMonitor.Start: %v", err)
	}
	var lockSubDir string
	for _, e := range entries {
		if e.IsDir() {
			lockSubDir = filepath.Join(sessionsDir, e.Name())
			break
		}
	}
	if lockSubDir == "" {
		t.Fatal("no lock subdirectory found")
	}

	// Inject PID 1 lock file (init/launchd — guaranteed alive on Unix).
	// Combined with preMonitor's own PID and initSession's PID (same process → same file),
	// total count = 2 (this process + PID 1). limit=1 → 2 > 1 → warning fires.
	pid1Lock := filepath.Join(lockSubDir, "1.lock")
	if err := os.WriteFile(pid1Lock, []byte(fmt.Sprintf(`{"pid":1,"dir":"%s","started_at":"2026-01-01T00:00:00Z"}`, lockSubDir)), 0644); err != nil {
		t.Fatalf("write pid1 lock: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(pid1Lock) })

	// With limit=1 and 2 active sessions (this process + PID 1), warning should fire.
	ctx := makeInitContextWithSessions(t, tmpDir, 1, true)

	output := captureStderr(func() {
		_ = initSession(ctx)
	})

	// On Windows stub, ActiveCount() == 0 so warning never fires. Skip.
	if ctx.sessionMonitor != nil && ctx.sessionMonitor.ActiveCount() == 0 {
		t.Skip("session monitor stub (Windows): ActiveCount() == 0, skipping warning check")
	}

	if !sessionContains(output, "초과") {
		t.Errorf("expected cloud-upgrade warning in stderr, got: %q", output)
	}

	if ctx.sessionMonitor != nil {
		_ = ctx.sessionMonitor.Stop()
	}
}

// TestInitSession_WindowsStub verifies the logic when ActiveCount() returns 0.
// On Windows, the session monitor stub always returns 0, so no warning fires.
func TestInitSession_WindowsStub(t *testing.T) {
	tmpDir := t.TempDir()
	// limit=0: condition is count > limit → 0 > 0 == false → no warning on stub.
	ctx := makeInitContextWithSessions(t, tmpDir, 0, true)

	output := captureStderr(func() {
		_ = initSession(ctx)
	})

	if ctx.sessionMonitor != nil {
		count := ctx.sessionMonitor.ActiveCount()
		if count == 0 {
			// Stub behaviour (Windows): no warning expected.
			if sessionContains(output, "초과") {
				t.Errorf("expected no warning when ActiveCount() == 0, got: %q", output)
			}
		}
		// On real Unix count > 0, so count > limit=0 → warning fires; that is expected.
		_ = ctx.sessionMonitor.Stop()
	}
}

// sessionContains is a substring check helper for session tests.
func sessionContains(s, substr string) bool {
	if len(substr) == 0 {
		return true
	}
	if len(s) < len(substr) {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
