package main

import (
	"bufio"
	"crypto/sha256"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/changmin/c4-core/internal/botstore"
	"github.com/changmin/c4-core/internal/mailbox"
	stdpkg "github.com/changmin/c4-core/internal/standards"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

//go:embed templates/claude_md.tmpl
var claudeMDTemplate string

//go:embed templates/hooks/c4-gate.sh
var gateHookContent string

//go:embed templates/hooks/c4-permission-reviewer.sh
var permissionReviewerContent string

//go:embed templates/config.yaml
var defaultConfigYAML string

// builtinC4Root is set at build time via -ldflags for skill deployment.
var builtinC4Root string

// initTier is the build tier written to .c4/config.yaml on project init.
// Valid values: "solo", "connected", "full" (default).
var initTier string

// sessionName is an optional name for the Claude Code session (-t flag).
var sessionName string

// botName is the optional bot name for --bot flag. Empty string means show menu.
var botName string

// Tool launcher commands: cq claude, cq codex, cq cursor
var (
	claudeCmd = &cobra.Command{
		Use:   "claude",
		Short: "Init C4 project and launch Claude Code",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return launchClaude(cmd)
		},
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
	geminiCmd = &cobra.Command{
		Use:   "gemini",
		Short: "Init C4 project and launch Gemini CLI",
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, args []string) error { return initAndLaunch("gemini") },
	}
)

func init() {
	// Register --tier flag on all init-related commands and the root command.
	for _, cmd := range []*cobra.Command{rootCmd, claudeCmd, codexCmd, cursorCmd, geminiCmd} {
		cmd.Flags().StringVar(&initTier, "tier", "", "build tier: solo|connected|full (written to .c4/config.yaml)")
	}
	// Register -t/--tag flag for named sessions (claude and gemini only, and root default).
	// -t (no value) = show session picker, -t name = direct.
	for _, cmd := range []*cobra.Command{rootCmd, claudeCmd, geminiCmd} {
		cmd.Flags().StringVarP(&sessionName, "tag", "t", "", "session name: resume or create named AI session")
	}
	// Register -t/--tag flag completion: list named session names.
	for _, cmd := range []*cobra.Command{rootCmd, claudeCmd, geminiCmd} {
		_ = cmd.RegisterFlagCompletionFunc("tag", completeSessionNames)
	}
	// Register --bot flag for telegram bot selection (claude and root only).
	// --bot (no value) = show bot menu, --bot=mybot = direct pick.
	for _, cmd := range []*cobra.Command{rootCmd, claudeCmd} {
		cmd.Flags().StringVar(&botName, "bot", "", "select telegram bot by name, or show bot menu if empty")
		cmd.Flag("bot").NoOptDefVal = " " // sentinel: --bot without value
	}
	sessionCmd.AddCommand(sessionNameCmd, sessionRmCmd, sessionMemoCmd)
	rootCmd.AddCommand(claudeCmd, codexCmd, cursorCmd, geminiCmd, sessionsCmd, sessionCmd)
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

// confirmProjectHooks prompts the user before installing project-level hooks
// (.claude/hooks/ and .claude/settings.json in the project directory).
// Returns true if the user accepts or if yesAll is set; false to skip.
func confirmProjectHooks(projectDir string) bool {
	if yesAll {
		return true
	}

	hookPath := filepath.Join(projectDir, ".claude", "hooks", "c4-gate.sh")
	settingsPath := filepath.Join(projectDir, ".claude", "settings.json")

	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "cq: The following PROJECT files will be created or modified:")
	fmt.Fprintf(os.Stderr, "  1. %s\n", hookPath)
	fmt.Fprintf(os.Stderr, "  2. %s\n", settingsPath)
	fmt.Fprintln(os.Stderr, "  These hooks gate tool use and review requests for safety.")
	fmt.Fprint(os.Stderr, "Allow? [y/N] ")

	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		answer := strings.TrimSpace(strings.ToLower(scanner.Text()))
		if answer == "y" || answer == "yes" {
			return true
		}
	}

	fmt.Fprintln(os.Stderr, "cq: skipping hook installation (run with --yes to suppress this prompt)")
	return false
}

