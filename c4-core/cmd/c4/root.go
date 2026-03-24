package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/changmin/c4-core/internal/serve"
	"github.com/spf13/cobra"
	_ "modernc.org/sqlite"
)

// version is set at build time via -ldflags "-X main.version=..."
var version = "dev"

// Built-in defaults — set at build time via -ldflags.
// These are PUBLIC values (anon key + RLS = safe to embed).
// Users don't need to configure these; they just run `cq auth login`.
var (
	builtinSupabaseURL = "" // -ldflags "-X main.builtinSupabaseURL=https://xxx.supabase.co"
	builtinSupabaseKey = "" // -ldflags "-X main.builtinSupabaseKey=eyJ..."
	builtinHubURL      = "" // -ldflags "-X main.builtinHubURL=..." (legacy, unused)
)

var (
	// Global flags
	cfgFile    string
	verbose    bool
	projectDir string
	yesAll     bool // --yes / -y: skip all interactive confirmations
	noServe    bool // --no-serve: skip auto-starting cq serve
)

var rootCmd = &cobra.Command{
	Use:     "cq",
	Short:   "CQ - AI orchestration system",
	Version: version,
	Long: `CQ is an AI orchestration system that automates project management
from planning through completion.

Run 'cq' to start the CQ service (login + background service).
Run 'cq claude' to launch Claude Code, 'cq cursor' for Cursor.`,
	SilenceUsage:  true,
	SilenceErrors: true,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Resolve project directory
		if projectDir == "" {
			var err error
			projectDir, err = os.Getwd()
			if err != nil {
				return fmt.Errorf("failed to get working directory: %w", err)
			}
		}

		absDir, err := filepath.Abs(projectDir)
		if err != nil {
			return fmt.Errorf("failed to resolve directory: %w", err)
		}
		projectDir = absDir

		// Walk up to find the best .c4 directory (prefer one with config.yaml).
		// This handles monorepo layouts where subdirectories have .c4/ without config.
		projectDir = findBestC4Root(projectDir)

		// Verify .c4 directory exists
		c4Dir := filepath.Join(projectDir, ".c4")
		if _, err := os.Stat(c4Dir); os.IsNotExist(err) {
			// Allow certain commands without .c4/
			if cmd.Name() != "mcp" && cmd.Name() != "cq" &&
				cmd.Name() != "claude" && cmd.Name() != "codex" && cmd.Name() != "cursor" &&
				cmd.Name() != "serve" && cmd.Name() != "mail" && cmd.Name() != "completion" &&
				cmd.Name() != "version" &&
				cmd.Name() != "__complete" && cmd.Name() != "__completeNoDesc" &&
				cmd.Parent() != nil && cmd.Parent().Name() != "hub" && cmd.Parent().Name() != "serve" &&
				cmd.Parent().Name() != "auth" && cmd.Parent().Name() != "mail" &&
				cmd.Parent().Name() != "tunnel" &&
				cmd.Name() != "transfer" {
				return fmt.Errorf("not a CQ project: %s (missing .c4/ directory)\n\nRun 'cq claude' to initialize this project.", projectDir)
			}
		}

		return nil
	},
	// Default: no subcommand → ensure service is running
	RunE: runCQStart,
}

// completionCmd generates shell completion scripts.
var completionCmd = &cobra.Command{
	Use:   "completion [bash|zsh|fish]",
	Short: "Generate shell completion script",
	Long: `Generate a shell completion script for cq.

Add to your shell profile:

  # bash (~/.bashrc or ~/.bash_profile)
  eval "$(cq completion bash)"

  # zsh (~/.zshrc)
  eval "$(cq completion zsh)"

  # fish (~/.config/fish/config.fish)
  cq completion fish | source
`,
	ValidArgs:             []string{"bash", "zsh", "fish"},
	Args:                  cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
	DisableFlagsInUseLine: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		switch args[0] {
		case "bash":
			return rootCmd.GenBashCompletion(os.Stdout)
		case "zsh":
			return rootCmd.GenZshCompletion(os.Stdout)
		case "fish":
			return rootCmd.GenFishCompletion(os.Stdout, true)
		}
		return nil
	},
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default: .c4/config.yaml)")
	rootCmd.PersistentFlags().BoolVar(&verbose, "verbose", false, "enable verbose output")
	rootCmd.PersistentFlags().StringVar(&projectDir, "dir", "", "project root directory (default: current directory)")
	rootCmd.PersistentFlags().BoolVarP(&yesAll, "yes", "y", false, "skip interactive confirmations (non-interactive/CI mode)")
	rootCmd.PersistentFlags().BoolVar(&noServe, "no-serve", false, "skip auto-starting cq serve in background")
	rootCmd.PersistentFlags().MarkHidden("no-serve")

	// Command groups for --help display
	rootCmd.AddGroup(&cobra.Group{ID: "ai", Title: "AI Tools:"})
	rootCmd.AddGroup(&cobra.Group{ID: "mgmt", Title: "Management:"})

	completionCmd.Hidden = true
	rootCmd.AddCommand(completionCmd)

	// applyCommandVisibility is called from main() before Execute()
	// so --help sees the correct grouping and hidden state.
}

