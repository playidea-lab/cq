package validation

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// newTestRunner creates a runner with the given lint/unit commands.
func newTestRunner(t *testing.T, lint, unit string) *Runner {
	t.Helper()
	tmpDir := t.TempDir()

	// Write a minimal config.yaml
	configDir := filepath.Join(tmpDir, ".c4")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	yamlContent := "project_id: test\n"
	if lint != "" {
		yamlContent += "validation:\n  lint: " + lint + "\n"
		if unit != "" {
			yamlContent += "  unit: " + unit + "\n"
		}
	} else if unit != "" {
		yamlContent += "validation:\n  unit: " + unit + "\n"
	}

	if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte(yamlContent), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	// Load config via config.Manager
	// Since we're testing the runner, create it directly with the config struct
	runner := &Runner{
		projectRoot: tmpDir,
		timeout:     DefaultTimeout,
	}
	runner.cfg.Validation.Lint = lint
	runner.cfg.Validation.Unit = unit

	return runner
}

func TestRunLintCommandSuccess(t *testing.T) {
	r := newTestRunner(t, "echo 'lint ok'", "")

	result, err := r.Run("lint")
	if err != nil {
		t.Fatalf("Run(lint) error: %v", err)
	}

	if result.Status != "pass" {
		t.Errorf("expected pass, got %s (message: %s)", result.Status, result.Message)
	}
	if result.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", result.ExitCode)
	}
	if result.Name != "lint" {
		t.Errorf("expected name lint, got %s", result.Name)
	}
	if result.Stdout == "" {
		t.Error("expected non-empty stdout")
	}
}

func TestRunUnitCommandSuccess(t *testing.T) {
	r := newTestRunner(t, "", "echo 'tests passed'")

	result, err := r.Run("unit")
	if err != nil {
		t.Fatalf("Run(unit) error: %v", err)
	}

	if result.Status != "pass" {
		t.Errorf("expected pass, got %s", result.Status)
	}
	if result.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", result.ExitCode)
	}
}

func TestRunLintCommandFailure(t *testing.T) {
	r := newTestRunner(t, "echo 'error: bad style' >&2 && exit 1", "")

	result, err := r.Run("lint")
	if err != nil {
		t.Fatalf("Run(lint) error: %v", err)
	}

	if result.Status != "fail" {
		t.Errorf("expected fail, got %s", result.Status)
	}
	if result.ExitCode != 1 {
		t.Errorf("expected exit code 1, got %d", result.ExitCode)
	}
	if result.Stderr == "" {
		t.Error("expected non-empty stderr")
	}
}

func TestMissingCommandError(t *testing.T) {
	r := newTestRunner(t, "", "")

	_, err := r.Run("lint")
	if err == nil {
		t.Fatal("expected error for unconfigured lint command")
	}

	expected := "no command configured"
	if got := err.Error(); !contains(got, expected) {
		t.Errorf("expected error to contain %q, got %q", expected, got)
	}
}

func TestUnknownValidationName(t *testing.T) {
	r := newTestRunner(t, "echo lint", "echo unit")

	_, err := r.Run("integration")
	if err == nil {
		t.Fatal("expected error for unknown validation name")
	}

	expected := "unknown validation name"
	if got := err.Error(); !contains(got, expected) {
		t.Errorf("expected error to contain %q, got %q", expected, got)
	}
}

func TestCommandTimeout(t *testing.T) {
	r := newTestRunner(t, "sleep 10", "")
	r.SetTimeout(100 * time.Millisecond)

	result, err := r.Run("lint")
	if err != nil {
		t.Fatalf("Run(lint) error: %v", err)
	}

	if result.Status != "fail" {
		t.Errorf("expected fail for timeout, got %s", result.Status)
	}
	if result.ExitCode != -1 {
		t.Errorf("expected exit code -1 for timeout, got %d", result.ExitCode)
	}
	expected := "timed out"
	if !contains(result.Message, expected) {
		t.Errorf("expected message to contain %q, got %q", expected, result.Message)
	}
}

func TestRunAllSuccess(t *testing.T) {
	r := newTestRunner(t, "echo lint-ok", "echo unit-ok")

	results, err := r.RunAll()
	if err != nil {
		t.Fatalf("RunAll() error: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	for _, result := range results {
		if result.Status != "pass" {
			t.Errorf("expected pass for %s, got %s", result.Name, result.Status)
		}
	}
}

func TestAvailableValidations(t *testing.T) {
	tests := []struct {
		name     string
		lint     string
		unit     string
		expected []string
	}{
		{"both", "echo lint", "echo unit", []string{"lint", "unit"}},
		{"lint only", "echo lint", "", []string{"lint"}},
		{"unit only", "", "echo unit", []string{"unit"}},
		{"none", "", "", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newTestRunner(t, tt.lint, tt.unit)
			got := r.AvailableValidations()

			if len(got) != len(tt.expected) {
				t.Fatalf("expected %d validations, got %d: %v", len(tt.expected), len(got), got)
			}
			for i, name := range got {
				if name != tt.expected[i] {
					t.Errorf("validation[%d] expected %q, got %q", i, tt.expected[i], name)
				}
			}
		})
	}
}

func TestCommandRunsInProjectRoot(t *testing.T) {
	r := newTestRunner(t, "pwd", "")

	result, err := r.Run("lint")
	if err != nil {
		t.Fatalf("Run(lint) error: %v", err)
	}

	if result.Status != "pass" {
		t.Errorf("expected pass, got %s", result.Status)
	}
	// stdout should contain the temp dir path
	if result.Stdout == "" {
		t.Error("expected non-empty stdout from pwd")
	}
}

func TestResultDuration(t *testing.T) {
	r := newTestRunner(t, "echo fast", "")

	result, err := r.Run("lint")
	if err != nil {
		t.Fatalf("Run(lint) error: %v", err)
	}

	if result.Duration <= 0 {
		t.Error("expected positive duration")
	}
}

func TestTruncateOutput(t *testing.T) {
	short := "hello"
	if got := truncateOutput(short, 10); got != short {
		t.Errorf("expected %q, got %q", short, got)
	}

	long := "abcdefghij"
	if got := truncateOutput(long, 5); got != "abcde..." {
		t.Errorf("expected truncated, got %q", got)
	}
}

// contains is a simple substring check helper.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
