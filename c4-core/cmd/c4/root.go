package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

// version is set at build time via -ldflags "-X main.version=..."
var version = "dev"

var (
	// Global flags
	cfgFile    string
	verbose    bool
	projectDir string
)

var rootCmd = &cobra.Command{
	Use:     "c4",
	Short:   "C4 - AI orchestration system",
	Version: version,
	Long: `C4 is an AI orchestration system that automates project management
from planning through completion. It manages tasks, workers, checkpoints,
and knowledge across the entire development lifecycle.`,
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
			if cmd.Name() != "mcp" && cmd.Name() != "c4" {
				return fmt.Errorf("not a C4 project: %s (missing .c4/ directory)", projectDir)
			}
		}

		return nil
	},
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default: .c4/config.yaml)")
	rootCmd.PersistentFlags().BoolVar(&verbose, "verbose", false, "enable verbose output")
	rootCmd.PersistentFlags().StringVar(&projectDir, "dir", "", "project root directory (default: current directory)")
}

// c4Dir returns the path to the .c4 directory.
func c4Dir() string {
	return filepath.Join(projectDir, ".c4")
}

// dbPath returns the path to the tasks database.
func dbPath() string {
	return filepath.Join(c4Dir(), "tasks.db")
}
