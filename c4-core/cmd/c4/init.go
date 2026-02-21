package main

import (
	"bufio"
	"crypto/sha256"
	_ "embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
)

//go:embed templates/claude_md.tmpl
var claudeMDTemplate string

//go:embed templates/hooks/c4-bash-security-hook.sh
var hookShContent string

//go:embed templates/config.yaml
var defaultConfigYAML string

// builtinC4Root is set at build time via -ldflags for skill deployment.
var builtinC4Root string

// initTier is the build tier written to .c4/config.yaml on project init.
// Valid values: "solo", "connected", "full" (default).
var initTier string

// Tool launcher commands: cq claude, cq codex, cq cursor
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
	// Register --tier flag on all init-related commands and the root command.
	for _, cmd := range []*cobra.Command{rootCmd, claudeCmd, codexCmd, cursorCmd} {
		cmd.Flags().StringVar(&initTier, "tier", "", "build tier: solo|connected|full (written to .c4/config.yaml)")
	}
	rootCmd.AddCommand(claudeCmd, codexCmd, cursorCmd)
}

// writeDefaultConfig writes the embedded default config.yaml to .c4/config.yaml
// if the file does not already exist. Existing configs are never overwritten.
func writeDefaultConfig(dir string) error {
	configPath := filepath.Join(dir, ".c4", "config.yaml")
	if _, err := os.Stat(configPath); err == nil {
		return nil // already exists — do not overwrite
	}
	if err := os.WriteFile(configPath, []byte(defaultConfigYAML), 0644); err != nil {
		return fmt.Errorf("writing default config.yaml: %w", err)
	}
	fmt.Fprintln(os.Stderr, "cq: .c4/config.yaml created (default)")
	return nil
}

// validTiers is the set of accepted --tier values.
var validTiers = map[string]bool{"solo": true, "connected": true, "full": true}

// writeTierConfig writes the tier key to .c4/config.yaml if --tier was specified.
// It creates the file if it does not exist, or appends/updates the tier line.
func writeTierConfig(dir, tier string) error {
	if tier == "" {
		return nil
	}
	if !validTiers[tier] {
		return fmt.Errorf("invalid tier %q: must be solo, connected, or full", tier)
	}

	configPath := filepath.Join(dir, ".c4", "config.yaml")

	// Read existing config or start with empty content.
	var existing string
	if data, err := os.ReadFile(configPath); err == nil {
		existing = string(data)
	}

	// Replace existing tier line or append one.
	const prefix = "tier: "
	lines := strings.Split(existing, "\n")
	found := false
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), prefix) {
			lines[i] = prefix + tier
			found = true
			break
		}
	}
	if !found {
		// Append tier line (ensure no double blank line at top)
		if existing != "" && !strings.HasSuffix(existing, "\n") {
			existing += "\n"
		}
		existing += prefix + tier + "\n"
		if err := os.WriteFile(configPath, []byte(existing), 0644); err != nil {
			return fmt.Errorf("writing tier to config.yaml: %w", err)
		}
		fmt.Fprintf(os.Stderr, "cq: tier=%s written to .c4/config.yaml\n", tier)
		return nil
	}

	content := strings.Join(lines, "\n")
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("updating tier in config.yaml: %w", err)
	}
	fmt.Fprintf(os.Stderr, "cq: tier=%s updated in .c4/config.yaml\n", tier)
	return nil
}

