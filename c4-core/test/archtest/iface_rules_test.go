package archtest_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/changmin/c4-core/test/archtest"
)

// compileTimeTarget represents a (struct, interface) pair that should have
// a compile-time assertion of the form: var _ Interface = (*Struct)(nil)
type compileTimeTarget struct {
	structPkg  string // relative pkg path, e.g. "internal/mcp/handlers"
	structName string
	ifacePkg   string // relative pkg path, e.g. "internal/store"
	ifaceName  string
}

// TestCompileTimeAssertions checks that key (struct, interface) pairs have
// a compile-time assertion of the form: var _ I = (*T)(nil).
// Missing assertions are advisory — they emit t.Logf warnings but do not
// fail the test. The intent is to document the gap so T-ARCH-005b can add
// the actual assertions.
func TestCompileTimeAssertions(t *testing.T) {
	root := archtest.FindRoot(t)

	targets := []compileTimeTarget{
		{
			structPkg:  "internal/mcp/handlers",
			structName: "SQLiteStore",
			ifacePkg:   "internal/store",
			ifaceName:  "Store",
		},
		{
			structPkg:  "internal/llm",
			structName: "AnthropicProvider",
			ifacePkg:   "internal/llm",
			ifaceName:  "Provider",
		},
		{
			structPkg:  "internal/llm",
			structName: "GeminiProvider",
			ifacePkg:   "internal/llm",
			ifaceName:  "Provider",
		},
		{
			structPkg:  "internal/llm",
			structName: "OpenAIProvider",
			ifacePkg:   "internal/llm",
			ifaceName:  "Provider",
		},
		{
			structPkg:  "internal/llm",
			structName: "OllamaProvider",
			ifacePkg:   "internal/llm",
			ifaceName:  "Provider",
		},
		{
			structPkg:  "internal/cloud",
			structName: "KnowledgeCloudClient",
			ifacePkg:   "internal/knowledge",
			ifaceName:  "CloudSyncer",
		},
	}

	for _, tc := range targets {
		tc := tc
		t.Run(tc.structName+"_implements_"+tc.ifaceName, func(t *testing.T) {
			found := assertionExistsInDir(t, root, tc.structPkg, tc.structName, tc.ifaceName)
			if !found {
				// Advisory: log the missing assertion but do not fail.
				// T-ARCH-005b is responsible for adding the actual assertions.
				t.Logf("MISSING compile-time assertion: var _ %s = (*%s)(nil) in package %s",
					tc.ifaceName, tc.structName, tc.structPkg)
				return
			}
			t.Logf("OK: found var _ %s = (*%s)(nil) in %s", tc.ifaceName, tc.structName, tc.structPkg)
		})
	}
}

// assertionExistsInDir searches all .go files in pkgRelPath for a line
// containing both "var _ " and structName and ifaceName.
func assertionExistsInDir(t *testing.T, root, pkgRelPath, structName, ifaceName string) bool {
	t.Helper()
	dir := filepath.Join(root, filepath.FromSlash(pkgRelPath))
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Logf("warning: cannot read dir %s: %v", dir, err)
		return false
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") {
			continue
		}
		content, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(content), "\n") {
			if strings.Contains(line, "var _ ") &&
				strings.Contains(line, structName) &&
				strings.Contains(line, ifaceName) {
				return true
			}
		}
	}
	return false
}

// TestInterfaceInConsumer is an advisory test that logs a warning when an
// interface is defined in the same package as its primary implementor rather
// than in the consumer package. This follows the Go convention that interfaces
// belong to the consumer, not the implementor.
//
// This test never fails — it only emits t.Logf warnings.
func TestInterfaceInConsumer(t *testing.T) {
	root := archtest.FindRoot(t)
	pkgs := archtest.ParsePackages(root)

	// Advisory: for each package that defines both interfaces and structs,
	// warn that the interfaces might belong in consumer packages instead.
	for _, pkg := range pkgs {
		ifaceCount := 0
		structCount := 0
		for _, fi := range pkg.Files {
			ifaceCount += len(fi.Interfaces)
			structCount += len(fi.Structs)
		}
		if ifaceCount > 0 && structCount > 0 {
			t.Logf("advisory: package %q defines both %d interface(s) and %d struct(s) — "+
				"consider moving interfaces to consumer packages per Go conventions",
				pkg.PkgPath, ifaceCount, structCount)
		}
	}
}

// TestExportedStructHasConstructor is an advisory test that warns when an
// exported struct in a package has no corresponding New*() constructor
// function in the same package.
//
// This test never fails — it only emits t.Logf warnings.
func TestExportedStructHasConstructor(t *testing.T) {
	root := archtest.FindRoot(t)
	pkgs := archtest.ParsePackages(root)

	for _, pkg := range pkgs {
		// Collect exported struct names and all function names in the package.
		var exportedStructs []string
		allFuncs := map[string]bool{}

		for _, fi := range pkg.Files {
			for _, s := range fi.Structs {
				if len(s) > 0 && s[0] >= 'A' && s[0] <= 'Z' {
					exportedStructs = append(exportedStructs, s)
				}
			}
			for _, fn := range fi.Functions {
				allFuncs[fn] = true
			}
		}

		for _, name := range exportedStructs {
			ctorName := "New" + name
			if !allFuncs[ctorName] {
				t.Logf("advisory: %s.%s has no %s() constructor",
					pkg.PkgPath, name, ctorName)
			}
		}
	}
}
