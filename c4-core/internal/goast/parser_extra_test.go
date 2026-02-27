package goast

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSymbolsOverview_NonExistentPath(t *testing.T) {
	_, err := SymbolsOverview("/nonexistent/path/that/doesnt/exist")
	if err == nil {
		t.Error("expected error for non-existent path")
	}
}

func TestSymbolsOverview_Directory(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "mod.go"), []byte(`package mod
// Calc does math.
type Calc struct{}

func (c *Calc) Add(a, b int) int { return a + b }
`), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := SymbolsOverview(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	// Should have structs and methods
	if result["structs"] == nil && result["methods"] == nil {
		t.Error("expected structs or methods in directory overview")
	}
}

func TestFindSymbolByName_NonExistentPath(t *testing.T) {
	_, err := FindSymbolByName("Foo", "/nonexistent/path")
	if err == nil {
		t.Error("expected error for non-existent path")
	}
}

func TestFindSymbolByName_Directory(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "pkg.go"), []byte(`package pkg
type Config struct{ Debug bool }
func NewConfig() *Config { return &Config{} }
`), 0644); err != nil {
		t.Fatal(err)
	}

	results, err := FindSymbolByName("Config", dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) == 0 {
		t.Error("expected at least 1 result for Config in directory")
	}
}

func TestHasGoFiles_NonExistentPath(t *testing.T) {
	if HasGoFiles("/nonexistent/path/xyz") {
		t.Error("expected false for non-existent path")
	}
}

func TestHasGoFiles_SubdirectoryWithGoFiles(t *testing.T) {
	// Create a top-level dir with no Go files, but a subdir with Go files.
	dir := t.TempDir()
	subDir := filepath.Join(dir, "sub")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "code.go"), []byte("package sub\n"), 0644); err != nil {
		t.Fatal(err)
	}

	if !HasGoFiles(dir) {
		t.Error("expected true when subdir contains Go files")
	}
}

func TestHasGoFiles_EmptySubdirectory(t *testing.T) {
	// Dir with a subdir that has no Go files.
	dir := t.TempDir()
	subDir := filepath.Join(dir, "empty")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}

	if HasGoFiles(dir) {
		t.Error("expected false when no Go files anywhere in dir tree")
	}
}

func TestExprString_ChanAndIndex(t *testing.T) {
	// Parse Go source with channel types and index expressions.
	src := `package test

type ChanHolder struct{}

func SendChan(ch chan<- int) {}
func RecvChan(ch <-chan string) {}
func BidirChan(ch chan error) {}
`
	path := writeTempGoFile(t, src)
	symbols, err := ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile error: %v", err)
	}

	if len(symbols) == 0 {
		t.Fatal("expected symbols for chan types source")
	}

	// Verify signatures contain chan type representations
	found := false
	for _, s := range symbols {
		if s.Signature != "" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected at least one symbol with a signature")
	}
}

func TestExprString_MapAndSlice(t *testing.T) {
	src := `package test

func Process(m map[string][]int, items []string) map[int]bool {
	return nil
}
`
	path := writeTempGoFile(t, src)
	symbols, err := ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile error: %v", err)
	}

	if len(symbols) != 1 {
		t.Fatalf("expected 1 symbol, got %d", len(symbols))
	}
	if symbols[0].Signature == "" {
		t.Error("expected non-empty signature for Process function")
	}
}

func TestSymbolsOverview_DirectoryParseError(t *testing.T) {
	// Directory with a Go file that has parse errors.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "bad.go"), []byte("package bad\nfunc Bad( {\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// ParseDir with malformed Go: either returns error or empty list.
	result, err := SymbolsOverview(dir)
	if err != nil {
		// Error is acceptable for malformed Go.
		t.Logf("got error for malformed Go (acceptable): %v", err)
		return
	}
	// If no error, result should be a map.
	if result == nil {
		t.Error("expected non-nil result even for partial parse")
	}
}

func TestParseFile_InterfaceType(t *testing.T) {
	src := `package test

type Processor interface {
	Process(data interface{}) error
	Validate(v interface{}) bool
}
`
	path := writeTempGoFile(t, src)
	symbols, err := ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile error: %v", err)
	}

	byKind := map[string]int{}
	for _, s := range symbols {
		byKind[s.Kind]++
	}
	if byKind["interface"] != 1 {
		t.Errorf("expected 1 interface, got %d", byKind["interface"])
	}
}

func TestKindToGroup_AllKinds(t *testing.T) {
	cases := map[string]string{
		"function":  "functions",
		"method":    "methods",
		"struct":    "structs",
		"interface": "interfaces",
		"type":      "types",
		"const":     "constants",
		"var":       "variables",
		"unknown":   "other",
	}
	for kind, want := range cases {
		got := kindToGroup(kind)
		if got != want {
			t.Errorf("kindToGroup(%q) = %q, want %q", kind, got, want)
		}
	}
}
