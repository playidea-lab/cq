package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/piqsol/c4/c5/internal/model"
	"github.com/spf13/cobra"
)

func workerCmd() *cobra.Command {
	var (
		serverURL string
		hostname  string
		gpuCount  int
		gpuModel  string
		totalVRAM float64
		pollSec   int
		apiKey    string
	)

	cmd := &cobra.Command{
		Use:   "worker",
		Short: "Start a C5 worker agent",
		RunE: func(cmd *cobra.Command, args []string) error {
			if hostname == "" {
				h, _ := os.Hostname()
				hostname = h
			}
			return runWorker(workerConfig{
				serverURL: serverURL,
				hostname:  hostname,
				gpuCount:  gpuCount,
				gpuModel:  gpuModel,
				totalVRAM: totalVRAM,
				pollSec:   pollSec,
				apiKey:    apiKey,
			})
		},
	}

	cmd.Flags().StringVar(&serverURL, "server", "http://localhost:8585", "C5 server URL")
	cmd.Flags().StringVar(&hostname, "hostname", "", "Worker hostname (default: OS hostname)")
	cmd.Flags().IntVar(&gpuCount, "gpu-count", 0, "Number of GPUs available")
	cmd.Flags().StringVar(&gpuModel, "gpu-model", "", "GPU model name")
	cmd.Flags().Float64Var(&totalVRAM, "total-vram", 0, "Total VRAM in GB")
	cmd.Flags().IntVar(&pollSec, "poll-interval", 5, "Poll interval in seconds")
	cmd.Flags().StringVar(&apiKey, "api-key", os.Getenv("C5_API_KEY"), "API key for authentication")

	return cmd
}

type workerConfig struct {
	serverURL string
	hostname  string
	gpuCount  int
	gpuModel  string
	totalVRAM float64
	pollSec   int
	apiKey    string
}

func runWorker(cfg workerConfig) error {
	client := &workerClient{
		baseURL: strings.TrimRight(cfg.serverURL, "/"),
		apiKey:  cfg.apiKey,
		http:    &http.Client{Timeout: 30 * time.Second},
	}

	// Register
	workerID, err := client.register(&model.WorkerRegisterRequest{
		Hostname:  cfg.hostname,
		GPUCount:  cfg.gpuCount,
		GPUModel:  cfg.gpuModel,
		TotalVRAM: cfg.totalVRAM,
		FreeVRAM:  cfg.totalVRAM,
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

	heartbeatTicker := time.NewTicker(30 * time.Second)
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

			lease, job, err := client.acquireLease(workerID)
			if err != nil {
				log.Printf("c5-worker: acquire error: %v", err)
				continue
			}
			if lease == nil {
				continue // no jobs
			}

			running.Store(true)
			log.Printf("c5-worker: acquired job %s (%s)", job.ID, job.Name)

			// Execute job in a goroutine
			go func(j *model.Job, leaseID string) {
				defer running.Store(false)

				exitCode := executeJob(client, j, leaseID, workerID, cfg.gpuCount)

				status := "SUCCEEDED"
				if exitCode != 0 {
					status = "FAILED"
				}

				if err := client.completeJob(j.ID, status, exitCode); err != nil {
					log.Printf("c5-worker: complete error: %v", err)
				} else {
					log.Printf("c5-worker: job %s %s (exit %d)", j.ID, status, exitCode)
				}
			}(job, lease.ID)
		}
	}
}

func executeJob(client *workerClient, job *model.Job, leaseID, workerID string, gpuCount int) int {
	// Set up context with optional timeout
	ctx := context.Background()
	var cancel context.CancelFunc
	if job.TimeoutSec > 0 {
		ctx, cancel = context.WithTimeout(ctx, time.Duration(job.TimeoutSec)*time.Second)
	} else {
		ctx, cancel = context.WithCancel(ctx)
	}
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", job.Command)
	cmd.Dir = job.Workdir

	// Env injection: inherit current env + job env vars
	env := os.Environ()
	for k, v := range job.Env {
		env = append(env, k+"="+v)
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
	cmd.Env = env

	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	if err := cmd.Start(); err != nil {
		log.Printf("c5-worker: start error: %v", err)
		return 1
	}

	// Stream logs with metrics auto-parsing on stdout
	mc := newMetricsCollector(client, job.ID)
	go streamLogs(client, job.ID, stdout, "stdout", mc)
	go streamLogs(client, job.ID, stderr, "stderr", nil)

	// Lease renew + cancel detection goroutine
	done := make(chan struct{})
	go func() {
		renewTicker := time.NewTicker(2 * time.Minute)
		cancelTicker := time.NewTicker(30 * time.Second)
		defer renewTicker.Stop()
		defer cancelTicker.Stop()
		for {
			select {
			case <-done:
				return
			case <-renewTicker.C:
				client.renewLease(leaseID, workerID)
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

	if err != nil {
		// Check if killed by timeout
		if ctx.Err() == context.DeadlineExceeded {
			log.Printf("c5-worker: job %s timed out after %ds", job.ID, job.TimeoutSec)
			return 124
		}
		// Check if killed by cancel
		if ctx.Err() == context.Canceled {
			log.Printf("c5-worker: job %s cancelled", job.ID)
			return 130
		}
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode()
		}
		return 1
	}
	return 0
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
	mc.timer = time.AfterFunc(5*time.Second, mc.flush)

	// Flush immediately if buffer has many keys
	if len(mc.pending) >= 10 {
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
// Worker HTTP client
// =========================================================================

type workerClient struct {
	baseURL string
	apiKey  string
	http    *http.Client
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
		data, _ := io.ReadAll(resp.Body)
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

func (c *workerClient) acquireLease(workerID string) (*model.Lease, *model.Job, error) {
	var resp model.LeaseAcquireResponse
	if err := c.doJSON("POST", "/v1/leases/acquire", &model.LeaseAcquireRequest{
		WorkerID: workerID,
	}, &resp); err != nil {
		return nil, nil, err
	}
	if resp.LeaseID == "" {
		return nil, nil, nil // no jobs
	}
	lease := &model.Lease{
		ID:       resp.LeaseID,
		JobID:    resp.JobID,
		WorkerID: workerID,
	}
	return lease, &resp.Job, nil
}

func (c *workerClient) renewLease(leaseID, workerID string) {
	c.doJSON("POST", "/v1/leases/renew", &model.LeaseRenewRequest{
		LeaseID:  leaseID,
		WorkerID: workerID,
	}, nil)
}

func (c *workerClient) completeJob(jobID, status string, exitCode int) error {
	return c.doJSON("POST", "/v1/jobs/"+jobID+"/complete", &model.JobCompleteRequest{
		Status:   status,
		ExitCode: exitCode,
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
