package archtest_test

import (
	"strings"
	"testing"

	"github.com/changmin/c4-core/test/archtest"
)

// TestNoInternalImportCmd enforces that no internal/* package imports cmd/.
// The cmd package is the application entry point; it must not be imported by
// library code inside internal/.
func TestNoInternalImportCmd(t *testing.T) {
	t.Helper()
	root := archtest.FindRoot(t)
	pkgs := archtest.ParsePackages(root)

	var violations []string
	for _, pkg := range pkgs {
		if !strings.HasPrefix(pkg.PkgPath, "internal/") {
			continue
		}
		for _, fi := range pkg.Files {
			for _, imp := range fi.Imports {
				if strings.Contains(imp, "/cmd/") || imp == "github.com/changmin/c4-core/cmd" {
					violations = append(violations,
						fi.Path+": imports "+imp)
				}
			}
		}
	}

	if len(violations) > 0 {
		t.Errorf("internal/* must not import cmd/: %d violation(s)", len(violations))
		for _, v := range violations {
			t.Errorf("  %s", v)
		}
	}
}

// TestLeafNoHandlerImport enforces that the nine leaf packages do not import
// the handlers package (internal/mcp/handlers).  Leaf packages form the
// bottom of the dependency graph; importing a high-level orchestration package
// from them would create a cycle or tight coupling.
//
// Leaf packages: store, config, task, goast, dartast, secrets, mailbox,
// session, worker.
func TestLeafNoHandlerImport(t *testing.T) {
	t.Helper()
	root := archtest.FindRoot(t)
	pkgs := archtest.ParsePackages(root)

	leafPrefixes := []string{
		"internal/store",
		"internal/config",
		"internal/task",
		"internal/goast",
		"internal/dartast",
		"internal/secrets",
		"internal/mailbox",
		"internal/session",
		"internal/worker",
	}

	handlersPath := "github.com/changmin/c4-core/internal/mcp/handlers"

	var violations []string
	for _, pkg := range pkgs {
		isLeaf := false
		for _, lp := range leafPrefixes {
			if pkg.PkgPath == lp || strings.HasPrefix(pkg.PkgPath, lp+"/") {
				isLeaf = true
				break
			}
		}
		if !isLeaf {
			continue
		}
		for _, fi := range pkg.Files {
			for _, imp := range fi.Imports {
				if imp == handlersPath || strings.HasPrefix(imp, handlersPath+"/") {
					violations = append(violations,
						fi.Path+": imports "+imp)
				}
			}
		}
	}

	if len(violations) > 0 {
		t.Errorf("leaf packages must not import handlers: %d violation(s)", len(violations))
		for _, v := range violations {
			t.Errorf("  %s", v)
		}
	}
}

// TestStoreOnlyStdlib enforces that internal/store/*.go uses only standard
// library imports (no github.com/* dependencies).  The store package defines
// the core data types and Store interface; keeping it stdlib-only ensures it
// stays a true leaf in the dependency graph.
func TestStoreOnlyStdlib(t *testing.T) {
	t.Helper()
	root := archtest.FindRoot(t)
	pkgs := archtest.ParsePackages(root)

	var violations []string
	for _, pkg := range pkgs {
		if pkg.PkgPath != "internal/store" {
			continue
		}
		for _, fi := range pkg.Files {
			for _, imp := range fi.Imports {
				if strings.HasPrefix(imp, "github.com/") {
					violations = append(violations,
						fi.Path+": non-stdlib import "+imp)
				}
			}
		}
	}

	if len(violations) > 0 {
		t.Errorf("internal/store must only use stdlib: %d violation(s)", len(violations))
		for _, v := range violations {
			t.Errorf("  %s", v)
		}
	}
}

