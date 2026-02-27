package archtest_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/changmin/c4-core/test/archtest"
)

// countFmtErrorfNoWrap counts fmt.Errorf calls in a file that do NOT use %w.
// This is the violation metric for the ratchet test.
func countFmtErrorfNoWrap(path string) int {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		return 0
	}
	count := 0
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		id, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}
		if id.Name != "fmt" || sel.Sel.Name != "Errorf" {
			return true
		}
		hasWrap := false
		for _, arg := range call.Args {
			lit, ok := arg.(*ast.BasicLit)
			if ok && strings.Contains(lit.Value, "%w") {
				hasWrap = true
				break
			}
		}
		if !hasWrap {
			count++
		}
		return true
	})
	return count
}

// TestFmtErrorfUsesWrap enforces that fmt.Errorf calls use %w for error wrapping.
// This is a ratchet test: new code must not introduce additional violations.
// Existing violations are recorded in DefaultAllowlist; reduce counts when fixed.
func TestFmtErrorfUsesWrap(t *testing.T) {
	root := archtest.FindRoot(t)

	dirs := []string{
		filepath.Join(root, "internal"),
		filepath.Join(root, "cmd", "c4"),
	}

	failed := false
	for _, base := range dirs {
		err := filepath.WalkDir(base, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}

			rel, relErr := filepath.Rel(root, path)
			if relErr != nil {
				return nil
			}
			rel = filepath.ToSlash(rel)

			actual := countFmtErrorfNoWrap(path)
			if actual == 0 {
				return nil
			}

			allowed, ok := archtest.DefaultAllowlist[rel]
			if !ok {
				// New file with violations — not in allowlist, so max=0.
				allowed = 0
			}

			if actual > allowed {
				t.Errorf("ratchet: %s has %d fmt.Errorf-without-%%w violations, max %d\n"+
					"  Fix by adding %%w or update DefaultAllowlist if intentional.",
					rel, actual, allowed)
				failed = true
			}
			return nil
		})
		if err != nil {
			t.Errorf("WalkDir %s: %v", base, err)
		}
	}

	if failed {
		t.Log("To suppress existing violations, add the file to DefaultAllowlist in allowlist.go.")
		t.Log("To tighten the ratchet, decrease the count in DefaultAllowlist when violations are fixed.")
	}
}

// sentinelInfo holds information about a var Err* declaration in a Go file.
type sentinelInfo struct {
	file    string
	line    int
	name    string
	usesFmt bool // true if initialized with fmt.Errorf
}

// findSentinelErrors finds all package-level var Err* declarations in a file
// and reports whether they use fmt.Errorf (advisory warning) vs errors.New.
func findSentinelErrors(path string) []sentinelInfo {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		return nil
	}

	var results []sentinelInfo
	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, spec := range gd.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for i, name := range vs.Names {
				if !strings.HasPrefix(name.Name, "Err") {
					continue
				}
				usesFmt := false
				if i < len(vs.Values) {
					if call, ok := vs.Values[i].(*ast.CallExpr); ok {
						if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
							if id, ok := sel.X.(*ast.Ident); ok {
								if id.Name == "fmt" && sel.Sel.Name == "Errorf" {
									usesFmt = true
								}
							}
						}
					}
				}
				results = append(results, sentinelInfo{
					file:    path,
					line:    fset.Position(name.Pos()).Line,
					name:    name.Name,
					usesFmt: usesFmt,
				})
			}
		}
	}
	return results
}

// TestSentinelErrorsPattern checks that package-level sentinel errors (var Err*)
// prefer errors.New over fmt.Errorf. fmt.Errorf for sentinels is advisory only
// (does not fail the test) but is flagged as a style warning.
//
// Rationale: sentinel errors should be static values. fmt.Errorf is intended
// for dynamic error messages with context, not for package-level constants.
func TestSentinelErrorsPattern(t *testing.T) {
	root := archtest.FindRoot(t)

	dirs := []string{
		filepath.Join(root, "internal"),
		filepath.Join(root, "cmd", "c4"),
	}

	for _, base := range dirs {
		_ = filepath.WalkDir(base, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}

			sentinels := findSentinelErrors(path)
			for _, s := range sentinels {
				if s.usesFmt {
					rel, _ := filepath.Rel(root, s.file)
					// Advisory: log but do not fail.
					t.Logf("advisory: %s:%d: sentinel %q uses fmt.Errorf; prefer errors.New for static sentinels",
						filepath.ToSlash(rel), s.line, s.name)
				}
			}
			return nil
		})
	}
	// This test always passes — it only logs advisories.
}

// TestDuplicateErrNotFound enforces that the name "ErrNotFound" appears at
// most once per package directory. Packages in errNotFoundAllowlist may have
// exactly one such declaration (they are known to each define their own).
func TestDuplicateErrNotFound(t *testing.T) {
	root := archtest.FindRoot(t)

	// These packages legitimately each define their own ErrNotFound.
	// Having one declaration per package is fine; having two in the same
	// package would be a duplicate.
	errNotFoundAllowlist := map[string]bool{
		"internal/mailbox": true,
		"internal/secrets": true,
	}

	dirs := []string{
		filepath.Join(root, "internal"),
		filepath.Join(root, "cmd", "c4"),
	}

	// Map from package-dir (relative) → list of files that define ErrNotFound.
	pkgDefs := map[string][]string{}

	for _, base := range dirs {
		_ = filepath.WalkDir(base, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}

			fset := token.NewFileSet()
			f, parseErr := parser.ParseFile(fset, path, nil, 0)
			if parseErr != nil {
				return nil
			}

			found := false
			for _, decl := range f.Decls {
				gd, ok := decl.(*ast.GenDecl)
				if !ok {
					continue
				}
				for _, spec := range gd.Specs {
					vs, ok := spec.(*ast.ValueSpec)
					if !ok {
						continue
					}
					for _, name := range vs.Names {
						if name.Name == "ErrNotFound" {
							found = true
						}
					}
				}
			}

			if found {
				dir := filepath.Dir(path)
				rel, _ := filepath.Rel(root, dir)
				rel = filepath.ToSlash(rel)
				pkgDefs[rel] = append(pkgDefs[rel], filepath.ToSlash(path))
			}
			return nil
		})
	}

	for pkgDir, files := range pkgDefs {
		if len(files) > 1 {
			t.Errorf("duplicate ErrNotFound in package %s (defined in %d files: %v)",
				pkgDir, len(files), files)
			continue
		}
		// Exactly one definition: check it's in the allowlist or report it.
		if !errNotFoundAllowlist[pkgDir] {
			t.Errorf("unexpected ErrNotFound in package %s (file: %s); add to allowlist or rename",
				pkgDir, files[0])
		}
	}
}
