package experiment

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGitCommitSHA(t *testing.T) {
	sha, err := GitCommitSHA()
	if err != nil {
		// If not in a git repo (e.g. CI without git), skip rather than fail.
		t.Skipf("GitCommitSHA skipped (not in git repo): %v", err)
	}
	if len(sha) != 40 {
		t.Errorf("expected 40-char SHA, got %q (len=%d)", sha, len(sha))
	}
	for _, c := range sha {
		if !strings.ContainsRune("0123456789abcdef", c) {
			t.Errorf("SHA contains non-hex character %q", c)
			break
		}
	}
}

func TestConfigHash(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "config-*.yaml")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	content := []byte("lr: 0.01\nbatch_size: 32\n")
	if _, err := f.Write(content); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	f.Close()

	hash, err := ConfigHash(f.Name())
	if err != nil {
		t.Fatalf("ConfigHash: %v", err)
	}
	if len(hash) != 64 {
		t.Errorf("expected 64-char SHA256 hex, got %q (len=%d)", hash, len(hash))
	}

	// Same content → same hash (deterministic).
	hash2, err := ConfigHash(f.Name())
	if err != nil {
		t.Fatalf("ConfigHash second call: %v", err)
	}
	if hash != hash2 {
		t.Errorf("hash not deterministic: %q != %q", hash, hash2)
	}
}

func TestConfigHash_MissingFile(t *testing.T) {
	_, err := ConfigHash("/nonexistent/path/config.yaml")
	if err == nil {
		t.Error("expected error for missing file, got nil")
	}
}

func TestGitIsDirty_CleanRepo(t *testing.T) {
	dir := t.TempDir()
	// Init a git repo, add and commit a file so the tree is clean.
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if err := cmd.Run(); err != nil {
			t.Skipf("git command failed (%v); skipping dirty-check test", err)
		}
	}
	run("init")
	run("config", "user.email", "test@test.com")
	run("config", "user.name", "Test")
	f := filepath.Join(dir, "README")
	if err := os.WriteFile(f, []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "README")
	run("commit", "-m", "init")

	dirty, err := GitIsDirty(dir)
	if err != nil {
		t.Fatalf("GitIsDirty clean repo: %v", err)
	}
	if dirty {
		t.Error("expected clean repo to report not dirty")
	}
}

func TestGitIsDirty_DirtyRepo(t *testing.T) {
	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if err := cmd.Run(); err != nil {
			t.Skipf("git command failed (%v); skipping dirty-check test", err)
		}
	}
	run("init")
	run("config", "user.email", "test@test.com")
	run("config", "user.name", "Test")
	// Stage a file without committing → dirty.
	f := filepath.Join(dir, "dirty.txt")
	if err := os.WriteFile(f, []byte("untracked"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "dirty.txt")

	dirty, err := GitIsDirty(dir)
	if err != nil {
		t.Fatalf("GitIsDirty dirty repo: %v", err)
	}
	if !dirty {
		t.Error("expected dirty repo to report dirty")
	}
}

func TestGitIsDirty_NotARepo(t *testing.T) {
	dir := t.TempDir() // plain dir, not a git repo
	_, err := GitIsDirty(dir)
	if err == nil {
		t.Error("expected error for non-git directory, got nil")
	}
}

func TestWarnIfDirty_NoPanic(t *testing.T) {
	// WarnIfDirty must not panic even for a non-repo dir.
	WarnIfDirty(t.TempDir())
}