// completeSessionNames is a cobra flag completion function for -t/--tag.
// It returns the list of saved named session names.
func completeSessionNames(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
	sessions, err := loadNamedSessions()
	if err != nil || len(sessions) == 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	names := make([]string, 0, len(sessions))
	for name := range sessions {
		names = append(names, name)
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}

// setupShellCompletion adds `eval "$(cq completion <shell>)"` to shell rc files
// if not already present. Non-fatal: errors are silently skipped.
func setupShellCompletion() {
	type rcEntry struct {
		path  string
		shell string
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return
	}
	entries := []rcEntry{
		{filepath.Join(homeDir, ".zshrc"), "zsh"},
		{filepath.Join(homeDir, ".bashrc"), "bash"},
	}
	for _, e := range entries {
		if _, statErr := os.Stat(e.path); statErr != nil {
			continue // rc file doesn't exist
		}
		data, readErr := os.ReadFile(e.path)
		if readErr != nil {
			continue
		}
		marker := `cq completion ` + e.shell
		if strings.Contains(string(data), marker) {
			continue // already set up
		}
		line := "\n# cq shell completion\neval \"$(cq completion " + e.shell + ")\"\n"
		f, openErr := os.OpenFile(e.path, os.O_APPEND|os.O_WRONLY, 0644)
		if openErr != nil {
			continue
		}
		if _, writeErr := f.WriteString(line); writeErr != nil {
			f.Close()
			continue
		}
		fmt.Fprintf(os.Stderr, "cq: shell completion added to %s\n", e.path)
		f.Close()
	}
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
	fmt.Fprintln(os.Stderr, "✓ C4 Engine 로드")

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
	fmt.Fprintln(os.Stderr, "✓ 지식 베이스 연결")

	// 3. Create/update CLAUDE.md with C4 overrides
	if err := setupClaudeMD(dir); err != nil {
		fmt.Fprintf(os.Stderr, "cq: warning: CLAUDE.md setup failed: %v\n", err)
	}

	// 3b. Apply standards (rules, skills from embedded standards) — always run.
	// If .piki-lock.yaml already exists with a team set, preserve existing team/langs.
	// Otherwise apply common-only and hint the user to run `cq standards apply` for team rules.
	{
		applyTeam := ""
		var applyLangs []string
		if lock, lockErr := stdpkg.ReadLock(dir); lockErr == nil && lock.Team != "" {
			applyTeam = lock.Team
			applyLangs = lock.Langs
		} else if lockErr == nil {
			// lock exists but no team — common-only
		} else {
			// no lock file — common-only, print hint
			fmt.Fprintln(os.Stderr, "cq: hint: run `cq standards apply --team <team>` to apply team-specific rules")
		}
		result, err := stdpkg.Apply(dir, applyTeam, applyLangs, stdpkg.ApplyOptions{})
		if err != nil {
			fmt.Fprintf(os.Stderr, "cq: warning: standards setup failed: %v\n", err)
		} else {
			fmt.Fprintf(os.Stderr, "✓ 표준 규칙 적용 (%d files, team=%s, langs=%v)\n",
				len(result.FilesCreated), result.Team, result.Langs)
		}
	}

	// 4. Deploy C4 skills (symlinks from C4 source)
	if err := setupSkills(dir); err != nil {
		fmt.Fprintf(os.Stderr, "cq: warning: skills setup failed: %v\n", err)
	}

	// 5. Install project hooks to .claude/hooks/ (requires user confirmation)
	if confirmProjectHooks(dir) {
		if err := setupProjectHooks(dir); err != nil {
			fmt.Fprintf(os.Stderr, "cq: warning: hooks setup failed: %v\n", err)
		}
	}

	// 5b. Add shell completion to ~/.zshrc / ~/.bashrc (non-fatal)
	setupShellCompletion()

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
	fmt.Fprintln(os.Stderr, "✓ MCP 서버 준비")

	// 7. Check cloud auth status and prompt login if needed
	ensureCloudAuth(nil, yesAll)

	// 7b. Ensure cq serve is running in background (non-fatal if it fails)
	ensureServeRunning(noServe)
	fmt.Fprintln(os.Stderr, "✓ 세션 컨텍스트 주입")

	printReadyBox(os.Stderr)

	// 8. Launch AI tool

	// Bot token injection: propagate C4_TELEGRAM_BOT_TOKEN → TELEGRAM_BOT_TOKEN so the
	// telegram plugin can start the bot server when claude launches.
	if tool == "claude" {
		if tok := os.Getenv("C4_TELEGRAM_BOT_TOKEN"); tok != "" {
			if err := os.Setenv("TELEGRAM_BOT_TOKEN", tok); err != nil {
				fmt.Fprintf(os.Stderr, "cq: warning: failed to set TELEGRAM_BOT_TOKEN: %v\n", err)
			}
		}
	}

	if (tool == "claude" || tool == "gemini") && sessionName != "" {
		return launchToolNamed(tool, dir, sessionName)
	}
	return launchTool(tool, dir)
}

// launchClaude is the unified entry point for cq and cq claude.
// Handles -t (session) and -b (bot) flags independently.
func launchClaude(cmd *cobra.Command) error {
	hasBot := cmd.Flag("bot").Changed

	// -b: handle bot selection → sets C4_TELEGRAM_BOT_TOKEN
	if hasBot {
		name := strings.TrimSpace(botName)
		store, err := botstore.New(projectDir)
		if err != nil {
			return fmt.Errorf("botstore: %w", err)
		}
		if name == "" || name == " " {
			// Interactive menu — select bot, set env, continue to launch
			bot, err := botMenuSelect(store)
			if err != nil {
				return err
			}
			if bot == nil {
				return nil // user cancelled
			}
			if err := os.Setenv("C4_TELEGRAM_BOT_TOKEN", bot.Token); err != nil {
				return fmt.Errorf("set env: %w", err)
			}
			fmt.Fprintf(os.Stderr, "봇 선택: @%s\n", bot.Username)
		} else {
			// Direct pick by name
			bots, err := store.List()
			if err != nil {
				return fmt.Errorf("botstore list: %w", err)
			}
			name = strings.TrimPrefix(name, "@")
			var found *botstore.Bot
			for i, bot := range bots {
				if strings.EqualFold(bot.Username, name) {
					found = &bots[i]
					break
				}
			}
			if found == nil {
				return fmt.Errorf("봇 '%s'을(를) 찾을 수 없습니다. cq --bot 으로 목록을 확인하세요.", name)
			}
			if err := os.Setenv("C4_TELEGRAM_BOT_TOKEN", found.Token); err != nil {
				return fmt.Errorf("set env: %w", err)
			}
			fmt.Fprintf(os.Stderr, "봇 선택: @%s\n", found.Username)
		}
	} else {
		// No bot → suppress telegram
		os.Setenv("CQ_NO_TELEGRAM", "1")
	}

	return initAndLaunch("claude")
}

// printReadyBox prints a box with ready message to w.
func printReadyBox(w io.Writer) {
	lines := []string{
		"준비 완료 — Claude Code 시작합니다",
		"도움말: /help | 상태: /c4-status",
	}
	maxLen := 0
	for _, l := range lines {
		if len([]rune(l)) > maxLen {
			maxLen = len([]rune(l))
		}
	}
	border := strings.Repeat("─", maxLen+2)
	fmt.Fprintf(w, "┌%s┐\n", border)
	for _, l := range lines {
		padding := maxLen - len([]rune(l))
		fmt.Fprintf(w, "│ %s%s │\n", l, strings.Repeat(" ", padding))
	}
	fmt.Fprintf(w, "└%s┘\n", border)
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

// setupProjectHooks installs C4 hooks to {projectDir}/.claude/hooks/.
// Deploys c4-gate.sh (PreToolUse) and c4-permission-reviewer.sh (PermissionRequest),
// then registers them in {projectDir}/.claude/settings.json using $CLAUDE_PROJECT_DIR.
func setupProjectHooks(projectDir string) error {
	hooksDir := filepath.Join(projectDir, ".claude", "hooks")
	if err := os.MkdirAll(hooksDir, 0755); err != nil {
		return fmt.Errorf("creating hooks dir: %w", err)
	}

	gateHookPath := filepath.Join(hooksDir, "c4-gate.sh")
	gateUpdated := hookNeedsUpdate(gateHookPath, gateHookContent)
	if gateUpdated {
		if err := os.WriteFile(gateHookPath, []byte(gateHookContent), 0755); err != nil {
			return fmt.Errorf("writing gate hook: %w", err)
		}
		fmt.Fprintln(os.Stderr, "cq: gate hook installed → "+gateHookPath)
	}

	permHookPath := filepath.Join(hooksDir, "c4-permission-reviewer.sh")
	permUpdated := hookNeedsUpdate(permHookPath, permissionReviewerContent)
	if permUpdated {
		if err := os.WriteFile(permHookPath, []byte(permissionReviewerContent), 0755); err != nil {
			return fmt.Errorf("writing permission reviewer hook: %w", err)
		}
		fmt.Fprintln(os.Stderr, "cq: permission reviewer hook installed → "+permHookPath)
	}

	if !gateUpdated && !permUpdated {
		fmt.Fprintln(os.Stderr, "cq: hooks up-to-date")
	}

	// Register hooks in {projectDir}/.claude/settings.json using $CLAUDE_PROJECT_DIR
	if err := patchProjectSettings(projectDir); err != nil {
		return fmt.Errorf("patching settings.json: %w", err)
	}

	return nil
}

// patchProjectSettings registers C4 hooks in {projectDir}/.claude/settings.json.
// Uses $CLAUDE_PROJECT_DIR so the paths work on any machine after cq init.
// Installs:
//   - PreToolUse: Bash|Edit|Write   → c4-gate.sh
//   - PermissionRequest: (all tools except AskUserQuestion) → c4-permission-reviewer.sh
//   - permissions.allow: Read, Glob, Grep, WebFetch, WebSearch
//
// It is idempotent. Corrupted JSON is backed up and replaced.
func patchProjectSettings(projectDir string) error {
	settingsDir := filepath.Join(projectDir, ".claude")
	settingsPath := filepath.Join(settingsDir, "settings.json")

	// Use $CLAUDE_PROJECT_DIR variable in hook command paths (portable across machines)
	gateCmd := `"$CLAUDE_PROJECT_DIR"/.claude/hooks/c4-gate.sh`
	permCmd := `"$CLAUDE_PROJECT_DIR"/.claude/hooks/c4-permission-reviewer.sh`

	// Read existing settings or start fresh
	var settings map[string]any
	data, err := os.ReadFile(settingsPath)
	if err == nil {
		if jsonErr := json.Unmarshal(data, &settings); jsonErr != nil {
			backupName := fmt.Sprintf("settings.json.bak.%s", time.Now().Format("20060102150405"))
			backupPath := filepath.Join(settingsDir, backupName)
			_ = os.WriteFile(backupPath, data, 0644)
			fmt.Fprintf(os.Stderr, "cq: settings.json corrupted, backed up → %s\n", backupPath)
			settings = map[string]any{}
		}
	} else {
		settings = map[string]any{}
	}

	// Navigate to hooks
	hooks, _ := settings["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
	}

	// Helper: idempotently patch a hook event array.
	// deprecatedNames lists legacy basenames that should be replaced by the new entry
	// (e.g. "permission-reviewer.py" superseded by "c4-permission-reviewer.sh").
	patchHookEvent := func(eventName, matcher, cmdStr, baseName string, timeout int, deprecatedNames ...string) {
		eventRaw, _ := hooks[eventName]
		var eventArr []any
		if arr, ok := eventRaw.([]any); ok {
			eventArr = arr
		}

		hookEntry := map[string]any{"type": "command", "command": cmdStr}
		if timeout > 0 {
			hookEntry["timeout"] = timeout
		}

		matchesKnownBaseName := func(cmd string) bool {
			if strings.Contains(cmd, baseName) {
				return true
			}
			for _, dep := range deprecatedNames {
				if strings.Contains(cmd, dep) {
					return true
				}
			}
			return false
		}

		// Phase 1: scan ALL entries for baseName (or deprecated names) regardless of
		// matcher. Handles stale entries with a different matcher string or a legacy
		// script name, ensuring we never create duplicate entries.
		for i, entry := range eventArr {
			entryMap, ok := entry.(map[string]any)
			if !ok {
				continue
			}
			innerHooks, _ := entryMap["hooks"].([]any)
			for j, h := range innerHooks {
				hMap, ok := h.(map[string]any)
				if !ok {
					continue
				}
				cmd, _ := hMap["command"].(string)
				if cmd == cmdStr && entryMap["matcher"] == matcher {
					return // already correct, nothing to do
				}
				if matchesKnownBaseName(cmd) {
					// Stale entry (wrong path, wrong matcher, or deprecated script):
					// replace entire hook entry and update matcher so it is fully current.
					innerHooks[j] = hookEntry
					entryMap["hooks"] = innerHooks
					entryMap["matcher"] = matcher
					eventArr[i] = entryMap
					hooks[eventName] = eventArr
					return
				}
			}
		}

		// Phase 2: exact matcher match — replace hooks list (baseName not yet present).
		for i, entry := range eventArr {
			entryMap, ok := entry.(map[string]any)
			if !ok {
				continue
			}
			if entryMap["matcher"] != matcher {
				continue
			}
			entryMap["hooks"] = []any{hookEntry}
			eventArr[i] = entryMap
			hooks[eventName] = eventArr
			return
		}

		// Phase 3: no existing entry — append new one.
		eventArr = append(eventArr, map[string]any{
			"matcher": matcher,
			"hooks":   []any{hookEntry},
		})
		hooks[eventName] = eventArr
	}

	// PreToolUse: gate hook (Bash|Edit|Write)
	// deprecated: c4-bash-security-hook.sh, c4-edit-security-hook.sh (v0.23 global hooks)
	patchHookEvent("PreToolUse", "Bash|Edit|Write", gateCmd, "c4-gate.sh", 0,
		"c4-bash-security-hook.sh", "c4-edit-security-hook.sh")
	// PermissionRequest: permission reviewer (explicit include list; excludes AskUserQuestion)
	// deprecated: permission-reviewer.py (pre-v0.24 Python-based reviewer)
	patchHookEvent("PermissionRequest",
		"Bash|Read|Edit|Write|NotebookEdit|WebFetch|WebSearch|Search|Skill",
		permCmd, "c4-permission-reviewer.sh", 20,
		"permission-reviewer.py")

	settings["hooks"] = hooks

	// Ensure permissions.allow contains safe read-only tools (no side effects)
	perms, _ := settings["permissions"].(map[string]any)
	if perms == nil {
		perms = map[string]any{}
	}
	allowRaw, _ := perms["allow"]
	var allowList []any
	if arr, ok := allowRaw.([]any); ok {
		allowList = arr
	}
	for _, tool := range []string{"Read", "Glob", "Grep", "WebFetch", "WebSearch",
		"Skill(c4-*)", "Skill(c9-*)", "Skill(pi)", "Skill(research-loop)", "mcp__cq__*"} {
		found := false
		for _, v := range allowList {
			if s, ok := v.(string); ok && s == tool {
				found = true
				break
			}
		}
		if !found {
			allowList = append(allowList, tool)
		}
	}
	perms["allow"] = allowList
	settings["permissions"] = perms

	// Atomic write: tempfile → rename
	if mkErr := os.MkdirAll(settingsDir, 0755); mkErr != nil {
		return fmt.Errorf("creating settings dir: %w", mkErr)
	}
	out, marshalErr := json.MarshalIndent(settings, "", "  ")
	if marshalErr != nil {
		return fmt.Errorf("marshaling settings: %w", marshalErr)
	}
	out = append(out, '\n')

	tmpFile, tmpErr := os.CreateTemp(settingsDir, "settings-*.json.tmp")
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
	_ = os.Chmod(settingsPath, 0644)

	fmt.Fprintln(os.Stderr, "cq: hooks registered in .claude/settings.json")
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

// --- Named session support ---

type namedSessionEntry struct {
	UUID    string `json:"uuid"`
	Dir     string `json:"dir"`
	Tool    string `json:"tool,omitempty"` // claude, codex, cursor
	Memo    string `json:"memo,omitempty"` // user-defined description
	Updated string `json:"updated"`
}

func namedSessionsFile() string {
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, ".c4", "named-sessions.json")
}

func loadNamedSessions() (map[string]namedSessionEntry, error) {
	data, err := os.ReadFile(namedSessionsFile())
	if os.IsNotExist(err) {
		return map[string]namedSessionEntry{}, nil
	}
	if err != nil {
		return nil, err
	}
	var m map[string]namedSessionEntry
	if err := json.Unmarshal(data, &m); err != nil {
		return map[string]namedSessionEntry{}, nil
	}
	return m, nil
}

func saveNamedSessions(m map[string]namedSessionEntry) error {
	f := namedSessionsFile()
	if err := os.MkdirAll(filepath.Dir(f), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(f, data, 0600)
}

// claudeProjectDir returns the ~/.claude/projects/<encoded-path> directory for the given project.
func claudeProjectDir(projectDir string) (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	absDir, err := filepath.Abs(projectDir)
	if err != nil {
		return "", err
	}
	// Claude Code encodes the path: replace path separators with '-'
	encoded := strings.ReplaceAll(absDir, string(os.PathSeparator), "-")
	return filepath.Join(homeDir, ".claude", "projects", encoded), nil
}

// listJSONLNames returns a set of JSONL filenames in the given directory.
func listJSONLNames(dir string) map[string]struct{} {
	m := map[string]struct{}{}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".jsonl") {
			m[e.Name()] = struct{}{}
		}
	}
	return m
}

// rebootFlagFile returns the path to the reboot-request flag file.
func rebootFlagFile() string {
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, ".c4", ".reboot")
}

