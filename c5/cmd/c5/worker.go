package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/piqsol/c4/c5/internal/model"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

const (
	LeaseRenewInterval          = 2 * time.Minute
	WorkerHeartbeatInterval     = 30 * time.Second
	MetricsFlushDebounce        = 5 * time.Second
	MetricsBufferFlushThreshold = 10
)

// capabilitiesFile is a YAML file declaring worker capabilities.
// Format:
//
//	capabilities:
//	  - name: train_model
//	    description: "Run PyTorch training"
//	    command: "python scripts/train.py"
//	    input_schema:
//	      type: object
//	      properties:
//	        config_path: {type: string}
//	        epochs: {type: integer}
//	    tags: [gpu, pytorch]
type capabilitiesYAML struct {
	Capabilities []struct {
		Name        string         `yaml:"name"`
		Description string         `yaml:"description"`
		Command     string         `yaml:"command"`
		InputSchema map[string]any `yaml:"input_schema"`
		Tags        []string       `yaml:"tags"`
		Version     string         `yaml:"version"`
	} `yaml:"capabilities"`
}

// loadCapabilities reads a YAML capabilities file and returns model.Capability slice.
func loadCapabilities(path string) ([]model.Capability, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read capabilities file: %w", err)
	}
	var cf capabilitiesYAML
	if err := yaml.Unmarshal(data, &cf); err != nil {
		return nil, fmt.Errorf("parse capabilities YAML: %w", err)
	}
	caps := make([]model.Capability, 0, len(cf.Capabilities))
	for _, c := range cf.Capabilities {
		caps = append(caps, model.Capability{
			Name:        c.Name,
			Description: c.Description,
			Command:     c.Command,
			InputSchema: c.InputSchema,
			Tags:        c.Tags,
			Version:     c.Version,
		})
	}
	return caps, nil
}

func workerCmd() *cobra.Command {
	var (
		serverURL        string
		hostname         string
		gpuCount         int
		gpuModel         string
		totalVRAM        float64
		pollSec          int
		apiKey           string
		capabilitiesFile string
	)

	cmd := &cobra.Command{
		Use:   "worker",
		Short: "Start a C5 worker agent",
		RunE: func(cmd *cobra.Command, args []string) error {
			if hostname == "" {
				h, _ := os.Hostname()
				hostname = h
			}

			var caps []model.Capability
			if capabilitiesFile != "" {
				var err error
				caps, err = loadCapabilities(capabilitiesFile)
				if err != nil {
					return fmt.Errorf("load capabilities: %w", err)
				}
				log.Printf("c5-worker: loaded %d capabilities from %s", len(caps), capabilitiesFile)
			}

			return runWorker(workerConfig{
				serverURL:    serverURL,
				hostname:     hostname,
				gpuCount:     gpuCount,
				gpuModel:     gpuModel,
				totalVRAM:    totalVRAM,
				pollSec:      pollSec,
				apiKey:       apiKey,
				capabilities: caps,
			})
		},
	}

	defaultServer := builtinServerURL
	if defaultServer == "" {
		defaultServer = os.Getenv("C5_HUB_URL")
	}
	if defaultServer == "" {
		defaultServer = "http://localhost:8585"
	}
	cmd.Flags().StringVar(&serverURL, "server", defaultServer, "C5 server URL")
	cmd.Flags().StringVar(&hostname, "hostname", "", "Worker hostname (default: OS hostname)")
	cmd.Flags().IntVar(&gpuCount, "gpu-count", 0, "Number of GPUs available")
	cmd.Flags().StringVar(&gpuModel, "gpu-model", "", "GPU model name")
	cmd.Flags().Float64Var(&totalVRAM, "total-vram", 0, "Total VRAM in GB")
	cmd.Flags().IntVar(&pollSec, "poll-interval", 5, "Poll interval in seconds")
	cmd.Flags().StringVar(&apiKey, "api-key", os.Getenv("C5_API_KEY"), "API key for authentication")
	cmd.Flags().StringVar(&capabilitiesFile, "capabilities", "", "Path to capabilities YAML file")

	return cmd
}

