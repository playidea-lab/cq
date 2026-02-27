package fileops

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- matchGlob ---

func TestMatchGlob_SimpleWildcard(t *testing.T) {
	tests := []struct {
		pattern, name, relPath string
		want                   bool
	}{
		{"*.go", "main.go", "main.go", true},
		{"*.go", "main.py", "main.py", false},
		{"*.go", "main.go", "src/main.go", true},
		{"*.py", "test.py", "tests/unit/test.py", true},
	}
	for _, tt := range tests {
		if got := matchGlob(tt.pattern, tt.name, tt.relPath); got != tt.want {
			t.Errorf("matchGlob(%q, %q, %q) = %v, want %v", tt.pattern, tt.name, tt.relPath, got, tt.want)
		}
	}
}

func TestMatchGlob_DoubleStarPrefix(t *testing.T) {
	tests := []struct {
		pattern, name, relPath string
		want                   bool
	}{
		{"**/*.go", "main.go", "main.go", true},
		{"**/*.go", "main.go", "src/main.go", true},
		{"**/*.go", "main.go", "a/b/c/main.go", true},
		{"**/*.py", "main.go", "src/main.go", false},
	}
	for _, tt := range tests {
		if got := matchGlob(tt.pattern, tt.name, tt.relPath); got != tt.want {
			t.Errorf("matchGlob(%q, %q, %q) = %v, want %v", tt.pattern, tt.name, tt.relPath, got, tt.want)
		}
	}
}

func TestMatchGlob_ExactName(t *testing.T) {
	if !matchGlob("Makefile", "Makefile", "Makefile") {
		t.Error("exact name should match")
	}
	if matchGlob("Makefile", "README.md", "README.md") {
		t.Error("different name should not match")
	}
}

func TestMatchGlob_RelPathMatch(t *testing.T) {
	// Pattern matches the relative path, not just the filename
	if !matchGlob("src/*.go", "main.go", "src/main.go") {
		t.Error("relPath match should work")
	}
}

// --- resolvePath ---

func TestResolvePath_Relative(t *testing.T) {
	root := "/project"
	got, err := resolvePath(root, "src/main.go")
	if err != nil {
		t.Fatal(err)
	}
	if got != "/project/src/main.go" {
		t.Errorf("got %q, want /project/src/main.go", got)
	}
}

func TestResolvePath_Absolute(t *testing.T) {
	// Valid absolute path within rootDir
	got, err := resolvePath("/project", "/project/src/main.go")
	if err != nil {
		t.Fatalf("unexpected error for valid absolute path: %v", err)
	}
	if got != "/project/src/main.go" {
		t.Errorf("got %q, want /project/src/main.go", got)
	}
}

func TestResolvePath_AbsoluteEscape(t *testing.T) {
	// Invalid absolute path escaping rootDir
	_, err := resolvePath("/project", "/tmp/file.txt")
	if err == nil {
		t.Error("expected error for absolute path escaping project root")
	}
	if err != nil && !strings.Contains(err.Error(), "absolute path escapes project root") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestResolvePath_Traversal(t *testing.T) {
	_, err := resolvePath("/project", "../../../etc/passwd")
	if err == nil {
		t.Error("expected error for traversal attempt")
	}
}

// --- handleFindFile ---

func TestHandleFindFile_Basic(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main"), 0644)
	os.WriteFile(filepath.Join(dir, "test.py"), []byte("pass"), 0644)
	os.MkdirAll(filepath.Join(dir, "sub"), 0755)
	os.WriteFile(filepath.Join(dir, "sub", "util.go"), []byte("package sub"), 0644)

	args, _ := json.Marshal(findFileArgs{Pattern: "*.go"})
	result, err := handleFindFile(dir, args)
	if err != nil {
		t.Fatal(err)
	}
	m := result.(map[string]any)
	count := m["count"].(int)
	if count != 2 {
		t.Errorf("count = %d, want 2", count)
	}
}

func TestHandleFindFile_DoubleStarPattern(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "a", "b"), 0755)
	os.WriteFile(filepath.Join(dir, "a", "b", "deep.go"), []byte("package deep"), 0644)

	args, _ := json.Marshal(findFileArgs{Pattern: "**/*.go"})
	result, err := handleFindFile(dir, args)
	if err != nil {
		t.Fatal(err)
	}
	m := result.(map[string]any)
	if m["count"].(int) != 1 {
		t.Errorf("count = %d, want 1", m["count"])
	}
}

func TestHandleFindFile_NoMatches(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "readme.md"), []byte("# hi"), 0644)

	args, _ := json.Marshal(findFileArgs{Pattern: "*.go"})
	result, err := handleFindFile(dir, args)
	if err != nil {
		t.Fatal(err)
	}
	m := result.(map[string]any)
	if m["count"].(int) != 0 {
		t.Errorf("count = %d, want 0", m["count"])
	}
}

