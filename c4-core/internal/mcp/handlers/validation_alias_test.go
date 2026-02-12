package handlers

import "testing"

func TestBuildValidationAliasMapPrefersPytest(t *testing.T) {
	alias := buildValidationAliasMap([]validationDef{
		{Name: "ruff"},
		{Name: "pytest"},
		{Name: "go-test"},
	})

	if got := alias["lint"]; got != "ruff" {
		t.Fatalf("lint alias = %q, want ruff", got)
	}
	if got := alias["unit"]; got != "pytest" {
		t.Fatalf("unit alias = %q, want pytest", got)
	}
	if got := alias["test"]; got != "pytest" {
		t.Fatalf("test alias = %q, want pytest", got)
	}
}

func TestBuildValidationAliasMapFallbackToGoTest(t *testing.T) {
	alias := buildValidationAliasMap([]validationDef{
		{Name: "go-test"},
	})

	if got := alias["unit"]; got != "go-test" {
		t.Fatalf("unit alias = %q, want go-test", got)
	}
	if got := alias["tests"]; got != "go-test" {
		t.Fatalf("tests alias = %q, want go-test", got)
	}
}
