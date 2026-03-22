package handlers

import (
	"fmt"
	"os/exec"
	"strconv"
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

// diffStatLines returns the total number of lines changed (insertions + deletions)
// for the given commit SHA. Returns 0 when rootDir is empty or commit can't be resolved.
func diffStatLines(rootDir, commitSHA string) int {
	if rootDir == "" || commitSHA == "" {
		return 0
	}
	// git show --stat --format='' outputs only the diff stat lines.
	// The last line is "N files changed, X insertions(+), Y deletions(-)"
	out, err := runGit(rootDir, "show", "--stat", "--format=", commitSHA)
	if err != nil {
		return 0
	}
	lines := strings.Split(out, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if strings.Contains(line, "insertion") || strings.Contains(line, "deletion") {
			total := 0
			// parse "X insertions(+)" and "Y deletions(-)"
			for _, part := range strings.Split(line, ",") {
				part = strings.TrimSpace(part)
				fields := strings.Fields(part)
				if len(fields) >= 2 {
					n, err := strconv.Atoi(fields[0])
					if err == nil && (strings.HasPrefix(fields[1], "insertion") || strings.HasPrefix(fields[1], "deletion")) {
						total += n
					}
				}
			}
			return total
		}
	}
	return 0
}
