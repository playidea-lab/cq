package main

import (
	"context"
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
	"gopkg.in/yaml.v3"
)

var hubCmd = &cobra.Command{
	Use:   "hub",
	Short: "Manage PiQ Hub connection",
	Long: `Manage PiQ Hub connection for remote GPU job execution.

Subcommands:
  status   - Check Hub connection and show queue stats
  register - Register this machine as a worker
  run      - Register + claim jobs in a loop (worker daemon)
  watch    - Watch job metrics in real-time via WebSocket
  edge     - Manage edge devices for artifact deployment`,
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
  cq hub register
  cq hub register --name "lab-rtx4090"`,
	RunE: runHubRegister,
}

var hubWatchCmd = &cobra.Command{
	Use:   "watch [job_id]",
	Short: "Watch job metrics in real-time via WebSocket",
	Long: `Stream training metrics for a Hub job in real-time.

Connects via WebSocket and prints metrics as they arrive.
Stops when the job completes or is cancelled.

Example:
  cq hub watch job-abc123
  cq hub watch job-abc123 --history`,
	Args: cobra.ExactArgs(1),
	RunE: runHubWatch,
}

var hubRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Run worker daemon (register + heartbeat + claim loop)",
	Long: `Start a worker daemon that registers with the Hub,
sends periodic heartbeats, and claims jobs from the queue.

The daemon runs until interrupted (Ctrl+C).

Example:
  cq hub run
  cq hub run --interval 30`,
	RunE: runHubRun,
}

var hubEdgeCmd = &cobra.Command{
	Use:   "edge",
	Short: "Manage edge devices for artifact deployment",
	Long: `Manage edge devices registered with the Hub for model deployment.

Subcommands:
  init     - Configure edge agent credentials (hub URL + API key)
  start    - Start c5 edge-agent subprocess
  install  - Install as a system service (systemd / launchd)
  register - Register this machine as an edge device
  list     - List all registered edge devices`,
}

var hubEdgeRegisterCmd = &cobra.Command{
	Use:   "register",
	Short: "Register this machine as an edge device",
	Long: `Register this machine as an edge device for artifact deployment.

Edge devices receive trained model artifacts from Hub workers.
Supports architecture/runtime detection for deployment filtering.

Example:
  cq hub edge register --name "jetson-factory-1" --tags onnx,arm64
  cq hub edge register --name "rpi-fleet" --tags tflite --runtime tflite`,
	RunE: runHubEdgeRegister,
}

var hubEdgeListCmd = &cobra.Command{
	Use:   "list",
	Short: "List registered edge devices",
	RunE:  runHubEdgeList,
}

var hubEdgeControlCmd = &cobra.Command{
	Use:   "control <edge-id> <action>",
	Short: "Send a control message to an edge device",
	Long: `Send a control message to a registered edge device.

Actions:
  collect   - Upload a file from edge to Drive (requires --param local_path=<path>)

Example:
  cq hub edge control edge-123 collect --param local_path=/home/pi/model.onnx`,
	Args: cobra.ExactArgs(2),
	RunE: runHubEdgeControl,
}

var hubWorkersCmd = &cobra.Command{
	Use:   "workers",
	Short: "List registered Hub workers",
	RunE:  runHubWorkers,
}

var hubWorkersPruneCmd = &cobra.Command{
	Use:   "prune",
	Short: "Remove offline zombie workers",
	RunE:  runHubWorkersPrune,
}

var hubSubmitCmd = &cobra.Command{
	Use:   "submit",
	Short: "Upload current dir snapshot and submit a Hub job",
	Long: `Upload the current directory as a Drive CAS snapshot, then submit a job to the Hub.

The --run flag specifies the command to execute on the worker.

Example:
  cq hub submit --run "python3 train.py"`,
	RunE: runHubSubmit,
}

// Flags
var (
	hubWorkerName      string
	hubHeartbeatSec    int
	hubWatchHistory    bool
	hubEdgeName        string
	hubEdgeTags        string
	hubEdgeRuntime     string
	hubSubmitRun       string
	hubWorkersAll      bool
	hubPruneDryRun     bool
	hubEdgeControlParams []string
)

func init() {
	hubRegisterCmd.Flags().StringVar(&hubWorkerName, "name", "", "worker name (default: hostname)")
	hubRunCmd.Flags().StringVar(&hubWorkerName, "name", "", "worker name (default: hostname)")
	hubRunCmd.Flags().IntVar(&hubHeartbeatSec, "interval", 60, "heartbeat interval in seconds")

	hubWatchCmd.Flags().BoolVar(&hubWatchHistory, "history", false, "include historical metrics on connect")

	hubEdgeRegisterCmd.Flags().StringVar(&hubEdgeName, "name", "", "edge device name (required)")
	hubEdgeRegisterCmd.Flags().StringVar(&hubEdgeTags, "tags", "", "comma-separated tags (e.g. onnx,arm64)")
	hubEdgeRegisterCmd.Flags().StringVar(&hubEdgeRuntime, "runtime", "", "inference runtime (onnx, tflite, tensorrt)")
	hubEdgeRegisterCmd.MarkFlagRequired("name")

	hubSubmitCmd.Flags().StringVar(&hubSubmitRun, "run", "", "command to execute on the worker")

	hubWorkersCmd.Flags().BoolVar(&hubWorkersAll, "all", false, "include offline workers")
	hubWorkersPruneCmd.Flags().BoolVar(&hubPruneDryRun, "dry-run", false, "show what would be pruned without deleting")
	hubWorkersCmd.AddCommand(hubWorkersPruneCmd)

	hubEdgeControlCmd.Flags().StringArrayVar(&hubEdgeControlParams, "param", nil, "action parameters as key=value (e.g. --param local_path=/tmp/model.onnx)")

	hubEdgeCmd.AddCommand(hubEdgeRegisterCmd)
	hubEdgeCmd.AddCommand(hubEdgeListCmd)
	hubEdgeCmd.AddCommand(hubEdgeControlCmd)

	hubCmd.AddCommand(hubStatusCmd)
	hubCmd.AddCommand(hubRegisterCmd)
	hubCmd.AddCommand(hubWatchCmd)
	hubCmd.AddCommand(hubRunCmd)
	hubCmd.AddCommand(hubSubmitCmd)
	hubCmd.AddCommand(hubWorkersCmd)
	hubCmd.AddCommand(hubEdgeCmd)
	rootCmd.AddCommand(hubCmd)
}

// newHubClient creates a Hub client from project config.
func newHubClient() (*hub.Client, error) {
	cfgMgr, err := config.New(projectDir, config.CloudDefaults{
		URL:     builtinSupabaseURL,
		AnonKey: builtinSupabaseKey,
	})
	if err != nil {
		return nil, fmt.Errorf("loading config: %w", err)
	}

	hubCfg := cfgMgr.GetConfig().Hub

	// Apply env/builtin fallback: C5_HUB_URL env → builtinHubURL (ldflags).
	// Auto-enable hub so users don't need to edit config.yaml.
	if hubCfg.URL == "" {
		if v := os.Getenv("C5_HUB_URL"); v != "" {
			hubCfg.URL = v
			hubCfg.Enabled = true
		} else if builtinHubURL != "" {
			hubCfg.URL = builtinHubURL
			hubCfg.Enabled = true
		}
	}

	if !hubCfg.Enabled {
		return nil, fmt.Errorf("hub is not enabled in .c4/config.yaml\n\nAdd:\n  hub:\n    enabled: true\n    url: \"http://<hub-ip>:8000\"")
	}

	// JWT fallback: if no API key configured, try cloud session JWT (cq auth login).
	apiKey := hubCfg.APIKey
	if apiKey == "" {
		if jwt := loadCloudSessionJWT(); jwt != "" {
			apiKey = jwt
		}
	}

	client := hub.NewClient(hub.HubConfig{
		Enabled:   hubCfg.Enabled,
		URL:       hubCfg.URL,
		APIPrefix: hubCfg.APIPrefix,
		APIKey:    apiKey,
		APIKeyEnv: hubCfg.APIKeyEnv,
		TeamID:    hubCfg.TeamID,
	})

	if !client.IsAvailable() {
		return nil, fmt.Errorf("hub API key not configured. Run 'cq auth login' or set %s environment variable or hub.api_key in config", hubCfg.APIKeyEnv)
	}

	return client, nil
}

// =========================================================================
// cq hub status
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
// cq hub workers
// =========================================================================

func runHubWorkers(cmd *cobra.Command, args []string) error {
	client, err := newHubClient()
	if err != nil {
		return err
	}

	workers, err := client.ListWorkers(!hubWorkersAll)
	if err != nil {
		return fmt.Errorf("list workers: %w", err)
	}

	if len(workers) == 0 {
		fmt.Println("No workers registered.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "NAME\tSTATUS\tUPTIME\tLAST JOB\tCAPABILITIES\n")
	for _, wk := range workers {
		name := wk.Name
		if name == "" {
			name = wk.Hostname
		}
		if name == "" {
			name = wk.ID
		}
		caps := "-"
		if len(wk.Capabilities) > 0 {
			caps = strings.Join(wk.Capabilities, ",")
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			name, wk.Status, formatUptime(wk.UptimeSec), formatLastJob(wk.LastJobAt), caps)
	}
	w.Flush()
	return nil
}

func runHubWorkersPrune(cmd *cobra.Command, args []string) error {
	client, err := newHubClient()
	if err != nil {
		return err
	}

	purged, err := client.PruneWorkers(hubPruneDryRun)
	if err != nil {
		return fmt.Errorf("prune workers: %w", err)
	}

	if hubPruneDryRun {
		fmt.Printf("Would prune %d offline workers.\n", purged)
	} else {
		fmt.Printf("Pruned %d offline workers.\n", purged)
	}
	return nil
}

// =========================================================================
// cq hub register
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
// cq hub watch
// =========================================================================

func runHubWatch(cmd *cobra.Command, args []string) error {
	client, err := newHubClient()
	if err != nil {
		return err
	}

	jobID := args[0]

	// First, show current job status
	job, err := client.GetJob(jobID)
	if err != nil {
		return fmt.Errorf("get job: %w", err)
	}
	fmt.Printf("Job: %s  Status: %s  Name: %s\n", job.ID, job.Status, job.Name)

	if hub.IsTerminal(job.Status) {
		fmt.Printf("Job already finished (%s)\n", job.Status)
		// Show final metrics
		metrics, mErr := client.GetMetrics(jobID, 5)
		if mErr == nil && len(metrics.Metrics) > 0 {
			fmt.Printf("Last %d metrics:\n", len(metrics.Metrics))
			for _, m := range metrics.Metrics {
				j, _ := json.Marshal(m.Metrics)
				fmt.Printf("  step %d: %s\n", m.Step, j)
			}
		}
		return nil
	}

	fmt.Println("Streaming metrics (Ctrl+C to stop)...")
	fmt.Println()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle Ctrl+C
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Println("\nStopping...")
		cancel()
	}()

	err = client.StreamMetrics(ctx, jobID, hubWatchHistory, func(msg hub.MetricMessage) {
		switch msg.Type {
		case "metric":
			j, _ := json.Marshal(msg.Metrics)
			fmt.Printf("[step %d] %s\n", msg.Step, j)
		case "status":
			fmt.Printf("[status] %s\n", msg.Status)
		case "history":
			j, _ := json.Marshal(msg.Metrics)
			fmt.Printf("[history step %d] %s\n", msg.Step, j)
		case "error":
			fmt.Fprintf(os.Stderr, "[error] %s\n", msg.Error)
		}
	})

	if err != nil && ctx.Err() == nil {
		return fmt.Errorf("stream: %w", err)
	}

	fmt.Println("Stream ended.")
	return nil
}

// =========================================================================
// cq hub run (daemon mode)
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

// =========================================================================
// cq hub edge register
// =========================================================================

func runHubEdgeRegister(cmd *cobra.Command, args []string) error {
	client, err := newHubClient()
	if err != nil {
		return err
	}

	if !client.HealthCheck() {
		return fmt.Errorf("Hub is unreachable")
	}

	hostname, _ := os.Hostname()
	caps := map[string]any{
		"hostname": hostname,
	}
	if hubEdgeRuntime != "" {
		caps["runtime"] = hubEdgeRuntime
	}

	// Detect architecture
	caps["arch"] = detectArch()

	var tags []string
	if hubEdgeTags != "" {
		tags = strings.Split(hubEdgeTags, ",")
	}

	fmt.Printf("Registering edge device '%s' with Hub...\n", hubEdgeName)
	if len(tags) > 0 {
		fmt.Printf("  Tags: %s\n", strings.Join(tags, ", "))
	}
	if hubEdgeRuntime != "" {
		fmt.Printf("  Runtime: %s\n", hubEdgeRuntime)
	}

	edgeID, err := client.RegisterEdge(hubEdgeName, tags, caps)
	if err != nil {
		return fmt.Errorf("registration failed: %w", err)
	}

	fmt.Printf("\nRegistered as edge: %s\n", edgeID)
	return nil
}

// =========================================================================
// cq hub edge list
// =========================================================================

func runHubEdgeList(cmd *cobra.Command, args []string) error {
	client, err := newHubClient()
	if err != nil {
		return err
	}

	edges, err := client.ListEdges()
	if err != nil {
		return err
	}

	if len(edges) == 0 {
		fmt.Println("No edge devices registered.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "ID\tNAME\tSTATUS\tARCH\tRUNTIME\tTAGS\n")
	for _, e := range edges {
		tags := "-"
		if len(e.Tags) > 0 {
			tags = strings.Join(e.Tags, ",")
		}
		arch := e.Arch
		if arch == "" {
			arch = "-"
		}
		runtime := e.Runtime
		if runtime == "" {
			runtime = "-"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			e.ID, e.Name, e.Status, arch, runtime, tags)
	}
	w.Flush()
	return nil
}

// =========================================================================
// cq hub edge control
// =========================================================================

func runHubEdgeControl(cmd *cobra.Command, args []string) error {
	edgeID := args[0]
	action := args[1]

	// Parse --param key=value flags into params map.
	params := make(map[string]any, len(hubEdgeControlParams))
	for _, p := range hubEdgeControlParams {
		k, v, ok := strings.Cut(p, "=")
		if !ok {
			return fmt.Errorf("invalid --param %q: expected key=value format", p) //nolint:goerr113
		}
		params[k] = v
	}

	client, err := newHubClient()
	if err != nil {
		return err
	}

	req := &hub.EdgeControlRequest{Action: action}
	if len(params) > 0 {
		req.Params = params
	}

	resp, err := client.EdgeControl(edgeID, req)
	if err != nil {
		return err
	}

	fmt.Printf("Control message sent: %s\n", resp.MessageID)
	fmt.Printf("Status: %s\n", resp.Status)
	return nil
}

// =========================================================================
// cq hub submit
// =========================================================================

// experimentConfig holds the optional `experiment:` section from cq.yaml.
type experimentConfig struct {
	Name     string         `yaml:"name"`
	Tags     []string       `yaml:"tags"`
	Config   map[string]any `yaml:"config"`
	Datasets struct {
		WorkerPath string `yaml:"worker_path"`
	} `yaml:"datasets"`
}

// cqYamlFile represents the structure of cq.yaml.
type cqYamlFile struct {
	Run        string           `yaml:"run"`
	Experiment experimentConfig `yaml:"experiment"`
}

func runHubSubmit(cmd *cobra.Command, args []string) error {
	// Parse cq.yaml once; used for both `run` fallback and experiment metadata.
	var cqYaml cqYamlFile
	if data, err := os.ReadFile("cq.yaml"); err == nil {
		_ = yaml.Unmarshal(data, &cqYaml)
	}

	command := hubSubmitRun
	if command == "" {
		command = cqYaml.Run
	}
	if command == "" {
		return fmt.Errorf("--run flag is required or set `run` in cq.yaml")
	}

	client, err := newHubClient()
	if err != nil {
		return err
	}

	ctx := context.Background()
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getwd: %w", err)
	}

	// Upload current dir to Drive CAS (best-effort: skip if drive not configured).
	var snapshotHash string
	dc, dcErr := newDatasetClient()
	if dcErr == nil {
		projectID := getActiveProjectID(projectDir)
		snapshotName := "hub-submit-" + projectID
		result, upErr := dc.Upload(ctx, cwd, snapshotName, "")
		if upErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: Drive upload failed (submitting without snapshot): %v\n", upErr)
		} else {
			snapshotHash = result.VersionHash
			fmt.Printf("Snapshot uploaded: %s (changed=%v, files=%d)\n",
				snapshotHash, result.Changed, result.FilesUploaded+result.FilesSkipped)
		}
	} else if verbose {
		fmt.Fprintf(os.Stderr, "Drive not configured, skipping snapshot upload: %v\n", dcErr)
	}

	// git rev-parse HEAD (optional).
	var gitHash string
	if out, err := exec.Command("git", "rev-parse", "HEAD").Output(); err == nil {
		gitHash = strings.TrimSpace(string(out))
	}

	req := &hub.JobSubmitRequest{
		Name:                "hub-submit",
		Workdir:             cwd,
		Command:             command,
		SnapshotVersionHash: snapshotHash,
		GitHash:             gitHash,
		ProjectID:           getActiveProjectID(projectDir),
	}

	// Apply experiment metadata from cq.yaml experiment: section.
	exp := cqYaml.Experiment
	if exp.Name != "" {
		req.ExpID = exp.Name
		req.Tags = exp.Tags
		if len(exp.Config) > 0 {
			if memo, err := json.Marshal(exp.Config); err == nil {
				req.Memo = string(memo)
			}
		}
		if exp.Datasets.WorkerPath != "" {
			if req.Env == nil {
				req.Env = make(map[string]string)
			}
			req.Env["C5_DATASET_PATH"] = exp.Datasets.WorkerPath
		}
	}

	resp, err := client.SubmitJob(req)
	if err != nil {
		return fmt.Errorf("submit job: %w", err)
	}

	fmt.Printf("Job submitted: %s (status=%s, queue_position=%d)\n",
		resp.JobID, resp.Status, resp.QueuePosition)
	if gitHash != "" {
		fmt.Printf("  git:      %s\n", gitHash)
	}
	if snapshotHash != "" {
		fmt.Printf("  snapshot: %s\n", snapshotHash)
	}
	if exp.Name != "" {
		fmt.Printf("  exp:      %s\n", exp.Name)
	}
	return nil
}

// detectArch returns the machine architecture (arm64, amd64, etc.).
func detectArch() string {
	out, err := exec.Command("uname", "-m").Output()
	if err != nil {
		return "unknown"
	}
	arch := strings.TrimSpace(string(out))
	switch arch {
	case "x86_64":
		return "amd64"
	case "aarch64":
		return "arm64"
	default:
		return arch
	}
}