type workerConfig struct {
	serverURL    string
	hostname     string
	gpuCount     int
	gpuModel     string
	totalVRAM    float64
	pollSec      int
	apiKey       string
	capabilities []model.Capability
	drive        driveClient // optional; nil skips Drive pipeline
}

// getWorkerVersion returns the cq version string for Hub registration.
// It reads CQ_VERSION env var; falls back to "unknown" if unset.
func getWorkerVersion() string {
	if v := os.Getenv("CQ_VERSION"); v != "" {
		return v
	}
	return "unknown"
}

func runWorker(cfg workerConfig) error {
	client := &workerClient{
		baseURL:      strings.TrimRight(cfg.serverURL, "/"),
		apiKey:       cfg.apiKey,
		http:         &http.Client{Timeout: WorkerHeartbeatInterval},
		artifactHTTP: &http.Client{Timeout: 30 * time.Minute}, // large artifacts need generous but finite timeout
	}

	// Register
	workerID, err := client.register(&model.WorkerRegisterRequest{
		Hostname:      cfg.hostname,
		GPUCount:      cfg.gpuCount,
		GPUModel:      cfg.gpuModel,
		TotalVRAM:     cfg.totalVRAM,
		FreeVRAM:      cfg.totalVRAM,
		CapabilitySet: cfg.capabilities,
		Version:       getWorkerVersion(),
	})
	if err != nil {
		return fmt.Errorf("register worker: %w", err)
	}
	log.Printf("c5-worker: registered as %s", workerID)

	// Graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	ticker := time.NewTicker(time.Duration(cfg.pollSec) * time.Second)
	defer ticker.Stop()

	heartbeatTicker := time.NewTicker(WorkerHeartbeatInterval)
	defer heartbeatTicker.Stop()

	var running atomic.Bool

	for {
		select {
		case <-sigCh:
			log.Println("c5-worker: shutting down...")
			return nil

		case <-heartbeatTicker.C:
			client.heartbeat(workerID, cfg.totalVRAM)

		case <-ticker.C:
			if running.Load() {
				continue
			}

			lease, job, inputArtifacts, ctrl, err := client.acquireLease(workerID)
			if err != nil {
				log.Printf("c5-worker: acquire error: %v", err)
				continue
			}
			if ctrl != nil {
				switch ctrl.Action {
				case "upgrade":
					log.Println("c5-worker: control: upgrade received, running cq upgrade...")
					if err := exec.Command("cq", "upgrade").Run(); err != nil {
						log.Printf("c5-worker: cq upgrade failed: %v — retrying next poll", err)
					} else {
						os.Exit(0)
					}
				case "shutdown":
					log.Println("c5-worker: control: shutdown received, stopping after current job")
					return nil
				}
			}
			if lease == nil {
				continue // no jobs
			}

			running.Store(true)
			log.Printf("c5-worker: acquired job %s (%s)", job.ID, job.Name)

			// Execute job in a goroutine
			go func(j *model.Job, leaseID string, artifacts []model.InputPresignedArtifact) {
				defer running.Store(false)

				// Download input artifacts before job execution
				if len(artifacts) > 0 {
					log.Printf("c5-worker: downloading %d input artifacts", len(artifacts))
					if err := downloadInputArtifacts(client.artifactHTTP, artifacts); err != nil {
						log.Printf("c5-worker: input artifact download failed: %v", err)
						exitCode := 1
						if cErr := client.completeJob(j.ID, "FAILED", exitCode, nil); cErr != nil {
							log.Printf("c5-worker: complete error: %v", cErr)
						}
						return
					}
				}

				// Drive pipeline: snapshot pull → cq.yaml → artifacts → run → push
				var exitCode int
				var resultFile string
				if cfg.drive != nil && j.SnapshotVersionHash != "" {
					exitCode, resultFile = runWithDrivePipeline(cfg.drive, client, j, leaseID, workerID, cfg.gpuCount, j.SnapshotVersionHash)
				} else {
					exitCode, resultFile = executeJob(client, j, leaseID, workerID, cfg.gpuCount)
				}

				// Upload output artifacts on success
				if exitCode == 0 && len(j.OutputArtifacts) > 0 {
					log.Printf("c5-worker: uploading %d output artifacts", len(j.OutputArtifacts))
					if err := uploadOutputArtifacts(client, j.ID, j.OutputArtifacts); err != nil {
						log.Printf("c5-worker: job %s command succeeded (exit 0) but artifact upload failed: %v", j.ID, err)
						exitCode = 1
					}
				}

				status := "SUCCEEDED"
				if exitCode != 0 {
					status = "FAILED"
				}

				// Read structured result from C5_RESULT_FILE if capability handler wrote one.
				var result map[string]any
				if resultFile != "" && exitCode == 0 {
					if data, err := os.ReadFile(resultFile); err == nil {
						if jsonErr := json.Unmarshal(data, &result); jsonErr != nil {
							log.Printf("c5-worker: job %s: result file invalid JSON: %v", j.ID, jsonErr)
							result = nil
						}
					}
					os.Remove(resultFile)
				}

				if err := client.completeJob(j.ID, status, exitCode, result); err != nil {
					log.Printf("c5-worker: complete error: %v", err)
				} else {
					log.Printf("c5-worker: job %s %s (exit %d)", j.ID, status, exitCode)
				}
			}(job, lease.ID, inputArtifacts)
		}
	}
}

