package main

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
)

// atomicExtractCounter ensures unique tmpfile names across concurrent goroutines.
var atomicExtractCounter atomic.Int64

// extractC5 extracts the c5 binary from embFS to ~/.c4/bin/c5 and returns the path.
//
// Fast path: if ~/.c4/bin/.c5-version already contains version, the path is returned
// immediately without re-extracting.
//
// Concurrent calls are safe: a tmpfile + atomic rename pattern ensures exactly one
// binary is written even under concurrent goroutines.
func extractC5(embFS fs.FS, version string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("embed_c5: get home dir: %w", err)
	}

	binDir := filepath.Join(home, ".c4", "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		return "", fmt.Errorf("embed_c5: mkdir %s: %w", binDir, err)
	}

	// Determine platform-specific binary name.
	binName := "c5"
	if runtime.GOOS == "windows" {
		binName = "c5.exe"
	}
	destPath := filepath.Join(binDir, binName)
	versionPath := filepath.Join(binDir, ".c5-version")

	// Fast path: check installed version.
	if version != "" {
		versionData, readErr := os.ReadFile(versionPath)
		if readErr == nil {
			installed := strings.TrimSpace(string(versionData))
			if installed == version {
				return destPath, nil
			}
		}
	}

	// Determine embedded binary path.
	embedPath := "embed/hub/hub"
	if runtime.GOOS == "windows" {
		embedPath = "embed/hub/hub.exe"
	}

	src, err := embFS.Open(embedPath)
	if err != nil {
		return "", fmt.Errorf("embed_c5: open embedded binary %q: %w", embedPath, err)
	}
	defer src.Close()

	// Write to a temporary file on the same filesystem to ensure atomic rename.
	// Use a unique suffix (pid + counter) to avoid conflicts between concurrent goroutines.
	tmpPath := fmt.Sprintf("%s.tmp.%d.%d", destPath, os.Getpid(), atomicExtractCounter.Add(1))

	f, err := os.OpenFile(tmpPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0755)
	if err != nil {
		return "", fmt.Errorf("embed_c5: create tmp file %s: %w", tmpPath, err)
	}

	if _, err := io.Copy(f, src); err != nil {
		f.Close()
		os.Remove(tmpPath)
		return "", fmt.Errorf("embed_c5: copy binary: %w", err)
	}
	if err := f.Close(); err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("embed_c5: close tmp file: %w", err)
	}

	// Atomic rename: replaces destPath even if another goroutine wrote it.
	if err := os.Rename(tmpPath, destPath); err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("embed_c5: rename to %s: %w", destPath, err)
	}

	// Atomically write the version file.
	if version != "" {
		tmpVer := fmt.Sprintf("%s.tmp.%d.%d", versionPath, os.Getpid(), atomicExtractCounter.Add(1))
		defer os.Remove(tmpVer) // no-op if rename succeeds
		if err := os.WriteFile(tmpVer, []byte(version), 0644); err == nil {
			_ = os.Rename(tmpVer, versionPath)
		}
	}

	return destPath, nil
}
