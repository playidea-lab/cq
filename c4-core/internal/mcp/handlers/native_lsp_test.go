package handlers

import (
	"os"
	"path/filepath"
	"testing"
)

// =========================================================================
// Go AST handler tests
// =========================================================================

func writeGoFixture(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	src := `package fixture

// Greeter provides greeting functionality.
type Greeter struct {
	Name string
}

// Hello returns a greeting message.
func (g *Greeter) Hello() string {
	return "Hello, " + g.Name
}

// Version is the module version.
const Version = "1.0.0"

// Add returns a + b.
func Add(a, b int) int {
	return a + b
}
`
	if err := os.WriteFile(filepath.Join(dir, "fixture.go"), []byte(src), 0644); err != nil {
		t.Fatalf("write fixture.go: %v", err)
	}
}

func TestHandleGoFindSymbol(t *testing.T) {
	tmpDir := t.TempDir()
	fixtureDir := filepath.Join(tmpDir, "pkg")
	writeGoFixture(t, fixtureDir)

	result, err := handleGoFindSymbol("Greeter", fixtureDir, tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	if m["success"] != true {
		t.Error("expected success=true")
	}
	symbols := m["symbols"].([]map[string]any)
	if len(symbols) == 0 {
		t.Fatal("expected at least 1 symbol for 'Greeter'")
	}
	found := false
	for _, s := range symbols {
		if s["name"] == "Greeter" && s["type"] == "struct" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("did not find Greeter struct in results: %v", symbols)
	}
}

func TestHandleGoFindSymbol_Function(t *testing.T) {
	tmpDir := t.TempDir()
	fixtureDir := filepath.Join(tmpDir, "pkg")
	writeGoFixture(t, fixtureDir)

	result, err := handleGoFindSymbol("Add", fixtureDir, tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	symbols := m["symbols"].([]map[string]any)
	if m["count"] == 0 {
		t.Fatal("expected at least 1 symbol for 'Add'")
	}
	if symbols[0]["name"] != "Add" {
		t.Errorf("name = %v, want Add", symbols[0]["name"])
	}
}

func TestHandleGoFindSymbol_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	fixtureDir := filepath.Join(tmpDir, "pkg")
	writeGoFixture(t, fixtureDir)

	result, err := handleGoFindSymbol("NonExistent", fixtureDir, tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	if m["count"] != 0 {
		t.Errorf("count = %v, want 0 for nonexistent symbol", m["count"])
	}
}

func TestHandleGoSymbolsOverview(t *testing.T) {
	tmpDir := t.TempDir()
	fixtureDir := filepath.Join(tmpDir, "pkg")
	writeGoFixture(t, fixtureDir)

	result, err := handleGoSymbolsOverview(fixtureDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	if m["success"] != true {
		t.Error("expected success=true")
	}
}

func TestHandleGoSymbolsOverview_EmptyDir(t *testing.T) {
	tmpDir := t.TempDir()

	result, err := handleGoSymbolsOverview(tmpDir)
	if err != nil {
		// error for empty dir (no Go files) is expected
		t.Logf("got expected error for empty dir: %v", err)
		return
	}
	// If no error, result should be a valid map
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatal("expected map result")
	}
	if m["success"] != true {
		t.Errorf("expected success=true, got %v", m["success"])
	}
}

// =========================================================================
// Dart AST handler tests
// =========================================================================

func writeDartFixture(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	src := `/// A greeting class.
class Greeter {
  final String name;
  Greeter(this.name);

  /// Returns a greeting.
  String hello() => 'Hello, $name';
}

/// Adds two numbers.
int add(int a, int b) => a + b;
`
	if err := os.WriteFile(filepath.Join(dir, "fixture.dart"), []byte(src), 0644); err != nil {
		t.Fatalf("write fixture.dart: %v", err)
	}
}

func TestHandleDartFindSymbol(t *testing.T) {
	tmpDir := t.TempDir()
	fixtureDir := filepath.Join(tmpDir, "lib")
	writeDartFixture(t, fixtureDir)

	result, err := handleDartFindSymbol("Greeter", fixtureDir, tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	if m["success"] != true {
		t.Error("expected success=true")
	}
	symbols := m["symbols"].([]map[string]any)
	if len(symbols) == 0 {
		t.Fatal("expected at least 1 symbol for 'Greeter'")
	}
}

func TestHandleDartFindSymbol_Function(t *testing.T) {
	tmpDir := t.TempDir()
	fixtureDir := filepath.Join(tmpDir, "lib")
	writeDartFixture(t, fixtureDir)

	result, err := handleDartFindSymbol("add", fixtureDir, tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	if m["count"] == 0 {
		t.Fatal("expected at least 1 symbol for 'add'")
	}
}

func TestHandleDartSymbolsOverview(t *testing.T) {
	tmpDir := t.TempDir()
	fixtureDir := filepath.Join(tmpDir, "lib")
	writeDartFixture(t, fixtureDir)

	result, err := handleDartSymbolsOverview(fixtureDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	if m["success"] != true {
		t.Error("expected success=true")
	}
}

func TestHandleGoFindSymbol_RelativePath(t *testing.T) {
	tmpDir := t.TempDir()
	fixtureDir := filepath.Join(tmpDir, "nested", "pkg")
	writeGoFixture(t, fixtureDir)

	result, err := handleGoFindSymbol("Version", fixtureDir, tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	relPath := m["relative_path"].(string)
	if filepath.IsAbs(relPath) {
		t.Errorf("relative_path should be relative, got absolute: %s", relPath)
	}
}