func executeJob(client *workerClient, job *model.Job, leaseID, workerID string, gpuCount int) (int, string) {
	// Set up context with optional timeout
	ctx := context.Background()
	var cancel context.CancelFunc
	if job.TimeoutSec > 0 {
		ctx, cancel = context.WithTimeout(ctx, time.Duration(job.TimeoutSec)*time.Second)
	} else {
		ctx, cancel = context.WithCancel(ctx)
	}
	defer cancel()

	// Resolve command: capability sentinel → look up in YAML if needed.
	// The hub sets Command = "__capability__:<name>" when no explicit command was
	// defined in the capability registration. In that case, let the capability
	// handler discover what to run via C5_CAPABILITY env var (e.g. a script in
	// ./capabilities/<name>). We fall back to a no-op echo so the job completes.
	command := job.Command
	if strings.HasPrefix(command, "__capability__:") {
		rawName := strings.TrimPrefix(command, "__capability__:")
		// Sanitize: reject names with path separators to prevent traversal.
		capName := filepath.Base(rawName)
		if capName == "." || capName == ".." || strings.ContainsAny(rawName, "/\\") {
			log.Printf("c5-worker: unsafe capability name %q — running no-op", rawName)
			command = "true"
		} else {
			// Try conventional script location: capabilities/<name> or capabilities/<name>.sh
			found := false
			for _, candidate := range []string{
				filepath.Join("capabilities", capName),
				filepath.Join("capabilities", capName+".sh"),
			} {
				if _, err := os.Stat(candidate); err == nil {
					command = candidate
					found = true
					break
				}
			}
			if !found {
				log.Printf("c5-worker: no handler found for capability %q, running no-op", capName)
				command = "true"
			}
		}
	}

	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Dir = job.Workdir

	// Env injection: inherit current env + job env vars.
	// C4_PROJECT_ID is appended last to take priority over job.Env overrides.
	env := os.Environ()
	for k, v := range job.Env {
		env = append(env, k+"="+v)
	}
	if job.ProjectID != "" {
		env = append(env, "C4_PROJECT_ID="+job.ProjectID)
	}

	// Capability params injection: C5_PARAMS (JSON) + C5_CAPABILITY + C5_RESULT_FILE
	var resultFile string
	if job.Capability != "" {
		env = append(env, "C5_CAPABILITY="+job.Capability)
		if job.Params != nil {
			paramsJSON, _ := json.Marshal(job.Params)
			env = append(env, "C5_PARAMS="+string(paramsJSON))
		}
		// Provide a temp file path for the handler to write structured JSON result.
		resultFile = filepath.Join(os.TempDir(), "c5-result-"+job.ID+".json")
		env = append(env, "C5_RESULT_FILE="+resultFile)
	}

	// GPU assignment: set CUDA_VISIBLE_DEVICES if job requires GPU
	if job.RequiresGPU && gpuCount > 0 {
		// Build device list: "0", "0,1", "0,1,2", etc.
		devices := make([]string, gpuCount)
		for i := range devices {
			devices[i] = fmt.Sprintf("%d", i)
		}
		env = append(env, "CUDA_VISIBLE_DEVICES="+strings.Join(devices, ","))
	}

	// Artifact directory hints for job scripts
	env = append(env, "C5_INPUT_DIR=.")
	env = append(env, "C5_OUTPUT_DIR=.")

	cmd.Env = env

	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	if err := cmd.Start(); err != nil {
		log.Printf("c5-worker: start error: %v", err)
		return 1, resultFile
	}

	// Stream logs with metrics auto-parsing on stdout
	mc := newMetricsCollector(client, job.ID)
	var logWg sync.WaitGroup
	logWg.Add(2)
	go func() { defer logWg.Done(); streamLogs(client, job.ID, stdout, "stdout", mc) }()
	go func() { defer logWg.Done(); streamLogs(client, job.ID, stderr, "stderr", nil) }()

	// Lease renew + cancel detection goroutine
	done := make(chan struct{})
	go func() {
		var renewFailures int
		renewTicker := time.NewTicker(LeaseRenewInterval)
		cancelTicker := time.NewTicker(WorkerHeartbeatInterval)
		defer renewTicker.Stop()
		defer cancelTicker.Stop()
		for {
			select {
			case <-done:
				return
			case <-renewTicker.C:
				if err := client.renewLease(leaseID, workerID); err != nil {
					renewFailures++
					if renewFailures >= 3 {
						log.Printf("c5-worker: WARNING: lease %s renewal failed %d times consecutively: %v", leaseID, renewFailures, err)
					} else {
						log.Printf("c5-worker: lease %s renewal error: %v", leaseID, err)
					}
				} else {
					renewFailures = 0
				}
			case <-cancelTicker.C:
				// Check if job was cancelled on server
				status, err := client.getJobStatus(job.ID)
				if err == nil && status == string(model.StatusCancelled) {
					log.Printf("c5-worker: job %s cancelled, killing process", job.ID)
					cancel()
					return
				}
			}
		}
	}()

	err := cmd.Wait()
	close(done)
	logWg.Wait() // ensure all log lines are flushed before returning

	if err != nil {
		// Check if killed by timeout
		if ctx.Err() == context.DeadlineExceeded {
			log.Printf("c5-worker: job %s timed out after %ds", job.ID, job.TimeoutSec)
			return 124, resultFile
		}
		// Check if killed by cancel
		if ctx.Err() == context.Canceled {
			log.Printf("c5-worker: job %s cancelled", job.ID)
			return 130, resultFile
		}
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode(), resultFile
		}
		return 1, resultFile
	}
	return 0, resultFile
}

