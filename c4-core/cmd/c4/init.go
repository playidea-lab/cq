package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
)

//go:embed templates/claude_md.tmpl
var claudeMDTemplate string

// builtinC4Root is set at build time via -ldflags for skill deployment.
var builtinC4Root string

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

	// 3. Create/update CLAUDE.md with C4 overrides
	if err := setupClaudeMD(dir); err != nil {
		fmt.Fprintf(os.Stderr, "c4: warning: CLAUDE.md setup failed: %v\n", err)
	}

	// 4. Deploy C4 skills (symlinks from C4 source)
	if err := setupSkills(dir); err != nil {
		fmt.Fprintf(os.Stderr, "c4: warning: skills setup failed: %v\n", err)
	}

	// 5. Launch AI tool
	return launchTool(tool, dir)
}

// setupClaudeMD creates or updates CLAUDE.md with C4 override rules.
// If CLAUDE.md already contains the C4 marker, it is left unchanged.
// If CLAUDE.md exists without the marker, the C4 section is prepended.
// If CLAUDE.md does not exist, a new one is created from template.
func setupClaudeMD(dir string) error {
	claudePath := filepath.Join(dir, "CLAUDE.md")
	const marker = "## CRITICAL: C4 Overrides"

	// Check for AGENTS.md symlink (C4 repo itself — skip template deployment)
	if target, err := os.Readlink(claudePath); err == nil {
		if strings.Contains(target, "AGENTS.md") {
			fmt.Fprintln(os.Stderr, "c4: CLAUDE.md is AGENTS.md symlink (C4 repo), skipping template")
			return nil
		}
	}

	if data, err := os.ReadFile(claudePath); err == nil {
		content := string(data)
		if strings.Contains(content, marker) {
			fmt.Fprintln(os.Stderr, "c4: CLAUDE.md already has C4 overrides")
			return nil
		}
		// Prepend C4 section to existing CLAUDE.md
		newContent := claudeMDTemplate + "\n\n---\n\n<!-- Original CLAUDE.md content below -->\n\n" + content
		if err := os.WriteFile(claudePath, []byte(newContent), 0644); err != nil {
			return fmt.Errorf("updating CLAUDE.md: %w", err)
		}
		fmt.Fprintln(os.Stderr, "c4: CLAUDE.md updated with C4 overrides (existing content preserved)")
		return nil
	}

	// Create new CLAUDE.md from template
	if err := os.WriteFile(claudePath, []byte(claudeMDTemplate), 0644); err != nil {
		return fmt.Errorf("creating CLAUDE.md: %w", err)
	}
	fmt.Fprintln(os.Stderr, "c4: CLAUDE.md created")
	return nil
}

// setupSkills deploys C4 skills to the target project via symlinks.
// Each skill directory is symlinked from {c4Root}/.claude/skills/{name}/ to
// {dir}/.claude/skills/{name}/. Existing skills are not overwritten.
func setupSkills(dir string) error {
	c4Root, err := findC4Root()
	if err != nil {
		fmt.Fprintln(os.Stderr, "c4: skills setup skipped (C4 source root not found)")
		return nil
	}

	sourceSkillsDir := filepath.Join(c4Root, ".claude", "skills")
	entries, err := os.ReadDir(sourceSkillsDir)
	if err != nil {
		return nil // No skills to deploy
	}

	targetSkillsDir := filepath.Join(dir, ".claude", "skills")
	if err := os.MkdirAll(targetSkillsDir, 0755); err != nil {
		return fmt.Errorf("creating .claude/skills/: %w", err)
	}

	count := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		target := filepath.Join(targetSkillsDir, entry.Name())
		// Skip if already exists (file, dir, or symlink)
		if _, err := os.Lstat(target); err == nil {
			continue
		}
		source := filepath.Join(sourceSkillsDir, entry.Name())
		if err := os.Symlink(source, target); err != nil {
			fmt.Fprintf(os.Stderr, "c4: warning: symlink skill %s: %v\n", entry.Name(), err)
			continue
		}
		count++
	}
	if count > 0 {
		fmt.Fprintf(os.Stderr, "c4: %d skills deployed (symlinked from %s)\n", count, c4Root)
	} else {
		fmt.Fprintln(os.Stderr, "c4: skills up to date")
	}
	return nil
}

// findC4Root locates the C4 source repository root directory.
// Search order: C4_SOURCE_ROOT env, builtinC4Root (ldflags), ~/.c4-install-path,
// then derivation from binary path (c4-core/bin/c4 → repo root).
func findC4Root() (string, error) {
	// 1. Environment variable
	if root := os.Getenv("C4_SOURCE_ROOT"); root != "" {
		if hasSkills(root) {
			return root, nil
		}
	}

	// 2. Build-time embedded via ldflags
	if builtinC4Root != "" && hasSkills(builtinC4Root) {
		return builtinC4Root, nil
	}

	// 3. ~/.c4-install-path
	if home, err := os.UserHomeDir(); err == nil {
		if data, err := os.ReadFile(filepath.Join(home, ".c4-install-path")); err == nil {
			root := strings.TrimSpace(string(data))
			if hasSkills(root) {
				return root, nil
			}
		}
	}

	// 4. Derive from binary path: {root}/c4-core/bin/c4 → {root}
	if binPath, err := os.Executable(); err == nil {
		if binPath, err = filepath.EvalSymlinks(binPath); err == nil {
			// Try c4-core/bin/c4 layout
			root := filepath.Dir(filepath.Dir(filepath.Dir(binPath)))
			if hasSkills(root) {
				return root, nil
			}
		}
	}

	return "", fmt.Errorf("C4 source root not found")
}

// hasSkills checks if the given directory has .claude/skills/.
func hasSkills(root string) bool {
	info, err := os.Stat(filepath.Join(root, ".claude", "skills"))
	return err == nil && info.IsDir()
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