// TestDependencyMatrix validates that each tracked internal package imports
// only packages in its allowed set.  The allowedDeps baseline was derived from
// the actual import graph recorded in .c4/arch-recon/handlers-imports.txt.
//
// NOTE: When a new sub-package is added during Wave 1/2 refactoring, add it
// to the relevant slice below to keep this test green.
func TestDependencyMatrix(t *testing.T) {
	t.Helper()
	root := archtest.FindRoot(t)
	pkgs := archtest.ParsePackages(root)

	modPrefix := "github.com/changmin/c4-core/"

	// allowedDeps maps a PkgPath (relative to module root) to the set of
	// internal imports it is permitted to have.  Only packages explicitly
	// listed here are checked; unlisted packages are not constrained.
	// Add new sub-packages here when they are introduced.
	allowedDeps := map[string][]string{
		"internal/store": {
			// stdlib only — no internal imports allowed
		},
		"internal/config": {
			// config may only depend on third-party libraries (e.g. viper),
			// not on other internal packages.
		},
		"internal/task": {
			// task is a pure leaf — no internal imports.
		},
		"internal/goast": {
			// goast is a pure leaf — no internal imports.
		},
		"internal/dartast": {
			// dartast is a pure leaf — no internal imports.
		},
		"internal/secrets": {
			// secrets is a pure leaf — no internal imports.
		},
		"internal/mailbox": {
			// mailbox is a pure leaf — no internal imports.
		},
		"internal/session": {
			// session is a pure leaf — no internal imports.
		},
		"internal/worker": {
			// worker is a pure leaf — no internal imports.
		},
		"internal/cloud": {
			"internal/store",
			"internal/knowledge", // KnowledgeCloudClient implements knowledge.CloudSyncer
		},
		"internal/mcp/handlers/fileops": {
			// fileops depends only on internal/mcp (Registry)
			"internal/mcp",
		},
		"internal/mcp/handlers/gitops": {
			// gitops depends only on internal/mcp (Registry)
			"internal/mcp",
		},
		"internal/mcp/handlers/webcontent": {
			// webcontent depends on internal/mcp and internal/webcontent
			"internal/mcp",
			"internal/webcontent",
		},
		"internal/mcp/handlers": {
			"internal/persona",
			"internal/webcontent",
			"internal/workspace",
			"internal/cdp",
			"internal/chat",
			"internal/cloud",
			"internal/config",
			"internal/daemon",
			"internal/dartast",
			"internal/drive",
			"internal/eventbus",
			"internal/eventbus/pb",
			"internal/gate",
			"internal/goast",
			"internal/guard",
			"internal/hub",
			"internal/knowledge",
			"internal/llm",
			"internal/mailbox",
			"internal/mcp",
			"internal/mcp/handlers/artifacthandler",
			"internal/mcp/handlers/c1handler",
			"internal/mcp/handlers/cfghandler",
			"internal/mcp/handlers/exechandler",
			"internal/mcp/handlers/fileops",
			"internal/mcp/handlers/gitops",
			"internal/mcp/handlers/cdphandler",
			"internal/mcp/handlers/drivehandler",
			"internal/mcp/handlers/eventbushandler",
			"internal/mcp/handlers/gpuhandler",
			"internal/mcp/handlers/hubhandler",
			"internal/mcp/handlers/knowledgehandler",
			"internal/mcp/handlers/llmhandler",
			"internal/mcp/handlers/pophandler",
			"internal/mcp/handlers/mailhandler",
			"internal/mcp/handlers/skillevalhandler",
			"internal/mcp/handlers/notifyhandler",
			"internal/mcp/handlers/researchhandler",
			"internal/mcp/handlers/secrethandler",
			"internal/mcp/handlers/webcontent",
			"internal/observe",
			"internal/research",
			"internal/secrets",
			"internal/state",
			"internal/store",
			"internal/task",
			"internal/worker",
		},
		// Wave 1b sub-packages: cfghandler and secrethandler
		"internal/mcp/handlers/cfghandler": {
			"internal/config",
			"internal/mcp",
		},
		"internal/mcp/handlers/secrethandler": {
			"internal/mcp",
			"internal/secrets",
		},
		// Wave 1c sub-packages: mailhandler and artifacthandler
		"internal/mcp/handlers/mailhandler": {
			"internal/chat",
			"internal/mailbox",
			"internal/mcp",
		},
		"internal/mcp/handlers/artifacthandler": {
			"internal/mcp",
		},
		// Wave 2c sub-packages: c1handler, gpuhandler, knowledgehandler, researchhandler
		"internal/mcp/handlers/c1handler": {
			"internal/cloud",
			"internal/llm",
			"internal/mcp",
		},
		"internal/mcp/handlers/gpuhandler": {
			"internal/daemon",
			"internal/mcp",
		},
		"internal/mcp/handlers/knowledgehandler": {
			"internal/webcontent",
			"internal/eventbus",
			"internal/knowledge",
			"internal/llm",
			"internal/mcp",
		},
		"internal/mcp/handlers/researchhandler": {
			"internal/eventbus",
			"internal/knowledge",
			"internal/mcp",
			"internal/research",
		},
		"internal/mcp/handlers/pophandler": {
			"internal/knowledge",
			"internal/llm",
			"internal/mcp",
			"internal/pop",
		},
	}

	for checkPkg, allowed := range allowedDeps {
		allowedSet := make(map[string]bool, len(allowed))
		for _, a := range allowed {
			allowedSet[a] = true
		}

		for _, pkg := range pkgs {
			if pkg.PkgPath != checkPkg {
				continue
			}
			for _, fi := range pkg.Files {
				// Skip test files: _test.go imports follow different rules
				// (external test packages import the package under test).
				if strings.HasSuffix(fi.Path, "_test.go") {
					continue
				}
				for _, imp := range fi.Imports {
					if !strings.HasPrefix(imp, modPrefix) {
						continue // third-party or stdlib — not checked here
					}
					rel := strings.TrimPrefix(imp, modPrefix)
					if !allowedSet[rel] {
						t.Errorf("%s: unexpected internal import %q not in allowedDeps baseline; add it if intentional",
							checkPkg, rel)
					}
				}
			}
		}
	}
}

