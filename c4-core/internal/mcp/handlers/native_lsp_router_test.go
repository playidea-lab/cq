package handlers

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

// =========================================================================
// goAwareFindSymbol router tests
// =========================================================================

// callGoAwareFindSymbol calls the goAwareFindSymbol handler with the given path.
// A nil proxy is safe when the path resolves to Go/Dart files (native path).
// A real empty proxy is used for non-native fallback paths.
func callGoAwareFindSymbol(t *testing.T, proxy *BridgeProxy, rootDir, path, name string) (any, error) {
	t.Helper()
	handler := goAwareFindSymbol(proxy, rootDir)
	args, err := json.Marshal(map[string]any{"name": name, "path": path})
	if err != nil {
		t.Fatalf("marshal args: %v", err)
	}
	return handler(args)
}

// callGoAwareSymbolsOverview calls the goAwareSymbolsOverview handler with the given path.
func callGoAwareSymbolsOverview(t *testing.T, proxy *BridgeProxy, rootDir, path string) (any, error) {
	t.Helper()
	handler := goAwareSymbolsOverview(proxy, rootDir)
	args, err := json.Marshal(map[string]any{"path": path})
	if err != nil {
		t.Fatalf("marshal args: %v", err)
	}
	return handler(args)
}

func TestGoAwareFindSymbol_RoutesToGoNative(t *testing.T) {
	tmpDir := t.TempDir()
	fixtureDir := filepath.Join(tmpDir, "pkg")
	writeGoFixture(t, fixtureDir)

	result, err := callGoAwareFindSymbol(t, nil, tmpDir, fixtureDir, "Greeter")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map result, got %T", result)
	}
	if m["success"] != true {
		t.Errorf("expected success=true, got %v", m["success"])
	}
	if m["pattern"] != "Greeter" {
		t.Errorf("pattern = %v, want Greeter", m["pattern"])
	}
}

func TestGoAwareFindSymbol_RoutesToDartNative(t *testing.T) {
	tmpDir := t.TempDir()
	fixtureDir := filepath.Join(tmpDir, "lib")
	writeDartFixture(t, fixtureDir)

	result, err := callGoAwareFindSymbol(t, nil, tmpDir, fixtureDir, "Greeter")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map result, got %T", result)
	}
	if m["success"] != true {
		t.Errorf("expected success=true for dart native route, got %v", m["success"])
	}
}

func TestGoAwareFindSymbol_FallsBackToProxyForPython(t *testing.T) {
	tmpDir := t.TempDir()
	// No Go/Dart files in tmpDir — should fall through to proxy.
	// BridgeProxy with no addr will return an error, which is the expected fallback.
	proxy := &BridgeProxy{}
	result, _ := callGoAwareFindSymbol(t, proxy, tmpDir, tmpDir, "SomeFunc")
	// With an empty proxy, result is nil or an error map — we just verify no panic.
	_ = result
}

func TestGoAwareFindSymbol_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	proxy := &BridgeProxy{}
	handler := goAwareFindSymbol(proxy, tmpDir)
	// Invalid JSON falls back to proxy handler which may error.
	result, _ := handler(json.RawMessage(`{invalid`))
	_ = result
}

func TestGoAwareFindSymbol_PathEscapesRoot(t *testing.T) {
	tmpDir := t.TempDir()
	proxy := &BridgeProxy{}
	handler := goAwareFindSymbol(proxy, tmpDir)
	args, _ := json.Marshal(map[string]any{"name": "Foo", "path": "../../../etc"})
	_, err := handler(args)
	if err == nil {
		t.Error("expected error for path escaping project root")
	}
}

func TestGoAwareFindSymbol_RelativePath(t *testing.T) {
	tmpDir := t.TempDir()
	fixtureDir := filepath.Join(tmpDir, "pkg")
	writeGoFixture(t, fixtureDir)

	// Use relative path "pkg" from rootDir.
	result, err := callGoAwareFindSymbol(t, nil, tmpDir, "pkg", "Add")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map result, got %T", result)
	}
	if m["success"] != true {
		t.Errorf("expected success=true, got %v", m["success"])
	}
}

// =========================================================================
// goAwareSymbolsOverview router tests
// =========================================================================

func TestGoAwareSymbolsOverview_RoutesToGoNative(t *testing.T) {
	tmpDir := t.TempDir()
	fixtureDir := filepath.Join(tmpDir, "pkg")
	writeGoFixture(t, fixtureDir)

	result, err := callGoAwareSymbolsOverview(t, nil, tmpDir, fixtureDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map result, got %T", result)
	}
	if m["success"] != true {
		t.Errorf("expected success=true, got %v", m["success"])
	}
}

