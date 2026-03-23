package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

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

var (
	upgradeTier  string
	upgradeCheck bool
)

func init() {
	upgradeCmd.Flags().StringVar(&upgradeTier, "tier", "", "override tier (solo|connected|full)")
	upgradeCmd.Flags().BoolVar(&upgradeCheck, "check", false, "check for updates without installing")
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

	// --check: version comparison only
	if upgradeCheck {
		return runUpgradeCheck(url, artifact)
	}

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
	newVersionBytes, _ := exec.Command(tmp.Name(), "version").Output()
	newVersion := strings.TrimSpace(string(newVersionBytes))

	// Skip if already up to date
	currentVersion := strings.TrimSpace(version)
	if newVersion != "" && strings.Contains(newVersion, currentVersion) {
		fmt.Printf("Already up to date (%s)\n", currentVersion)
		return nil
	}

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

	fmt.Printf("Updated: %s → %s\n", currentVersion, newVersion)

	// Auto-restart cq serve if running.
	// "cq serve install" handles: uninstall old → install new → start.
	// This works for both OS service (LaunchAgent/systemd) and manual mode.
	if isServeRunning() {
		fmt.Println("Restarting cq serve...")
		cmd := exec.Command(self, "serve", "install", "--dir", detectProjectDir())
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			// Fallback: manual stop + start
			fmt.Fprintf(os.Stderr, "serve install failed (%v), trying manual restart...\n", err)
			exec.Command(self, "serve", "stop").Run()
			time.Sleep(1 * time.Second)
			startCmd := exec.Command(self, "serve")
			startCmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
			if startErr := startCmd.Start(); startErr != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to restart serve: %v\n", startErr)
			} else {
				fmt.Println("cq serve restarted (manual mode).")
			}
		}
	}

	return nil
}

// runUpgradeCheck checks for available updates without installing.
func runUpgradeCheck(url, artifact string) error {
	// HEAD request to check if release exists
	resp, err := http.Head(url) //nolint:gosec
	if err != nil {
		return fmt.Errorf("check failed: %w", err)
	}
	resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		fmt.Printf("Update available: %s\nRun 'cq upgrade' to install.\n", artifact)
	} else if resp.StatusCode == http.StatusNotFound {
		fmt.Println("No release found for your platform.")
	} else {
		fmt.Printf("Current version: %s (could not check latest)\n", version)
	}
	return nil
}

// isServeRunning checks if cq serve is running.
func isServeRunning() bool {
	out, err := exec.Command("cq", "serve", "status").CombinedOutput()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), "running")
}

// detectProjectDir returns the current working directory (used for serve install --dir).
func detectProjectDir() string {
	dir, err := os.Getwd()
	if err != nil {
		return "."
	}
	return dir
}
