package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/changmin/c4-core/internal/config"
	"github.com/changmin/c4-core/internal/hub"
	"github.com/spf13/cobra"
)

var hubCmd = &cobra.Command{
	Use:   "hub",
	Short: "Manage PiQ Hub connection",
	Long: `Manage PiQ Hub connection for remote GPU job execution.

Subcommands:
  status   - Check Hub connection and show queue stats
  register - Register this machine as a worker
  run      - Register + claim jobs in a loop (worker daemon)`,
}

var hubStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check Hub connection and show stats",
	RunE:  runHubStatus,
}

var hubRegisterCmd = &cobra.Command{
	Use:   "register",
	Short: "Register this machine as a Hub worker",
	Long: `Register this machine as a GPU worker in the PiQ Hub.

Detects local GPU capabilities automatically via nvidia-smi
and registers with the Hub. The worker ID is printed on success.

Example:
  c4 hub register
  c4 hub register --name "lab-rtx4090"`,
	RunE: runHubRegister,
}

var hubRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Run worker daemon (register + heartbeat + claim loop)",
	Long: `Start a worker daemon that registers with the Hub,
sends periodic heartbeats, and claims jobs from the queue.

The daemon runs until interrupted (Ctrl+C).

Example:
  c4 hub run
  c4 hub run --interval 30`,
	RunE: runHubRun,
}

// Flags
var (
	hubWorkerName     string
	hubHeartbeatSec   int
)

func init() {
	hubRegisterCmd.Flags().StringVar(&hubWorkerName, "name", "", "worker name (default: hostname)")
	hubRunCmd.Flags().StringVar(&hubWorkerName, "name", "", "worker name (default: hostname)")
	hubRunCmd.Flags().IntVar(&hubHeartbeatSec, "interval", 60, "heartbeat interval in seconds")

	hubCmd.AddCommand(hubStatusCmd)
	hubCmd.AddCommand(hubRegisterCmd)
	hubCmd.AddCommand(hubRunCmd)
	rootCmd.AddCommand(hubCmd)
}

// newHubClient creates a Hub client from project config.
func newHubClient() (*hub.Client, error) {
	cfgMgr, err := config.New(projectDir)
	if err != nil {
		return nil, fmt.Errorf("loading config: %w", err)
	}

	hubCfg := cfgMgr.GetConfig().Hub
	if !hubCfg.Enabled {
		return nil, fmt.Errorf("hub is not enabled in .c4/config.yaml\n\nAdd:\n  hub:\n    enabled: true\n    url: \"http://<hub-ip>:8000\"\n    api_key_env: \"C4_HUB_API_KEY\"\n    team_id: \"my-team\"")
	}

	client := hub.NewClient(hub.HubConfig{
		Enabled:   hubCfg.Enabled,
		URL:       hubCfg.URL,
		APIKey:    hubCfg.APIKey,
		APIKeyEnv: hubCfg.APIKeyEnv,
		TeamID:    hubCfg.TeamID,
	})

	if !client.IsAvailable() {
		return nil, fmt.Errorf("hub API key not configured. Set %s environment variable or hub.api_key in config", hubCfg.APIKeyEnv)
	}

	return client, nil
}

// =========================================================================
// c4 hub status
// =========================================================================

func runHubStatus(cmd *cobra.Command, args []string) error {
	client, err := newHubClient()
	if err != nil {
		return err
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	// Health check
	healthy := client.HealthCheck()
	if healthy {
		fmt.Fprintf(w, "Hub:\tconnected\n")
	} else {
		fmt.Fprintf(w, "Hub:\tunreachable\n")
		w.Flush()
		return fmt.Errorf("cannot connect to Hub")
	}

	// Queue stats
	stats, err := client.GetQueueStats()
	if err != nil {
		fmt.Fprintf(w, "Stats:\terror (%v)\n", err)
		w.Flush()
		return nil
	}

	fmt.Fprintf(w, "\nQueue:\n")
	fmt.Fprintf(w, "  Queued:\t%d\n", stats.Queued)
	fmt.Fprintf(w, "  Running:\t%d\n", stats.Running)
	fmt.Fprintf(w, "  Succeeded:\t%d\n", stats.Succeeded)
	fmt.Fprintf(w, "  Failed:\t%d\n", stats.Failed)
	total := stats.Queued + stats.Running + stats.Succeeded + stats.Failed + stats.Cancelled
	fmt.Fprintf(w, "  Total:\t%d\n", total)

	// Workers
	workers, err := client.ListWorkers()
	if err == nil && len(workers) > 0 {
		fmt.Fprintf(w, "\nWorkers:\t%d\n", len(workers))
		for _, wk := range workers {
			gpu := wk.GPUModel
			if gpu == "" {
				gpu = "no GPU"
			}
			fmt.Fprintf(w, "  %s:\t%s  %s  (%.1f/%.1f GB VRAM)\n",
				wk.ID, wk.Status, gpu, wk.FreeVRAM, wk.TotalVRAM)
		}
	}

	w.Flush()
	return nil
}

// =========================================================================
// c4 hub register
// =========================================================================

func runHubRegister(cmd *cobra.Command, args []string) error {
	client, err := newHubClient()
	if err != nil {
		return err
	}

	// Health check first
	if !client.HealthCheck() {
		return fmt.Errorf("Hub is unreachable")
	}

	// Detect GPU capabilities
	caps, err := detectGPUCapabilities()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: GPU detection failed: %v\n", err)
		caps = map[string]any{"gpu_count": 0, "backend": "cpu"}
	}

	if hubWorkerName != "" {
		caps["hostname"] = hubWorkerName
	}

	fmt.Printf("Registering worker with Hub...\n")
	if gpuCount, ok := caps["gpu_count"].(int); ok {
		fmt.Printf("  GPUs detected: %d\n", gpuCount)
	}
	if model, ok := caps["gpu_model"].(string); ok && model != "" {
		fmt.Printf("  GPU model: %s\n", model)
	}
	if vram, ok := caps["total_vram_gb"].(float64); ok {
		fmt.Printf("  Total VRAM: %.1f GB\n", vram)
	}

	workerID, err := client.RegisterWorker(caps)
	if err != nil {
		return fmt.Errorf("registration failed: %w", err)
	}

	fmt.Printf("\nRegistered as worker: %s\n", workerID)
	return nil
}

