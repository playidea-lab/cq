package main

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update cq to the latest release",
	Long:  `Downloads and installs the latest cq binary from GitHub Releases.`,
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("Updating cq...")
		sh := exec.Command("sh", "-c",
			`curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/install.sh | sh`)
		sh.Stdout = os.Stdout
		sh.Stderr = os.Stderr
		sh.Stdin = os.Stdin
		if err := sh.Run(); err != nil {
			return fmt.Errorf("update failed: %w", err)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(updateCmd)
}