// TestSubpackageNoSQLiteStore enforces that handlers/ subpackages do NOT
// import the parent internal/mcp/handlers package.  Subpackages (fileops,
// gitops, webcontent, cfghandler, secrethandler, mailhandler, artifacthandler,
// etc.) should only depend on mcp.Registry and their own external deps — not
// on the parent handlers package which contains SQLiteStore and heavy coupling.
func TestSubpackageNoSQLiteStore(t *testing.T) {
	t.Helper()
	root := archtest.FindRoot(t)
	pkgs := archtest.ParsePackages(root)
	modPath := archtest.ModulePath(root)

	handlersPkg := modPath + "/internal/mcp/handlers"
	subpkgPrefix := "internal/mcp/handlers/"

	for _, pkg := range pkgs {
		if !strings.HasPrefix(pkg.PkgPath, subpkgPrefix) {
			continue
		}
		// This is a subpackage of handlers/
		for imp := range pkg.Imports {
			if imp == handlersPkg {
				t.Errorf("subpackage %s imports parent handlers package — forbidden (SQLiteStore coupling)", pkg.PkgPath)
			}
		}
	}
}

// TestSubpackageNoCrossImport enforces that handlers/ subpackages do NOT
// import each other (sibling imports forbidden).  Each subpackage must be
// independently composable via mcp.Registry; sibling coupling would recreate
// the same God-package problem the Wave 1/2 refactor aims to eliminate.
func TestSubpackageNoCrossImport(t *testing.T) {
	t.Helper()
	root := archtest.FindRoot(t)
	pkgs := archtest.ParsePackages(root)
	modPath := archtest.ModulePath(root)

	subpkgRelPrefix := "internal/mcp/handlers/"
	subpkgFullPrefix := modPath + "/internal/mcp/handlers/"

	for _, pkg := range pkgs {
		if !strings.HasPrefix(pkg.PkgPath, subpkgRelPrefix) {
			continue
		}
		// This is a subpackage — check it doesn't import other subpackages
		pkgFull := modPath + "/" + pkg.PkgPath
		for imp := range pkg.Imports {
			if imp != pkgFull && strings.HasPrefix(imp, subpkgFullPrefix) {
				t.Errorf("%s imports sibling subpackage %s — cross-import forbidden", pkg.PkgPath, imp)
			}
		}
	}
}

// TestHandlersNoCmd enforces that internal/mcp/handlers does not import any
// cmd/ package.  The handlers layer is library code; it must be usable without
// pulling in the application entry-point binary.
func TestHandlersNoCmd(t *testing.T) {
	t.Helper()
	root := archtest.FindRoot(t)
	pkgs := archtest.ParsePackages(root)

	var violations []string
	for _, pkg := range pkgs {
		if !strings.HasPrefix(pkg.PkgPath, "internal/mcp/handlers") {
			continue
		}
		for _, fi := range pkg.Files {
			for _, imp := range fi.Imports {
				if strings.Contains(imp, "/cmd/") || imp == "github.com/changmin/c4-core/cmd" {
					violations = append(violations,
						fi.Path+": imports "+imp)
				}
			}
		}
	}

	if len(violations) > 0 {
		t.Errorf("internal/mcp/handlers must not import cmd/: %d violation(s)", len(violations))
		for _, v := range violations {
			t.Errorf("  %s", v)
		}
	}
}
