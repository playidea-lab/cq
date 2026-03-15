package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/changmin/c4-core/internal/config"
	"github.com/changmin/c4-core/internal/mcp/handlers/cfghandler"
	"github.com/spf13/cobra"
)

var configGlobal bool

func init() {
	configCmd.AddCommand(configGetCmd)
	configCmd.AddCommand(configSetCmd)
	configSetCmd.Flags().BoolVar(&configGlobal, "global", false, "write to ~/.c4/config.yaml (shared across all projects)")
	rootCmd.AddCommand(configCmd)
}

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Read or write config values",
	Long: `Read or write configuration values.

Keys use dot-notation (e.g. serve.mcp_http.enabled).

Config layers (highest priority first):
  1. Environment variables (C4_*)
  2. Project config (.c4/config.yaml)
  3. Global config (~/.c4/config.yaml)
  4. Built-in defaults

Examples:
  cq config get serve.mcp_http.port
  cq config set serve.mcp_http.enabled true
  cq config set --global cloud.mode cloud-primary
  cq config set --global permission_reviewer.enabled true`,
}

var configGetCmd = &cobra.Command{
	Use:   "get <key>",
	Short: "Get a config value",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfgMgr, err := config.New(projectDir)
		if err != nil && cfgMgr == nil {
			return fmt.Errorf("config load: %w", err)
		}
		fmt.Println(cfgMgr.Get(args[0]))
		return nil
	},
}

var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a config value",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		key, value := args[0], args[1]

		var configPath string
		if configGlobal {
			home, err := os.UserHomeDir()
			if err != nil {
				return fmt.Errorf("home dir: %w", err)
			}
			globalDir := filepath.Join(home, ".c4")
			if err := os.MkdirAll(globalDir, 0o755); err != nil {
				return fmt.Errorf("create ~/.c4: %w", err)
			}
			configPath = filepath.Join(globalDir, "config.yaml")
		} else {
			configPath = cfghandler.ConfigFilePath(projectDir)
		}

		if err := cfghandler.UpdateYAMLValue(configPath, key, value); err != nil {
			return fmt.Errorf("config set: %w", err)
		}

		scope := "project"
		if configGlobal {
			scope = "global"
		}
		fmt.Fprintf(os.Stderr, "cq: config [%s] %q = %q\n", scope, key, value)
		return nil
	},
}
