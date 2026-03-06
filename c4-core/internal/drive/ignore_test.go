package drive

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWalkDir_GitignoreApplied(t *testing.T) {
	root := t.TempDir()

	// Create .gitignore with __pycache__ pattern.
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("__pycache__\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create a normal file and a __pycache__ directory with a file inside.
	if err := os.WriteFile(filepath.Join(root, "main.py"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "__pycache__"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "__pycache__", "cached.pyc"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	entries, err := WalkDir(root, "")
	if err != nil {
		t.Fatalf("WalkDir error: %v", err)
	}

	for _, e := range entries {
		if e.RelPath == filepath.Join("__pycache__", "cached.pyc") || e.RelPath == "__pycache__" {
			t.Errorf("expected __pycache__ to be excluded, got %q", e.RelPath)
		}
	}
	// Verify main.py is present.
	found := false
	for _, e := range entries {
		if e.RelPath == "main.py" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected main.py in entries, got %v", entries)
	}
}

func TestWalkDir_NoGitignore(t *testing.T) {
	root := t.TempDir()

	// No .gitignore — all files should be included.
	for _, name := range []string{"a.txt", "b.txt"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("data"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	entries, err := WalkDir(root, "")
	if err != nil {
		t.Fatalf("WalkDir error: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("expected 2 entries, got %d", len(entries))
	}
}

func TestWalkDir_ExtraIgnore(t *testing.T) {
	root := t.TempDir()

	// Create a .cqdriveignore that excludes *.log files.
	ignoreFile := filepath.Join(root, ".cqdriveignore")
	if err := os.WriteFile(ignoreFile, []byte("*.log\n"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(root, "app.log"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	entries, err := WalkDir(root, ignoreFile)
	if err != nil {
		t.Fatalf("WalkDir error: %v", err)
	}

	for _, e := range entries {
		if e.RelPath == "app.log" {
			t.Errorf("expected app.log to be excluded")
		}
	}
	// .cqdriveignore and main.go are both present; .cqdriveignore is not excluded by itself.
	found := false
	for _, e := range entries {
		if e.RelPath == "main.go" {
			found = true
		}
	}
	if !found {
		t.Error("expected main.go to be included")
	}
}

func TestWalkDir_EmptyDir(t *testing.T) {
	root := t.TempDir()

	entries, err := WalkDir(root, "")
	if err != nil {
		t.Fatalf("WalkDir error: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries for empty dir, got %d", len(entries))
	}
}
