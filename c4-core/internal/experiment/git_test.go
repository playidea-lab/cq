package experiment

import (
	"os"
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
