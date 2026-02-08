package handlers

import (
	"encoding/json"
	"os"
	"os/exec"
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

	// Filter if names specified
	var toRun []validationDef
	if len(args.Names) > 0 {
		nameSet := make(map[string]bool)
		for _, n := range args.Names {
			nameSet[n] = true
		}
		for _, v := range available {
			if nameSet[v.Name] {
				toRun = append(toRun, v)
			}
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
