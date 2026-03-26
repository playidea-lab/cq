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

// cqGitignoreEntries are files/dirs CQ creates that should not be committed.
// CLAUDE.md and .mcp.json are intentionally NOT here — they should be committed
// (CLAUDE.md = project AI instructions, .mcp.json = team-shared MCP config, no secrets).
var cqGitignoreEntries = []string{
	".c4/",
	".cqdata",
	".piki-lock.yaml",
}

// ensureGitignore adds CQ entries to .gitignore if not already present.
// Non-fatal — silently returns on any error.
func ensureGitignore(dir string) {
	gitignorePath := filepath.Join(dir, ".gitignore")
	data, _ := os.ReadFile(gitignorePath)
	content := string(data)

	existingLines := make(map[string]bool)
	for _, line := range strings.Split(content, "\n") {
		existingLines[strings.TrimSpace(line)] = true
	}

	var toAdd []string
	for _, entry := range cqGitignoreEntries {
		if !existingLines[entry] {
			toAdd = append(toAdd, entry)
		}
	}

	if len(toAdd) == 0 {
		return
	}

	suffix := "\n"
	if len(content) > 0 && !strings.HasSuffix(content, "\n") {
		suffix = "\n\n"
	}
	block := suffix + "# CQ\n"
	for _, entry := range toAdd {
		block += entry + "\n"
	}
	if err := os.WriteFile(gitignorePath, []byte(content+block), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "cq: warning: .gitignore update failed: %v\n", err)
	}
}

// postMergeHookContent is the content written to .git/hooks/post-merge.
// Runs cq dataset sync in the background after every git pull so the working
// tree stays in sync with .cqdata without blocking the pull operation.
const postMergeHookContent = `#!/bin/sh
# CQ: auto-sync datasets after git pull
# Runs cq dataset sync to pull changed datasets from .cqdata
if command -v cq >/dev/null 2>&1 && [ -f .cqdata ]; then
    cq dataset sync 2>/dev/null &
fi
`

// installPostMergeHook installs or appends the CQ post-merge hook to gitDir.
// gitDir is the .git directory of the project (e.g. /path/to/project/.git).
//
// Idempotent: if the hook already contains "cq dataset sync" it is skipped.
// If the hook file does not exist it is created with mode 0755.
// If the hook file exists but does not contain the CQ fragment, the fragment
// is appended after a blank-line separator.
func installPostMergeHook(gitDir string) error {
	hookPath := filepath.Join(gitDir, "hooks", "post-merge")

	data, err := os.ReadFile(hookPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("reading post-merge hook: %w", err)
	}

	existing := string(data)

	// Already installed — nothing to do.
	if strings.Contains(existing, "cq dataset sync") {
		return nil
	}

	if existing == "" {
		// Create new hook file.
		if err := os.WriteFile(hookPath, []byte(postMergeHookContent), 0755); err != nil {
			return fmt.Errorf("writing post-merge hook: %w", err)
		}
		fmt.Fprintln(os.Stderr, "cq: post-merge hook installed → "+hookPath)
		return nil
	}

	// Append to existing hook.
	if !strings.HasSuffix(existing, "\n") {
		existing += "\n"
	}
	combined := existing + "\n" + postMergeHookContent
	if err := os.WriteFile(hookPath, []byte(combined), 0755); err != nil {
		return fmt.Errorf("appending to post-merge hook: %w", err)
	}
	fmt.Fprintln(os.Stderr, "cq: post-merge hook updated → "+hookPath)
	return nil
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
