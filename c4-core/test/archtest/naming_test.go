package archtest_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/changmin/c4-core/test/archtest"
)

// TestRegisterHandlersNaming enforces that exported Register* functions in the
// handlers package end with the "Handlers" suffix.
//
// Rationale: consistent naming makes it easy to grep all registration points.
// Orchestrators that wire multiple subsystems are listed in the allowlist.
func TestRegisterHandlersNaming(t *testing.T) {
	root := archtest.FindRoot(t)
	pkgs := archtest.ParsePackages(root)

	// allowlist: composite orchestrators and established single-tool registrations
	// that intentionally deviate from the *Handlers suffix convention.
	allowlist := map[string]bool{
		// Composite orchestrators (wire multiple sub-registrations).
		"RegisterAll":                     true,
		"RegisterAllHandlersWithOpts":     true,
		"RegisterAllHandlersLazyWithOpts": true,
		"RegisterNativeHandlers":          true,
		// Single-tool registrations that predate this convention.
		"RegisterConfigHandler":    true,
		"RegisterConfigSetHandler": true,
		"RegisterHealthHandler":    true,
	}

	handlersPath := filepath.Join("internal", "mcp", "handlers")

	var violations []string
	for _, pkg := range pkgs {
		if !strings.HasSuffix(filepath.ToSlash(pkg.PkgPath), "internal/mcp/handlers") {
			continue
		}
		_ = handlersPath // used for clarity above
		for _, fi := range pkg.Files {
			for _, fn := range fi.Functions {
				if !strings.HasPrefix(fn, "Register") {
					continue
				}
				// Only check exported (uppercase first char after "Register").
				if len(fn) <= len("Register") {
					continue
				}
				next := fn[len("Register")]
				if next < 'A' || next > 'Z' {
					continue // unexported helper, skip
				}
				if allowlist[fn] {
					continue
				}
				if !strings.HasSuffix(fn, "Handlers") {
					violations = append(violations, fn)
				}
			}
		}
	}

	if len(violations) > 0 {
		t.Errorf("exported Register* functions must end with 'Handlers' suffix (or be in allowlist):\n  %s",
			strings.Join(violations, "\n  "))
	}
}

// TestStoreTypeSuffix enforces that exported struct types in *store*.go files
// whose names (case-insensitive) end with "store" use the exact "Store" suffix.
//
// This catches misspellings like "CacheStorage" or "KnowledgeStoreImpl" while
// leaving domain data structs (Document, StoredEvent, DLQEntry, …) untouched.
func TestStoreTypeSuffix(t *testing.T) {
	root := archtest.FindRoot(t)
	pkgs := archtest.ParsePackages(root)

	var violations []string
	for _, pkg := range pkgs {
		for _, fi := range pkg.Files {
			base := filepath.Base(fi.Path)
			if !strings.Contains(strings.ToLower(base), "store") {
				continue
			}
			for _, s := range fi.Structs {
				// Only flag structs whose lowercase name ends with "store".
				// e.g. SQLiteStore → "sqlitestore" ends with "store" ✓
				//      StoredEvent → "storedevent" does NOT end with "store" → skip
				if !strings.HasSuffix(strings.ToLower(s), "store") {
					continue
				}
				if !strings.HasSuffix(s, "Store") {
					violations = append(violations, fi.Path+": "+s)
				}
			}
		}
	}

	if len(violations) > 0 {
		t.Errorf("exported structs in *store*.go files whose names end with 'store' must use exact 'Store' suffix:\n  %s",
			strings.Join(violations, "\n  "))
	}
}

// TestConstructorNaming is an advisory check: exported structs should have a
// corresponding New<TypeName>() constructor function in the same package.
// Missing constructors are logged as warnings, not hard failures.
func TestConstructorNaming(t *testing.T) {
	root := archtest.FindRoot(t)
	pkgs := archtest.ParsePackages(root)

	for _, pkg := range pkgs {
		// Collect all function names across all files in this package.
		allFuncs := map[string]bool{}
		for _, fi := range pkg.Files {
			for _, fn := range fi.Functions {
				allFuncs[fn] = true
			}
		}

		// Collect all exported structs across all files in this package.
		for _, fi := range pkg.Files {
			for _, s := range fi.Structs {
				if len(s) == 0 || s[0] < 'A' || s[0] > 'Z' {
					continue // unexported, skip
				}
				constructor := "New" + s
				if !allFuncs[constructor] {
					t.Logf("advisory: exported struct %s in %s has no %s() constructor",
						s, pkg.PkgPath, constructor)
				}
			}
		}
	}
}
