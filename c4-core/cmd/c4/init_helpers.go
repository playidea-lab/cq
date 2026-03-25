package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/changmin/c4-core/internal/cqdata"
)

// ensureGitRepo initialises a git repository in dir if none exists.
func ensureGitRepo(dir string) error {
	if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
		return nil // already a git repo
	}
	cmd := exec.Command("git", "init", "-b", "main", dir)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git init: %w", err)
	}
	fmt.Fprintln(os.Stderr, "✓ git 저장소 초기화")
	return nil
}

// ensureGitignoreEntry appends an entry to .gitignore if it's not already present.
// Non-fatal — silently returns on any error.
func ensureGitignoreEntry(dir, entry string) {
	gitignorePath := filepath.Join(dir, ".gitignore")
	data, _ := os.ReadFile(gitignorePath)
	content := string(data)

	// Check if entry already exists (exact line match)
	for _, line := range strings.Split(content, "\n") {
		if strings.TrimSpace(line) == entry {
			return // already present
		}
	}

	// Append entry
	suffix := "\n"
	if len(content) > 0 && !strings.HasSuffix(content, "\n") {
		suffix = "\n\n"
	}
	newContent := content + suffix + "# CQ local state\n" + entry + "\n"
	if err := os.WriteFile(gitignorePath, []byte(newContent), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "cq: warning: .gitignore update failed: %v\n", err)
		return
	}
	fmt.Fprintln(os.Stderr, "✓ .gitignore에 .c4/ 추가")
}

// ensureCQData creates an empty .cqdata template in dir if one does not exist.
func ensureCQData(dir string) error {
	path := filepath.Join(dir, ".cqdata")
	if _, err := os.Stat(path); err == nil {
		return nil // already exists
	}
	cd := &cqdata.CQData{}
	if err := cd.Save(dir); err != nil {
		return fmt.Errorf("creating .cqdata: %w", err)
	}
	fmt.Fprintln(os.Stderr, "✓ .cqdata 생성")
	return nil
}
