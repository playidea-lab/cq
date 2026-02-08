// Package validation runs project validation commands defined in .c4/config.yaml.
//
// The runner reads validation commands from the config's validations section
// and executes them via os/exec, capturing stdout, stderr, and exit code.
//
// Usage:
//
//	r := validation.NewRunner(configManager, projectRoot)
//	result, err := r.Run("lint")
package validation

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/changmin/c4-core/internal/config"
)

// DefaultTimeout is the default command execution timeout.
const DefaultTimeout = 30 * time.Second

// Result holds the outcome of a validation run.
type Result struct {
	Name     string `json:"name"`
	Status   string `json:"status"` // "pass" or "fail"
	Message  string `json:"message"`
	ExitCode int    `json:"exit_code"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	Duration time.Duration `json:"duration"`
}

// Runner executes validation commands from config.
type Runner struct {
	cfg         config.C4Config
	projectRoot string
	timeout     time.Duration
}

// NewRunner creates a validation runner using config and project root.
func NewRunner(mgr *config.Manager, projectRoot string) *Runner {
	return &Runner{
		cfg:         mgr.GetConfig(),
		projectRoot: projectRoot,
		timeout:     DefaultTimeout,
	}
}

// SetTimeout overrides the default command timeout.
func (r *Runner) SetTimeout(d time.Duration) {
	r.timeout = d
}

// Run executes a named validation command and returns the result.
//
// The validation name must match a key in the config's validation section
// (e.g., "lint", "unit"). Returns an error if the validation name is
// unknown or has no command configured.
func (r *Runner) Run(name string) (*Result, error) {
	command, err := r.resolveCommand(name)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	return r.execute(ctx, name, command)
}

// RunAll executes all configured validations and returns their results.
func (r *Runner) RunAll() ([]*Result, error) {
	var results []*Result
	names := r.AvailableValidations()

	for _, name := range names {
		result, err := r.Run(name)
		if err != nil {
			return results, fmt.Errorf("validation %q: %w", name, err)
		}
		results = append(results, result)
	}

	return results, nil
}

// AvailableValidations returns the names of all configured validations.
func (r *Runner) AvailableValidations() []string {
	var names []string
	if r.cfg.Validation.Lint != "" {
		names = append(names, "lint")
	}
	if r.cfg.Validation.Unit != "" {
		names = append(names, "unit")
	}
	return names
}

// resolveCommand looks up the command string for a validation name.
func (r *Runner) resolveCommand(name string) (string, error) {
	switch name {
	case "lint":
		if r.cfg.Validation.Lint == "" {
			return "", fmt.Errorf("validation %q: no command configured", name)
		}
		return r.cfg.Validation.Lint, nil
	case "unit":
		if r.cfg.Validation.Unit == "" {
			return "", fmt.Errorf("validation %q: no command configured", name)
		}
		return r.cfg.Validation.Unit, nil
	default:
		return "", fmt.Errorf("validation %q: unknown validation name", name)
	}
}

// execute runs a shell command and captures its output.
func (r *Runner) execute(ctx context.Context, name, command string) (*Result, error) {
	start := time.Now()

	// Use shell to support pipes, redirects, etc.
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Dir = r.projectRoot

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	duration := time.Since(start)

	result := &Result{
		Name:     name,
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		Duration: duration,
	}

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			result.Status = "fail"
			result.ExitCode = -1
			result.Message = fmt.Sprintf("command timed out after %s", r.timeout)
			return result, nil
		}

		if exitErr, ok := err.(*exec.ExitError); ok {
			result.Status = "fail"
			result.ExitCode = exitErr.ExitCode()
			result.Message = truncateOutput(stderr.String(), 500)
			if result.Message == "" {
				result.Message = fmt.Sprintf("exited with code %d", result.ExitCode)
			}
			return result, nil
		}

		// Command not found or other exec error
		return nil, fmt.Errorf("validation %q: %w", name, err)
	}

	result.Status = "pass"
	result.ExitCode = 0
	result.Message = "ok"
	return result, nil
}

// truncateOutput trims output to maxLen characters.
func truncateOutput(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
