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

	// symbols[] 각 항목에도 _edit_hint 있어야 함
	syms, _ := m["symbols"].([]map[string]any)
	for i, sym := range syms {
		if sym["_edit_hint"] == nil {
			t.Errorf("symbols[%d] missing _edit_hint", i)
		}
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

	// functions[] 각 항목에도 _edit_hint 있어야 함
	if fns, ok := m["functions"].([]map[string]any); ok {
		for i, fn := range fns {
			if fn["_edit_hint"] == nil {
				t.Errorf("functions[%d] missing _edit_hint", i)
			}
		}
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

	// symbols[] 각 항목에도 _edit_hint 있어야 함
	syms, _ := m["symbols"].([]map[string]any)
	for i, sym := range syms {
		if sym["_edit_hint"] == nil {
			t.Errorf("symbols[%d] missing _edit_hint", i)
		}
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

	// classes[]/functions[] 각 항목에도 _edit_hint 있어야 함
	for _, key := range []string{"classes", "functions"} {
		if items, ok := m[key].([]map[string]any); ok {
			for i, item := range items {
				if item["_edit_hint"] == nil {
					t.Errorf("%s[%d] missing _edit_hint", key, i)
				}
			}
		}
	}
}
