package main

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

// mcpFallbackCmd implements the MCP server with Go-to-Python fallback.
var mcpFallbackCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Start MCP server (with Python fallback)",
	Long: `Start the MCP server on stdin/stdout. By default, the Go-native
implementation is used. If it fails to start (missing dependencies,
runtime panic, etc.), the system falls back to the Python MCP server.

Set C4_FORCE_PYTHON=1 to always use the Python server.`,
	RunE: runMCPFallback,
}

func init() {
	rootCmd.AddCommand(mcpFallbackCmd)
}

// FallbackConfig controls the Go-to-Python fallback behavior.
type FallbackConfig struct {
	// ForcePython skips the Go server entirely.
	ForcePython bool
	// PythonCommand is the command to run the Python MCP server.
	PythonCommand string
	// PythonArgs are the arguments for the Python command.
	PythonArgs []string
	// GoServerFunc is the function that starts the Go MCP server.
	// Returns nil on clean exit, error on failure.
	GoServerFunc func() error
}

// DefaultFallbackConfig returns the default fallback configuration.
func DefaultFallbackConfig() *FallbackConfig {
	return &FallbackConfig{
		ForcePython:   os.Getenv("C4_FORCE_PYTHON") == "1",
		PythonCommand: "uv",
		PythonArgs:    []string{"run", "python", "-m", "c4.mcp"},
		GoServerFunc:  startGoMCPServer,
	}
}

func runMCPFallback(cmd *cobra.Command, args []string) error {
	cfg := DefaultFallbackConfig()
	return RunWithFallback(cfg)
}

// RunWithFallback attempts to run the Go MCP server, falling back to Python
// if the Go server fails or if ForcePython is set.
func RunWithFallback(cfg *FallbackConfig) error {
	// Check for force-Python mode
	if cfg.ForcePython {
		fmt.Fprintln(os.Stderr, "cq: C4_FORCE_PYTHON=1 set, using Python MCP server")
		return runPythonMCP(cfg)
	}

	// Try the Go server first
	err := safeRunGoServer(cfg)
	if err == nil {
		return nil
	}

	// Go server failed - fall back to Python
	fmt.Fprintf(os.Stderr, "cq: Go MCP server failed: %v\n", err)
	fmt.Fprintln(os.Stderr, "cq: Falling back to Python MCP server...")

	return runPythonMCP(cfg)
}

// safeRunGoServer wraps the Go server start in a panic recovery.
func safeRunGoServer(cfg *FallbackConfig) (retErr error) {
	defer func() {
		if r := recover(); r != nil {
			retErr = fmt.Errorf("panic: %v", r)
		}
	}()

	return cfg.GoServerFunc()
}

// startGoMCPServer starts the actual Go MCP server with all tools.
func startGoMCPServer() error {
	return runMCP()
}

// runPythonMCP starts the Python MCP server as a subprocess.
// It connects stdin/stdout/stderr of the subprocess to the parent process,
// effectively replacing the current process behavior.
func runPythonMCP(cfg *FallbackConfig) error {
	pythonCmd, err := findPythonMCP(cfg)
	if err != nil {
		return fmt.Errorf("failed to find Python MCP server: %w", err)
	}

	cmd := exec.Command(pythonCmd, cfg.PythonArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("Python MCP server exited with error: %w", err)
	}

	return nil
}

// findPythonMCP locates the Python command for running the MCP server.
func findPythonMCP(cfg *FallbackConfig) (string, error) {
	cmd := cfg.PythonCommand
	path, err := exec.LookPath(cmd)
	if err != nil {
		// Try alternatives
		for _, alt := range []string{"python3", "python"} {
			if altPath, altErr := exec.LookPath(alt); altErr == nil {
				// For direct python, adjust args to remove "run"
				cfg.PythonArgs = []string{"-m", "c4.mcp"}
				return altPath, nil
			}
		}
		return "", fmt.Errorf("neither %q nor python3/python found in PATH", cmd)
	}
	return path, nil
}