// confirmGlobalChanges prompts the user before modifying global files
// (~/.claude/hooks and ~/.claude/settings.json). Returns true if the user
// accepts or if yesAll is set; false if the user declines (skip the step).
// The prompt is written to stderr; the response is read from stdin.
func confirmGlobalChanges(homeDir string) bool {
	if yesAll {
		return true
	}

	hookPath := filepath.Join(homeDir, ".claude", "hooks", "c4-bash-security-hook.sh")
	settingsPath := filepath.Join(homeDir, ".claude", "settings.json")

	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "cq: The following GLOBAL files will be created or modified:")
	fmt.Fprintf(os.Stderr, "  1. %s\n", hookPath)
	fmt.Fprintf(os.Stderr, "  2. %s\n", settingsPath)
	fmt.Fprintln(os.Stderr, "  These hooks review Bash commands for safety before execution.")
	fmt.Fprint(os.Stderr, "Allow? [y/N] ")

	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		answer := strings.TrimSpace(strings.ToLower(scanner.Text()))
		if answer == "y" || answer == "yes" {
			return true
		}
	}

	fmt.Fprintln(os.Stderr, "cq: skipping global hook installation (run with --yes to suppress this prompt)")
	return false
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
	fmt.Fprintln(os.Stderr, "cq: .c4/ directory initialized")

	// 1b. Write default config.yaml if it doesn't exist yet.
	if err := writeDefaultConfig(dir); err != nil {
		return fmt.Errorf("writing default config: %w", err)
	}

	// 1c. Write tier to .c4/config.yaml if --tier was specified
	if err := writeTierConfig(dir, initTier); err != nil {
		return fmt.Errorf("writing tier config: %w", err)
	}

	// 2. Create/update .mcp.json
	if err := setupMCPConfig(dir); err != nil {
		return fmt.Errorf("setting up .mcp.json: %w", err)
	}

	// 3. Create/update CLAUDE.md with C4 overrides
	if err := setupClaudeMD(dir); err != nil {
		fmt.Fprintf(os.Stderr, "cq: warning: CLAUDE.md setup failed: %v\n", err)
	}

	// 4. Deploy C4 skills (symlinks from C4 source)
	if err := setupSkills(dir); err != nil {
		fmt.Fprintf(os.Stderr, "cq: warning: skills setup failed: %v\n", err)
	}

	// 5. Install global hooks to ~/.claude/hooks/ (requires user confirmation)
	if homeDir, err := os.UserHomeDir(); err == nil {
		if confirmGlobalChanges(homeDir) {
			if err := setupGlobalHooks(homeDir); err != nil {
				fmt.Fprintf(os.Stderr, "cq: warning: hooks setup failed: %v\n", err)
			}
		}
	} else {
		fmt.Fprintf(os.Stderr, "cq: warning: could not determine home dir: %v\n", err)
	}

	// 6. Codex-specific setup
	if tool == "codex" {
		if err := setupCodexConfig(dir); err != nil {
			fmt.Fprintf(os.Stderr, "cq: warning: codex config setup failed: %v\n", err)
		}
		if err := setupCodexAgents(dir); err != nil {
			fmt.Fprintf(os.Stderr, "cq: warning: codex agents setup failed: %v\n", err)
		}
	}

	// 6b. Cursor-specific setup: .cursor/mcp.json so Cursor loads C4 MCP on start
	if tool == "cursor" {
		if err := setupCursorMCPConfig(dir); err != nil {
			return fmt.Errorf("setting up .cursor/mcp.json: %w", err)
		}
	}

	// 7. Launch AI tool
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
			fmt.Fprintln(os.Stderr, "cq: CLAUDE.md is AGENTS.md symlink (C4 repo), skipping template")
			return nil
		}
	}

	if data, err := os.ReadFile(claudePath); err == nil {
		content := string(data)
		if strings.Contains(content, marker) {
			fmt.Fprintln(os.Stderr, "cq: CLAUDE.md already has C4 overrides")
			return nil
		}
		// Prepend C4 section to existing CLAUDE.md
		newContent := claudeMDTemplate + "\n\n---\n\n<!-- Original CLAUDE.md content below -->\n\n" + content
		if err := os.WriteFile(claudePath, []byte(newContent), 0644); err != nil {
			return fmt.Errorf("updating CLAUDE.md: %w", err)
		}
		fmt.Fprintln(os.Stderr, "cq: CLAUDE.md updated with C4 overrides (existing content preserved)")
		return nil
	}

	// Create new CLAUDE.md from template
	if err := os.WriteFile(claudePath, []byte(claudeMDTemplate), 0644); err != nil {
		return fmt.Errorf("creating CLAUDE.md: %w", err)
	}
	fmt.Fprintln(os.Stderr, "cq: CLAUDE.md created")
	return nil
}

// setupSkills deploys C4 skills to the target project.
// Priority order:
//  1. findC4Root() succeeds → symlink mode (development)
//  2. EmbeddedSkillsFS != nil → extract to ~/.c4/skills/ (installed binary)
//  3. Neither → skip gracefully
func setupSkills(dir string) error {
	// 1. Development mode: symlink from source root
	if c4Root, err := findC4Root(); err == nil {
		return setupSkillsSymlink(dir, c4Root)
	}

	// 2. Embedded mode: extract to ~/.c4/skills/
	if EmbeddedSkillsFS != nil {
		home, err := os.UserHomeDir()
		if err != nil {
			slog.Warn("skills embed: cannot determine home dir", "err", err)
			return nil
		}
		skillsDir := filepath.Join(home, ".c4", "skills")
		if err := extractEmbeddedSkills(skillsDir); err != nil {
			slog.Warn("skills embed: extraction failed", "err", err)
			return nil
		}
		return setupSkillsFromExtracted(dir, skillsDir)
	}

	// 3. Neither available
	slog.Info("skills not embedded, skipping")
	return nil
}

