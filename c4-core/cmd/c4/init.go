package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"github.com/spf13/cobra"
)

// Tool launcher commands: c4 claude, c4 codex, c4 cursor
var (
	claudeCmd = &cobra.Command{
		Use:   "claude",
		Short: "Init C4 project and launch Claude Code",
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, args []string) error { return initAndLaunch("claude") },
	}
	codexCmd = &cobra.Command{
		Use:   "codex",
		Short: "Init C4 project and launch Codex CLI",
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, args []string) error { return initAndLaunch("codex") },
	}
	cursorCmd = &cobra.Command{
		Use:   "cursor",
		Short: "Init C4 project and launch Cursor",
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, args []string) error { return initAndLaunch("cursor") },
	}
)

func init() {
	rootCmd.AddCommand(claudeCmd, codexCmd, cursorCmd)
}

// initAndLaunch initializes the C4 project and launches the AI tool.
func initAndLaunch(tool string) error {
	dir := projectDir

	// 1. Create .c4/ directory structure
	dirs := []string{
		filepath.Join(dir, ".c4"),
		filepath.Join(dir, ".c4", "knowledge", "docs"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			return fmt.Errorf("creating directory %s: %w", d, err)
		}
	}
	fmt.Fprintln(os.Stderr, "c4: .c4/ directory initialized")

	// 2. Create/update .mcp.json
	if err := setupMCPConfig(dir); err != nil {
		return fmt.Errorf("setting up .mcp.json: %w", err)
	}

	// 3. Launch AI tool
	return launchTool(tool, dir)
}

// setupMCPConfig creates or updates .mcp.json with the C4 MCP server entry.
func setupMCPConfig(dir string) error {
	mcpPath := filepath.Join(dir, ".mcp.json")

	binPath, err := findC4Binary()
	if err != nil {
		return err
	}

	c4Entry := map[string]any{
		"type":    "stdio",
		"command": binPath,
		"args":    []string{"mcp", "--dir", dir},
		"env": map[string]string{
			"C4_PROJECT_ROOT": dir,
		},
	}

	// Read existing .mcp.json or create new
	var config map[string]any
	if data, readErr := os.ReadFile(mcpPath); readErr == nil {
		if json.Unmarshal(data, &config) != nil {
			config = nil
		}
	}
	if config == nil {
		config = map[string]any{}
	}

	servers, ok := config["mcpServers"].(map[string]any)
	if !ok {
		servers = map[string]any{}
	}
	servers["c4"] = c4Entry
	config["mcpServers"] = servers

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}
	data = append(data, '\n')

	if err := os.WriteFile(mcpPath, data, 0644); err != nil {
		return fmt.Errorf("writing .mcp.json: %w", err)
	}

	fmt.Fprintln(os.Stderr, "c4: .mcp.json configured")
	return nil
}

// findC4Binary locates the c4 Go binary path for .mcp.json configuration.
func findC4Binary() (string, error) {
	binPath, err := os.Executable()
	if err == nil {
		binPath, err = filepath.EvalSymlinks(binPath)
		if err == nil {
			return binPath, nil
		}
	}
	if p, err := exec.LookPath("c4"); err == nil {
		return filepath.Abs(p)
	}
	return "", fmt.Errorf("cannot determine c4 binary path")
}

// launchTool launches the specified AI coding tool, replacing the current process.
func launchTool(tool, dir string) error {
	var toolCmd string
	var toolArgs []string

	switch tool {
	case "claude":
		toolCmd = "claude"
		toolArgs = []string{"claude"}
	case "codex":
		toolCmd = "codex"
		toolArgs = []string{"codex"}
	case "cursor":
		toolCmd = "cursor"
		toolArgs = []string{"cursor", dir}
	default:
		return fmt.Errorf("unknown tool: %s (supported: claude, codex, cursor)", tool)
	}

	toolPath, err := exec.LookPath(toolCmd)
	if err != nil {
		return fmt.Errorf("%s not found in PATH (install it first): %w", toolCmd, err)
	}

	fmt.Fprintf(os.Stderr, "c4: launching %s...\n", tool)
	return syscall.Exec(toolPath, toolArgs, os.Environ())
}
