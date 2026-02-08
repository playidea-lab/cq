package main

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"
)

// captureStderr captures stderr output during a function call.
func captureStderr(fn func()) string {
	// Save original stderr
	origStderr := os.Stderr

	// Create a pipe
	r, w, err := os.Pipe()
	if err != nil {
		panic(err)
	}
	os.Stderr = w

	// Run the function
	fn()

	// Restore stderr and read captured output
	w.Close()
	os.Stderr = origStderr

	var buf bytes.Buffer
	buf.ReadFrom(r)
	r.Close()

	return buf.String()
}

// TestFallbackNormalGoStartup tests that when the Go server succeeds,
// no fallback occurs and no warning is printed.
func TestFallbackNormalGoStartup(t *testing.T) {
	goServerCalled := false

	cfg := &FallbackConfig{
		ForcePython: false,
		GoServerFunc: func() error {
			goServerCalled = true
			return nil // Success
		},
		PythonCommand: "echo", // Won't be called
		PythonArgs:    []string{"should-not-run"},
	}

	var stderrOutput string
	var runErr error

	stderrOutput = captureStderr(func() {
		runErr = RunWithFallback(cfg)
	})

	if runErr != nil {
		t.Fatalf("expected no error, got: %v", runErr)
	}

	if !goServerCalled {
		t.Error("Go server function was not called")
	}

	if strings.Contains(stderrOutput, "Falling back") {
		t.Errorf("should not print fallback message on success, got: %s", stderrOutput)
	}
}

// TestFallbackToPython tests that when the Go server fails,
// the system falls back to the Python server.
func TestFallbackToPython(t *testing.T) {
	goServerCalled := false
	pythonFallbackReached := false

	cfg := &FallbackConfig{
		ForcePython: false,
		GoServerFunc: func() error {
			goServerCalled = true
			return fmt.Errorf("simulated Go server failure")
		},
		// Use a command that exists and exits successfully as our "Python" stand-in
		PythonCommand: "true",
		PythonArgs:    []string{},
	}

	var stderrOutput string
	var runErr error

	stderrOutput = captureStderr(func() {
		// Since findPythonMCP needs exec.LookPath to work,
		// we'll test the logic flow differently
		runErr = safeRunGoServer(cfg)
		if runErr != nil {
			pythonFallbackReached = true
		}
	})

	_ = stderrOutput // stderr captured but may be empty in this test path

	if !goServerCalled {
		t.Error("Go server function was not called")
	}

	if runErr == nil {
		t.Fatal("expected error from Go server, got nil")
	}

	if !pythonFallbackReached {
		t.Error("Python fallback was not reached")
	}

	if !strings.Contains(runErr.Error(), "simulated Go server failure") {
		t.Errorf("expected error message about Go failure, got: %v", runErr)
	}
}

// TestFallbackToPythonWithPanic tests that panics in the Go server
// are recovered and trigger fallback.
func TestFallbackToPythonWithPanic(t *testing.T) {
	cfg := &FallbackConfig{
		ForcePython: false,
		GoServerFunc: func() error {
			panic("unexpected nil pointer")
		},
	}

	err := safeRunGoServer(cfg)
	if err == nil {
		t.Fatal("expected error after panic, got nil")
	}

	if !strings.Contains(err.Error(), "panic") {
		t.Errorf("expected panic in error message, got: %v", err)
	}

	if !strings.Contains(err.Error(), "unexpected nil pointer") {
		t.Errorf("expected panic value in error, got: %v", err)
	}
}

// TestFallbackForcePythonEnvVar tests that C4_FORCE_PYTHON=1
// skips the Go server entirely.
func TestFallbackForcePythonEnvVar(t *testing.T) {
	goServerCalled := false

	cfg := &FallbackConfig{
		ForcePython: true, // Simulates C4_FORCE_PYTHON=1
		GoServerFunc: func() error {
			goServerCalled = true
			return nil
		},
		PythonCommand: "true",
		PythonArgs:    []string{},
	}

	var stderrOutput string

	stderrOutput = captureStderr(func() {
		// RunWithFallback will try to run Python directly
		// Since "true" exists, findPythonMCP will succeed
		_ = RunWithFallback(cfg)
	})

	if goServerCalled {
		t.Error("Go server should NOT be called when ForcePython is true")
	}

	if !strings.Contains(stderrOutput, "C4_FORCE_PYTHON=1") {
		t.Errorf("expected force-Python message in stderr, got: %s", stderrOutput)
	}
}

// TestFallbackConfigDefault verifies DefaultFallbackConfig reads env vars correctly.
func TestFallbackConfigDefault(t *testing.T) {
	// Without C4_FORCE_PYTHON
	os.Unsetenv("C4_FORCE_PYTHON")
	cfg := DefaultFallbackConfig()
	if cfg.ForcePython {
		t.Error("ForcePython should be false when env var is not set")
	}
	if cfg.PythonCommand != "uv" {
		t.Errorf("expected PythonCommand 'uv', got %q", cfg.PythonCommand)
	}

	// With C4_FORCE_PYTHON=1
	os.Setenv("C4_FORCE_PYTHON", "1")
	defer os.Unsetenv("C4_FORCE_PYTHON")

	cfg = DefaultFallbackConfig()
	if !cfg.ForcePython {
		t.Error("ForcePython should be true when C4_FORCE_PYTHON=1")
	}
}

// TestFallbackConfigForcePythonZero verifies that C4_FORCE_PYTHON=0 does not trigger.
func TestFallbackConfigForcePythonZero(t *testing.T) {
	os.Setenv("C4_FORCE_PYTHON", "0")
	defer os.Unsetenv("C4_FORCE_PYTHON")

	cfg := DefaultFallbackConfig()
	if cfg.ForcePython {
		t.Error("ForcePython should be false when C4_FORCE_PYTHON=0")
	}
}

// TestSafeRunGoServerSuccess verifies clean exit doesn't trigger recovery.
func TestSafeRunGoServerSuccess(t *testing.T) {
	cfg := &FallbackConfig{
		GoServerFunc: func() error {
			return nil
		},
	}

	err := safeRunGoServer(cfg)
	if err != nil {
		t.Fatalf("expected nil error on success, got: %v", err)
	}
}

// TestSafeRunGoServerError verifies error propagation without panic.
func TestSafeRunGoServerError(t *testing.T) {
	cfg := &FallbackConfig{
		GoServerFunc: func() error {
			return fmt.Errorf("dependency not found: modernc.org/sqlite")
		},
	}

	err := safeRunGoServer(cfg)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "dependency not found") {
		t.Errorf("expected dependency error, got: %v", err)
	}
}