// =========================================================================
// c4 hub run (daemon mode)
// =========================================================================

func runHubRun(cmd *cobra.Command, args []string) error {
	client, err := newHubClient()
	if err != nil {
		return err
	}

	if !client.HealthCheck() {
		return fmt.Errorf("Hub is unreachable")
	}

	// Detect and register
	caps, err := detectGPUCapabilities()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: GPU detection failed: %v\n", err)
		caps = map[string]any{"gpu_count": 0, "backend": "cpu"}
	}
	if hubWorkerName != "" {
		caps["hostname"] = hubWorkerName
	}

	workerID, err := client.RegisterWorker(caps)
	if err != nil {
		return fmt.Errorf("registration failed: %w", err)
	}
	fmt.Printf("Registered as worker: %s\n", workerID)

	// Signal handling
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	ticker := time.NewTicker(time.Duration(hubHeartbeatSec) * time.Second)
	defer ticker.Stop()

	fmt.Printf("Worker daemon started (heartbeat every %ds, Ctrl+C to stop)\n", hubHeartbeatSec)

	for {
		select {
		case <-sigCh:
			fmt.Println("\nShutting down worker...")
			_ = client.Heartbeat("offline")
			return nil

		case <-ticker.C:
			if err := client.Heartbeat("online"); err != nil {
				fmt.Fprintf(os.Stderr, "Heartbeat failed: %v\n", err)
			} else if verbose {
				fmt.Println("Heartbeat sent")
			}
		}
	}
}

// =========================================================================
// GPU detection (nvidia-smi)
// =========================================================================

// detectGPUCapabilities collects GPU info from nvidia-smi.
// Falls back gracefully if nvidia-smi is not available.
func detectGPUCapabilities() (map[string]any, error) {
	hostname, _ := os.Hostname()
	caps := map[string]any{
		"hostname":  hostname,
		"gpu_count": 0,
		"backend":   "cpu",
	}

	// Try nvidia-smi JSON query
	out, err := exec.Command("nvidia-smi",
		"--query-gpu=index,name,memory.total,memory.free,utilization.gpu,temperature.gpu",
		"--format=csv,noheader,nounits").Output()
	if err != nil {
		// nvidia-smi not available — check for macOS MPS
		if _, mpsErr := os.Stat("/System/Library/Frameworks/Metal.framework"); mpsErr == nil {
			caps["backend"] = "mps"
			caps["gpu_count"] = 1
			caps["gpu_model"] = "Apple Silicon"
		}
		return caps, err
	}

	caps["backend"] = "cuda"
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")

	gpus := []map[string]any{}
	var totalVRAM, freeVRAM float64
	var gpuModel string

	for _, line := range lines {
		fields := strings.Split(line, ", ")
		if len(fields) < 6 {
			continue
		}

		var index int
		var memTotal, memFree float64
		var util int
		var temp float64

		fmt.Sscanf(strings.TrimSpace(fields[0]), "%d", &index)
		name := strings.TrimSpace(fields[1])
		fmt.Sscanf(strings.TrimSpace(fields[2]), "%f", &memTotal)
		fmt.Sscanf(strings.TrimSpace(fields[3]), "%f", &memFree)
		fmt.Sscanf(strings.TrimSpace(fields[4]), "%d", &util)
		fmt.Sscanf(strings.TrimSpace(fields[5]), "%f", &temp)

		// MiB → GB
		memTotalGB := memTotal / 1024.0
		memFreeGB := memFree / 1024.0

		gpus = append(gpus, map[string]any{
			"index":            index,
			"name":             name,
			"total_vram_gb":    memTotalGB,
			"free_vram_gb":     memFreeGB,
			"gpu_util_percent": util,
			"temperature":      temp,
		})

		totalVRAM += memTotalGB
		freeVRAM += memFreeGB
		if gpuModel == "" {
			gpuModel = name
		}
	}

	caps["gpu_count"] = len(gpus)
	caps["gpu_model"] = gpuModel
	caps["total_vram_gb"] = totalVRAM
	caps["free_vram_gb"] = freeVRAM
	caps["gpus"] = gpus

	// Serialize for verbose output
	if verbose {
		j, _ := json.MarshalIndent(caps, "", "  ")
		fmt.Fprintf(os.Stderr, "GPU capabilities:\n%s\n", j)
	}

	return caps, nil
}
