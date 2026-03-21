package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
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

var hubJobCmd = &cobra.Command{
	Use:   "job",
	Short: "Manage Hub jobs",
}

var hubJobLogCmd = &cobra.Command{
	Use:   "log <job-id>",
	Short: "Show or follow logs for a Hub job",
	Long: `Display stdout/stderr logs captured from a Hub worker job.

With --follow, polls for new lines every 2 seconds until the job terminates.

Example:
  cq hub job log job-abc123
  cq hub job log job-abc123 --follow
  cq hub job log job-abc123 --offset 50`,
	Args: cobra.ExactArgs(1),
	RunE: runHubJobLog,
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
	hubWorkerName        string
	hubHeartbeatSec      int
	hubWatchHistory      bool
	hubEdgeName          string
	hubEdgeTags          string
	hubEdgeRuntime       string
	hubSubmitRun         string
	hubSubmitExperiment  string
	hubWorkersAll        bool
	hubPruneDryRun       bool
	hubEdgeControlParams []string
	hubJobLogFollow      bool
	hubJobLogOffset      int
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
	hubSubmitCmd.Flags().StringVar(&hubSubmitExperiment, "experiment", "", "experiment name to register as a Hub experiment run (requires Hub)")

	hubWorkersCmd.Flags().BoolVar(&hubWorkersAll, "all", false, "include offline workers")
	hubWorkersPruneCmd.Flags().BoolVar(&hubPruneDryRun, "dry-run", false, "show what would be pruned without deleting")
	hubWorkersCmd.AddCommand(hubWorkersPruneCmd)

	hubEdgeControlCmd.Flags().StringArrayVar(&hubEdgeControlParams, "param", nil, "action parameters as key=value (e.g. --param local_path=/tmp/model.onnx)")

	hubEdgeCmd.AddCommand(hubEdgeRegisterCmd)
	hubEdgeCmd.AddCommand(hubEdgeListCmd)
	hubEdgeCmd.AddCommand(hubEdgeControlCmd)

	hubJobLogCmd.Flags().BoolVarP(&hubJobLogFollow, "follow", "f", false, "poll for new log lines until job completes")
	hubJobLogCmd.Flags().IntVar(&hubJobLogOffset, "offset", 0, "start reading from this line offset")
	hubJobCmd.AddCommand(hubJobLogCmd)

	hubCmd.AddCommand(hubStatusCmd)
	hubCmd.AddCommand(hubRegisterCmd)
	hubCmd.AddCommand(hubWatchCmd)
	hubCmd.AddCommand(hubRunCmd)
	hubCmd.AddCommand(hubSubmitCmd)
	hubCmd.AddCommand(hubWorkersCmd)
	hubCmd.AddCommand(hubEdgeCmd)
	hubCmd.AddCommand(hubJobCmd)
	hubCmd.AddCommand(hubTransferCmd)
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

	// Auto-enable hub if Supabase cloud is configured (no separate hub.url needed).
	cloudCfg := cfgMgr.GetConfig().Cloud
	if !hubCfg.Enabled && cloudCfg.URL != "" {
		hubCfg.Enabled = true
	}
	if !hubCfg.Enabled {
		return nil, fmt.Errorf("hub is not enabled — run: cq auth login")
	}

	// Resolve Supabase URL/key from cloud config for PostgREST access.
	supabaseURL := cloudCfg.URL

	// supabaseKey = anon_key (for apikey header). Service role overrides if available.
	supabaseKey := os.Getenv("SUPABASE_SERVICE_ROLE_KEY")
	if supabaseKey == "" {
		if cfgData, err := os.ReadFile(filepath.Join(projectDir, ".c4", "config.yaml")); err == nil {
			var cfgMap map[string]any
			if yaml.Unmarshal(cfgData, &cfgMap) == nil {
				if cloud, ok := cfgMap["cloud"].(map[string]any); ok {
					if sk, ok := cloud["service_key"].(string); ok {
						supabaseKey = sk
					}
				}
			}
		}
	}
	if supabaseKey == "" {
		supabaseKey = cloudCfg.AnonKey
	}

	// apiKey = legacy Hub API key (not used for Supabase auth).
	apiKey := hubCfg.APIKey

	client := hub.NewClient(hub.HubConfig{
		Enabled:      hubCfg.Enabled,
		URL:          hubCfg.URL,
		APIPrefix:    hubCfg.APIPrefix,
		APIKey:       apiKey,
		APIKeyEnv:    hubCfg.APIKeyEnv,
		TeamID:       hubCfg.TeamID,
		SupabaseURL:  supabaseURL,
		SupabaseKey:  supabaseKey,
	})

	// Set JWT token function for Supabase auth (Authorization: Bearer JWT).
	// This keeps apikey=anon_key while Authorization=Bearer JWT.
	client.SetTokenFunc(loadCloudSessionJWT)

	if !client.IsAvailable() {
		return nil, fmt.Errorf("not configured. Run 'cq auth login'")
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

	_ = context.Background() // reserved for future Drive upload
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getwd: %w", err)
	}

	// Drive snapshot upload is optional — skip for now (Supabase-native mode).
	// TODO: re-enable with proper context cancellation support in drive.Upload.
	var snapshotHash string

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

	// --experiment flag: register an experiment run on the Hub before submitting.
	if hubSubmitExperiment != "" {
		runID, err := client.CreateExperimentRun(hubSubmitExperiment, "")
		if err != nil {
			return fmt.Errorf("--experiment: Hub is required (start Hub or omit --experiment): %w", err)
		}
		req.ExpRunID = runID
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
	if req.ExpRunID != "" {
		fmt.Printf("  run_id:   %s\n", req.ExpRunID)
	}

	return nil
}

// =========================================================================
// cq hub job log
// =========================================================================

func runHubJobLog(cmd *cobra.Command, args []string) error {
	client, err := newHubClient()
	if err != nil {
		return err
	}

	jobID := args[0]
	offset := hubJobLogOffset
	const batchSize = 200

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	printBatch := func(resp *hub.JobLogsResponse) {
		for _, line := range resp.Lines {
			fmt.Println(line)
		}
	}

	for {
		resp, err := client.GetJobLogsCtx(ctx, jobID, offset, batchSize)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("get logs: %w", err)
		}
		printBatch(resp)
		offset += len(resp.Lines)

		if !hubJobLogFollow {
			return nil
		}

		// In follow mode: if no more lines and job is terminal → stop.
		if !resp.HasMore {
			job, jerr := client.GetJob(jobID)
			if jerr == nil && hub.IsTerminal(job.Status) {
				return nil
			}
			// Wait before polling again.
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(2 * time.Second):
			}
		}
	}
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
