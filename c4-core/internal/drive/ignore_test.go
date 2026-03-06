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
		if e.RelPath == "__pycache__/cached.pyc" || e.RelPath == "__pycache__" {
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

func TestWalkDir_NegationPattern(t *testing.T) {
	root := t.TempDir()

	// .gitignore ignores *.txt but negates !kept.txt
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("*.txt\n!kept.txt\n"), 0644); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"excluded.txt", "also-excluded.txt", "kept.txt", "main.go"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(""), 0644); err != nil {
			t.Fatal(err)
		}
	}

	entries, err := WalkDir(root, "")
	if err != nil {
		t.Fatalf("WalkDir error: %v", err)
	}

	relPaths := map[string]bool{}
	for _, e := range entries {
		relPaths[e.RelPath] = true
	}

	// excluded.txt and also-excluded.txt should be gone.
	for _, excluded := range []string{"excluded.txt", "also-excluded.txt"} {
		if relPaths[excluded] {
			t.Errorf("expected %q to be excluded by *.txt pattern", excluded)
		}
	}
	// kept.txt should survive the negation pattern.
	if !relPaths["kept.txt"] {
		t.Errorf("expected kept.txt to be included due to !kept.txt negation, got entries: %v", entries)
	}
	// main.go should be present.
	if !relPaths["main.go"] {
		t.Errorf("expected main.go to be included")
	}
}

func TestWalkDir_AncestorGitignore(t *testing.T) {
	root := t.TempDir()

	// Root .gitignore excludes __pycache__ pattern.
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("__pycache__\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create deep nested structure: root/src/pkg/__pycache__/
	deepDir := filepath.Join(root, "src", "pkg", "__pycache__")
	if err := os.MkdirAll(deepDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(deepDir, "module.pyc"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "pkg", "module.py"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	entries, err := WalkDir(root, "")
	if err != nil {
		t.Fatalf("WalkDir error: %v", err)
	}

	for _, e := range entries {
		if e.RelPath == "src/pkg/__pycache__/module.pyc" || e.RelPath == "src/pkg/__pycache__" {
			t.Errorf("root .gitignore should exclude deep __pycache__, got %q", e.RelPath)
		}
	}

	// src/pkg/module.py should be present.
	found := false
	for _, e := range entries {
		if e.RelPath == "src/pkg/module.py" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected src/pkg/module.py in entries, got %v", entries)
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
