// Package edgeagent implements the C5 edge agent: register, heartbeat, poll assignments, download artifacts, run post_command, report status.
package edgeagent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/piqsol/c4/c5/internal/model"
)

// Config holds edge agent configuration.
type Config struct {
	HubURL      string
	APIKey      string
	EdgeName    string
	Workdir     string
	PollInterval time.Duration

	// Metrics reporting (optional — disabled if MetricsCommand is empty).
	MetricsCommand  string
	MetricsInterval time.Duration // default 60s

	// Health check timeout (default 30s).
	HealthCheckTimeout time.Duration

	// Drive upload for collect control action (optional).
	DriveURL   string
	DriveAPIKey string
}

// runHealthCheck runs the health check command within the given timeout.
func runHealthCheck(ctx context.Context, command string, timeout time.Duration) error {
	hctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(hctx, "sh", "-c", command)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("health check failed: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// Run registers the edge, starts heartbeat, and runs the assignment poll loop until ctx is done.
func Run(ctx context.Context, cfg Config) error {
	baseURL := strings.TrimRight(cfg.HubURL, "/")
	client := &http.Client{Timeout: 30 * time.Second}

	if cfg.MetricsInterval <= 0 {
		cfg.MetricsInterval = 60 * time.Second
	}
	if cfg.HealthCheckTimeout <= 0 {
		cfg.HealthCheckTimeout = 30 * time.Second
	}

	// Register edge (retry on failure)
	var edgeID string
	for {
		reqBody, _ := json.Marshal(model.EdgeRegisterRequest{Name: cfg.EdgeName})
		req, err := http.NewRequestWithContext(ctx, "POST", baseURL+"/v1/edges/register", strings.NewReader(string(reqBody)))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		if cfg.APIKey != "" {
			req.Header.Set("X-API-Key", cfg.APIKey)
		}
		resp, err := client.Do(req)
		if err != nil {
			log.Printf("edge-agent: register failed: %v; retrying in 5s", err)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(5 * time.Second):
				continue
			}
		}
		if resp.StatusCode != http.StatusCreated {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			log.Printf("edge-agent: register returned %d: %s; retrying in 5s", resp.StatusCode, string(body))
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(5 * time.Second):
				continue
			}
		}
		var regResp model.EdgeRegisterResponse
		if err := json.NewDecoder(resp.Body).Decode(&regResp); err != nil {
			resp.Body.Close()
			return fmt.Errorf("decode register response: %w", err)
		}
		resp.Body.Close()
		edgeID = regResp.EdgeID
		break
	}
	log.Printf("edge-agent: registered as %s", edgeID)

	// MetricsReporter goroutine (no-op if MetricsCommand is empty)
	if cfg.MetricsCommand != "" {
		mr := newMetricsReporter(edgeID, baseURL, cfg.APIKey, cfg.MetricsCommand, cfg.MetricsInterval, client)
		go mr.Start(ctx)
	}

	// ControlPoller goroutine
	cp := newControlPoller(edgeID, baseURL, cfg.APIKey, cfg.DriveURL, cfg.DriveAPIKey, client)
	go cp.Start(ctx)

	// Heartbeat goroutine
	go func() {
		tick := time.NewTicker(30 * time.Second)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
				reqBody, _ := json.Marshal(model.EdgeHeartbeatRequest{EdgeID: edgeID})
				req, _ := http.NewRequestWithContext(ctx, "POST", baseURL+"/v1/edges/heartbeat", strings.NewReader(string(reqBody)))
				req.Header.Set("Content-Type", "application/json")
				if cfg.APIKey != "" {
					req.Header.Set("X-API-Key", cfg.APIKey)
				}
				client.Do(req)
			}
		}
	}()

	// Poll loop
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		req, err := http.NewRequestWithContext(ctx, "GET", baseURL+"/v1/deploy/assignments/"+edgeID, nil)
		if err != nil {
			log.Printf("edge-agent: assignments request: %v", err)
			sleepOrDone(ctx, cfg.PollInterval)
			continue
		}
		if cfg.APIKey != "" {
			req.Header.Set("X-API-Key", cfg.APIKey)
		}
		resp, err := client.Do(req)
		if err != nil {
			log.Printf("edge-agent: GET assignments: %v", err)
			sleepOrDone(ctx, cfg.PollInterval)
			continue
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			sleepOrDone(ctx, cfg.PollInterval)
			continue
		}
		var assignments []model.DeployAssignmentResponse
		if err := json.NewDecoder(resp.Body).Decode(&assignments); err != nil {
			resp.Body.Close()
			log.Printf("edge-agent: decode assignments: %v", err)
			sleepOrDone(ctx, cfg.PollInterval)
			continue
		}
		resp.Body.Close()

		for _, a := range assignments {
			processAssignment(ctx, client, baseURL, cfg.APIKey, edgeID, cfg.Workdir, cfg.HealthCheckTimeout, &a)
		}

		sleepOrDone(ctx, cfg.PollInterval)
	}
}

