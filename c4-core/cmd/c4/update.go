package main

import (
	"github.com/spf13/cobra"
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update cq to the latest release (alias for 'cq upgrade')",
	Long:  `Downloads and installs the latest cq binary from GitHub Releases. This is an alias for 'cq upgrade'.`,
	Args:  cobra.NoArgs,
	RunE:  runUpgrade,
}

func init() {
	updateCmd.Flags().StringVar(&upgradeTier, "tier", "", "override tier (solo|connected|full)")
	updateCmd.Flags().BoolVar(&upgradeCheck, "check", false, "check for updates without installing")
	rootCmd.AddCommand(updateCmd)
}