// downloadInputArtifacts fetches each presigned artifact URL to its local path.
// Required artifacts (art.Required == true) cause a hard failure; optional
// artifacts log a warning and continue.
func downloadInputArtifacts(httpClient *http.Client, artifacts []model.InputPresignedArtifact) error {
	for _, art := range artifacts {
		localPath := art.LocalPath
		if localPath == "" {
			localPath = filepath.Base(art.Path)
		}
		// Path traversal defense: reject absolute paths and .. components
		cleaned := filepath.Clean(localPath)
		if filepath.IsAbs(cleaned) || strings.HasPrefix(cleaned, "..") {
			err := fmt.Errorf("unsafe local path for artifact %s: %s", art.Path, localPath)
			if !art.Required {
				log.Printf("c5-worker: WARNING: optional artifact skipped: %v", err)
				continue
			}
			return err
		}
		localPath = cleaned
		if err := os.MkdirAll(filepath.Dir(localPath), 0755); err != nil {
			if !art.Required {
				log.Printf("c5-worker: WARNING: optional artifact %s skipped (mkdir): %v", art.Path, err)
				continue
			}
			return fmt.Errorf("create dir for artifact %s: %w", art.Path, err)
		}

		if err := downloadSingleArtifact(httpClient, art.URL, art.Path, localPath); err != nil {
			if !art.Required {
				log.Printf("c5-worker: WARNING: optional artifact %s skipped: %v", art.Path, err)
				continue
			}
			return err
		}
		log.Printf("c5-worker: downloaded %s → %s", art.Path, localPath)
	}
	return nil
}