func sleepOrDone(ctx context.Context, d time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}

func processAssignment(ctx context.Context, client *http.Client, baseURL, apiKey, edgeID, workdir string, healthCheckTimeout time.Duration, a *model.DeployAssignmentResponse) {
	deployDir := filepath.Join(workdir, a.DeployID)
	if err := os.MkdirAll(deployDir, 0o755); err != nil {
		reportTargetStatus(ctx, client, baseURL, apiKey, a.DeployID, edgeID, "failed", err.Error())
		return
	}

	rb := newRollbackManager(deployDir)
	// Backup existing deploy dir before downloading new artifacts.
	if err := rb.BeforeDeploy(deployDir); err != nil {
		log.Printf("edge-agent: BeforeDeploy: %v", err)
		// Non-fatal: proceed without rollback capability
	}

	// Report downloading
	reportTargetStatus(ctx, client, baseURL, apiKey, a.DeployID, edgeID, "downloading", "")

	// Download artifacts
	for _, art := range a.Artifacts {
		dest := filepath.Join(deployDir, art.Path)
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			reportTargetStatus(ctx, client, baseURL, apiKey, a.DeployID, edgeID, "failed", "mkdir: "+err.Error())
			return
		}
		req, err := http.NewRequestWithContext(ctx, "GET", art.URL, nil)
		if err != nil {
			reportTargetStatus(ctx, client, baseURL, apiKey, a.DeployID, edgeID, "failed", "request: "+err.Error())
			return
		}
		resp, err := client.Do(req)
		if err != nil {
			reportTargetStatus(ctx, client, baseURL, apiKey, a.DeployID, edgeID, "failed", "download: "+err.Error())
			return
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			reportTargetStatus(ctx, client, baseURL, apiKey, a.DeployID, edgeID, "failed", fmt.Sprintf("download %s: status %d", art.Path, resp.StatusCode))
			return
		}
		f, err := os.Create(dest)
		if err != nil {
			resp.Body.Close()
			reportTargetStatus(ctx, client, baseURL, apiKey, a.DeployID, edgeID, "failed", "create file: "+err.Error())
			return
		}
		_, err = io.Copy(f, resp.Body)
		f.Close()
		resp.Body.Close()
		if err != nil {
			reportTargetStatus(ctx, client, baseURL, apiKey, a.DeployID, edgeID, "failed", "write: "+err.Error())
			return
		}
	}

	// Report deploying
	reportTargetStatus(ctx, client, baseURL, apiKey, a.DeployID, edgeID, "deploying", "")

	// Run post_command if set
	if strings.TrimSpace(a.PostCommand) != "" {
		cmd := exec.CommandContext(ctx, "sh", "-c", a.PostCommand)
		cmd.Dir = deployDir
		cmd.Env = os.Environ()
		out, err := cmd.CombinedOutput()
		if err != nil {
			reportTargetStatus(ctx, client, baseURL, apiKey, a.DeployID, edgeID, "failed", string(out))
			return
		}
	}

	// Health check gate
	if a.HealthCheck.Command != "" {
		timeout := healthCheckTimeout
		if a.HealthCheck.TimeoutSec > 0 {
			timeout = time.Duration(a.HealthCheck.TimeoutSec) * time.Second
		}
		if err := runHealthCheck(ctx, a.HealthCheck.Command, timeout); err != nil {
			log.Printf("edge-agent: health check failed for deploy %s: %v; rolling back", a.DeployID, err)
			if rbErr := rb.Rollback(deployDir); rbErr != nil {
				log.Printf("edge-agent: rollback failed: %v", rbErr)
			}
			reportTargetStatus(ctx, client, baseURL, apiKey, a.DeployID, edgeID, "failed", err.Error())
			return
		}
	}

	rb.Cleanup(deployDir)
	reportTargetStatus(ctx, client, baseURL, apiKey, a.DeployID, edgeID, "succeeded", "")
}

func reportTargetStatus(ctx context.Context, client *http.Client, baseURL, apiKey, deployID, edgeID, status, errMsg string) {
	body := model.DeployTargetStatusRequest{
		DeployID: deployID,
		EdgeID:   edgeID,
		Status:   status,
		Error:    errMsg,
	}
	reqBody, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, "POST", baseURL+"/v1/deploy/target-status", strings.NewReader(string(reqBody)))
	if err != nil {
		log.Printf("edge-agent: report status: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("X-API-Key", apiKey)
	}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("edge-agent: POST target-status: %v", err)
		return
	}
	resp.Body.Close()
}
