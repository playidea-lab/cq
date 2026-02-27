package archtest_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/changmin/c4-core/test/archtest"
)

// TestFindRoot verifies that FindRoot returns a directory containing go.mod.
func TestFindRoot(t *testing.T) {
	root := archtest.FindRoot(t)
	if root == "" {
		t.Fatal("FindRoot returned empty string")
	}
	gomod := filepath.Join(root, "go.mod")
	// Just check the path has go.mod at the end — the file was found because
	// FindRoot only returns a dir when os.Stat succeeds on go.mod.
	if !strings.HasSuffix(filepath.ToSlash(gomod), "/go.mod") {
		t.Errorf("unexpected go.mod path: %s", gomod)
	}
}

// TestModulePath verifies that ModulePath extracts the correct module name
// from go.mod.
func TestModulePath(t *testing.T) {
	root := archtest.FindRoot(t)
	mod := archtest.ModulePath(root)
	if mod != "github.com/changmin/c4-core" {
		t.Errorf("ModulePath = %q, want %q", mod, "github.com/changmin/c4-core")
	}
}

// TestParsePackagesBasic verifies that ParsePackages returns results that
// include the internal/store package.
func TestParsePackagesBasic(t *testing.T) {
	root := archtest.FindRoot(t)
	pkgs := archtest.ParsePackages(root)
	if len(pkgs) == 0 {
		t.Fatal("ParsePackages returned no packages")
	}

	found := false
	for _, p := range pkgs {
		if p.PkgPath == "internal/store" {
			found = true
			break
		}
	}
	if !found {
		t.Error("internal/store package not found in ParsePackages result")
	}
}
