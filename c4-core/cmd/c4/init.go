package main

import (
	"bufio"
	_ "embed"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/changmin/c4-core/internal/botstore"
	"github.com/changmin/c4-core/internal/knowledge"
	stdpkg "github.com/changmin/c4-core/internal/standards"
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
	// Register --tier flag on AI tool commands (not root — root is service start now).
	for _, cmd := range []*cobra.Command{claudeCmd, codexCmd, cursorCmd, geminiCmd} {
		cmd.Flags().StringVar(&initTier, "tier", "", "build tier: solo|connected|full (written to .c4/config.yaml)")
	}
	// Register -t/--tag flag for named sessions (claude and gemini only).
	for _, cmd := range []*cobra.Command{claudeCmd, geminiCmd} {
		cmd.Flags().StringVarP(&sessionName, "tag", "t", "", "session name: resume or create named AI session")
		_ = cmd.RegisterFlagCompletionFunc("tag", completeSessionNames)
	}
	// Register --bot flag for telegram bot selection (claude only).
	for _, cmd := range []*cobra.Command{claudeCmd} {
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

	// When launched from `cq claude` (non-verbose), stderr is /dev/null.
	// The prompt would be invisible but stdin still blocks. Auto-approve.
	if os.Stderr != nil && os.Stderr.Name() == os.DevNull {
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
	verbose := os.Getenv("CQ_VERBOSE") == "1"
	var warnings []string
	logStep := func(msg string) {
		if verbose {
			fmt.Fprintln(os.Stderr, "✓ "+msg)
		}
	}
	logWarn := func(msg string) {
		warnings = append(warnings, msg)
		if verbose {
			fmt.Fprintf(os.Stderr, "cq: warning: %s\n", msg)
		}
	}

	// Quiet mode: suppress setup functions' stderr output unless verbose.
	// Errors are still propagated via return values — only informational
	// messages (e.g. "cq: CLAUDE.md created") are suppressed.
	var origStderr *os.File
	if !verbose {
		devNull, _ := os.Open(os.DevNull)
		if devNull != nil {
			origStderr = os.Stderr
			os.Stderr = devNull
		}
	}

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
	logStep("C4 Engine 로드")

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
	logStep("지식 베이스 연결")

	// 3. Create/update CLAUDE.md with C4 overrides
	if err := setupClaudeMD(dir); err != nil {
		logWarn(fmt.Sprintf("CLAUDE.md setup failed: %v", err))
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
			// no lock file — common-only
		}
		result, err := stdpkg.Apply(dir, applyTeam, applyLangs, stdpkg.ApplyOptions{})
		if err != nil {
			logWarn(fmt.Sprintf("standards setup failed: %v", err))
		} else {
			logStep(fmt.Sprintf("표준 규칙 적용 (%d files)", len(result.FilesCreated)))
		}
	}

	// 3c. Load relevant documents from global knowledge store into project store (non-fatal).
	{
		projectKnowledgeDir := filepath.Join(dir, ".c4", "knowledge")
		projectStore, psErr := knowledge.NewStore(projectKnowledgeDir)
		if psErr == nil {
			gkm := knowledge.NewGlobalKnowledgeManager()
			domains := knowledge.DetectProjectDomains(dir)
			n, loadErr := gkm.LoadRelevant(projectStore, domains)
			if loadErr != nil {
				logWarn(fmt.Sprintf("global knowledge load failed: %v", loadErr))
			} else if n > 0 {
				logStep(fmt.Sprintf("글로벌 지식 %d건 로드", n))
			}
			projectStore.Close()
		}
	}

	// 4. Deploy C4 skills (symlinks from C4 source)
	if err := setupSkills(dir); err != nil {
		logWarn(fmt.Sprintf("skills setup failed: %v", err))
	}

	// 5. Install project hooks to .claude/hooks/ (requires user confirmation)
	if confirmProjectHooks(dir) {
		if err := setupProjectHooks(dir); err != nil {
			logWarn(fmt.Sprintf("hooks setup failed: %v", err))
		}
	}

	// 5b. Add shell completion to ~/.zshrc / ~/.bashrc (non-fatal)
	setupShellCompletion()

	// 6. Codex-specific setup
	if tool == "codex" {
		if err := setupCodexConfig(dir); err != nil {
			logWarn(fmt.Sprintf("codex config setup failed: %v", err))
		}
		if err := setupCodexAgents(dir); err != nil {
			logWarn(fmt.Sprintf("codex agents setup failed: %v", err))
		}
	}

	// 6b. Cursor-specific setup: .cursor/mcp.json so Cursor loads C4 MCP on start
	if tool == "cursor" {
		if err := setupCursorMCPConfig(dir); err != nil {
			return fmt.Errorf("setting up .cursor/mcp.json: %w", err)
		}
	}
	logStep("MCP 서버 준비")

	// 6c. Ensure git repo and .cqdata exist (non-fatal)
	if err := ensureGitRepo(dir); err != nil {
		logWarn(fmt.Sprintf("git init failed: %v", err))
	}
	if err := ensureCQData(dir); err != nil {
		logWarn(fmt.Sprintf(".cqdata setup failed: %v", err))
	}

	// 6e. Install post-merge git hook for automatic dataset sync (non-fatal)
	if err := installPostMergeHook(filepath.Join(dir, ".git")); err != nil {
		logWarn(fmt.Sprintf("post-merge hook setup failed: %v", err))
	}

	// 6d. Ensure CQ files are in .gitignore (non-fatal)
	ensureGitignore(dir)

	// 7. Check cloud auth status and prompt login if needed
	ensureCloudAuth(nil, yesAll)

	// 7b. Ensure cq serve is running in background (non-fatal if it fails)
	ensureServeRunning(noServe)
	logStep("세션 컨텍스트 주입")

	// Restore stderr before printing ready message.
	if origStderr != nil {
		os.Stderr = origStderr
	}

	// Print compact ready message (or verbose box)
	if len(warnings) > 0 {
		for _, w := range warnings {
			fmt.Fprintf(os.Stderr, "⚠ %s\n", w)
		}
		fmt.Fprintln(os.Stderr, "CQ ready (with warnings).")
	} else if verbose {
		printReadyBox(os.Stderr)
	} else {
		fmt.Fprintln(os.Stderr, "CQ ready.")
	}

	// 8. Launch AI tool

	// Bot token injection: propagate C4_TELEGRAM_BOT_TOKEN → TELEGRAM_BOT_TOKEN so the
	// telegram plugin can start the bot server when claude launches.
	// Skip when CQ_NO_TELEGRAM is set (user launched without --bot).
	if tool == "claude" && os.Getenv("CQ_NO_TELEGRAM") == "" {
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
		if name == "" {
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