func TestHandleFindFile_SkipsDotGit(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".git"), 0755)
	os.WriteFile(filepath.Join(dir, ".git", "config.go"), []byte("x"), 0644)
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main"), 0644)

	args, _ := json.Marshal(findFileArgs{Pattern: "*.go"})
	result, err := handleFindFile(dir, args)
	if err != nil {
		t.Fatal(err)
	}
	m := result.(map[string]any)
	if m["count"].(int) != 1 {
		t.Errorf("count = %d, want 1 (.git should be skipped)", m["count"])
	}
}

// --- handleSearchForPattern ---

func TestHandleSearchForPattern_Basic(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc hello() {}\n"), 0644)
	os.WriteFile(filepath.Join(dir, "test.go"), []byte("package main\nfunc TestHello() {}\n"), 0644)

	args, _ := json.Marshal(searchPatternArgs{Pattern: "func.*Hello"})
	result, err := handleSearchForPattern(dir, args)
	if err != nil {
		t.Fatal(err)
	}
	m := result.(map[string]any)
	if m["count"].(int) != 1 {
		t.Errorf("count = %d, want 1", m["count"])
	}
}

func TestHandleSearchForPattern_MaxResults(t *testing.T) {
	dir := t.TempDir()
	content := "match\nmatch\nmatch\nmatch\nmatch\n"
	os.WriteFile(filepath.Join(dir, "file.txt"), []byte(content), 0644)

	args, _ := json.Marshal(searchPatternArgs{Pattern: "match", MaxResults: 3})
	result, err := handleSearchForPattern(dir, args)
	if err != nil {
		t.Fatal(err)
	}
	m := result.(map[string]any)
	if m["count"].(int) > 3 {
		t.Errorf("count = %d, should be <= 3", m["count"])
	}
}

func TestHandleSearchForPattern_InvalidRegex(t *testing.T) {
	dir := t.TempDir()
	args, _ := json.Marshal(searchPatternArgs{Pattern: "[invalid"})
	_, err := handleSearchForPattern(dir, args)
	if err == nil {
		t.Error("expected error for invalid regex")
	}
}

func TestHandleSearchForPattern_FileFilter(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("hello world"), 0644)
	os.WriteFile(filepath.Join(dir, "main.py"), []byte("hello world"), 0644)

	args, _ := json.Marshal(searchPatternArgs{Pattern: "hello", FilePattern: "*.go"})
	result, err := handleSearchForPattern(dir, args)
	if err != nil {
		t.Fatal(err)
	}
	m := result.(map[string]any)
	if m["count"].(int) != 1 {
		t.Errorf("count = %d, want 1 (should only match .go)", m["count"])
	}
}

// --- handleReadFile ---

func TestHandleReadFile_Full(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "file.txt"), []byte("line1\nline2\nline3\n"), 0644)

	args, _ := json.Marshal(readFileArgs{Path: "file.txt"})
	result, err := handleReadFile(dir, args)
	if err != nil {
		t.Fatal(err)
	}
	m := result.(map[string]any)
	if m["total_lines"].(int) != 4 { // trailing newline creates empty 4th element
		t.Errorf("total_lines = %d", m["total_lines"])
	}
}

func TestHandleReadFile_LineRange(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "file.txt"), []byte("a\nb\nc\nd\ne\n"), 0644)

	args, _ := json.Marshal(readFileArgs{Path: "file.txt", StartLine: 2, EndLine: 4})
	result, err := handleReadFile(dir, args)
	if err != nil {
		t.Fatal(err)
	}
	m := result.(map[string]any)
	if m["content"].(string) != "b\nc\nd" {
		t.Errorf("content = %q", m["content"])
	}
}

func TestHandleReadFile_MissingFile(t *testing.T) {
	dir := t.TempDir()
	args, _ := json.Marshal(readFileArgs{Path: "nonexistent.txt"})
	_, err := handleReadFile(dir, args)
	if err == nil {
		t.Error("expected error for missing file")
	}
}

// --- handleListDir ---

func TestHandleListDir_Basic(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "file.txt"), []byte("x"), 0644)
	os.MkdirAll(filepath.Join(dir, "subdir"), 0755)

	args, _ := json.Marshal(listDirArgs{})
	result, err := handleListDir(dir, args)
	if err != nil {
		t.Fatal(err)
	}
	m := result.(map[string]any)
	count := m["count"].(int)
	if count != 2 {
		t.Errorf("count = %d, want 2", count)
	}
}

func TestHandleListDir_Subdir(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "sub"), 0755)
	os.WriteFile(filepath.Join(dir, "sub", "a.txt"), []byte("a"), 0644)
	os.WriteFile(filepath.Join(dir, "sub", "b.txt"), []byte("b"), 0644)

	args, _ := json.Marshal(listDirArgs{Path: "sub"})
	result, err := handleListDir(dir, args)
	if err != nil {
		t.Fatal(err)
	}
	m := result.(map[string]any)
	if m["count"].(int) != 2 {
		t.Errorf("count = %d, want 2", m["count"])
	}
}