// findGeminiSessionIndex executes 'gemini --list-sessions' and parses the output
// to find the index number corresponding to the given UUID.
func findGeminiSessionIndex(uuid string) string {
	if uuid == "" {
		return "latest"
	}
	out, err := exec.Command("gemini", "--list-sessions").Output()
	if err != nil {
		return "latest"
	}
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		if strings.Contains(line, uuid) {
			// Extract index from line like "  10. some text [uuid]"
			trimmed := strings.TrimSpace(line)
			dotIdx := strings.Index(trimmed, ".")
			if dotIdx != -1 {
				return trimmed[:dotIdx]
			}
		}
	}
	return "latest"
}

// launchToolNamed starts or resumes a named AI tool session with a reboot loop.
// For claude: uses --session-id (new) or --resume (existing) with fixed UUIDs.
// For gemini: uses --resume with index-based lookup (best effort).
// Env vars CQ_SESSION_NAME and CQ_SESSION_UUID are injected into the subprocess.
func launchToolNamed(tool, projectDir, name string) error {
	sessions, err := loadNamedSessions()
	if err != nil {
		return fmt.Errorf("loading named sessions: %w", err)
	}

	toolPath, err := exec.LookPath(tool)
	if err != nil {
		return fmt.Errorf("%s not found in PATH: %w", tool, err)
	}

	// Determine or create UUID for this session.
	currentUUID := ""
	isNew := true
	if entry, ok := sessions[name]; ok {
		if entry.Dir != "" && entry.Dir != projectDir {
			fmt.Fprintf(os.Stderr, "cq: session '%s' belongs to %s (current: %s), starting new session...\n",
				name, entry.Dir, projectDir)
			delete(sessions, name)
		} else {
			currentUUID = entry.UUID
			isNew = false
		}
	}

	// For new sessions, generate a UUID upfront (no JSONL scanning needed).
	if currentUUID == "" {
		currentUUID = uuid.New().String()
		sessions[name] = namedSessionEntry{
			UUID:    currentUUID,
			Dir:     projectDir,
			Tool:    tool,
			Updated: time.Now().Format(time.RFC3339),
		}
		if err := saveNamedSessions(sessions); err != nil {
			fmt.Fprintf(os.Stderr, "cq: warning: failed to save session: %v\n", err)
		}
	}

	// Reboot loop: re-launches the tool when ~/.c4/.reboot exists after exit.
	for {
		os.Remove(rebootFlagFile())

		var toolArgs []string
		if isNew {
			fmt.Fprintf(os.Stderr, "cq: launching %s (session: '%s')...\n", tool, name)
			if tool == "claude" {
				toolArgs = []string{"--session-id", currentUUID, "--name", name}
			}
			if isFirstRun() {
				toolArgs = append(toolArgs, "--append-system-prompt", onboardingMsg)
				if err := markFirstRun(); err != nil {
					fmt.Fprintf(os.Stderr, "cq: warning: markFirstRun: %v\n", err)
				}
			}
		} else {
			fmt.Fprintf(os.Stderr, "cq: resuming %s session '%s' (%s...)...\n", tool, name, currentUUID[:8])
			if tool == "gemini" {
				resumeID := findGeminiSessionIndex(currentUUID)
				toolArgs = []string{"--resume", resumeID}
			} else {
				toolArgs = []string{"--resume", currentUUID}
			}
		}

		// Attach telegram channel if configured.
		if tool == "claude" && telegramChannelConfigured() {
			toolArgs = append(toolArgs, "--channels", "plugin:telegram@claude-plugins-official")
		}

		// Inject session context into subprocess environment.
		env := append(os.Environ(),
			"CQ_SESSION_NAME="+name,
			"CQ_SESSION_UUID="+currentUUID,
		)

		cmd := exec.Command(toolPath, toolArgs...)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Env = env

		if err := cmd.Start(); err != nil {
			return fmt.Errorf("start %s: %w", tool, err)
		}

		// Watch for .reboot file — auto-terminate when detected.
		rebootDetected := make(chan struct{}, 1)
		go func() {
			ticker := time.NewTicker(2 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					if _, err := os.Stat(rebootFlagFile()); err == nil {
						select {
						case rebootDetected <- struct{}{}:
						default:
						}
						if cmd.Process != nil {
							_ = cmd.Process.Signal(os.Interrupt)
						}
						return
					}
				case <-rebootDetected:
					return
				}
			}
		}()

		runErr := cmd.Wait()

		// If resume failed, retry as new session with --session-id.
		if runErr != nil && !isNew {
			if exitErr, ok := runErr.(*exec.ExitError); ok && exitErr.ExitCode() != 0 {
				fmt.Fprintf(os.Stderr, "cq: session '%s' resume failed, starting new session...\n", name)
				currentUUID = uuid.New().String()
				isNew = true
				sessions[name] = namedSessionEntry{
					UUID:    currentUUID,
					Dir:     projectDir,
					Tool:    tool,
					Updated: time.Now().Format(time.RFC3339),
				}
				_ = saveNamedSessions(sessions)
				continue
			}
		}

		// After first successful run, future iterations are resumes.
		isNew = false

		// Check reboot flag.
		if data, err := os.ReadFile(rebootFlagFile()); err == nil {
			os.Remove(rebootFlagFile())
			if overrideUUID := strings.TrimSpace(string(data)); overrideUUID != "" && overrideUUID != currentUUID {
				fmt.Fprintf(os.Stderr, "cq: reboot: overriding UUID → %s\n", overrideUUID[:min(8, len(overrideUUID))])
				currentUUID = overrideUUID
			}
			fmt.Fprintf(os.Stderr, "cq: rebooting session '%s'...\n", name)
			continue
		}

		break
	}

	return nil
}

