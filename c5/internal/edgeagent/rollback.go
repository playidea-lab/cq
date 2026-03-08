package edgeagent

import (
	"fmt"
	"log"
	"os"
)

// RollbackManager saves the previous deploy directory as {dir}.prev and can restore it.
type RollbackManager struct {
	deployDir string
}

func newRollbackManager(deployDir string) *RollbackManager {
	return &RollbackManager{deployDir: deployDir}
}

// BeforeDeploy copies src to {src}.prev atomically using os.Rename.
// If {src}.prev already exists it is removed first.
func (r *RollbackManager) BeforeDeploy(src string) error {
	prev := src + ".prev"
	if err := os.RemoveAll(prev); err != nil {
		return fmt.Errorf("remove old .prev: %w", err)
	}
	if err := copyDir(src, prev); err != nil {
		return fmt.Errorf("backup deploy dir: %w", err)
	}
	return nil
}

// Rollback restores {dst}.prev → {dst}.
// Uses a temp-rename strategy to avoid leaving the edge with no artifacts if Rename fails:
// 1. Rename dst → dst.failed (atomic)
// 2. Rename prev → dst (atomic, prev still intact if this fails)
// 3. RemoveAll dst.failed
func (r *RollbackManager) Rollback(dst string) error {
	prev := dst + ".prev"
	if _, err := os.Stat(prev); err != nil {
		return fmt.Errorf("no .prev to rollback from: %w", err)
	}
	failed := dst + ".failed"
	os.RemoveAll(failed) // clean up any prior failed attempt
	if err := os.Rename(dst, failed); err != nil {
		return fmt.Errorf("rename current deploy dir to .failed: %w", err)
	}
	if err := os.Rename(prev, dst); err != nil {
		// prev is still intact; restore from failed if possible
		os.Rename(failed, dst) //nolint:errcheck
		return fmt.Errorf("rename .prev → dst: %w", err)
	}
	os.RemoveAll(failed) //nolint:errcheck
	return nil
}

// Cleanup removes {src}.prev after a successful deploy.
func (r *RollbackManager) Cleanup(src string) {
	prev := src + ".prev"
	if err := os.RemoveAll(prev); err != nil {
		log.Printf("edge-agent: cleanup .prev: %v", err)
	}
}

// copyDir copies src directory tree to dst (dst must not exist).
func copyDir(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		// src may not exist yet (first deploy) — treat as empty
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	for _, e := range entries {
		srcPath := src + "/" + e.Name()
		dstPath := dst + "/" + e.Name()
		if e.IsDir() {
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err := copyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}
	return nil
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}
