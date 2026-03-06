package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/spf13/cobra"
)

var upgradeCmd = &cobra.Command{
	Use:   "upgrade",
	Short: "Upgrade cq to the latest version",
	Long: `Download and install the latest cq release from GitHub.

Uses the same tier as the current binary (solo/connected/full).
Override with --tier if needed.`,
	Args: cobra.NoArgs,
	RunE: runUpgrade,
}

var upgradeTier string

func init() {
	upgradeCmd.Flags().StringVar(&upgradeTier, "tier", "", "override tier (solo|connected|full)")
	rootCmd.AddCommand(upgradeCmd)
}

func runUpgrade(cmd *cobra.Command, args []string) error {
	// Determine tier: flag > binary's built-in tier > "connected" default
	t := upgradeTier
	if t == "" {
		t = tier
	}
	if t == "" {
		t = "connected"
	}

	goos := runtime.GOOS
	goarch := runtime.GOARCH
	if goarch == "aarch64" {
		goarch = "arm64"
	}

	artifact := fmt.Sprintf("cq-%s-%s-%s", t, goos, goarch)
	url := fmt.Sprintf("https://github.com/PlayIdea-Lab/cq/releases/latest/download/%s", artifact)

	fmt.Printf("Upgrading cq (tier: %s, %s/%s)...\n", t, goos, goarch)

	// Download to temp file
	tmp, err := os.CreateTemp("", "cq-upgrade-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	defer os.Remove(tmp.Name())

	resp, err := http.Get(url) //nolint:gosec // URL is constructed from known safe values
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed (HTTP %d) — check https://github.com/PlayIdea-Lab/cq/releases", resp.StatusCode)
	}

	if _, err := io.Copy(tmp, resp.Body); err != nil {
		return fmt.Errorf("write download: %w", err)
	}
	tmp.Close()

	if err := os.Chmod(tmp.Name(), 0755); err != nil {
		return fmt.Errorf("chmod: %w", err)
	}

	// Get new version before replacing
	newVersion, _ := exec.Command(tmp.Name(), "version").Output()

	// Replace current binary
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve self path: %w", err)
	}
	self, _ = filepath.EvalSymlinks(self)

	// Atomic replace: rename old, move new
	old := self + ".old"
	if err := os.Rename(self, old); err != nil {
		return fmt.Errorf("backup current binary: %w (try sudo?)", err)
	}
	if err := os.Rename(tmp.Name(), self); err != nil {
		// Rollback
		os.Rename(old, self)
		return fmt.Errorf("install new binary: %w", err)
	}
	os.Remove(old)

	fmt.Printf("Upgraded: %s\n", self)
	if len(newVersion) > 0 {
		fmt.Printf("Version:  %s", newVersion)
	}
	fmt.Println("\nDone! Restart your shell or MCP server to use the new version.")
	return nil
}