// downloadSingleArtifact downloads a single URL to localPath.
func downloadSingleArtifact(httpClient *http.Client, url, artifactPath, localPath string) error {
	resp, err := httpClient.Get(url)
	if err != nil {
		return fmt.Errorf("download artifact %s: %w", artifactPath, err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return fmt.Errorf("download artifact %s: HTTP %d", artifactPath, resp.StatusCode)
	}
	f, err := os.Create(localPath)
	if err != nil {
		resp.Body.Close()
		return fmt.Errorf("create file %s: %w", localPath, err)
	}
	_, copyErr := io.Copy(f, resp.Body)
	resp.Body.Close()
	f.Close()
	if copyErr != nil {
		os.Remove(localPath)
		return fmt.Errorf("write artifact %s: %w", artifactPath, copyErr)
	}
	return nil
}

// uploadOutputArtifacts uploads each output artifact to signed URLs and confirms them.
func uploadOutputArtifacts(client *workerClient, jobID string, artifacts []model.ArtifactRef) error {
	for _, art := range artifacts {
		localPath := art.LocalPath
		if localPath == "" {
			localPath = filepath.Base(art.Path)
		}
		// Path traversal defense: reject absolute paths and .. components
		cleaned := filepath.Clean(localPath)
		if filepath.IsAbs(cleaned) || strings.HasPrefix(cleaned, "..") {
			return fmt.Errorf("unsafe local path for artifact %s: %s", art.Path, localPath)
		}
		localPath = cleaned

		// Check if file exists
		info, err := os.Stat(localPath)
		if err != nil {
			if !art.Required {
				log.Printf("c5-worker: optional output artifact %s not found, skipping", art.Path)
				continue
			}
			return fmt.Errorf("required output artifact %s not found at %s: %w", art.Path, localPath, err)
		}

		// Get signed upload URL
		uploadURL, err := client.getPresignedURL(art.Path)
		if err != nil {
			return fmt.Errorf("get upload URL for %s: %w", art.Path, err)
		}

		// Upload file
		f, err := os.Open(localPath)
		if err != nil {
			return fmt.Errorf("open %s: %w", localPath, err)
		}

		req, err := http.NewRequest("PUT", uploadURL, f)
		if err != nil {
			f.Close()
			return fmt.Errorf("create upload request for %s: %w", art.Path, err)
		}
		req.ContentLength = info.Size()
		req.Header.Set("Content-Type", "application/octet-stream")

		resp, err := client.artifactHTTP.Do(req)
		f.Close()
		if err != nil {
			return fmt.Errorf("upload %s: %w", art.Path, err)
		}
		resp.Body.Close()
		if resp.StatusCode >= 400 {
			return fmt.Errorf("upload %s: HTTP %d", art.Path, resp.StatusCode)
		}

		// Compute content hash (SHA-256)
		hash := computeFileHash(localPath)

		// Confirm artifact
		if err := client.confirmArtifact(jobID, art.Path, hash, info.Size()); err != nil {
			return fmt.Errorf("confirm artifact %s: %w", art.Path, err)
		}

		log.Printf("c5-worker: uploaded output artifact %s (%d bytes)", art.Path, info.Size())
	}
	return nil
}

func computeFileHash(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	h := sha256.New()
	io.Copy(h, f)
	return fmt.Sprintf("%x", h.Sum(nil))
}

// metricsCollector buffers parsed metrics and batch-sends them.
type metricsCollector struct {
	client  *workerClient
	jobID   string
	mu      sync.Mutex
	step    int
	pending map[string]float64
	timer   *time.Timer
}

func newMetricsCollector(client *workerClient, jobID string) *metricsCollector {
	return &metricsCollector{
		client:  client,
		jobID:   jobID,
		pending: make(map[string]float64),
	}
}

// kvPattern matches key=value pairs like "loss=0.5" or "accuracy=0.95"
var kvPattern = regexp.MustCompile(`(\w+)=([\d.eE+-]+)`)

func (mc *metricsCollector) parseLine(line string) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return
	}

	metrics := make(map[string]float64)

	// Try JSON first: {"loss": 0.5, "step": 100}
	if strings.HasPrefix(trimmed, "{") {
		var parsed map[string]any
		if json.Unmarshal([]byte(trimmed), &parsed) == nil && len(parsed) > 0 {
			for k, v := range parsed {
				switch n := v.(type) {
				case float64:
					metrics[k] = n
				case int:
					metrics[k] = float64(n)
				}
			}
		}
	}

	// Fallback: key=value pattern
	if len(metrics) == 0 {
		matches := kvPattern.FindAllStringSubmatch(trimmed, -1)
		for _, m := range matches {
			if val, err := strconv.ParseFloat(m[2], 64); err == nil {
				metrics[m[1]] = val
			}
		}
	}

	if len(metrics) == 0 {
		return
	}

	mc.mu.Lock()
	defer mc.mu.Unlock()

	// Extract step if present
	if s, ok := metrics["step"]; ok {
		mc.step = int(s)
		delete(metrics, "step")
	}

	for k, v := range metrics {
		mc.pending[k] = v
	}

	// Reset flush timer (debounce 5 seconds)
	if mc.timer != nil {
		mc.timer.Stop()
	}
	mc.timer = time.AfterFunc(MetricsFlushDebounce, mc.flush)

	// Flush immediately if buffer has many keys
	if len(mc.pending) >= MetricsBufferFlushThreshold {
		mc.timer.Stop()
		go mc.flush()
	}
}