// currentSessionUUID detects the current Claude Code session UUID.
// Priority: CQ_SESSION_UUID env var → JSONL content timestamp → file ModTime.
// Walks up parent directories to find the correct Claude project JSONL dir.
func currentSessionUUID(dir string) string {
	// 1. Prefer env var (set by cq claude -t).
	if uuid := os.Getenv("CQ_SESSION_UUID"); uuid != "" {
		return uuid
	}

	// 2. Try dir and parent directories (handles subdirectory execution).
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return ""
	}
	var sessionDir string
	for d := absDir; d != filepath.Dir(d); d = filepath.Dir(d) {
		candidate, err := claudeProjectDir(d)
		if err != nil {
			continue
		}
		entries, _ := os.ReadDir(candidate)
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".jsonl") {
				sessionDir = candidate
				break
			}
		}
		if sessionDir != "" {
			break
		}
	}
	if sessionDir == "" {
		return ""
	}
	entries, err := os.ReadDir(sessionDir)
	if err != nil {
		return ""
	}

	type candidate struct {
		uuid      string
		timestamp time.Time // from JSONL content
		modTime   time.Time // file system fallback
	}
	var best candidate

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		uuid := strings.TrimSuffix(e.Name(), ".jsonl")
		ts := jsonlLastTimestamp(filepath.Join(sessionDir, e.Name()))
		c := candidate{uuid: uuid, timestamp: ts, modTime: info.ModTime()}

		// Prefer the candidate with the most recent JSONL content timestamp.
		// Fall back to modTime when timestamps are equal or unavailable.
		var bestTs, cTs time.Time
		if !best.timestamp.IsZero() {
			bestTs = best.timestamp
		} else {
			bestTs = best.modTime
		}
		if !c.timestamp.IsZero() {
			cTs = c.timestamp
		} else {
			cTs = c.modTime
		}
		if cTs.After(bestTs) {
			best = c
		}
	}
	return best.uuid
}