// setupSkillsSymlink creates symlinks from c4Root skills to the project.
func setupSkillsSymlink(dir, c4Root string) error {
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
			fmt.Fprintf(os.Stderr, "cq: warning: symlink skill %s: %v\n", entry.Name(), err)
			continue
		}
		count++
	}
	if count > 0 {
		fmt.Fprintf(os.Stderr, "cq: %d skills deployed (symlinked from %s)\n", count, c4Root)
	} else {
		fmt.Fprintln(os.Stderr, "cq: skills up to date")
	}
	return nil
}

// setupSkillsFromExtracted creates symlinks from the extracted skills dir to the project.
func setupSkillsFromExtracted(dir, skillsDir string) error {
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		return nil
	}

	targetSkillsDir := filepath.Join(dir, ".claude", "skills")
	if err := os.MkdirAll(targetSkillsDir, 0755); err != nil {
		return fmt.Errorf("creating .claude/skills/: %w", err)
	}

	count := 0
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == ".version" {
			continue
		}
		target := filepath.Join(targetSkillsDir, entry.Name())
		if _, err := os.Lstat(target); err == nil {
			continue
		}
		source := filepath.Join(skillsDir, entry.Name())
		if err := os.Symlink(source, target); err != nil {
			fmt.Fprintf(os.Stderr, "cq: warning: symlink skill %s: %v\n", entry.Name(), err)
			continue
		}
		count++
	}
	if count > 0 {
		fmt.Fprintf(os.Stderr, "cq: %d skills deployed (extracted from embed)\n", count)
	} else {
		fmt.Fprintln(os.Stderr, "cq: skills up to date")
	}
	return nil
}

// extractEmbeddedSkills extracts skills from EmbeddedSkillsFS to destDir.
// It is version-aware: if ~/.c4/skills/.version matches the embedded version, extraction is skipped.
// Write failures are logged as warnings and return nil (CI read-only mount safety).
func extractEmbeddedSkills(destDir string) error {
	// Read embedded version from skills_src/.version
	embeddedVersionBytes, err := fs.ReadFile(EmbeddedSkillsFS, "skills_src/.version")
	embeddedVersion := strings.TrimSpace(string(embeddedVersionBytes))
	if err != nil {
		embeddedVersion = ""
	}

	// Fast path: check if installed version matches embedded version
	if embeddedVersion != "" {
		installedVersionBytes, err := os.ReadFile(filepath.Join(destDir, ".version"))
		if err == nil {
			installedVersion := strings.TrimSpace(string(installedVersionBytes))
			if installedVersion == embeddedVersion {
				return nil // already up to date
			}
		}
	}

	// Extract all files from skills_src/
	if err := fs.WalkDir(EmbeddedSkillsFS, "skills_src", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Compute destination path by stripping "skills_src/" prefix
		rel, stripErr := filepath.Rel("skills_src", path)
		if stripErr != nil {
			return stripErr
		}
		dest := filepath.Join(destDir, rel)

		if d.IsDir() {
			mkErr := os.MkdirAll(dest, 0755)
			if mkErr != nil {
				slog.Warn("skills embed: mkdir failed", "path", dest, "err", mkErr)
			}
			return nil
		}

		data, readErr := fs.ReadFile(EmbeddedSkillsFS, path)
		if readErr != nil {
			return readErr
		}

		if writeErr := os.WriteFile(dest, data, 0644); writeErr != nil {
			slog.Warn("skills embed: write failed", "path", dest, "err", writeErr)
			return nil // graceful: don't abort on write failure
		}
		return nil
	}); err != nil {
		return fmt.Errorf("extractEmbeddedSkills walk: %w", err)
	}

	// Update .version file
	if embeddedVersion != "" {
		versionPath := filepath.Join(destDir, ".version")
		if writeErr := os.WriteFile(versionPath, []byte(embeddedVersion+"\n"), 0644); writeErr != nil {
			slog.Warn("skills embed: version write failed", "err", writeErr)
		}
	}

	return nil
}

