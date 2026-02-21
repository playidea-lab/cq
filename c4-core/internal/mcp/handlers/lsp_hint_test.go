package handlers

import (
	"path/filepath"
	"testing"
)

func TestGoFindSymbolHasEditHint(t *testing.T) {
	tmpDir := t.TempDir()
	fixtureDir := filepath.Join(tmpDir, "pkg")
	writeGoFixture(t, fixtureDir)

	result, err := handleGoFindSymbol("Greeter", fixtureDir, tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	hint, ok := m["_edit_hint"]
	if !ok {
		t.Fatal("expected _edit_hint key in result")
	}
	if hint != "Go file: use Edit tool for modifications (c4_replace_symbol_body not supported)" {
		t.Errorf("unexpected _edit_hint value: %v", hint)
	}
}

func TestGoOverviewHasEditHint(t *testing.T) {
	tmpDir := t.TempDir()
	fixtureDir := filepath.Join(tmpDir, "pkg")
	writeGoFixture(t, fixtureDir)

	result, err := handleGoSymbolsOverview(fixtureDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	hint, ok := m["_edit_hint"]
	if !ok {
		t.Fatal("expected _edit_hint key in result")
	}
	if hint != "Go file: use Edit tool for modifications (c4_replace_symbol_body not supported)" {
		t.Errorf("unexpected _edit_hint value: %v", hint)
	}
}

func TestDartFindSymbolHasEditHint(t *testing.T) {
	tmpDir := t.TempDir()
	fixtureDir := filepath.Join(tmpDir, "lib")
	writeDartFixture(t, fixtureDir)

	result, err := handleDartFindSymbol("Greeter", fixtureDir, tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	hint, ok := m["_edit_hint"]
	if !ok {
		t.Fatal("expected _edit_hint key in result")
	}
	if hint != "Dart file: use Edit tool for modifications (c4_replace_symbol_body not supported)" {
		t.Errorf("unexpected _edit_hint value: %v", hint)
	}
}

func TestDartOverviewHasEditHint(t *testing.T) {
	tmpDir := t.TempDir()
	fixtureDir := filepath.Join(tmpDir, "lib")
	writeDartFixture(t, fixtureDir)

	result, err := handleDartSymbolsOverview(fixtureDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	hint, ok := m["_edit_hint"]
	if !ok {
		t.Fatal("expected _edit_hint key in result")
	}
	if hint != "Dart file: use Edit tool for modifications (c4_replace_symbol_body not supported)" {
		t.Errorf("unexpected _edit_hint value: %v", hint)
	}
}
