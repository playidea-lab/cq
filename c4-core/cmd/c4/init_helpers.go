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