// jsonlLastTimestamp reads the last JSON record from a JSONL file and returns
// its "timestamp" field. Returns zero time if unreadable or not present.
func jsonlLastTimestamp(path string) time.Time {
	f, err := os.Open(path)
	if err != nil {
		return time.Time{}
	}
	defer f.Close()

	// Seek to last 4KB to find the last complete line efficiently.
	const tailSize = 4096
	if fi, err := f.Stat(); err == nil && fi.Size() > tailSize {
		_, _ = f.Seek(-tailSize, io.SeekEnd)
	}
	buf, err := io.ReadAll(f)
	if err != nil || len(buf) == 0 {
		return time.Time{}
	}
	// Find the last non-empty line.
	lines := strings.Split(strings.TrimRight(string(buf), "\n"), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		var rec struct {
			Timestamp string `json:"timestamp"`
		}
		if err := json.Unmarshal([]byte(line), &rec); err == nil && rec.Timestamp != "" {
			if t, err := time.Parse(time.RFC3339Nano, rec.Timestamp); err == nil {
				return t
			}
			if t, err := time.Parse(time.RFC3339, rec.Timestamp); err == nil {
				return t
			}
		}
		break
	}
	return time.Time{}
}

// sessionsCmd lists named sessions in tmux-style format.
// Detects the current session via CQ_SESSION_UUID env var or filesystem.
var sessionsCmd = &cobra.Command{
	Use:   "sessions",
	Short: "List named Claude Code sessions (tmux-style)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		sessions, err := loadNamedSessions()
		if err != nil {
			return err
		}
		if len(sessions) == 0 {
			fmt.Println("No named sessions. Use 'cq claude -t <name>' to create one.")
			return nil
		}
		// Detect current session UUID: prefer env var, fall back to filesystem.
		curUUID := os.Getenv("CQ_SESSION_UUID")
		if curUUID == "" {
			curUUID = currentSessionUUID(projectDir)
		}
		// Open mailbox for unread counts (best-effort; errors silently skipped).
		var ms *mailbox.MailStore
		if homeDir, hErr := os.UserHomeDir(); hErr == nil {
			if store, msErr := mailbox.NewMailStore(filepath.Join(homeDir, ".c4", "mailbox.db")); msErr == nil {
				ms = store
				defer ms.Close()
			}
		}
		// Sort names for stable output
		names := make([]string, 0, len(sessions))
		for n := range sessions {
			names = append(names, n)
		}
		for i := 0; i < len(names)-1; i++ {
			for j := i + 1; j < len(names); j++ {
				if names[i] > names[j] {
					names[i], names[j] = names[j], names[i]
				}
			}
		}
		// Compute max name display width for column alignment.
		maxNameW := 8
		for _, n := range names {
			if w := lsDispWidth(n); w > maxNameW {
				maxNameW = w
			}
		}
		const dirColW = 22
		activeCurUUID := curUUID // snapshot for first-match duplicate prevention
		for _, n := range names {
			entry := sessions[n]
			t, tErr := time.Parse(time.RFC3339, entry.Updated)
			dateStr := "--"
			if tErr == nil {
				dateStr = t.Format("Jan 02 15:04")
			}
			shortDir := entry.Dir
			if homeDir, hErr := os.UserHomeDir(); hErr == nil {
				shortDir = strings.Replace(shortDir, homeDir, "~", 1)
			}
			if lsDispWidth(shortDir) > dirColW {
				shortDir = lsTruncateToWidth(shortDir, dirColW-1) + "…"
			}
			isCurrent := activeCurUUID != "" && entry.UUID == activeCurUUID
			if isCurrent {
				activeCurUUID = ""
			}
			indicator := "  "
			if isCurrent {
				indicator = "● "
			}
			extra := ""
			if ms != nil {
				if count, err := ms.UnreadCount(n); err == nil && count > 0 {
					extra = fmt.Sprintf("  ✉%d", count)
				}
			}
			fmt.Printf("%s%s  %s  %s  %s%s\n",
				indicator,
				lsPadToWidth(n, maxNameW),
				entry.UUID[:8],
				lsPadToWidth(shortDir, dirColW),
				dateStr,
				extra)
			if entry.Memo != "" {
				fmt.Printf("    %s\n", entry.Memo)
			}
		}
		return nil
	},
}

