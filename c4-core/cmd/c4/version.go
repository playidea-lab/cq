package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

// tier is set at build time via -ldflags "-X main.tier=solo|connected|full"
var tier = ""

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the cq version",
	Long:  "Print the cq version and build tier.",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if tier != "" {
			fmt.Printf("cq version %s (tier: %s)\n", version, tier)
		} else {
			fmt.Printf("cq version %s\n", version)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