// hookNeedsUpdate returns true if the file at path doesn't exist or
// its SHA256 hash differs from the embedded content.
func hookNeedsUpdate(path string, embeddedContent string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return true // file missing
	}
	existing := sha256.Sum256(data)
	embedded := sha256.Sum256([]byte(embeddedContent))
	return existing != embedded
}

// setupGlobalHooks installs the C4 bash security hook to ~/.claude/hooks/.
// The hook script is embedded in the binary. Hook configuration is read from
// .c4/config.yaml (permission_reviewer section); any existing .conf files
// are left untouched for backward compatibility.
func setupGlobalHooks(homeDir string) error {
	hooksDir := filepath.Join(homeDir, ".claude", "hooks")
	if err := os.MkdirAll(hooksDir, 0700); err != nil {
		return fmt.Errorf("creating hooks dir: %w", err)
	}

	hookPath := filepath.Join(hooksDir, "c4-bash-security-hook.sh")
	if hookNeedsUpdate(hookPath, hookShContent) {
		if err := os.WriteFile(hookPath, []byte(hookShContent), 0755); err != nil {
			return fmt.Errorf("writing hook: %w", err)
		}
		fmt.Fprintln(os.Stderr, "cq: hook installed → "+hookPath)
	} else {
		fmt.Fprintln(os.Stderr, "cq: hooks up-to-date")
	}

	// Register hook in ~/.claude/settings.json
	if err := patchClaudeSettings(homeDir, hookPath); err != nil {
		return fmt.Errorf("patching settings.json: %w", err)
	}

	return nil
}

// patchClaudeSettings registers the Bash security hook in ~/.claude/settings.json.
// It is idempotent: if the hook entry already exists, it does nothing.
// Corrupted JSON is backed up and replaced with a fresh structure.
func patchClaudeSettings(homeDir string, hookPath string) error {
	settingsPath := filepath.Join(homeDir, ".claude", "settings.json")

	// Read existing settings or start fresh
	var settings map[string]any
	data, err := os.ReadFile(settingsPath)
	if err == nil {
		if jsonErr := json.Unmarshal(data, &settings); jsonErr != nil {
			// Corrupted JSON → backup and start fresh
			backupName := fmt.Sprintf("settings.json.bak.%s", time.Now().Format("20060102150405"))
			backupPath := filepath.Join(filepath.Dir(settingsPath), backupName)
			_ = os.WriteFile(backupPath, data, 0644)
			fmt.Fprintf(os.Stderr, "cq: settings.json corrupted, backed up → %s\n", backupPath)
			settings = map[string]any{}
		}
	} else {
		settings = map[string]any{}
	}

	// Navigate to hooks → PreToolUse
	hooks, _ := settings["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
	}

	preToolUseRaw, _ := hooks["PreToolUse"]
	var preToolUse []any
	if arr, ok := preToolUseRaw.([]any); ok {
		preToolUse = arr
	}

	// Check if Bash matcher with this hookPath already exists; update stale path in-place.
	const hookBaseName = "c4-bash-security-hook.sh"
	updated := false
	for i, entry := range preToolUse {
		entryMap, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		if entryMap["matcher"] != "Bash" {
			continue
		}
		innerHooks, _ := entryMap["hooks"].([]any)
		for j, h := range innerHooks {
			hMap, ok := h.(map[string]any)
			if !ok {
				continue
			}
			cmd, _ := hMap["command"].(string)
			if cmd == hookPath {
				// Already registered with correct path
				return nil
			}
			if strings.Contains(cmd, hookBaseName) {
				// Same hook script, stale path – update in place
				hMap["command"] = hookPath
				innerHooks[j] = hMap
				entryMap["hooks"] = innerHooks
				preToolUse[i] = entryMap
				updated = true
				break
			}
		}
		if updated {
			break
		}
	}

	if !updated {
		// No existing Bash hook entry for this script; add new entry
		newEntry := map[string]any{
			"matcher": "Bash",
			"hooks": []any{
				map[string]any{
					"type":    "command",
					"command": hookPath,
				},
			},
		}
		preToolUse = append(preToolUse, newEntry)
	}
	hooks["PreToolUse"] = preToolUse
	settings["hooks"] = hooks

	// Atomic write: tempfile → rename
	dir := filepath.Dir(settingsPath)
	if mkErr := os.MkdirAll(dir, 0755); mkErr != nil {
		return fmt.Errorf("creating settings dir: %w", mkErr)
	}
	out, marshalErr := json.MarshalIndent(settings, "", "  ")
	if marshalErr != nil {
		return fmt.Errorf("marshaling settings: %w", marshalErr)
	}
	out = append(out, '\n')

	tmpFile, tmpErr := os.CreateTemp(dir, "settings-*.json.tmp")
	if tmpErr != nil {
		return fmt.Errorf("creating temp file: %w", tmpErr)
	}
	tmpPath := tmpFile.Name()
	if _, wErr := tmpFile.Write(out); wErr != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("writing temp file: %w", wErr)
	}
	if cErr := tmpFile.Close(); cErr != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("closing temp file: %w", cErr)
	}
	if rErr := os.Rename(tmpPath, settingsPath); rErr != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("renaming temp file: %w", rErr)
	}
	// Restore readable permissions (os.CreateTemp creates with 0600)
	_ = os.Chmod(settingsPath, 0644)

	fmt.Fprintln(os.Stderr, "cq: registered Bash hook in ~/.claude/settings.json")
	return nil
}