// Note: lsCmd is now defined in bot.go (lists bots).
// sessionsCmd above replaces the old lsCmd for session listing.

// lsIsWide reports whether rune r occupies 2 terminal columns (CJK, Hangul, etc.).
func lsIsWide(r rune) bool {
	return (r >= 0x1100 && r <= 0x115F) || // Hangul Jamo
		(r >= 0x2E80 && r <= 0x303E) || // CJK Radicals, Kangxi
		(r >= 0x3040 && r <= 0xA4CF) || // Hiragana/Katakana/CJK Unified
		(r >= 0xAC00 && r <= 0xD7A3) || // Hangul Syllables
		(r >= 0xF900 && r <= 0xFAFF) || // CJK Compatibility
		(r >= 0xFE10 && r <= 0xFE1F) ||
		(r >= 0xFE30 && r <= 0xFE4F) ||
		(r >= 0xFF00 && r <= 0xFF60) || // Fullwidth forms
		(r >= 0xFFE0 && r <= 0xFFE6) ||
		(r >= 0x1F300 && r <= 0x1F64F) || // Emoji
		(r >= 0x20000 && r <= 0x2FFFD) || // CJK Extension B+
		(r >= 0x30000 && r <= 0x3FFFD)
}

// lsDispWidth returns the terminal display width of s.
func lsDispWidth(s string) int {
	w := 0
	for _, r := range s {
		if lsIsWide(r) {
			w += 2
		} else {
			w++
		}
	}
	return w
}

