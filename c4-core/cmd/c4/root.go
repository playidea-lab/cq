package main

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	_ "modernc.org/sqlite"
)

// version is set at build time via -ldflags "-X main.version=..."
var version = "dev"

// Built-in Supabase defaults — set at build time via -ldflags.
// These are PUBLIC values (anon key + RLS = safe to embed).
// Users don't need to configure these; they just run `cq auth login`.
var (
	builtinSupabaseURL = "" // -ldflags "-X main.builtinSupabaseURL=https://xxx.supabase.co"
	builtinSupabaseKey = "" // -ldflags "-X main.builtinSupabaseKey=eyJ..."
)

var (
	// Global flags
	cfgFile    string
	verbose    bool
	projectDir string
	yesAll     bool // --yes / -y: skip all interactive confirmations
)

var rootCmd = &cobra.Command{
	Use:     "cq",
	Short:   "CQ - AI orchestration system",
	Version: version,
	Long: `CQ is an AI orchestration system that automates project management
from planning through completion. It manages tasks, workers, checkpoints,
and knowledge across the entire development lifecycle.

Run 'cq' or 'cq claude' to init a project and launch Claude Code.
Run 'cq codex' or 'cq cursor' for other AI tools.`,
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

		// Verify .c4 directory exists
		c4Dir := filepath.Join(projectDir, ".c4")
		if _, err := os.Stat(c4Dir); os.IsNotExist(err) {
			// Allow certain commands without .c4/
			if cmd.Name() != "mcp" && cmd.Name() != "cq" &&
			cmd.Name() != "claude" && cmd.Name() != "codex" && cmd.Name() != "cursor" &&
			cmd.Name() != "serve" && cmd.Name() != "mail" &&
			cmd.Parent() != nil && cmd.Parent().Name() != "hub" && cmd.Parent().Name() != "serve" &&
			cmd.Parent() != nil && cmd.Parent().Name() != "auth" && cmd.Parent().Name() != "mail" {
				return fmt.Errorf("not a CQ project: %s (missing .c4/ directory)\n\nRun 'cq claude' to initialize this project.", projectDir)
			}
		}

		return nil
	},
	// Default: no subcommand → init + launch claude
	RunE: func(cmd *cobra.Command, args []string) error {
		return initAndLaunch("claude")
	},
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default: .c4/config.yaml)")
	rootCmd.PersistentFlags().BoolVar(&verbose, "verbose", false, "enable verbose output")
	rootCmd.PersistentFlags().StringVar(&projectDir, "dir", "", "project root directory (default: current directory)")
	rootCmd.PersistentFlags().BoolVarP(&yesAll, "yes", "y", false, "skip interactive confirmations (non-interactive/CI mode)")
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