// findBestC4Root walks up from dir to find the best project root.
// Prefers a .c4/ directory that contains config.yaml (complete project).
// Falls back to the first .c4/ directory found if none have config.yaml.
// Returns the original dir if no .c4/ is found anywhere.
func findBestC4Root(dir string) string {
	home, _ := os.UserHomeDir()
	first := "" // first dir with .c4/ (fallback)
	cur := dir
	for {
		// Never treat the home directory as a project root.
		// ~/.c4/ is the global config directory used by cq serve/auth — not a project.
		if home != "" && cur == home {
			break
		}

		c4 := filepath.Join(cur, ".c4")
		if info, err := os.Stat(c4); err == nil && info.IsDir() {
			if first == "" {
				first = cur
			}
			// Prefer directory with config.yaml (complete project setup)
			cfg := filepath.Join(c4, "config.yaml")
			if _, err := os.Stat(cfg); err == nil {
				return cur
			}
		}

		// Stop at git repository root — .git/ is the project boundary.
		// This prevents climbing out of a repo into a parent that happens
		// to have .c4/ (e.g., home directory or a monorepo container).
		gitDir := filepath.Join(cur, ".git")
		if info, err := os.Stat(gitDir); err == nil && (info.IsDir() || info.Mode().IsRegular()) {
			// .git can be a file (worktree) or directory — both mean git root.
			break
		}

		parent := filepath.Dir(cur)
		if parent == cur {
			break // reached filesystem root
		}
		cur = parent
	}
	if first != "" {
		return first
	}
	return dir
}

// c4Dir returns the path to the .c4 directory.
func c4Dir() string {
	return filepath.Join(projectDir, ".c4")
}

// dbPath returns the path to the main C4 database.
// The Python daemon uses .c4/c4.db as the primary database.
func dbPath() string {
	// Prefer c4.db (shared with Python daemon)
	primary := filepath.Join(c4Dir(), "c4.db")
	if _, err := os.Stat(primary); err == nil {
		return primary
	}
	// Fallback to tasks.db (standalone Go)
	return filepath.Join(c4Dir(), "tasks.db")
}

// openDB opens the SQLite database with WAL mode and busy timeout.
// MaxOpenConns=1 prevents SQLITE_BUSY_SNAPSHOT (517) from Go's connection pool.
func openDB() (*sql.DB, error) {
	db, err := sql.Open("sqlite", dbPath())
	if err != nil {
		return nil, err
	}
	// Single connection prevents WAL snapshot conflicts between pooled connections
	db.SetMaxOpenConns(1)
	db.Exec("PRAGMA journal_mode=WAL")
	db.Exec("PRAGMA busy_timeout=30000") // 30s: supports up to ~8 concurrent sessions
	return db, nil
}

// applyCommandVisibility assigns groups to visible commands and hides the rest.
func applyCommandVisibility() {
	aiTools := map[string]bool{"claude": true, "cursor": true, "codex": true, "gemini": true}
	mgmt := map[string]bool{"status": true, "stop": true, "update": true, "doctor": true}

	for _, c := range rootCmd.Commands() {
		name := c.Name()
		switch {
		case aiTools[name]:
			c.GroupID = "ai"
		case mgmt[name]:
			c.GroupID = "mgmt"
		case name == "help":
			// keep visible, no group
		default:
			c.Hidden = true
		}
	}
}

// runCQStart is the default command when `cq` is run without subcommands.
// It ensures login → service install → prints status.
func runCQStart(cmd *cobra.Command, args []string) error {
	// Step 1: Check login status. If not logged in, initiate OAuth.
	authClient, err := newAuthClient()
	if err == nil {
		session, getErr := authClient.GetSession()
		if getErr != nil || session == nil || session.AccessToken == "" {
			fmt.Println("CQ requires authentication to connect to cloud services.")
			fmt.Println()
			if err := runAuthLogin(cmd, nil); err != nil {
				fmt.Fprintf(os.Stderr, "Login skipped: %v\n", err)
				fmt.Fprintln(os.Stderr, "Run 'cq auth login' later to enable cloud features.")
			}
		}
	}

	// Step 2: Ensure OS service is installed and running.
	alreadyRunning := serve.IsServeRunning()
	if !alreadyRunning {
		fmt.Println("Starting CQ service...")
		if err := installServeService(context.Background(), true); err != nil {
			fmt.Fprintf(os.Stderr, "Service install failed: %v\n", err)
			fmt.Fprintln(os.Stderr, "Try: cq serve (foreground mode)")
			return nil
		}
	}

	// Step 3: Print status summary box.
	fmt.Println()
	fmt.Printf("  CQ %s\n", version)
	fmt.Println("  " + strings.Repeat("-", 40))

	components, healthErr := fetchServeHealth(servePort)
	if healthErr != nil {
		fmt.Println("  Service: starting...")
	} else {
		okCount := 0
		for _, h := range components {
			if h.Status == "ok" {
				okCount++
			}
		}
		fmt.Printf("  Service: running (%d/%d components)\n", okCount, len(components))
	}

	fmt.Println("  " + strings.Repeat("-", 40))
	fmt.Println()
	if !alreadyRunning {
		fmt.Println("  Ready! Next steps:")
	} else {
		fmt.Println("  Next:")
	}
	fmt.Println("    cq claude        Start Claude Code")
	fmt.Println("    cq status        Service + project status")
	fmt.Println()
	return nil
}