// lsPadToWidth pads s with spaces until its display width equals width.
func lsPadToWidth(s string, width int) string {
	w := lsDispWidth(s)
	if w >= width {
		return s
	}
	return s + strings.Repeat(" ", width-w)
}

// lsTruncateToWidth truncates s so that its display width does not exceed maxW.
func lsTruncateToWidth(s string, maxW int) string {
	w := 0
	for i, r := range s {
		rw := 1
		if lsIsWide(r) {
			rw = 2
		}
		if w+rw > maxW {
			return s[:i]
		}
		w += rw
	}
	return s
}

// sessionCmd provides session management subcommands.
var sessionCmd = &cobra.Command{
	Use:   "session",
	Short: "Manage named Claude Code sessions",
}

var sessionNameForce bool
var sessionNameMemo string
var sessionNameUUID string

var sessionNameCmd = &cobra.Command{
	Use:   "name <session-name>",
	Short: "Attach a name to the current session",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		uuid := sessionNameUUID
		if uuid == "" {
			uuid = currentSessionUUID(projectDir)
		}
		if uuid == "" {
			return fmt.Errorf("could not detect current session UUID (no JSONL files found)")
		}
		sessions, err := loadNamedSessions()
		if err != nil {
			return err
		}
		// Conflict check: name already used by a different session.
		if existing, ok := sessions[name]; ok && existing.UUID != uuid {
			if !sessionNameForce {
				fmt.Printf("session '%s' already exists (uuid=%s...)\n", name, existing.UUID[:8])
				fmt.Printf("overwrite? [y/N] ")
				var answer string
				fmt.Fscan(cmd.InOrStdin(), &answer)
				if answer != "y" && answer != "Y" {
					fmt.Println("aborted")
					return nil
				}
			}
		}
		// Preserve memo/tool from existing entry for the same UUID (rename).
		// Delete ALL entries pointing to this UUID to avoid duplicate aliases.
		var prevMemo, prevTool string
		for k, v := range sessions {
			if v.UUID == uuid {
				if prevMemo == "" {
					prevMemo = v.Memo
				}
				if prevTool == "" {
					prevTool = v.Tool
				}
				delete(sessions, k)
			}
		}
		if sessionNameMemo != "" {
			prevMemo = sessionNameMemo
		}
		// Infer tool from environment if not already known.
		if prevTool == "" {
			if os.Getenv("CQ_SESSION_UUID") != "" || os.Getenv("CQ_SESSION_NAME") != "" {
				prevTool = "claude"
			}
		}
		sessions[name] = namedSessionEntry{
			UUID:    uuid,
			Dir:     projectDir,
			Tool:    prevTool,
			Memo:    prevMemo,
			Updated: time.Now().Format(time.RFC3339),
		}
		if err := saveNamedSessions(sessions); err != nil {
			return err
		}
		fmt.Printf("session '%s' → %s...\n", name, uuid[:8])
		fmt.Printf("Next time: cq claude -t %s\n", name)
		return nil
	},
}

var sessionMemoCmd = &cobra.Command{
	Use:   "memo <session-name> <text>",
	Short: "Set or update the memo for a named session",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		name, memo := args[0], args[1]
		sessions, err := loadNamedSessions()
		if err != nil {
			return err
		}
		entry, ok := sessions[name]
		if !ok {
			return fmt.Errorf("session '%s' not found", name)
		}
		entry.Memo = memo
		sessions[name] = entry
		if err := saveNamedSessions(sessions); err != nil {
			return err
		}
		fmt.Printf("session '%s' memo updated\n", name)
		return nil
	},
}

func init() {
	sessionNameCmd.Flags().BoolVarP(&sessionNameForce, "force", "f", false, "overwrite existing session name without confirmation")
	sessionNameCmd.Flags().StringVarP(&sessionNameMemo, "memo", "m", "", "short description of this session")
	sessionNameCmd.Flags().StringVar(&sessionNameUUID, "uuid", "", "explicitly set session UUID (bypass auto-detection)")
}

var sessionRmCmd = &cobra.Command{
	Use:   "rm <session-name>",
	Short: "Remove a named session",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		sessions, err := loadNamedSessions()
		if err != nil {
			return err
		}
		if _, ok := sessions[name]; !ok {
			return fmt.Errorf("session '%s' not found", name)
		}
		delete(sessions, name)
		if err := saveNamedSessions(sessions); err != nil {
			return err
		}
		fmt.Printf("session '%s' removed\n", name)
		return nil
	},
}

