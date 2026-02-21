package handlers

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/changmin/c4-core/internal/config"
)

func TestDetectValidations_GoProject(t *testing.T) {
	dir := t.TempDir()
	// Create c4-core/go.mod to trigger go-test detection
	goDir := filepath.Join(dir, "c4-core")
	if err := os.MkdirAll(goDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(goDir, "go.mod"), []byte("module test"), 0644); err != nil {
		t.Fatal(err)
	}

	defs := detectValidations(dir)
	found := map[string]bool{}
	for _, d := range defs {
		found[d.Name] = true
	}
	if !found["go-test"] {
		t.Error("expected go-test validation to be detected")
	}
}

func TestDetectValidations_PythonProject(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte("[project]"), 0644); err != nil {
		t.Fatal(err)
	}

	defs := detectValidations(dir)
	found := map[string]bool{}
	for _, d := range defs {
		found[d.Name] = true
	}
	if !found["pytest"] {
		t.Error("expected pytest validation to be detected")
	}
	if !found["ruff"] {
		t.Error("expected ruff validation to be detected")
	}
}

func TestDetectValidations_RustProject(t *testing.T) {
	dir := t.TempDir()
	cargoDir := filepath.Join(dir, "c1", "src-tauri")
	if err := os.MkdirAll(cargoDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cargoDir, "Cargo.toml"), []byte("[package]"), 0644); err != nil {
		t.Fatal(err)
	}

	defs := detectValidations(dir)
	found := map[string]bool{}
	for _, d := range defs {
		found[d.Name] = true
	}
	if !found["cargo-check"] {
		t.Error("expected cargo-check validation to be detected")
	}
}

func TestDetectValidations_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	defs := detectValidations(dir)
	if len(defs) != 0 {
		t.Errorf("expected 0 validations in empty dir, got %d", len(defs))
	}
}

func TestHandleRunValidation_ParseArgs(t *testing.T) {
	dir := t.TempDir()

	// No validations available, so should get empty results
	args := `{"names": ["pytest"]}`
	result, err := handleRunValidation(dir, json.RawMessage(args))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("result type = %T, want map[string]any", result)
	}

	// No matching validations
	if m["count"] != 0 {
		t.Errorf("count = %v, want 0", m["count"])
	}
	if m["error"] != "no matching validations found" {
		t.Errorf("error = %v, want 'no matching validations found'", m["error"])
	}
}

func TestHandleRunValidation_EmptyArgs(t *testing.T) {
	dir := t.TempDir()

	// Empty args = run all (none available in empty dir)
	result, err := handleRunValidation(dir, json.RawMessage("{}"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	if m["all_passed"] != true {
		t.Errorf("all_passed = %v, want true (vacuously)", m["all_passed"])
	}
}

func TestBuildValidationAliasMap(t *testing.T) {
	available := []validationDef{
		{Name: "pytest"},
		{Name: "ruff"},
		{Name: "go-test"},
	}

	aliases := buildValidationAliasMap(available)

	tests := []struct {
		alias    string
		expected string
	}{
		{"lint", "ruff"},
		{"linter", "ruff"},
		{"unit", "pytest"},
		{"test", "pytest"},
		{"tests", "pytest"},
		{"go", "go-test"},
	}

	for _, tt := range tests {
		if got := aliases[tt.alias]; got != tt.expected {
			t.Errorf("alias[%q] = %q, want %q", tt.alias, got, tt.expected)
		}
	}
}

func TestBuildValidationAliasMap_GoOnly(t *testing.T) {
	available := []validationDef{
		{Name: "go-test"},
	}

	aliases := buildValidationAliasMap(available)

	// Without pytest, unit/test should map to go-test
	if got := aliases["unit"]; got != "go-test" {
		t.Errorf("alias[unit] = %q, want go-test", got)
	}
	if got := aliases["test"]; got != "go-test" {
		t.Errorf("alias[test] = %q, want go-test", got)
	}
}

func TestDetectValidationsWithConfig_LintOverride(t *testing.T) {
	// Reset after test
	orig := validationCfg
	defer func() { validationCfg = orig }()

	cfg := &config.ValidationConfig{
		Lint: "uv run ruff check .",
	}
	SetValidationConfig(cfg)

	dir := t.TempDir()
	defs := detectValidationsWithConfig(dir)

	if len(defs) != 1 {
		t.Fatalf("expected 1 validation from config, got %d", len(defs))
	}
	if defs[0].Name != "lint" {
		t.Errorf("Name = %q, want lint", defs[0].Name)
	}
	if defs[0].Command != "uv" {
		t.Errorf("Command = %q, want uv", defs[0].Command)
	}
	wantArgs := []string{"run", "ruff", "check", "."}
	if len(defs[0].Args) != len(wantArgs) {
		t.Fatalf("Args = %v, want %v", defs[0].Args, wantArgs)
	}
	for i, a := range wantArgs {
		if defs[0].Args[i] != a {
			t.Errorf("Args[%d] = %q, want %q", i, defs[0].Args[i], a)
		}
	}
}

func TestDetectValidationsWithConfig_BothOverride(t *testing.T) {
	orig := validationCfg
	defer func() { validationCfg = orig }()

	cfg := &config.ValidationConfig{
		Lint: "golangci-lint run",
		Unit: "go test ./...",
	}
	SetValidationConfig(cfg)

	dir := t.TempDir()
	defs := detectValidationsWithConfig(dir)

	if len(defs) != 2 {
		t.Fatalf("expected 2 validations from config, got %d", len(defs))
	}
	names := map[string]bool{}
	for _, d := range defs {
		names[d.Name] = true
	}
	if !names["lint"] {
		t.Error("expected lint in config-based defs")
	}
	if !names["unit"] {
		t.Error("expected unit in config-based defs")
	}
}

func TestDetectValidationsWithConfig_FallbackWhenNilConfig(t *testing.T) {
	orig := validationCfg
	defer func() { validationCfg = orig }()

	// Ensure no config set
	validationCfg = nil

	dir := t.TempDir()
	// Create pyproject.toml so detectValidations finds something
	if err := os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte("[project]"), 0644); err != nil {
		t.Fatal(err)
	}

	defs := detectValidationsWithConfig(dir)
	found := map[string]bool{}
	for _, d := range defs {
		found[d.Name] = true
	}
	if !found["pytest"] {
		t.Error("expected fallback to detectValidations to find pytest")
	}
}

func TestDetectValidationsWithConfig_FallbackWhenEmptyConfig(t *testing.T) {
	orig := validationCfg
	defer func() { validationCfg = orig }()

	// Config with empty strings → should fallback to auto-detection
	cfg := &config.ValidationConfig{
		Lint: "",
		Unit: "",
	}
	SetValidationConfig(cfg)

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte("[project]"), 0644); err != nil {
		t.Fatal(err)
	}

	defs := detectValidationsWithConfig(dir)
	found := map[string]bool{}
	for _, d := range defs {
		found[d.Name] = true
	}
	if !found["pytest"] {
		t.Error("expected fallback to detectValidations when config has empty strings")
	}
}
