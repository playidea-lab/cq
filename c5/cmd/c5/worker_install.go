package main

import (
	"crypto/rand"
	"fmt"
	"log"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

func workerInstallCmd() *cobra.Command {
	var (
		image    string
		hubURL   string
		apiKey   string
		name     string
		noGPU    bool
		detach   bool
		dryRun   bool
		extraEnv []string
	)

	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install and start a C5 worker via Docker",
		Long: `Pull a Docker image and start a C5 worker container.
Automatically detects GPU (nvidia-smi) and adds --gpus all.

Example:
  c5 worker install --image ghcr.io/playidea-lab/c5-worker:latest \
    --hub-url https://hub.example.com:8585 --api-key <key>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWorkerInstall(workerInstallConfig{
				Image:    image,
				HubURL:   hubURL,
				APIKey:   apiKey,
				Name:     name,
				NoGPU:    noGPU,
				Detach:   detach,
				DryRun:   dryRun,
				ExtraEnv: extraEnv,
			})
		},
	}

	cmd.Flags().StringVar(&image, "image", "", "Docker image to pull and run (required)")
	cmd.Flags().StringVar(&hubURL, "hub-url", "", "C5 Hub URL (required, or C5_HUB_URL env)")
	cmd.Flags().StringVar(&apiKey, "api-key", "", "API key for Hub auth (or C5_API_KEY env)")
	cmd.Flags().StringVar(&name, "name", "", "Container name (default: c5-worker-<random>)")
	cmd.Flags().BoolVar(&noGPU, "no-gpu", false, "Disable GPU passthrough even if available")
	cmd.Flags().BoolVarP(&detach, "detach", "d", true, "Run container in background")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Print docker commands without executing")
	cmd.Flags().StringArrayVarP(&extraEnv, "env", "e", nil, "Additional environment variables (KEY=VALUE)")

	return cmd
}

type workerInstallConfig struct {
	Image    string
	HubURL   string
	APIKey   string
	Name     string
	NoGPU    bool
	Detach   bool
	DryRun   bool
	ExtraEnv []string
}

func runWorkerInstall(cfg workerInstallConfig) error {
	// Resolve required fields
	if cfg.Image == "" {
		return fmt.Errorf("--image is required")
	}
	if cfg.HubURL == "" {
		cfg.HubURL = os.Getenv("C5_HUB_URL")
	}
	if cfg.HubURL == "" {
		return fmt.Errorf("--hub-url is required (or set C5_HUB_URL)")
	}
	if cfg.APIKey == "" {
		cfg.APIKey = os.Getenv("C5_API_KEY")
	}
	if cfg.APIKey == "" {
		return fmt.Errorf("--api-key is required (or set C5_API_KEY)")
	}

	// Container name
	if cfg.Name == "" {
		cfg.Name = "c5-worker-" + randomHex(4)
	}

	// Docker pull
	pullArgs := []string{"pull", cfg.Image}
	if cfg.DryRun {
		log.Printf("[dry-run] docker %v", pullArgs)
	} else {
		log.Printf("c5-worker-install: pulling image %s", cfg.Image)
		if err := runDockerCmd(pullArgs); err != nil {
			return fmt.Errorf("docker pull: %w", err)
		}
	}

	// Build docker run args
	runArgs := []string{"run"}
	if cfg.Detach {
		runArgs = append(runArgs, "-d")
	}
	runArgs = append(runArgs, "--name", cfg.Name)

	// GPU detection
	if !cfg.NoGPU && hasNvidiaSMI() {
		runArgs = append(runArgs, "--gpus", "all")
		log.Printf("c5-worker-install: nvidia-smi detected, enabling --gpus all")
	}

	// Environment variables
	runArgs = append(runArgs,
		"-e", "C5_HUB_URL="+cfg.HubURL,
		"-e", "C5_API_KEY="+cfg.APIKey,
		"-e", "C5_CONTAINER_MODE=1",
	)
	for _, env := range cfg.ExtraEnv {
		runArgs = append(runArgs, "-e", env)
	}

	// Restart policy
	runArgs = append(runArgs, "--restart", "unless-stopped")

	// Image
	runArgs = append(runArgs, cfg.Image)

	if cfg.DryRun {
		log.Printf("[dry-run] docker %v", runArgs)
		return nil
	}

	log.Printf("c5-worker-install: starting container %s", cfg.Name)
	if err := runDockerCmd(runArgs); err != nil {
		return fmt.Errorf("docker run: %w", err)
	}

	log.Printf("c5-worker-install: container %s started successfully", cfg.Name)
	return nil
}

// runDockerCmd executes a docker command with stdout/stderr forwarded.
func runDockerCmd(args []string) error {
	cmd := exec.Command("docker", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// hasNvidiaSMI checks if nvidia-smi is available on the system.
func hasNvidiaSMI() bool {
	_, err := exec.LookPath("nvidia-smi")
	return err == nil
}

// randomHex returns n random bytes as hex string.
func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x", b)
}
