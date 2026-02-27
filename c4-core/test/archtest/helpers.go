// Package archtest provides AST parsing infrastructure for architecture tests.
// It parses all Go files under internal/ and cmd/c4/ to extract import and
// symbol information, enabling enforce-able architectural constraints.
package archtest

import (
	"bufio"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

// PackageInfo holds parsed information about a single Go package.
type PackageInfo struct {
	PkgPath string
	Imports map[string]bool
	Files   []FileInfo
}

// FileInfo holds parsed information about a single Go source file.
type FileInfo struct {
	Path       string
	Imports    []string
	Interfaces []string
	Structs    []string
	Functions  []string
	ErrorCalls []ErrorCall
}

// ErrorCall records a call to errors.New or fmt.Errorf in a source file.
type ErrorCall struct {
	Line    int
	HasWrap bool // true if the call uses %w (fmt.Errorf with wrapping)
}

// parseOnce caches the result of ParsePackages at the package level.
var (
	parseOnce   sync.Once
	parsedPkgs  []PackageInfo
	parsedRoot  string
)

// ParsePackages walks rootDir/internal/ and rootDir/cmd/c4/, parses every
// .go file (build-tag agnostic), and returns the collected PackageInfo slice.
// Results are cached after the first call for the same rootDir.
func ParsePackages(rootDir string) []PackageInfo {
	parseOnce.Do(func() {
		parsedRoot = rootDir
		parsedPkgs = doParse(rootDir)
	})
	// If called with a different rootDir after init, re-parse (rare in tests).
	if parsedRoot != rootDir {
		return doParse(rootDir)
	}
	return parsedPkgs
}

func doParse(rootDir string) []PackageInfo {
	dirs := []string{
		filepath.Join(rootDir, "internal"),
		filepath.Join(rootDir, "cmd", "c4"),
	}

	// Collect unique package directories.
	pkgDirs := map[string]bool{}
	for _, base := range dirs {
		_ = filepath.WalkDir(base, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				pkgDirs[path] = true
			}
			return nil
		})
	}

	var pkgs []PackageInfo
	for dir := range pkgDirs {
		info := parseDir(rootDir, dir)
		if info != nil {
			pkgs = append(pkgs, *info)
		}
	}
	return pkgs
}

// parseDir parses all .go files in a single directory.
func parseDir(rootDir, dir string) *PackageInfo {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var files []FileInfo
	allImports := map[string]bool{}

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		fi := parseFile(path)
		if fi == nil {
			continue
		}
		files = append(files, *fi)
		for _, imp := range fi.Imports {
			allImports[imp] = true
		}
	}

	if len(files) == 0 {
		return nil
	}

	// Use the directory path relative to rootDir as PkgPath.
	rel, err := filepath.Rel(rootDir, dir)
	if err != nil {
		rel = dir
	}
	// Normalize to forward slashes for consistency with import paths.
	pkgPath := filepath.ToSlash(rel)

	return &PackageInfo{
		PkgPath: pkgPath,
		Imports: allImports,
		Files:   files,
	}
}

// parseFile parses a single .go file using go/parser with ParseComments mode.
// Build constraints are intentionally ignored (no build tag filtering).
func parseFile(path string) *FileInfo {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		// Tolerate parse errors; skip the file.
		return nil
	}

	fi := &FileInfo{Path: path}

	// Collect imports.
	for _, imp := range f.Imports {
		val := strings.Trim(imp.Path.Value, `"`)
		fi.Imports = append(fi.Imports, val)
	}

	// Collect top-level declarations.
	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					switch s.Type.(type) {
					case *ast.InterfaceType:
						fi.Interfaces = append(fi.Interfaces, s.Name.Name)
					case *ast.StructType:
						fi.Structs = append(fi.Structs, s.Name.Name)
					}
				}
			}
		case *ast.FuncDecl:
			fi.Functions = append(fi.Functions, d.Name.Name)
			// Walk function body for error calls.
			if d.Body != nil {
				ast.Inspect(d.Body, func(n ast.Node) bool {
					call, ok := n.(*ast.CallExpr)
					if !ok {
						return true
					}
					ec := classifyErrorCall(fset, call)
					if ec != nil {
						fi.ErrorCalls = append(fi.ErrorCalls, *ec)
					}
					return true
				})
			}
		}
	}

	return fi
}

// classifyErrorCall returns an ErrorCall if the node is errors.New or
// fmt.Errorf; returns nil otherwise.
func classifyErrorCall(fset *token.FileSet, call *ast.CallExpr) *ErrorCall {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return nil
	}
	id, ok := sel.X.(*ast.Ident)
	if !ok {
		return nil
	}

	pkg := id.Name
	fn := sel.Sel.Name

	switch {
	case pkg == "errors" && fn == "New":
		return &ErrorCall{
			Line:    fset.Position(call.Pos()).Line,
			HasWrap: false,
		}
	case pkg == "fmt" && fn == "Errorf":
		hasWrap := false
		// Check if any string argument contains %w.
		for _, arg := range call.Args {
			lit, ok := arg.(*ast.BasicLit)
			if ok && strings.Contains(lit.Value, "%w") {
				hasWrap = true
				break
			}
		}
		return &ErrorCall{
			Line:    fset.Position(call.Pos()).Line,
			HasWrap: hasWrap,
		}
	}
	return nil
}

// ModulePath reads the go.mod file in rootDir and returns the module path.
func ModulePath(rootDir string) string {
	gomod := filepath.Join(rootDir, "go.mod")
	f, err := os.Open(gomod)
	if err != nil {
		return ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module "))
		}
	}
	return ""
}

// FindRoot returns the root directory of the c4-core module (the directory
// containing go.mod). Resolution order:
//  1. ARCHTEST_ROOT environment variable (CI / override).
//  2. Walk up from this source file's location until go.mod is found.
func FindRoot(t *testing.T) string {
	t.Helper()

	// 1. Environment variable override.
	if root := os.Getenv("ARCHTEST_ROOT"); root != "" {
		if _, err := os.Stat(filepath.Join(root, "go.mod")); err == nil {
			return root
		}
	}

	// 2. Walk up from the caller's file path.
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("archtest.FindRoot: runtime.Caller failed")
	}

	dir := filepath.Dir(file)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	t.Fatal("archtest.FindRoot: go.mod not found by walking up from", file)
	return ""
}
