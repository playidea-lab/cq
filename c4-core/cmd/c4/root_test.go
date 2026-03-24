package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindBestC4Root_StopsAtGitRoot(t *testing.T) {
	// Layout: /tmp/repo/.git/ + /tmp/repo/sub/
	// .c4/ only exists above repo (simulating home dir)
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	sub := filepath.Join(repo, "sub")
	os.MkdirAll(filepath.Join(repo, ".git"), 0755)
	os.MkdirAll(sub, 0755)

	// Put .c4/ with config.yaml at root (like home dir)
	os.MkdirAll(filepath.Join(root, ".c4"), 0755)
	os.WriteFile(filepath.Join(root, ".c4", "config.yaml"), []byte("cloud:"), 0644)

	// From sub/ → should NOT climb past .git/ to root
	got := findBestC4Root(sub)
	if got != sub {
		t.Errorf("findBestC4Root(%s) = %s, want %s (should stop at git root)", sub, got, sub)
	}
}

func TestFindBestC4Root_FindsC4InGitRoot(t *testing.T) {
	// Layout: /tmp/repo/.git/ + /tmp/repo/.c4/config.yaml + /tmp/repo/sub/
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	sub := filepath.Join(repo, "sub")
	os.MkdirAll(filepath.Join(repo, ".git"), 0755)
	os.MkdirAll(filepath.Join(repo, ".c4"), 0755)
	os.WriteFile(filepath.Join(repo, ".c4", "config.yaml"), []byte("cloud:"), 0644)
	os.MkdirAll(sub, 0755)

	// From sub/ → should find repo/.c4/ (within git boundary)
	got := findBestC4Root(sub)
	if got != repo {
		t.Errorf("findBestC4Root(%s) = %s, want %s", sub, got, repo)
	}
}

func TestFindBestC4Root_NeverReturnsHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home dir")
	}

	// Create a temp dir inside home to simulate a project without .c4/
	dir := t.TempDir()
	// If TempDir is not under home, simulate by using a subdir of home
	if !isUnder(dir, home) {
		t.Skip("TempDir not under home, cannot test home exclusion")
	}

	got := findBestC4Root(dir)
	if got == home {
		t.Errorf("findBestC4Root(%s) = %s (home dir), should never return home", dir, got)
	}
}

func TestFindBestC4Root_CWDWithC4(t *testing.T) {
	// CWD already has .c4/config.yaml → return CWD directly
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".c4"), 0755)
	os.WriteFile(filepath.Join(dir, ".c4", "config.yaml"), []byte("cloud:"), 0644)

	got := findBestC4Root(dir)
	if got != dir {
		t.Errorf("findBestC4Root(%s) = %s, want %s", dir, got, dir)
	}
}

func TestFindBestC4Root_GitWorktree(t *testing.T) {
	// .git can be a file (worktree pointer) — should still count as boundary
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	os.MkdirAll(repo, 0755)
	// .git as a file (worktree)
	os.WriteFile(filepath.Join(repo, ".git"), []byte("gitdir: /some/path"), 0644)

	sub := filepath.Join(repo, "sub")
	os.MkdirAll(sub, 0755)

	// .c4/ above repo
	os.MkdirAll(filepath.Join(root, ".c4"), 0755)
	os.WriteFile(filepath.Join(root, ".c4", "config.yaml"), []byte("cloud:"), 0644)

	got := findBestC4Root(sub)
	if got != sub {
		t.Errorf("findBestC4Root(%s) = %s, want %s (git worktree file should be boundary)", sub, got, sub)
	}
}

func isUnder(child, parent string) bool {
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	return rel != ".." && !filepath.IsAbs(rel)
}
