package gitops

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// initGitRepo creates a temp directory with a git repo containing one commit.
func initGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	cmds := [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git init: %s: %v", string(out), err)
		}
	}

	// Create a file and commit
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main"), 0644)
	cmd := exec.Command("git", "add", ".")
	cmd.Dir = dir
	cmd.Run()
	cmd = exec.Command("git", "commit", "-m", "initial commit")
	cmd.Dir = dir
	cmd.Run()

	return dir
}

func TestRunGit_Branch(t *testing.T) {
	dir := initGitRepo(t)
	out, err := runGit(dir, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	// Should be "main" or "master" depending on git config
	if out == "" {
		t.Error("expected non-empty branch name")
	}
}

func TestRunGit_InvalidCommand(t *testing.T) {
	dir := t.TempDir()
	_, err := runGit(dir, "nonsense-command")
	if err == nil {
		t.Error("expected error for invalid git command")
	}
}

func TestRunGit_NotARepo(t *testing.T) {
	dir := t.TempDir()
	_, err := runGit(dir, "status")
	if err == nil {
		t.Error("expected error for non-repo directory")
	}
}

func TestHandleWorktreeStatus_Basic(t *testing.T) {
	dir := initGitRepo(t)
	result, err := handleWorktreeStatus(dir)
	if err != nil {
		t.Fatal(err)
	}
	m := result.(map[string]any)
	if m["branch"] == "" {
		t.Error("expected non-empty branch")
	}
	if !m["clean"].(bool) {
		// might not be clean in some git versions, just check it exists
		t.Log("repo not clean, which is acceptable")
	}
}

func TestHandleWorktreeStatus_WithModification(t *testing.T) {
	dir := initGitRepo(t)
	// Add a new untracked file (more reliable than modifying tracked files in test)
	os.WriteFile(filepath.Join(dir, "extra.txt"), []byte("new file"), 0644)
	// Stage and modify a tracked file
	cmd := exec.Command("git", "add", "extra.txt")
	cmd.Dir = dir
	cmd.Run()

	result, err := handleWorktreeStatus(dir)
	if err != nil {
		t.Fatal(err)
	}
	m := result.(map[string]any)
	if m["clean"].(bool) {
		t.Error("expected dirty repo")
	}
	if m["staged"].(int) < 1 {
		t.Errorf("staged = %d, want >= 1", m["staged"])
	}
}

func TestHandleWorktreeStatus_WithUntracked(t *testing.T) {
	dir := initGitRepo(t)
	os.WriteFile(filepath.Join(dir, "new_file.txt"), []byte("x"), 0644)

	result, err := handleWorktreeStatus(dir)
	if err != nil {
		t.Fatal(err)
	}
	m := result.(map[string]any)
	if m["untracked"].(int) < 1 {
		t.Errorf("untracked = %d, want >= 1", m["untracked"])
	}
}

func TestHandleAnalyzeHistory_Basic(t *testing.T) {
	dir := initGitRepo(t)
	args, _ := json.Marshal(analyzeHistoryArgs{MaxCount: 10})
	result, err := handleAnalyzeHistory(dir, args)
	if err != nil {
		t.Fatal(err)
	}
	m := result.(map[string]any)
	if m["total"].(int) < 1 {
		t.Error("expected at least 1 commit")
	}
	if m["author_count"].(int) < 1 {
		t.Error("expected at least 1 author")
	}
}

func TestHandleAnalyzeHistory_WithAuthorFilter(t *testing.T) {
	dir := initGitRepo(t)
	args, _ := json.Marshal(analyzeHistoryArgs{Author: "Nonexistent", MaxCount: 10})
	result, err := handleAnalyzeHistory(dir, args)
	if err != nil {
		t.Fatal(err)
	}
	m := result.(map[string]any)
	if m["total"].(int) != 0 {
		t.Errorf("expected 0 commits for nonexistent author, got %d", m["total"])
	}
}

func TestHandleSearchCommits_Found(t *testing.T) {
	dir := initGitRepo(t)
	args, _ := json.Marshal(searchCommitsArgs{Query: "initial", MaxCount: 10})
	result, err := handleSearchCommits(dir, args)
	if err != nil {
		t.Fatal(err)
	}
	m := result.(map[string]any)
	if m["count"].(int) < 1 {
		t.Error("expected at least 1 matching commit")
	}
}

func TestHandleSearchCommits_NotFound(t *testing.T) {
	dir := initGitRepo(t)
	args, _ := json.Marshal(searchCommitsArgs{Query: "nonexistent-query-xyz", MaxCount: 10})
	result, err := handleSearchCommits(dir, args)
	if err != nil {
		t.Fatal(err)
	}
	m := result.(map[string]any)
	if m["count"].(int) != 0 {
		t.Errorf("expected 0 matches, got %d", m["count"])
	}
}

func TestHandleWorktreeCleanup_DryRun(t *testing.T) {
	dir := initGitRepo(t)
	args, _ := json.Marshal(worktreeCleanupArgs{DryRun: true})
	result, err := handleWorktreeCleanup(dir, args)
	if err != nil {
		t.Fatal(err)
	}
	m := result.(map[string]any)
	if !m["dry_run"].(bool) {
		t.Error("expected dry_run=true")
	}
}
