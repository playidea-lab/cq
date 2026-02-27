package handlers

import (
	"fmt"
	"os/exec"
	"strings"
)

// runGit executes a git command in rootDir and returns stdout.
func runGit(rootDir string, gitArgs ...string) (string, error) {
	cmd := exec.Command("git", gitArgs...)
	cmd.Dir = rootDir
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("git %s: %s", gitArgs[0], string(exitErr.Stderr))
		}
		return "", fmt.Errorf("git %s: %w", gitArgs[0], err)
	}
	return strings.TrimSpace(string(out)), nil
}
