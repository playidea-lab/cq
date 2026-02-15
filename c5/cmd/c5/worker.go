package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strings"
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

	running := false

	for {
		select {
		case <-sigCh:
			log.Println("c5-worker: shutting down...")
			return nil

		case <-heartbeatTicker.C:
			client.heartbeat(workerID, cfg.totalVRAM)

		case <-ticker.C:
			if running {
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

			running = true
			log.Printf("c5-worker: acquired job %s (%s)", job.ID, job.Name)

			// Execute job in a goroutine
			go func(jobID, leaseID, command, workdir string) {
				defer func() { running = false }()

				exitCode := executeJob(client, jobID, leaseID, workerID, command, workdir)

				status := "SUCCEEDED"
				if exitCode != 0 {
					status = "FAILED"
				}

				if err := client.completeJob(jobID, status, exitCode); err != nil {
					log.Printf("c5-worker: complete error: %v", err)
				} else {
					log.Printf("c5-worker: job %s %s (exit %d)", jobID, status, exitCode)
				}
			}(job.ID, lease.ID, job.Command, job.Workdir)
		}
	}
}

func executeJob(client *workerClient, jobID, leaseID, workerID, command, workdir string) int {
	cmd := exec.Command("sh", "-c", command)
	cmd.Dir = workdir

	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	if err := cmd.Start(); err != nil {
		log.Printf("c5-worker: start error: %v", err)
		return 1
	}

	// Stream logs
	go streamLogs(client, jobID, stdout, "stdout")
	go streamLogs(client, jobID, stderr, "stderr")

	// Renew lease periodically
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(2 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				client.renewLease(leaseID, workerID)
			}
		}
	}()

	err := cmd.Wait()
	close(done)

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode()
		}
		return 1
	}
	return 0
}

func streamLogs(client *workerClient, jobID string, r io.Reader, stream string) {
	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			lines := strings.Split(strings.TrimRight(string(buf[:n]), "\n"), "\n")
			for _, line := range lines {
				if line != "" {
					client.appendLog(jobID, line, stream)
				}
			}
		}
		if err != nil {
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