// setupCodexConfig creates or updates ~/.codex/config.toml with cq MCP server entry.
func setupCodexConfig(dir string) error {
	configPath, err := codexConfigPath()
	if err != nil {
		return err
	}
	binPath, err := findCQBinary()
	if err != nil {
		return err
	}

	content := ""
	if data, readErr := os.ReadFile(configPath); readErr == nil {
		content = string(data)
	} else if !os.IsNotExist(readErr) {
		return fmt.Errorf("reading %s: %w", configPath, readErr)
	}
	content = strings.ReplaceAll(content, "\r\n", "\n")

	updated := strings.Contains(content, "[mcp_servers.cq]")
	cleaned := removeTOMLTable(content, "[mcp_servers.cq]")
	cleaned = strings.TrimRight(cleaned, "\n")

	block := codexMCPBlock(binPath, dir)
	var final strings.Builder
	if cleaned != "" {
		final.WriteString(cleaned)
		final.WriteString("\n\n")
	}
	final.WriteString(block)
	final.WriteString("\n")

	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		return fmt.Errorf("creating codex config directory: %w", err)
	}
	if err := os.WriteFile(configPath, []byte(final.String()), 0644); err != nil {
		return fmt.Errorf("writing codex config: %w", err)
	}

	if updated {
		fmt.Fprintf(os.Stderr, "cq: codex MCP config updated (%s)\n", configPath)
	} else {
		fmt.Fprintf(os.Stderr, "cq: codex MCP config added (%s)\n", configPath)
	}
	return nil
}

func codexConfigPath() (string, error) {
	if path := os.Getenv("CODEX_CONFIG"); path != "" {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory for codex config: %w", err)
	}
	return filepath.Join(home, ".codex", "config.toml"), nil
}

func codexMCPBlock(binPath, dir string) string {
	lines := []string{
		"[mcp_servers.cq]",
		fmt.Sprintf("command = %s", strconv.Quote(binPath)),
		fmt.Sprintf("args = [\"mcp\", \"--dir\", %s]", strconv.Quote(dir)),
		fmt.Sprintf("env = { C4_PROJECT_ROOT = %s }", strconv.Quote(dir)),
	}
	return strings.Join(lines, "\n")
}

func removeTOMLTable(content, header string) string {
	if content == "" {
		return ""
	}
	lines := strings.Split(content, "\n")
	out := make([]string, 0, len(lines))
	skipping := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !skipping {
			if trimmed == header {
				skipping = true
				continue
			}
			out = append(out, line)
			continue
		}
		// End skip when next TOML table header begins.
		// Match [section] but not [[array-of-tables]] headers.
		if trimmed != "" && strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") &&
			!strings.HasPrefix(trimmed, "[[") {
			skipping = false
			out = append(out, line)
		}
	}

	return strings.Join(out, "\n")
}