func (mc *metricsCollector) flush() {
	mc.mu.Lock()
	if len(mc.pending) == 0 {
		mc.mu.Unlock()
		return
	}
	step := mc.step
	toSend := make(map[string]float64, len(mc.pending))
	for k, v := range mc.pending {
		toSend[k] = v
	}
	mc.pending = make(map[string]float64)
	mc.mu.Unlock()

	mc.client.logMetrics(mc.jobID, step, toSend)
}

// close stops the debounce timer and performs a final flush.
func (mc *metricsCollector) close() {
	mc.mu.Lock()
	if mc.timer != nil {
		mc.timer.Stop()
		mc.timer = nil
	}
	mc.mu.Unlock()
	mc.flush()
}

func streamLogs(client *workerClient, jobID string, r io.Reader, stream string, mc *metricsCollector) {
	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			lines := strings.Split(strings.TrimRight(string(buf[:n]), "\n"), "\n")
			for _, line := range lines {
				if line != "" {
					client.appendLog(jobID, line, stream)
					// Parse metrics from stdout only
					if stream == "stdout" && mc != nil {
						mc.parseLine(line)
					}
				}
			}
		}
		if err != nil {
			// Stop timer and flush remaining metrics on stream close
			if mc != nil && stream == "stdout" {
				mc.close()
			}
			return
		}
	}
}

// =========================================================================
// Drive interface — snapshot + artifact pull/push
// =========================================================================

// driveClient abstracts C0 Drive operations needed by the worker pipeline.
// The real implementation calls cq drive CLI or Drive HTTP API.
// In tests, a stub is injected.
type driveClient interface {
	// Pull downloads a snapshot or artifact by name to destDir.
	// version="" means "latest".
	Pull(name, destDir, version string) error
	// Upload uploads localPath to Drive under name.
	Upload(localPath, name string) error
}

// cqYAML represents the cq.yaml file at the snapshot root.
type cqYAML struct {
	Run string `yaml:"run"`
	// UV controls whether to prepend "uv run" to the run command.
	// Defaults to true when omitted — uv handles venv creation and dependency
	// installation automatically via pyproject.toml / uv.lock.
	// Set to false to run the command as-is (e.g. bash scripts, non-Python jobs).
	UV        *bool `yaml:"uv"`
	Artifacts struct {
		Input  []cqArtifact `yaml:"input"`
		Output []cqArtifact `yaml:"output"`
	} `yaml:"artifacts"`
}

type cqArtifact struct {
	Name string `yaml:"name"` // Drive artifact name
	Path string `yaml:"path"` // local relative path
}