// servePIDPath overrides the default PID file location; used in tests for path isolation.
var servePIDPath string

// ensureServeRunning checks if cq serve is running and starts it in the background if not.
// If noServe is true, it skips the check entirely.
// Failures are non-fatal: a warning is printed and execution continues.
func ensureServeRunning(noServe bool) {
	if noServe {
		return
	}

	pidPath := servePIDPath
	if pidPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return
		}
		pidPath = filepath.Join(home, ".c4", "serve", "serve.pid")
	}
	data, err := os.ReadFile(pidPath)
	if err == nil {
		pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
		if err == nil && isCQServeProcess(pid) {
			// Already running — nothing to do.
			return
		}
	}

	// Not running — start cq serve in background.
	cqPath, err := os.Executable()
	if err != nil {
		return
	}

	cmd := exec.Command(cqPath, "serve")
	setDetachedProcess(cmd)
	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "cq: warn: could not start serve: %v\n", err)
		return
	}
	pid := cmd.Process.Pid
	// Release resources; we don't wait for this background process.
	_ = cmd.Process.Release()

	// Wait up to 2s for serve to become healthy.
	// 4140 is cq serve's default listen port (matches serve command default).
	const serveHealthURL = "http://127.0.0.1:4140/health"
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(serveHealthURL)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				fmt.Fprintf(os.Stderr, "cq: serve started (pid=%d)\n", pid)
				return
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	fmt.Fprintf(os.Stderr, "cq: warn: serve started (pid=%d) but health check timed out (%s)\n", pid, serveHealthURL)
}

// isCQServeProcess returns true if the given PID is a running "cq serve" process.
func isCQServeProcess(pid int) bool {
	switch runtime.GOOS {
	case "linux":
		data, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
		if err != nil {
			return false
		}
		// cmdline fields are NUL-separated
		parts := strings.Split(string(data), "\x00")
		hasCQ, hasServe := false, false
		for _, p := range parts {
			base := filepath.Base(p)
			if base == "cq" || strings.HasSuffix(p, "/cq") {
				hasCQ = true
			}
			if p == "serve" {
				hasServe = true
			}
		}
		return hasCQ && hasServe
	case "darwin":
		out, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "command=").Output()
		if err != nil {
			return false
		}
		parts := strings.Fields(strings.TrimSpace(string(out)))
		if len(parts) < 2 {
			return false
		}
		return filepath.Base(parts[0]) == "cq" && parts[1] == "serve"
	default:
		// Unsupported OS — assume not running to trigger a start attempt.
		return false
	}
}

const onboardingMsg = "만들고 싶은 게 있으면 말씀해주세요. /c4-plan으로 시작합니다."

// telegramChannelConfigured returns true when ~/.claude/channels/telegram/.env
// contains a TELEGRAM_BOT_TOKEN line, meaning the user has set up the telegram plugin.
func telegramChannelConfigured() bool {
	if os.Getenv("CQ_NO_TELEGRAM") != "" {
		return false
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	data, err := os.ReadFile(filepath.Join(home, ".claude", "channels", "telegram", ".env"))
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "TELEGRAM_BOT_TOKEN=") && len(line) > len("TELEGRAM_BOT_TOKEN=") {
			return true
		}
	}
	return false
}

// buildLaunchArgs returns [tool, ...baseArgs] with --append-system-prompt appended when firstRun is true.
func buildLaunchArgs(firstRun bool, tool string, baseArgs []string) []string {
	args := append([]string{tool}, baseArgs...)
	if firstRun {
		args = append(args, "--append-system-prompt", onboardingMsg)
	}
	if tool == "claude" && telegramChannelConfigured() {
		args = append(args, "--channels", "plugin:telegram@claude-plugins-official")
	}
	return args
}

// isFirstRun returns true when ~/.c4/first_run does not exist.
// Returns false on any error other than ErrNotExist (e.g. permission denied, HOME unset).
func isFirstRun() bool {
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	_, err = os.Stat(filepath.Join(home, ".c4", "first_run"))
	if err == nil {
		return false
	}
	return os.IsNotExist(err)
}

// markFirstRun creates ~/.c4/first_run to record that onboarding has occurred.
func markFirstRun() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	dir := filepath.Join(home, ".c4")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(dir, "first_run"), os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	return f.Close()
}

// launchTool launches the specified AI coding tool, replacing the current process.
func launchTool(tool, dir string) error {
	var toolCmd string
	var baseArgs []string

	switch tool {
	case "claude":
		toolCmd = "claude"
		baseArgs = nil
	case "codex":
		toolCmd = "codex"
		baseArgs = nil
	case "cursor":
		toolCmd = "cursor"
		baseArgs = []string{dir}
	case "gemini":
		toolCmd = "gemini"
		baseArgs = nil
	default:
		return fmt.Errorf("unknown tool: %s (supported: claude, codex, cursor, gemini)", tool)
	}

	toolPath, err := exec.LookPath(toolCmd)
	if err != nil {
		return fmt.Errorf("%s not found in PATH (install it first): %w", toolCmd, err)
	}

	first := isFirstRun()
	toolArgs := buildLaunchArgs(first, toolCmd, baseArgs)

	fmt.Fprintf(os.Stderr, "cq: launching %s...\n", tool)
	if first {
		if err := markFirstRun(); err != nil {
			fmt.Fprintf(os.Stderr, "cq: warning: markFirstRun: %v\n", err)
		}
	}
	return syscall.Exec(toolPath, toolArgs, os.Environ())
}
