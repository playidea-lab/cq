package experiment

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// GitIsDirty reports whether the git working tree at dir contains uncommitted changes.
// It runs `git -C dir status --porcelain`; non-empty output means dirty.
// Returns false (not dirty) when git is unavailable or dir is not a repo.
func GitIsDirty(dir string) (bool, error) {
	out, err := exec.Command("git", "-C", dir, "status", "--porcelain").Output()
	if err != nil {
		return false, fmt.Errorf("git status --porcelain: %w", err)
	}
	return strings.TrimSpace(string(out)) != "", nil
}

// WarnIfDirty prints a warning to stderr when the git working tree at dir is dirty.
// It does not block — the warning is informational only.
func WarnIfDirty(dir string) {
	dirty, err := GitIsDirty(dir)
	if err != nil {
		return
	}
	if dirty {
		fmt.Fprintln(os.Stderr, "⚠ git working tree is dirty. Consider committing before experiment.")
	}
}

// GitCommitSHA returns the current HEAD commit SHA by running `git rev-parse HEAD`.
// Returns an empty string and the error if git is unavailable or not in a repo.
func GitCommitSHA() (string, error) {
	out, err := exec.Command("git", "rev-parse", "HEAD").Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse HEAD: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// ConfigHash returns the SHA256 hex digest of the file at path.
// Returns an empty string and an error if the file cannot be read.
func ConfigHash(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read config %s: %w", path, err)
	}
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%x", sum), nil
}