func TestHandleListDir_NonexistentDir(t *testing.T) {
	dir := t.TempDir()
	args, _ := json.Marshal(listDirArgs{Path: "nope"})
	_, err := handleListDir(dir, args)
	if err == nil {
		t.Error("expected error for nonexistent dir")
	}
}

// --- handleReplaceContent ---

func TestHandleReplaceContent_Success(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "file.txt"), []byte("hello world"), 0644)

	args, _ := json.Marshal(replaceContentArgs{Path: "file.txt", OldText: "hello", NewText: "hi"})
	result, err := handleReplaceContent(dir, args)
	if err != nil {
		t.Fatal(err)
	}
	m := result.(map[string]any)
	if !m["success"].(bool) {
		t.Error("expected success")
	}

	data, _ := os.ReadFile(filepath.Join(dir, "file.txt"))
	if string(data) != "hi world" {
		t.Errorf("file content = %q", string(data))
	}
}

func TestHandleReplaceContent_NotFound(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "file.txt"), []byte("hello world"), 0644)

	args, _ := json.Marshal(replaceContentArgs{Path: "file.txt", OldText: "xyz", NewText: "abc"})
	result, err := handleReplaceContent(dir, args)
	if err != nil {
		t.Fatal(err)
	}
	m := result.(map[string]any)
	if m["success"].(bool) {
		t.Error("expected success=false for not found")
	}
}

// --- handleCreateTextFile ---

func TestHandleCreateTextFile_Basic(t *testing.T) {
	dir := t.TempDir()
	args, _ := json.Marshal(createTextFileArgs{Path: "new.txt", Content: "hello"})
	result, err := handleCreateTextFile(dir, args)
	if err != nil {
		t.Fatal(err)
	}
	m := result.(map[string]any)
	if !m["success"].(bool) {
		t.Error("expected success")
	}
	data, _ := os.ReadFile(filepath.Join(dir, "new.txt"))
	if string(data) != "hello" {
		t.Errorf("content = %q", string(data))
	}
}

func TestHandleCreateTextFile_NestedDir(t *testing.T) {
	dir := t.TempDir()
	args, _ := json.Marshal(createTextFileArgs{Path: "a/b/c.txt", Content: "deep"})
	_, err := handleCreateTextFile(dir, args)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "a", "b", "c.txt"))
	if string(data) != "deep" {
		t.Errorf("content = %q", string(data))
	}
}

// --- handleReplaceContent: replace_all ---

func TestReplaceContent_ReplaceAll(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "file.txt"), []byte("foo bar foo baz foo"), 0644)

	args, _ := json.Marshal(replaceContentArgs{Path: "file.txt", OldText: "foo", NewText: "qux", ReplaceAll: true})
	result, err := handleReplaceContent(dir, args)
	if err != nil {
		t.Fatal(err)
	}
	m := result.(map[string]any)
	if !m["success"].(bool) {
		t.Error("expected success")
	}
	if m["replacements"].(int) != 3 {
		t.Errorf("replacements = %d, want 3", m["replacements"])
	}
	data, _ := os.ReadFile(filepath.Join(dir, "file.txt"))
	if string(data) != "qux bar qux baz qux" {
		t.Errorf("content = %q", string(data))
	}
}

func TestReplaceContent_FirstOnly(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "file.txt"), []byte("foo bar foo baz foo"), 0644)

	args, _ := json.Marshal(replaceContentArgs{Path: "file.txt", OldText: "foo", NewText: "qux", ReplaceAll: false})
	result, err := handleReplaceContent(dir, args)
	if err != nil {
		t.Fatal(err)
	}
	m := result.(map[string]any)
	if m["replacements"].(int) != 1 {
		t.Errorf("replacements = %d, want 1", m["replacements"])
	}
	data, _ := os.ReadFile(filepath.Join(dir, "file.txt"))
	if string(data) != "qux bar foo baz foo" {
		t.Errorf("content = %q", string(data))
	}
}

func TestReplaceContent_DefaultFirstOnly(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "file.txt"), []byte("foo bar foo baz foo"), 0644)

	// replace_all not specified — should default to first-only
	args, _ := json.Marshal(map[string]string{"path": "file.txt", "old_text": "foo", "new_text": "qux"})
	result, err := handleReplaceContent(dir, args)
	if err != nil {
		t.Fatal(err)
	}
	m := result.(map[string]any)
	if m["replacements"].(int) != 1 {
		t.Errorf("replacements = %d, want 1", m["replacements"])
	}
	data, _ := os.ReadFile(filepath.Join(dir, "file.txt"))
	if string(data) != "qux bar foo baz foo" {
		t.Errorf("content = %q", string(data))
	}
}