// parseCQYAML reads and parses cq.yaml from dir.
// Returns nil, nil if the file does not exist (hwardward-compat).
func parseCQYAML(dir string) (*cqYAML, error) {
	data, err := os.ReadFile(filepath.Join(dir, "cq.yaml"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read cq.yaml: %w", err)
	}
	var cfg cqYAML
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse cq.yaml: %w", err)
	}
	return &cfg, nil
}

// runWithDrivePipeline executes the full pipeline for a job that carries a
// snapshot_version_hash (Drive-backed jobs):
//  1. Pull snapshot → jobDir
//  2. Parse cq.yaml
//  3. Pull input artifacts
//  4. Run job (delegate to executeJob, overriding Command from cq.yaml if set)
//  5. Push output artifacts
//
// If snapshotHash is empty the function falls back to plain executeJob.
func runWithDrivePipeline(drive driveClient, client *workerClient, job *model.Job, leaseID, workerID string, gpuCount int, snapshotHash string) (int, string) {
	if snapshotHash == "" {
		return executeJob(client, job, leaseID, workerID, gpuCount)
	}

	// Step 1: Pull snapshot
	jobDir := filepath.Join(os.TempDir(), "job-"+job.ID)
	if err := os.MkdirAll(jobDir, 0755); err != nil {
		log.Printf("c5-worker: pipeline: mkdir %s: %v", jobDir, err)
		return 1, ""
	}
	defer os.RemoveAll(jobDir)

	log.Printf("c5-worker: pipeline: pulling snapshot %s → %s", snapshotHash, jobDir)
	if err := drive.Pull(snapshotHash, jobDir, ""); err != nil {
		log.Printf("c5-worker: pipeline: snapshot pull failed: %v", err)
		return 1, ""
	}

	// Step 2: Parse cq.yaml
	cfg, err := parseCQYAML(jobDir)
	if err != nil {
		log.Printf("c5-worker: pipeline: %v", err)
		return 1, ""
	}

	// Step 3: Pull input artifacts
	if cfg != nil {
		for _, art := range cfg.Artifacts.Input {
			destPath := filepath.Join(jobDir, art.Path)
			if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
				log.Printf("c5-worker: pipeline: mkdir for input artifact %s: %v", art.Name, err)
				return 1, ""
			}
			log.Printf("c5-worker: pipeline: pulling input artifact %s → %s", art.Name, destPath)
			if err := drive.Pull(art.Name, filepath.Dir(destPath), "latest"); err != nil {
				log.Printf("c5-worker: pipeline: input artifact pull failed %s: %v", art.Name, err)
				return 1, ""
			}
		}
	}

	// Step 4: Run — override Command and Workdir from cq.yaml if present.
	// If uv is not explicitly set to false, prepend "uv run" so that uv
	// automatically syncs the venv (pyproject.toml / uv.lock) before running.
	if cfg != nil && cfg.Run != "" {
		job = shallowCopyJob(job)
		useUV := cfg.UV == nil || *cfg.UV // default true
		if useUV {
			job.Command = "uv run " + cfg.Run
		} else {
			job.Command = cfg.Run
		}
	}
	job = shallowCopyJob(job)
	job.Workdir = jobDir

	exitCode, resultFile := executeJob(client, job, leaseID, workerID, gpuCount)

	// Step 5: Push output artifacts (only on success)
	if exitCode == 0 && cfg != nil {
		for _, art := range cfg.Artifacts.Output {
			localPath := filepath.Join(jobDir, art.Path)
			log.Printf("c5-worker: pipeline: uploading output artifact %s ← %s", art.Name, localPath)
			if err := drive.Upload(localPath, art.Name); err != nil {
				log.Printf("c5-worker: pipeline: output artifact upload failed %s: %v", art.Name, err)
				exitCode = 1
			}
		}
	}

	return exitCode, resultFile
}

// shallowCopyJob returns a shallow copy of job so we can mutate fields safely.
func shallowCopyJob(j *model.Job) *model.Job {
	cp := *j
	return &cp
}

// =========================================================================
// Worker HTTP client
// =========================================================================