func TestGoAwareSymbolsOverview_RoutesToDartNative(t *testing.T) {
	tmpDir := t.TempDir()
	fixtureDir := filepath.Join(tmpDir, "lib")
	writeDartFixture(t, fixtureDir)

	result, err := callGoAwareSymbolsOverview(t, nil, tmpDir, fixtureDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map result, got %T", result)
	}
	if m["success"] != true {
		t.Errorf("expected success=true for dart native route, got %v", m["success"])
	}
}

func TestGoAwareSymbolsOverview_FallsBackToProxyForNonNative(t *testing.T) {
	tmpDir := t.TempDir()
	proxy := &BridgeProxy{}
	result, _ := callGoAwareSymbolsOverview(t, proxy, tmpDir, tmpDir)
	_ = result
}

func TestGoAwareSymbolsOverview_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	proxy := &BridgeProxy{}
	handler := goAwareSymbolsOverview(proxy, tmpDir)
	result, _ := handler(json.RawMessage(`{invalid`))
	_ = result
}

func TestGoAwareSymbolsOverview_PathEscapesRoot(t *testing.T) {
	tmpDir := t.TempDir()
	proxy := &BridgeProxy{}
	handler := goAwareSymbolsOverview(proxy, tmpDir)
	args, _ := json.Marshal(map[string]any{"path": "../../../etc"})
	_, err := handler(args)
	if err == nil {
		t.Error("expected error for path escaping project root")
	}
}

func TestGoAwareSymbolsOverview_RelativePath(t *testing.T) {
	tmpDir := t.TempDir()
	fixtureDir := filepath.Join(tmpDir, "pkg")
	writeGoFixture(t, fixtureDir)

	result, err := callGoAwareSymbolsOverview(t, nil, tmpDir, "pkg")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map result, got %T", result)
	}
	if m["success"] != true {
		t.Errorf("expected success=true, got %v", m["success"])
	}
}

// =========================================================================
// languageGuardedProxy additional branch tests
// =========================================================================

func TestLanguageGuardEmptyFilePath(t *testing.T) {
	// Empty file_path should delegate to the proxy (guard requires a path).
	args, err := json.Marshal(map[string]any{"file_path": "", "symbol_name": "Foo"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	proxy := &BridgeProxy{}
	handler := languageGuardedProxy(proxy, "ReplaceSymbolBody", "c4_replace_symbol_body")
	result, _ := handler(args)
	// Language guard must not fire for empty file_path.
	if m, ok := result.(map[string]any); ok && m["language"] != nil {
		t.Errorf("language guard must not fire for empty file_path, got language=%v", m["language"])
	}
}

func TestLanguageGuardInvalidJSON(t *testing.T) {
	// Invalid JSON should delegate to the proxy.
	proxy := &BridgeProxy{}
	handler := languageGuardedProxy(proxy, "ReplaceSymbolBody", "c4_replace_symbol_body")
	result, _ := handler(json.RawMessage(`{bad json`))
	_ = result // no panic expected
}

func TestLanguageGuardJSFile(t *testing.T) {
	// JS files must be delegated to the sidecar.
	args, err := json.Marshal(map[string]any{"file_path": "src/app.js"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	proxy := &BridgeProxy{}
	handler := languageGuardedProxy(proxy, "RenameSymbol", "c4_rename_symbol")
	result, _ := handler(args)
	if m, ok := result.(map[string]any); ok && m["language"] != nil {
		t.Errorf("language guard must not fire for .js, got language=%v", m["language"])
	}
}

func TestLanguageGuardTSFile(t *testing.T) {
	// TS files must be delegated to the sidecar.
	args, err := json.Marshal(map[string]any{"file_path": "src/main.ts"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	proxy := &BridgeProxy{}
	handler := languageGuardedProxy(proxy, "InsertAfterSymbol", "c4_insert_after_symbol")
	result, _ := handler(args)
	if m, ok := result.(map[string]any); ok && m["language"] != nil {
		t.Errorf("language guard must not fire for .ts, got language=%v", m["language"])
	}
}

func TestLanguageGuardGoFileHint(t *testing.T) {
	// Verify hint fields are set for Go files.
	m := callLanguageGuard(t, "c4_insert_after_symbol", "InsertAfterSymbol", "cmd/main.go")
	if m["hint"] == nil {
		t.Error("expected hint key for go file guard response")
	}
	if m["supported_languages"] == nil {
		t.Error("expected supported_languages key for go file guard response")
	}
}
