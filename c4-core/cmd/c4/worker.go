package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
)

var workerCmd = &cobra.Command{
	Use:   "worker",
	Short: "Start a C5 capability worker",
	Long: `Start a C5 Hub worker that registers capabilities and processes jobs.

Looks for caps.yaml in: --caps flag > .c4/caps.yaml > ./caps.yaml
Uses hub URL from: --hub flag > config.yaml hub.url > built-in default

Examples:
  cq worker                          # use .c4/caps.yaml, built-in hub URL
  cq worker --caps ~/my/caps.yaml    # custom caps file
  cq worker --hub https://my-hub.io  # override hub URL`,
	Args: cobra.NoArgs,
	RunE: runWorker,
}

var (
	workerCapsFile string
	workerHubURL   string
)

func init() {
	workerCmd.Flags().StringVar(&workerCapsFile, "caps", "", "path to caps.yaml (default: .c4/caps.yaml or ./caps.yaml)")
	workerCmd.Flags().StringVar(&workerHubURL, "hub", "", "hub URL override")
	rootCmd.AddCommand(workerCmd)
}

func runWorker(cmd *cobra.Command, args []string) error {
	// Resolve caps file
	caps := workerCapsFile
	if caps == "" {
		c4caps := filepath.Join(projectDir, ".c4", "caps.yaml")
		localcaps := filepath.Join(projectDir, "caps.yaml")
		switch {
		case fileExists(c4caps):
			caps = c4caps
		case fileExists(localcaps):
			caps = localcaps
		default:
			return fmt.Errorf("caps.yaml not found. Create one or specify with --caps\n\nExample:\n  cq worker --caps ~/example-remote/caps.yaml")
		}
	}

	// Resolve hub URL
	hub := workerHubURL
	if hub == "" {
		hub = resolveHubURL()
	}
	if hub == "" {
		return fmt.Errorf("hub URL not configured. Set hub.url in .c4/config.yaml or use --hub <url>")
	}

	// Find c5 binary: embedded (full tier) > PATH > ~/.c4/bin/c5
	c5bin, err := resolveC5Binary()
	if err != nil {
		return err
	}

	fmt.Printf("Starting c5 worker\n  caps: %s\n  hub:  %s\n  bin:  %s\n\n", caps, hub, c5bin)

	c5cmd := exec.Command(c5bin, "worker", "--capabilities", caps, "--hub", hub)
	c5cmd.Stdout = os.Stdout
	c5cmd.Stderr = os.Stderr
	c5cmd.Stdin = os.Stdin
	return c5cmd.Run()
}

// resolveC5Binary finds the c5 binary: embedded > PATH > ~/.c4/bin/c5.
func resolveC5Binary() (string, error) {
	// Full tier: extract from embedded FS
	if EmbeddedC5FS != nil {
		path, err := ExtractEmbeddedC5()
		if err == nil {
			return path, nil
		}
	}

	// PATH
	if path, err := exec.LookPath("c5"); err == nil {
		return path, nil
	}

	// ~/.c4/bin/c5
	home, _ := os.UserHomeDir()
	cached := filepath.Join(home, ".c4", "bin", "c5")
	if fileExists(cached) {
		return cached, nil
	}

	return "", fmt.Errorf(`c5 binary not found.

Options:
  1. Upgrade to full tier:  cq upgrade --tier full
  2. Install c5 manually:   curl -fsSL https://github.com/PlayIdea-Lab/cq/releases/latest/download/c5-%s-%s -o ~/.local/bin/c5 && chmod +x ~/.local/bin/c5`,
		detectGOOS(), detectGOARCH())
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func detectGOOS() string {
	if v := os.Getenv("GOOS"); v != "" {
		return v
	}
	return "linux"
}

func detectGOARCH() string {
	if v := os.Getenv("GOARCH"); v != "" {
		return v
	}
	return "amd64"
}
