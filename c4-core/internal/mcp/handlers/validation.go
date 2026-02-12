package handlers

import (
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/changmin/c4-core/internal/mcp"
)

// RegisterValidationHandlers registers the validation runner tool.
func RegisterValidationHandlers(reg *mcp.Registry, rootDir string) {
	reg.Register(mcp.ToolSchema{
		Name:        "c4_run_validation",
		Description: "Run validation commands (tests, linters, type checks)",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"names": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Validation names to run (e.g. 'pytest', 'go-test', 'ruff'). Empty = run all.",
				},
			},
		},
	}, func(args json.RawMessage) (any, error) {
		return handleRunValidation(rootDir, args)
	})
}

type runValidationArgs struct {
	Names []string `json:"names"`
}

type validationDef struct {
	Name    string
	Command string
	Args    []string
}

func handleRunValidation(rootDir string, rawArgs json.RawMessage) (any, error) {
	var args runValidationArgs
	if len(rawArgs) > 0 {
		_ = json.Unmarshal(rawArgs, &args)
	}

	// Detect available validations
	available := detectValidations(rootDir)

	// Filter if names specified (supports aliases like lint/unit/test).
	var toRun []validationDef
	if len(args.Names) > 0 {
		byName := make(map[string]validationDef, len(available))
		for _, v := range available {
			byName[strings.ToLower(v.Name)] = v
		}
		aliasMap := buildValidationAliasMap(available)
		selected := make(map[string]bool)
		var unmatched []string

		for _, requested := range args.Names {
			key := strings.ToLower(strings.TrimSpace(requested))
			if key == "" {
				continue
			}

			name := key
			if _, ok := byName[name]; !ok {
				if mapped, ok := aliasMap[name]; ok {
					name = mapped
				}
			}

			if v, ok := byName[name]; ok {
				if !selected[name] {
					toRun = append(toRun, v)
					selected[name] = true
				}
			} else {
				unmatched = append(unmatched, requested)
			}
		}

		if len(toRun) == 0 {
			availableNames := make([]string, 0, len(available))
			for _, v := range available {
				availableNames = append(availableNames, v.Name)
			}
			return map[string]any{
				"all_passed":      false,
				"results":         []any{},
				"count":           0,
				"error":           "no matching validations found",
				"requested_names": args.Names,
				"unmatched_names": unmatched,
				"available_names": availableNames,
			}, nil
		}
	} else {
		toRun = available
	}

	type result struct {
		Name    string  `json:"name"`
		Passed  bool    `json:"passed"`
		Output  string  `json:"output"`
		Elapsed float64 `json:"elapsed_seconds"`
	}

	var results []result
	allPassed := true

	for _, v := range toRun {
		start := time.Now()
		cmd := exec.Command(v.Command, v.Args...)
		cmd.Dir = rootDir
		out, err := cmd.CombinedOutput()
		elapsed := time.Since(start).Seconds()

		passed := err == nil
		if !passed {
			allPassed = false
		}

		// Truncate long output
		output := string(out)
		if len(output) > 2000 {
			output = output[:1000] + "\n...(truncated)...\n" + output[len(output)-1000:]
		}

		results = append(results, result{
			Name:    v.Name,
			Passed:  passed,
			Output:  output,
			Elapsed: elapsed,
		})
	}

	return map[string]any{
		"all_passed": allPassed,
		"results":    results,
		"count":      len(results),
	}, nil
}

func buildValidationAliasMap(available []validationDef) map[string]string {
	has := make(map[string]bool, len(available))
	for _, v := range available {
		has[strings.ToLower(v.Name)] = true
	}

	alias := map[string]string{
		"ruff-check": "ruff",
		"go":         "go-test",
		"cargo":      "cargo-check",
	}

	if has["ruff"] {
		alias["lint"] = "ruff"
		alias["linter"] = "ruff"
	}
	if has["pytest"] {
		alias["unit"] = "pytest"
		alias["test"] = "pytest"
		alias["tests"] = "pytest"
	} else if has["go-test"] {
		alias["unit"] = "go-test"
		alias["test"] = "go-test"
		alias["tests"] = "go-test"
	}

	return alias
}

func detectValidations(rootDir string) []validationDef {
	var defs []validationDef

	// Python tests
	if fileExists(rootDir, "pyproject.toml") {
		defs = append(defs, validationDef{
			Name:    "pytest",
			Command: "uv",
			Args:    []string{"run", "pytest", "tests/unit/", "-x", "-q"},
		})
	}

	// Go tests
	if fileExists(rootDir, "c4-core/go.mod") {
		defs = append(defs, validationDef{
			Name:    "go-test",
			Command: "go",
			Args:    []string{"test", "./..."},
		})
	}

	// Ruff (Python linter)
	if fileExists(rootDir, "pyproject.toml") {
		defs = append(defs, validationDef{
			Name:    "ruff",
			Command: "uv",
			Args:    []string{"run", "ruff", "check", "c4/"},
		})
	}

	// Cargo (Rust)
	if fileExists(rootDir, "c1/src-tauri/Cargo.toml") {
		defs = append(defs, validationDef{
			Name:    "cargo-check",
			Command: "cargo",
			Args:    []string{"check", "--manifest-path", "c1/src-tauri/Cargo.toml"},
		})
	}

	return defs
}

func fileExists(rootDir, relPath string) bool {
	path := rootDir + "/" + relPath
	_, err := os.Stat(path)
	return err == nil
}
