package handlers

import (
	"encoding/json"
	"testing"
)

// newNilProxy returns a BridgeProxy that will panic if actually called.
// It is safe to use in language guard tests because the guard should return
// before delegating to the sidecar for blocked languages.
func newNilProxy() *BridgeProxy {
	return nil
}

func callLanguageGuard(t *testing.T, toolName, method, filePath string) map[string]any {
	t.Helper()
	args, err := json.Marshal(map[string]any{"file_path": filePath, "symbol_name": "Foo", "new_body": "body"})
	if err != nil {
		t.Fatalf("marshal args: %v", err)
	}
	// Use a nil proxy — guard must return before reaching the proxy for blocked langs.
	handler := languageGuardedProxy(newNilProxy(), method, toolName)
	result, err := handler(args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map result, got %T", result)
	}
	return m
}

func TestLanguageGuardGoFile(t *testing.T) {
	m := callLanguageGuard(t, "c4_replace_symbol_body", "ReplaceSymbolBody", "pkg/foo.go")

	wantErr := "c4_replace_symbol_body does not support go files"
	if m["error"] != wantErr {
		t.Errorf("error = %q, want %q", m["error"], wantErr)
	}
	if m["language"] != "go" {
		t.Errorf("language = %v, want go", m["language"])
	}
}

func TestLanguageGuardDartFile(t *testing.T) {
	m := callLanguageGuard(t, "c4_insert_before_symbol", "InsertBeforeSymbol", "lib/widget.dart")

	if m["error"] == nil {
		t.Error("expected error field for dart file")
	}
	if m["language"] != "dart" {
		t.Errorf("language = %v, want dart", m["language"])
	}
}

func TestLanguageGuardRustFile(t *testing.T) {
	m := callLanguageGuard(t, "c4_rename_symbol", "RenameSymbol", "src/main.rs")

	if m["error"] == nil {
		t.Error("expected error field for rust file")
	}
	if m["language"] != "rust" {
		t.Errorf("language = %v, want rust", m["language"])
	}
}

func TestLanguageGuardPythonFile(t *testing.T) {
	// Python files must be delegated to the sidecar, not short-circuited by the guard.
	// When the guard fires (wrong), it returns a map with a non-nil "language" key.
	// When the guard passes through (correct), the proxy is called; with an empty addr
	// it returns a nil map — so "language" is absent from the result.
	args, err := json.Marshal(map[string]any{"file_path": "utils.py", "symbol_name": "foo", "new_body": "pass"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	// BridgeProxy with empty addr: Call() returns (nil map, error).
	fake := &BridgeProxy{}
	handler := languageGuardedProxy(fake, "ReplaceSymbolBody", "c4_replace_symbol_body")
	result, _ := handler(args)
	// Language guard must not set a "language" key for .py files.
	if m, ok := result.(map[string]any); ok && m["language"] != nil {
		t.Errorf("language guard must not fire for .py: got language=%v", m["language"])
	}
}

func TestLanguageGuardNoExtension(t *testing.T) {
	// Files with no extension should be delegated to the sidecar.
	args, err := json.Marshal(map[string]any{"file_path": "Makefile", "symbol_name": "build", "new_body": ""})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	fake := &BridgeProxy{}
	handler := languageGuardedProxy(fake, "ReplaceSymbolBody", "c4_replace_symbol_body")
	result, _ := handler(args)
	if m, ok := result.(map[string]any); ok {
		if m["language"] != nil {
			t.Errorf("expected no language guard for no-ext file, got language=%v", m["language"])
		}
	}
}