type workerClient struct {
	baseURL      string
	apiKey       string
	http         *http.Client // short timeout for API calls / heartbeats
	artifactHTTP *http.Client // longer timeout for artifact uploads/downloads
}

func (c *workerClient) doJSON(method, path string, body any, result any) error {
	var bodyReader io.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, c.baseURL+path, bodyReader)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("X-API-Key", c.apiKey)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(data))
	}

	if result != nil {
		return json.NewDecoder(resp.Body).Decode(result)
	}
	return nil
}

func (c *workerClient) register(req *model.WorkerRegisterRequest) (string, error) {
	var resp model.WorkerRegisterResponse
	if err := c.doJSON("POST", "/v1/workers/register", req, &resp); err != nil {
		return "", err
	}
	return resp.WorkerID, nil
}

func (c *workerClient) heartbeat(workerID string, freeVRAM float64) {
	c.doJSON("POST", "/v1/workers/heartbeat", &model.HeartbeatRequest{
		WorkerID: workerID,
		FreeVRAM: freeVRAM,
	}, nil)
}

func (c *workerClient) acquireLease(workerID string) (*model.Lease, *model.Job, []model.InputPresignedArtifact, *model.ControlMessage, error) {
	var resp model.LeaseAcquireResponse
	if err := c.doJSON("POST", "/v1/leases/acquire", &model.LeaseAcquireRequest{
		WorkerID: workerID,
	}, &resp); err != nil {
		return nil, nil, nil, nil, err
	}
	if resp.Control != nil {
		return nil, nil, nil, resp.Control, nil
	}
	if resp.LeaseID == "" {
		return nil, nil, nil, nil, nil // no jobs
	}
	lease := &model.Lease{
		ID:       resp.LeaseID,
		JobID:    resp.JobID,
		WorkerID: workerID,
	}
	return lease, &resp.Job, resp.InputPresignedURLs, nil, nil
}

func (c *workerClient) renewLease(leaseID, workerID string) error {
	return c.doJSON("POST", "/v1/leases/renew", &model.LeaseRenewRequest{
		LeaseID:  leaseID,
		WorkerID: workerID,
	}, nil)
}

func (c *workerClient) completeJob(jobID, status string, exitCode int, result map[string]any) error {
	return c.doJSON("POST", "/v1/jobs/"+jobID+"/complete", &model.JobCompleteRequest{
		Status:   status,
		ExitCode: &exitCode,
		Result:   result,
	}, nil)
}

func (c *workerClient) appendLog(jobID, line, stream string) {
	// Best-effort log forwarding — fire and forget via inline request
	c.doJSON("POST", "/v1/jobs/"+jobID+"/logs", map[string]string{
		"line":   line,
		"stream": stream,
	}, nil)
}

func (c *workerClient) logMetrics(jobID string, step int, metrics map[string]float64) {
	m := make(map[string]any, len(metrics))
	for k, v := range metrics {
		m[k] = v
	}
	c.doJSON("POST", "/v1/metrics/"+jobID, &model.MetricsLogRequest{
		Step:    step,
		Metrics: m,
	}, nil)
}

func (c *workerClient) getJobStatus(jobID string) (string, error) {
	var job model.Job
	if err := c.doJSON("GET", "/v1/jobs/"+jobID, nil, &job); err != nil {
		return "", err
	}
	return string(job.Status), nil
}

// getPresignedURL requests a signed upload URL from C5 server.
func (c *workerClient) getPresignedURL(path string) (string, error) {
	var resp model.PresignedURLResponse
	if err := c.doJSON("POST", "/v1/storage/presigned-url", &model.PresignedURLRequest{
		Path:   path,
		Method: "PUT",
	}, &resp); err != nil {
		return "", err
	}
	return resp.URL, nil
}

// confirmArtifact confirms an uploaded artifact with the C5 server.
func (c *workerClient) confirmArtifact(jobID string, path string, contentHash string, sizeBytes int64) error {
	return c.doJSON("POST", "/v1/artifacts/"+jobID+"/confirm", &model.ArtifactConfirmRequest{
		Path:        path,
		ContentHash: contentHash,
		SizeBytes:   sizeBytes,
	}, nil)
}
