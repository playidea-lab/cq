package main

import (
	"fmt"
	"os"

	"github.com/changmin/c4-core/internal/config"
	"github.com/changmin/c4-core/internal/mcp/handlers/cfghandler"
	"github.com/spf13/cobra"
)

func init() {
	configCmd.AddCommand(configGetCmd)
	configCmd.AddCommand(configSetCmd)
	rootCmd.AddCommand(configCmd)
}

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Read or write .c4/config.yaml values",
	Long: `Read or write project configuration values in .c4/config.yaml.

Keys use dot-notation (e.g. serve.mcp_http.enabled).

Examples:
  cq config get serve.mcp_http.port
  cq config set serve.mcp_http.enabled true
  cq config set serve.mcp_http.api_key my-secret`,
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
	Short: "Set a config value in .c4/config.yaml",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		key, value := args[0], args[1]

		cfgMgr, err := config.New(projectDir)
		if err != nil && cfgMgr == nil {
			return fmt.Errorf("config load: %w", err)
		}

		configPath := cfghandler.ConfigFilePath(projectDir)
		if err := cfghandler.UpdateYAMLValue(configPath, key, value); err != nil {
			return fmt.Errorf("config set: %w", err)
		}
		fmt.Fprintf(os.Stderr, "cq: config %q = %q\n", key, value)
		return nil
	},
}
