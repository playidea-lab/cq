package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
)

// makeTestC5FS builds a minimal fstest.MapFS that looks like the embed/c5/ directory.
func makeTestC5FS(content string) fs.FS {
	m := fstest.MapFS{}
	m["embed/c5/c5"] = &fstest.MapFile{Data: []byte(content), Mode: 0755}
	return m
}

// TestExtractC5_Basic verifies that extractC5 correctly extracts the binary.
func TestExtractC5_Basic(t *testing.T) {
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)

	const testContent = "#!/bin/sh\necho hello\n"
	testFS := makeTestC5FS(testContent)

	binPath, err := extractC5(testFS, "test-v1.0")
	if err != nil {
		t.Fatalf("extractC5() error: %v", err)
	}

	// Verify binary was written.
	data, err := os.ReadFile(binPath)
	if err != nil {
		t.Fatalf("read extracted binary: %v", err)
	}
	if string(data) != testContent {
		t.Errorf("binary content mismatch: got %q, want %q", string(data), testContent)
	}

	// Verify it's in the expected location.
	expectedDir := filepath.Join(fakeHome, ".c4", "bin")
	if !strings.HasPrefix(binPath, expectedDir) {
		t.Errorf("binPath %q not under %q", binPath, expectedDir)
	}

	// Verify version file was written.
	versionPath := filepath.Join(expectedDir, ".c5-version")
	versionData, err := os.ReadFile(versionPath)
	if err != nil {
		t.Fatalf("read version file: %v", err)
	}
	if strings.TrimSpace(string(versionData)) != "test-v1.0" {
		t.Errorf("version file content: got %q, want %q", string(versionData), "test-v1.0")
	}
}

// TestExtractC5_FastPath verifies that when the installed version matches the
// embedded version, the binary is not re-extracted.
func TestExtractC5_FastPath(t *testing.T) {
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)

	binDir := filepath.Join(fakeHome, ".c4", "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Pre-populate: binary and version file with matching version.
	binPath := filepath.Join(binDir, "c5")
	sentinel := "# sentinel binary"
	if err := os.WriteFile(binPath, []byte(sentinel), 0755); err != nil {
		t.Fatalf("write sentinel binary: %v", err)
	}
	versionPath := filepath.Join(binDir, ".c5-version")
	if err := os.WriteFile(versionPath, []byte("v1.2.3\n"), 0644); err != nil {
		t.Fatalf("write version file: %v", err)
	}

	// Use a FS with different content but same version string.
	testFS := makeTestC5FS("#!/bin/sh\necho new\n")

	gotPath, err := extractC5(testFS, "v1.2.3")
	if err != nil {
		t.Fatalf("extractC5() error: %v", err)
	}

	// Sentinel should still be there (not overwritten).
	data, err := os.ReadFile(gotPath)
	if err != nil {
		t.Fatalf("read binary: %v", err)
	}
	if string(data) != sentinel {
		t.Errorf("binary was overwritten (fast path should have skipped extraction)")
	}
}

// TestExtractC5_AtomicWrite verifies that concurrent goroutines all extract
// successfully and end up with the same binary content (atomic rename).
func TestExtractC5_AtomicWrite(t *testing.T) {
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)

	const testContent = "#!/bin/sh\necho concurrent\n"
	testFS := makeTestC5FS(testContent)

	const numGoroutines = 10
	paths := make([]string, numGoroutines)
	errs := make([]error, numGoroutines)

	var wg sync.WaitGroup
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		i := i
		go func() {
			defer wg.Done()
			// version="" means no fast path — always extracts.
			paths[i], errs[i] = extractC5(testFS, "")
		}()
	}
	wg.Wait()

	// All goroutines must succeed.
	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d: extractC5() error: %v", i, err)
		}
	}

	// All paths must be the same.
	for i, p := range paths {
		if p != paths[0] {
			t.Errorf("goroutine %d: path %q != %q", i, p, paths[0])
		}
	}

	// The final binary must have the correct content.
	data, err := os.ReadFile(paths[0])
	if err != nil {
		t.Fatalf("read final binary: %v", err)
	}
	if string(data) != testContent {
		t.Errorf("final binary content mismatch: got %q, want %q", string(data), testContent)
	}
}
