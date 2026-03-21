package experiment

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

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