// setupCodexAgents deploys project-level Codex agent files via symlinks.
func setupCodexAgents(dir string) error {
	c4Root, err := findC4Root()
	if err != nil {
		fmt.Fprintln(os.Stderr, "cq: codex agents setup skipped (C4 source root not found)")
		return nil
	}

	sourceAgentsDir := filepath.Join(c4Root, ".codex", "agents")
	entries, err := os.ReadDir(sourceAgentsDir)
	if err != nil {
		return nil // No agents to deploy
	}

	targetAgentsDir := filepath.Join(dir, ".codex", "agents")
	if err := os.MkdirAll(targetAgentsDir, 0755); err != nil {
		return fmt.Errorf("creating .codex/agents/: %w", err)
	}

	count := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, "c4-") || !strings.HasSuffix(name, ".md") {
			continue
		}
		target := filepath.Join(targetAgentsDir, name)
		if _, err := os.Lstat(target); err == nil {
			continue
		}
		source := filepath.Join(sourceAgentsDir, name)
		if err := os.Symlink(source, target); err != nil {
			fmt.Fprintf(os.Stderr, "cq: warning: symlink codex agent %s: %v\n", name, err)
			continue
		}
		count++
	}
	if count > 0 {
		fmt.Fprintf(os.Stderr, "cq: %d codex agents deployed (symlinked from %s)\n", count, c4Root)
	} else {
		fmt.Fprintln(os.Stderr, "cq: codex agents up to date")
	}
	return nil
}

// findC4Root locates the C4 source repository root directory.
// Search order: C4_SOURCE_ROOT env, builtinC4Root (ldflags), ~/.c4-install-path,
// then derivation from binary path (c4-core/bin/cq → repo root).
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

	// 4. Derive from binary path: {root}/c4-core/bin/cq → {root}
	if binPath, err := os.Executable(); err == nil {
		if binPath, err = filepath.EvalSymlinks(binPath); err == nil {
			// Try c4-core/bin/cq layout
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

// setupCursorMCPConfig creates or updates .cursor/mcp.json so Cursor loads C4 MCP when opening the project.
func setupCursorMCPConfig(dir string) error {
	cursorDir := filepath.Join(dir, ".cursor")
	if err := os.MkdirAll(cursorDir, 0755); err != nil {
		return fmt.Errorf("creating .cursor: %w", err)
	}
	mcpPath := filepath.Join(cursorDir, "mcp.json")

	binPath, err := findCQBinary()
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

	var config map[string]any
	if data, readErr := os.ReadFile(mcpPath); readErr == nil {
		if json.Unmarshal(data, &config) != nil {
			fmt.Fprintf(os.Stderr, "cq: WARNING: %s has invalid JSON, overwriting with new config\n", mcpPath)
			config = nil
		}
	}
	if config == nil {
		config = map[string]any{}
	}

	servers, _ := config["mcpServers"].(map[string]any)
	if servers == nil {
		servers = map[string]any{}
	}
	servers["cq"] = c4Entry
	config["mcpServers"] = servers

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}
	data = append(data, '\n')

	if err := os.WriteFile(mcpPath, data, 0644); err != nil {
		return fmt.Errorf("writing .cursor/mcp.json: %w", err)
	}

	fmt.Fprintln(os.Stderr, "cq: .cursor/mcp.json configured (C4 MCP will load when Cursor opens this project)")
	return nil
}

// setupMCPConfig creates or updates .mcp.json with the C4 MCP server entry.
func setupMCPConfig(dir string) error {
	mcpPath := filepath.Join(dir, ".mcp.json")

	binPath, err := findCQBinary()
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
		fmt.Fprintf(os.Stderr, "cq: .mcp.json already exists, updating (cq entry will be overwritten)\n")
		if json.Unmarshal(data, &config) != nil {
			fmt.Fprintf(os.Stderr, "cq: WARNING: %s has invalid JSON, overwriting with new config\n", mcpPath)
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
	servers["cq"] = c4Entry
	config["mcpServers"] = servers

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}
	data = append(data, '\n')

	if err := os.WriteFile(mcpPath, data, 0644); err != nil {
		return fmt.Errorf("writing .mcp.json: %w", err)
	}

	fmt.Fprintln(os.Stderr, "cq: .mcp.json configured")
	return nil
}

// findCQBinary locates the cq Go binary path for .mcp.json configuration.
func findCQBinary() (string, error) {
	binPath, err := os.Executable()
	if err == nil {
		binPath, err = filepath.EvalSymlinks(binPath)
		if err == nil {
			return binPath, nil
		}
	}
	if p, err := exec.LookPath("cq"); err == nil {
		return filepath.Abs(p)
	}
	return "", fmt.Errorf("cannot determine cq binary path")
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

	fmt.Fprintf(os.Stderr, "cq: launching %s...\n", tool)
	return syscall.Exec(toolPath, toolArgs, os.Environ())
}
