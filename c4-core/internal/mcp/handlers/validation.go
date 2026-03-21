package handlers

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/changmin/c4-core/internal/config"
	"github.com/changmin/c4-core/internal/eventbus"
	"github.com/changmin/c4-core/internal/mcp"
)

// validationEventPub holds the optional EventBus publisher for validation events.
var validationEventPub eventbus.Publisher

// SetValidationEventBus sets the EventBus publisher for validation handlers.
func SetValidationEventBus(pub eventbus.Publisher) {
	validationEventPub = pub
}

// validationCfg holds the optional config-based validation command overrides.
var validationCfg *config.ValidationConfig

// SetValidationConfig sets config at init-time (written once before first handler call).
// Go memory model guarantees visibility for single-threaded MCP init.
func SetValidationConfig(cfg *config.ValidationConfig) {
	validationCfg = cfg
}

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
	Dir     string // working directory override (empty = rootDir)
}

func handleRunValidation(rootDir string, rawArgs json.RawMessage) (any, error) {
	var args runValidationArgs
	if len(rawArgs) > 0 {
		if err := json.Unmarshal(rawArgs, &args); err != nil {
			return nil, fmt.Errorf("parsing arguments: %w", err)
		}
	}

	// Detect available validations (config-based overrides take priority)
	available := detectValidationsWithConfig(rootDir)

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
		if v.Dir != "" {
			cmd.Dir = v.Dir
		} else {
			cmd.Dir = rootDir
		}
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

	// Publish validation events
	if validationEventPub != nil {
		for _, r := range results {
			evType := "validation.passed"
			if !r.Passed {
				evType = "validation.failed"
			}
			data, _ := json.Marshal(map[string]any{
				"name":    r.Name,
				"passed":  r.Passed,
				"elapsed": r.Elapsed,
			})
			validationEventPub.PublishAsync(evType, "c4.validation", data, "")
		}
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

// detectValidationsWithConfig returns config-defined validations if any are set,
// otherwise falls back to detectValidations auto-detection.
// shell quoting is not supported: use strings.Fields (space-split only).
func detectValidationsWithConfig(rootDir string) []validationDef {
	if validationCfg != nil {
		var defs []validationDef
		if validationCfg.Lint != "" {
			parts := strings.Fields(validationCfg.Lint)
			if len(parts) > 0 {
				defs = append(defs, validationDef{Name: "lint", Command: parts[0], Args: parts[1:]})
			}
		}
		if validationCfg.Unit != "" {
			parts := strings.Fields(validationCfg.Unit)
			if len(parts) > 0 {
				defs = append(defs, validationDef{Name: "unit", Command: parts[0], Args: parts[1:]})
			}
		}
		if len(defs) > 0 {
			return defs
		}
	}
	return detectValidations(rootDir)
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
			Dir:     rootDir + "/c4-core",
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

	return defs
}

func fileExists(rootDir, relPath string) bool {
	path := rootDir + "/" + relPath
	_, err := os.Stat(path)
	return err == nil
}
